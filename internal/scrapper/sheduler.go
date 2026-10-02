package scrapper

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/go-co-op/gocron/v2"
	"gitlab.education.tbank.ru/backend-academy-go-2025/homeworks/link-tracker/internal/client"
	"gitlab.education.tbank.ru/backend-academy-go-2025/homeworks/link-tracker/internal/domain"
)

const (
	limit            = 100
	offset           = 0
	mskOffsetSeconds = 3 * 60 * 60
)

var moscowTZ = time.FixedZone("MSK", mskOffsetSeconds)

// Scheduler периодически проверяет все отслеживаемые ссылки и отправляет
// уведомления боту при обнаружении изменений
type Scheduler struct {
	repo        domain.LinkRepository
	checkers    []LinkChecker
	sender      MessageSender
	state       domain.LinkStateRepository
	sched       gocron.Scheduler
	batchSize   int
	workerCount int
}

// NewScheduler создаёт и конфигурирует планировщик
func NewScheduler(
	repo domain.LinkRepository,
	checkers []LinkChecker,
	sender MessageSender,
	state domain.LinkStateRepository,
	interval time.Duration,
	batchSize int,
	workerCount int,
) (*Scheduler, error) {
	s, err := gocron.NewScheduler()
	if err != nil {
		return nil, fmt.Errorf("create gocron scheduler: %w", err)
	}

	sch := &Scheduler{
		repo:        repo,
		checkers:    checkers,
		sender:      sender,
		state:       state,
		sched:       s,
		batchSize:   batchSize,
		workerCount: workerCount,
	}

	_, err = s.NewJob(
		gocron.DurationJob(interval),
		gocron.NewTask(sch.checkAll),
		gocron.WithName("link-checker"),
		gocron.WithSingletonMode(gocron.LimitModeWait),
	)
	if err != nil {
		return nil, fmt.Errorf("schedule job: %w", err)
	}

	return sch, nil
}

func (s *Scheduler) Start() {
	slog.Info("scheduler started",
		"batchSize", s.batchSize,
		"workers", s.workerCount,
	)
	s.sched.Start()
}

func (s *Scheduler) Stop() error {
	if err := s.sched.Shutdown(); err != nil {
		return fmt.Errorf("scheduler shutdown: %w", err)
	}
	return nil
}

// checkAll - один цикл проверки: загружает ссылки батчами и обрабатывает параллельно
func (s *Scheduler) checkAll(ctx context.Context) {
	slog.Info("scheduler: starting check cycle")
	offset := 0
	total := 0

	for {
		links, err := s.repo.GetAllLinks(ctx, domain.Page{
			Limit:  s.batchSize,
			Offset: offset,
		})
		if err != nil {
			slog.Error("scheduler: get links batch failed", "offset", offset, "err", err)
			return
		}
		if len(links) == 0 {
			break
		}

		s.processBatch(ctx, links)
		total += len(links)

		if len(links) < s.batchSize {
			break // последняя страница
		}
		offset += s.batchSize
	}

	slog.Info("scheduler: check cycle complete", "total_links", total)
}

// linkJob описывает работу для одного воркера: одна уникальная ссылка
// и список чатов, которые её отслеживают
type linkJob struct {
	url     string
	chatIDs []int64
}

// processBatch обрабатывает один батч ссылок в пуле воркеров
func (s *Scheduler) processBatch(ctx context.Context, links []*domain.Link) {
	// Группируем по URL - несколько чатов могут отслеживать одну ссылку
	byURL := make(map[string]*linkJob, len(links))
	for _, l := range links {
		if j, ok := byURL[l.URL]; ok {
			j.chatIDs = append(j.chatIDs, l.ChatID)
		} else {
			byURL[l.URL] = &linkJob{url: l.URL, chatIDs: []int64{l.ChatID}}
		}
	}

	jobs := make(chan *linkJob, len(byURL))
	for _, j := range byURL {
		jobs <- j
	}
	close(jobs)

	// Сбор ошибок со всех воркеров
	var (
		mu         sync.Mutex
		failedJobs []*linkJob
	)

	var wg sync.WaitGroup
	for range s.workerCount {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for job := range jobs {
				if err := s.processLink(ctx, job); err != nil {
					slog.Warn("scheduler: link check failed",
						"url", job.url,
						"err", err,
					)
					mu.Lock()
					failedJobs = append(failedJobs, job)
					mu.Unlock()
				}
			}
		}()
	}
	wg.Wait()

	if len(failedJobs) > 0 {
		slog.Warn("scheduler: some links could not be checked", "failed", len(failedJobs))
	}
}

