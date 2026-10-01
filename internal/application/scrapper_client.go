package application

import (
	"context"

	"gitlab.education.tbank.ru/backend-academy-go-2025/homeworks/link-tracker/internal/domain"
)

// ScrapperClient определяет интерфейс взаимодействия с сервисом Scrapper
type ScrapperClient interface {
	RegisterChat(ctx context.Context, chatID int64) error
	AddLink(ctx context.Context, chatID int64, url string, tags []string, filters []string) (*domain.Link, error)
	RemoveLink(ctx context.Context, chatID int64, url string) (*domain.Link, error)
	GetLinks(ctx context.Context, chatID int64) ([]*domain.Link, error)
}
