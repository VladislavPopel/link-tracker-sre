package application

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"gitlab.education.tbank.ru/backend-academy-go-2025/homeworks/link-tracker/internal/domain"
)

const (
	StartMessage   = "Добро пожаловать! Используйте /help, чтобы посмотреть доступные команды."
	HelpMessage    = "Доступные команды:\n/start — регистрация\n/help — помощь\n/track — начать отслеживание ссылки\n/untrack — прекратить отслеживание ссылки\n/list [тег] — список отслеживаемых ссылок"
	UnknownMessage = "Не знаю такой команды, используйте /help."

	TrackAskURL     = "Отправьте ссылку для отслеживания:"
	TrackAskTags    = "Отправьте теги через запятую (или /skip для пропуска):"
	TrackSuccess    = "Ссылка успешно добавлена для отслеживания."
	TrackCancelled  = "Отслеживание отменено."
	TrackExists     = "Ссылка уже отслеживается."
	TrackInvalidURL = "Некорректная ссылка. Пожалуйста, отправьте корректный URL (начиная с http:// или https://)."

	UntrackAskURL   = "Отправьте ссылку для прекращения отслеживания:"
	UntrackSuccess  = "Ссылка удалена из отслеживания."
	UntrackNotFound = "Ссылка не найдена в списке отслеживаемых."

	ListEmpty = "У вас нет отслеживаемых ссылок."

	SubstringNumber = 2
)

// HandleFunc определяет тип функции-обработчика для команд
type HandleFunc func(ctx context.Context, update tgbotapi.Update) tgbotapi.MessageConfig

// BotClient описывает взаимодействие с Telegram API
type BotClient interface {
	Send(msg tgbotapi.MessageConfig) (int, error)
	SetCommands(commands []tgbotapi.BotCommand) error
}

// Commander отвечает за диспетчеризацию команд и ведение диалогов
type Commander struct {
	bot      BotClient
	repo     domain.UserRepository
	scrapper ScrapperClient
	sessions *SessionStore
	handlers map[string]HandleFunc
}

func NewCommander(bot BotClient, repo domain.UserRepository, scrapper ScrapperClient) (*Commander, error) {
	if bot == nil {
		return nil, errors.New("bot is required for Commander")
	}
	c := &Commander{
		bot:      bot,
		repo:     repo,
		scrapper: scrapper,
		sessions: NewSessionStore(),
		handlers: make(map[string]HandleFunc),
	}
	return c, nil
}

func (c *Commander) Init() error {
	return c.registerHandlers()
}

// Handle осуществляет диспетчеризацию входящих сообщений
func (c *Commander) Handle(ctx context.Context, update tgbotapi.Update) {
	if update.Message == nil {
		return
	}

	slog.Info("incoming message",
		"username", update.Message.From.UserName,
		"text", update.Message.Text,
		"chat_id", update.Message.Chat.ID,
	)

	msg := c.dispatch(ctx, update)
	msgID, err := c.bot.Send(msg)
	if err != nil {
		slog.Error("failed to send message",
			"err", err,
			"chat_id", update.Message.Chat.ID,
			"sent_message_id", msgID,
		)
	}
}

