package domain

import (
	"context"
	"time"
)

// Update — уведомление об изменении на отслеживаемой ссылке, адресованное одному чату.
type Update struct {
	ID          int64
	ChatID      int64
	URL         string
	Description string
	CreatedAt   time.Time
}

// UpdateRepository хранит уведомления, которые планировщик адресует чатам.
type UpdateRepository interface {
	// AddUpdates создаёт по одному уведомлению для каждого существующего чата из chatIDs.
	// Несуществующие (например, только что удалённые) чаты молча пропускаются.
	AddUpdates(ctx context.Context, chatIDs []int64, url, description string) error

	// GetUpdates возвращает до limit самых свежих уведомлений чата с id > afterID,
	// отсортированных по возрастанию id.
	GetUpdates(ctx context.Context, chatID, afterID int64, limit int) ([]*Update, error)
}
