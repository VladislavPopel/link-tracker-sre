package domain

import (
	"context"
	"errors"
)

var (
	ErrLinkNotFound = errors.New("link not found")
	ErrLinkExists   = errors.New("link already tracked")
	ErrChatNotFound = errors.New("chat not found")
	ErrChatExists   = errors.New("chat already registered")
)

// Link представляет отслеживаемую ссылку пользователя
type Link struct {
	ID      int64
	URL     string
	Tags    []string
	Filters []string
	ChatID  int64
}

// Page описывает параметры пагинации.
type Page struct {
	Limit  int
	Offset int
}

// LinkRepository определяет интерфейс хранилища ссылок
// Не содержит типов специфичных для конкретной реализации (sql.Row и т.д.)
type LinkRepository interface {
	AddChat(ctx context.Context, chatID int64) error
	RemoveChat(ctx context.Context, chatID int64) error
	AddLink(ctx context.Context, link *Link) (*Link, error)
	RemoveLink(ctx context.Context, chatID int64, url string) (*Link, error)
	GetLinks(ctx context.Context, chatID int64, page Page) ([]*Link, error)
	GetAllLinks(ctx context.Context, page Page) ([]*Link, error)
}
