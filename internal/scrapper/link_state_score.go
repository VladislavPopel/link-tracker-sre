package scrapper

import (
	"sync"
	"time"
)

// LinkStateStore хранит время последнего известного обновления ссылок
type LinkStateStore struct {
	mu       sync.RWMutex
	lastSeen map[string]time.Time
}

func NewLinkStateStore() *LinkStateStore {
	return &LinkStateStore{lastSeen: make(map[string]time.Time)}
}

func (s *LinkStateStore) Get(url string) time.Time {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.lastSeen[url]
}

// Set устанавливает время последнего обновления для ссылки
func (s *LinkStateStore) Set(url string, t time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastSeen[url] = t
}

func (s *LinkStateStore) GetAndUpdate(url string, updated time.Time) (changed bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	last, seen := s.lastSeen[url]
	if !seen {
		s.lastSeen[url] = updated
		return false
	}
	if updated.After(last) {
		s.lastSeen[url] = updated
		return true
	}
	return false
}

// Delete вызывается при удалении ссылки из репозитория
func (s *LinkStateStore) Delete(url string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.lastSeen, url)
}
