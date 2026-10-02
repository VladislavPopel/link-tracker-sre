package scrapper

import (
	"context"
	"fmt"

	"gitlab.education.tbank.ru/backend-academy-go-2025/homeworks/link-tracker/internal/client"
	"gitlab.education.tbank.ru/backend-academy-go-2025/homeworks/link-tracker/internal/domain"
)

// DBMessageSender доставляет уведомления в веб-интерфейс: кладёт их в БД,
// откуда фронтенд забирает их через GET /updates.
type DBMessageSender struct {
	repo domain.UpdateRepository
}

func NewDBMessageSender(repo domain.UpdateRepository) *DBMessageSender {
	return &DBMessageSender{repo: repo}
}

func (s *DBMessageSender) SendUpdate(ctx context.Context, update client.LinkUpdate) error {
	if err := s.repo.AddUpdates(ctx, update.TgChatIDs, update.URL, update.Description); err != nil {
		return fmt.Errorf("db message sender: %w", err)
	}
	return nil
}