// dispatch реализует машину состояний диалогов
func (c *Commander) dispatch(ctx context.Context, update tgbotapi.Update) tgbotapi.MessageConfig {
	text := update.Message.Text
	chatID := update.Message.Chat.ID

	// /cancel всегда сбрасывает текущий диалог
	if text == "/cancel" {
		c.sessions.Reset(chatID)
		return tgbotapi.NewMessage(chatID, TrackCancelled)
	}

	sess := c.sessions.Get(chatID)
	isCommand := strings.HasPrefix(text, "/")

	if sess.State != StateIdle {
		// /skip - специальная команда при ожидании тегов
		if text == "/skip" && sess.State == StateTrackWaitTags {
			return c.handleTrackTags(ctx, update, "/skip", sess.PendingURL)
		}
		if isCommand {
			// Любая другая команда прерывает диалог
			c.sessions.Reset(chatID)
		} else {
			switch sess.State {
			case StateTrackWaitURL:
				return c.handleTrackURL(update, text)
			case StateTrackWaitTags:
				return c.handleTrackTags(ctx, update, text, sess.PendingURL)
			case StateUntrackWaitURL:
				return c.handleUntrackURL(ctx, update, text)
			case StateIdle:
			}
		}
	}

	// Разбираем команду и аргумент
	parts := strings.SplitN(text, " ", SubstringNumber)
	cmd := parts[0]
	arg := ""
	if len(parts) > 1 {
		arg = strings.TrimSpace(parts[1])
	}

	if cmd == "/list" {
		return c.handleList(ctx, update, arg)
	}

	if handler, ok := c.handlers[cmd]; ok {
		return handler(ctx, update)
	}
	return c.handleUnknown(update)
}

func (c *Commander) registerHandlers() error {
	c.handlers["/start"] = func(ctx context.Context, u tgbotapi.Update) tgbotapi.MessageConfig {
		return c.handleStart(ctx, u)
	}
	c.handlers["/help"] = c.handleHelp
	c.handlers["/track"] = c.handleTrackStart
	c.handlers["/untrack"] = c.handleUntrackStart
	c.handlers["/list"] = func(ctx context.Context, u tgbotapi.Update) tgbotapi.MessageConfig {
		return c.handleList(ctx, u, "")
	}

	commands := []tgbotapi.BotCommand{
		{Command: "start", Description: "Запустить бота"},
		{Command: "help", Description: "Показать справку"},
		{Command: "track", Description: "Начать отслеживание ссылки"},
		{Command: "untrack", Description: "Прекратить отслеживание ссылки"},
		{Command: "list", Description: "Список отслеживаемых ссылок"},
	}

	if err := c.bot.SetCommands(commands); err != nil {
		return fmt.Errorf("set commands: %w", err)
	}
	return nil
}

// /start

func (c *Commander) handleStart(ctx context.Context, update tgbotapi.Update) tgbotapi.MessageConfig {
	userID := update.Message.From.ID
	chatID := update.Message.Chat.ID

	if err := c.repo.Save(&domain.User{ID: userID}); err != nil {
		slog.Error("failed to save user", "user_id", userID, "err", err)
		return tgbotapi.NewMessage(chatID, "Произошла ошибка при регистрации. Пожалуйста, попробуйте позже.")
	}

	if c.scrapper != nil {
		if err := c.scrapper.RegisterChat(ctx, chatID); err != nil {
			slog.Warn("failed to register chat in scrapper", "chat_id", chatID, "err", err)
		}
	}

	return tgbotapi.NewMessage(chatID, StartMessage)
}

// /help

func (c *Commander) handleHelp(_ context.Context, update tgbotapi.Update) tgbotapi.MessageConfig {
	return tgbotapi.NewMessage(update.Message.Chat.ID, HelpMessage)
}

// /unknown

func (c *Commander) handleUnknown(update tgbotapi.Update) tgbotapi.MessageConfig {
	return tgbotapi.NewMessage(update.Message.Chat.ID, UnknownMessage)
}

// /track

func (c *Commander) handleTrackStart(_ context.Context, update tgbotapi.Update) tgbotapi.MessageConfig {
	chatID := update.Message.Chat.ID
	c.sessions.Set(chatID, Session{State: StateTrackWaitURL})
	return tgbotapi.NewMessage(chatID, TrackAskURL)
}

func (c *Commander) handleTrackURL(update tgbotapi.Update, rawURL string) tgbotapi.MessageConfig {
	chatID := update.Message.Chat.ID

	if !isValidURL(rawURL) {
		return tgbotapi.NewMessage(chatID, TrackInvalidURL)
	}

	c.sessions.Set(chatID, Session{State: StateTrackWaitTags, PendingURL: rawURL})
	return tgbotapi.NewMessage(chatID, TrackAskTags)
}