// processLink проверяет одну ссылку и при наличии изменений отправляет уведомление
func (s *Scheduler) processLink(ctx context.Context, job *linkJob) error {
	checker := s.findChecker(job.url)
	if checker == nil {
		return nil
	}

	since, err := s.state.GetLastSeen(ctx, job.url)
	if err != nil {
		return fmt.Errorf("get state for %s: %w", job.url, err)
	}
	isFirstCheck := since.IsZero()

	result, err := checker.Check(ctx, job.url, since)
	if errors.Is(err, ErrNoNewEvents) || errors.Is(err, client.ErrNoUpdates) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("check %s: %w", job.url, err)
	}
	if result == nil {
		return nil
	}

	// Фиксируем новое состояние атомарно. Выигрывает ровно один экземпляр сервиса;
	// проигравший молча пропускает событие, иначе пользователь получил бы дубль.
	won, err := s.state.AdvanceLastSeen(ctx, job.url, since, result.UpdatedAt.Add(time.Second))
	if err != nil {
		return fmt.Errorf("advance state for %s: %w", job.url, err)
	}
	if !won {
		return nil
	}

	if isFirstCheck || result.Info == nil {
		return nil
	}

	update := client.LinkUpdate{
		URL:         job.url,
		Description: formatDescription(job.url, result.Info),
		TgChatIDs:   job.chatIDs,
	}
	if err = s.sender.SendUpdate(ctx, update); err != nil {
		return fmt.Errorf("send update for %s: %w", job.url, err)
	}

	slog.Info("scheduler: update sent",
		"url", job.url,
		"chats", len(job.chatIDs),
	)
	return nil
}

// // reportFailures собирает все неудачные ссылки по чатам и отправляет отчёт каждому пользователю
// func (s *Scheduler) reportFailures(ctx context.Context, failedJobs []*linkJob) {
// 	chatToURLs := make(map[int64][]string)
// 	for _, job := range failedJobs {
// 		for _, chatID := range job.chatIDs {
// 			chatToURLs[chatID] = append(chatToURLs[chatID], job.url)
// 		}
// 	}

// 	for chatID, urls := range chatToURLs {
// 		var sb strings.Builder
// 		sb.WriteString("⚠️ Не удалось проверить следующие ссылки:\n")
// 		for _, u := range urls {
// 			sb.WriteString("• ")
// 			sb.WriteString(u)
// 			sb.WriteByte('\n')
// 		}

// 		update := client.LinkUpdate{
// 			URL:         "",
// 			Description: sb.String(),
// 			TgChatIDs:   []int64{chatID},
// 		}
// 		if err := s.sender.SendUpdate(ctx, update); err != nil {
// 			slog.Error("scheduler: failed to report failures",
// 				"chat_id", chatID,
// 				"err", err,
// 			)
// 		}
// 	}
// }

// formatDescription формирует текст уведомления
func formatDescription(url string, info *UpdateInfo) string {
	return fmt.Sprintf(
		"🔔 Новое обновление по ссылке\n"+
			"🔗 %s\n"+
			"📌 %s\n"+
			"👤 Автор: %s\n"+
			"🕐 %s\n\n"+
			"%s",
		url,
		info.Title,
		info.Author,
		info.CreatedAt.In(moscowTZ).Format("02.01.2006 15:04 MSK"),
		info.Preview,
	)
}

func (s *Scheduler) findChecker(url string) LinkChecker {
	for _, c := range s.checkers {
		if c.Supports(url) {
			return c
		}
	}
	return nil
}
