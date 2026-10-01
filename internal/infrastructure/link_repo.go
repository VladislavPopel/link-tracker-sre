package infrastructure

import (
	"sync"

	"gitlab.education.tbank.ru/backend-academy-go-2025/homeworks/link-tracker/internal/domain"
)

// InMemoryLinkRepo Хранит список зарегистрированных чатов и ссылок.
type InMemoryLinkRepo struct {
	mu     sync.RWMutex
	chats  map[int64]struct{}       // зарегистрированные chat ID
	links  map[int64][]*domain.Link // chat ID - []Link
	nextID int64
}

func NewInMemoryLinkRepo() *InMemoryLinkRepo {
	return &InMemoryLinkRepo{
		chats: make(map[int64]struct{}),
		links: make(map[int64][]*domain.Link),
	}
}

// AddChat регистрирует чат
func (r *InMemoryLinkRepo) AddChat(chatID int64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.chats[chatID] = struct{}{}
	if _, ok := r.links[chatID]; !ok {
		r.links[chatID] = []*domain.Link{}
	}
	return nil
}

// RemoveChat удаляет чат вместе со всеми его ссылками
func (r *InMemoryLinkRepo) RemoveChat(chatID int64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.chats[chatID]; !ok {
		return domain.ErrChatNotFound
	}
	delete(r.chats, chatID)
	delete(r.links, chatID)
	return nil
}

// AddLink добавляет ссылку для чата
func (r *InMemoryLinkRepo) AddLink(link *domain.Link) (*domain.Link, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.chats[link.ChatID]; !ok {
		return nil, domain.ErrChatNotFound
	}

	for _, l := range r.links[link.ChatID] {
		if l.URL == link.URL {
			return nil, domain.ErrLinkExists
		}
	}

	r.nextID++
	link.ID = r.nextID
	stored := *link
	r.links[link.ChatID] = append(r.links[link.ChatID], &stored)
	return &stored, nil
}

func (r *InMemoryLinkRepo) RemoveLink(chatID int64, url string) (*domain.Link, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.chats[chatID]; !ok {
		return nil, domain.ErrChatNotFound
	}

	list := r.links[chatID]
	for i, l := range list {
		if l.URL == url {
			r.links[chatID] = append(list[:i], list[i+1:]...)
			return l, nil
		}
	}
	return nil, domain.ErrLinkNotFound
}

func (r *InMemoryLinkRepo) GetLinks(chatID int64) ([]*domain.Link, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if _, ok := r.chats[chatID]; !ok {
		return nil, domain.ErrChatNotFound
	}

	list := r.links[chatID]
	result := make([]*domain.Link, len(list))
	copy(result, list)
	return result, nil
}

func (r *InMemoryLinkRepo) GetAllLinks() ([]*domain.Link, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []*domain.Link
	for _, list := range r.links {
		result = append(result, list...)
	}
	return result, nil
}
