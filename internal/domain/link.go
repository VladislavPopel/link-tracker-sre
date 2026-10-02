package domain

import (
	"context"
	"errors"
	"time"
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

// Chat — зарегистрированный пользователь (рабочее пространство) системы
type Chat struct {
	ID        int64
	CreatedAt time.Time
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
	GetChats(ctx context.Context, page Page) ([]*Chat, error)

	AddLink(ctx context.Context, link *Link) (*Link, error)
	// UpdateLink полностью заменяет теги и фильтры подписки чата на ссылку (семантика PUT).
	UpdateLink(ctx context.Context, chatID, linkID int64, tags, filters []string) (*Link, error)
	RemoveLink(ctx context.Context, chatID int64, url string) (*Link, error)
	GetLinks(ctx context.Context, chatID int64, page Page) ([]*Link, error)
	GetAllLinks(ctx context.Context, page Page) ([]*Link, error)
}