func (c *Commander) handleTrackTags(ctx context.Context, update tgbotapi.Update, input, pendingURL string) tgbotapi.MessageConfig {
	chatID := update.Message.Chat.ID
	c.sessions.Reset(chatID)

	var tags []string
	if input != "/skip" {
		for _, t := range strings.Split(input, ",") {
			if t = strings.TrimSpace(t); t != "" {
				tags = append(tags, t)
			}
		}
	}

	if c.scrapper == nil {
		return tgbotapi.NewMessage(chatID, "Сервис недоступен. Попробуйте позже.")
	}

	if _, err := c.scrapper.AddLink(ctx, chatID, pendingURL, tags, nil); err != nil {
		if errors.Is(err, domain.ErrLinkExists) {
			return tgbotapi.NewMessage(chatID, TrackExists)
		}
		slog.Error("failed to add link", "chat_id", chatID, "url", pendingURL, "err", err)
		return tgbotapi.NewMessage(chatID, "Ошибка при добавлении ссылки. Попробуйте позже.")
	}

	return tgbotapi.NewMessage(chatID, TrackSuccess)
}

// /untrack

func (c *Commander) handleUntrackStart(_ context.Context, update tgbotapi.Update) tgbotapi.MessageConfig {
	chatID := update.Message.Chat.ID
	c.sessions.Set(chatID, Session{State: StateUntrackWaitURL})
	return tgbotapi.NewMessage(chatID, UntrackAskURL)
}

func (c *Commander) handleUntrackURL(ctx context.Context, update tgbotapi.Update, rawURL string) tgbotapi.MessageConfig {
	chatID := update.Message.Chat.ID
	c.sessions.Reset(chatID)

	if c.scrapper == nil {
		return tgbotapi.NewMessage(chatID, "Сервис недоступен. Попробуйте позже.")
	}

	if _, err := c.scrapper.RemoveLink(ctx, chatID, rawURL); err != nil {
		if errors.Is(err, domain.ErrLinkNotFound) {
			return tgbotapi.NewMessage(chatID, UntrackNotFound)
		}
		slog.Error("failed to remove link", "chat_id", chatID, "url", rawURL, "err", err)
		return tgbotapi.NewMessage(chatID, "Ошибка при удалении ссылки. Попробуйте позже.")
	}

	return tgbotapi.NewMessage(chatID, UntrackSuccess)
}

// /list

func (c *Commander) handleList(ctx context.Context, update tgbotapi.Update, tag string) tgbotapi.MessageConfig {
	chatID := update.Message.Chat.ID

	if c.scrapper == nil {
		return tgbotapi.NewMessage(chatID, "Сервис недоступен. Попробуйте позже.")
	}

	links, err := c.scrapper.GetLinks(ctx, chatID)
	if err != nil {
		slog.Error("failed to get links", "chat_id", chatID, "err", err)
		return tgbotapi.NewMessage(chatID, "Ошибка при получении списка ссылок. Попробуйте позже.")
	}

	if tag != "" {
		filtered := make([]*domain.Link, 0, len(links))
		for _, l := range links {
			for _, t := range l.Tags {
				if t == tag {
					filtered = append(filtered, l)
					break
				}
			}
		}
		links = filtered
	}

	if len(links) == 0 {
		return tgbotapi.NewMessage(chatID, ListEmpty)
	}

	var sb strings.Builder
	sb.WriteString("Ваши отслеживаемые ссылки:\n")
	for i, l := range links {
		fmt.Fprintf(&sb, "%d. %s", i+1, l.URL)
		if len(l.Tags) > 0 {
			fmt.Fprintf(&sb, " [%s]", strings.Join(l.Tags, ", "))
		}
		sb.WriteByte('\n')
	}

	return tgbotapi.NewMessage(chatID, sb.String())
}

// helpers

// isValidURL проверяет корректность URL
func isValidURL(rawURL string) bool {
	u, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	return u.Scheme == "http" || u.Scheme == "https"
}
