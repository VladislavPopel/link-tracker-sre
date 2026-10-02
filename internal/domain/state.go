package domain

import (
	"context"
	"time"
)

// LinkStateRepository хранит время последнего известного обновления каждой ссылки
type LinkStateRepository interface {
	// GetLastSeen возвращает время последнего известного обновлени
	GetLastSeen(ctx context.Context, url string) (time.Time, error)

	// AdvanceLastSeen атомарно меняет состояние с expected на next (compare-and-swap
	AdvanceLastSeen(ctx context.Context, url string, expected, next time.Time) (bool, error)
}
