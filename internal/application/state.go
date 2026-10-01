package application

import "sync"

type SessionState int

const (
	StateIdle           SessionState = iota
	StateTrackWaitURL                // /track: ожидаем URL
	StateTrackWaitTags               // /track: ожидаем теги
	StateUntrackWaitURL              // /untrack: ожидаем URL
)

type Session struct {
	State      SessionState
	PendingURL string
}

type SessionStore struct {
	mu       sync.RWMutex
	sessions map[int64]Session
}

func NewSessionStore() *SessionStore {
	return &SessionStore{sessions: make(map[int64]Session)}
}

func (s *SessionStore) Get(chatID int64) Session {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if sess, ok := s.sessions[chatID]; ok {
		return sess
	}
	return Session{State: StateIdle}
}

func (s *SessionStore) Set(chatID int64, sess Session) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sessions[chatID] = sess
}

func (s *SessionStore) Reset(chatID int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.sessions, chatID)
}
