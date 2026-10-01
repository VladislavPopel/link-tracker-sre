package scrapper

import (
	"context"
	"fmt"

	"gitlab.education.tbank.ru/backend-academy-go-2025/homeworks/link-tracker/internal/client"
)

// MessageSender - интерфейс для отправки уведомлений scrapper - bot
type MessageSender interface {
	SendUpdate(ctx context.Context, update client.LinkUpdate) error
}

// HTTPMessageSender отправляет уведомления через HTTP
type HTTPMessageSender struct {
	botClient *client.BotHTTPClient
}

func NewHTTPMessageSender(botURL string) *HTTPMessageSender {
	return &HTTPMessageSender{botClient: client.NewBotHTTPClient(botURL)}
}

func (s *HTTPMessageSender) SendUpdate(ctx context.Context, update client.LinkUpdate) error {
	if err := s.botClient.SendUpdate(ctx, update); err != nil {
		return fmt.Errorf("http message sender: %w", err)
	}
	return nil
}
