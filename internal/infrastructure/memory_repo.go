package infrastructure

import (
	"errors"
	"sync"

	"gitlab.education.tbank.ru/backend-academy-go-2025/homeworks/link-tracker/internal/domain"
)

type InMemoryUserRepo struct {
	mu    sync.RWMutex
	users map[int64]*domain.User
}

func NewInMemoryUserRepo() *InMemoryUserRepo {
	return &InMemoryUserRepo{
		users: make(map[int64]*domain.User),
	}
}

func (r *InMemoryUserRepo) Get(id int64) (*domain.User, error) {
	r.mu.RLock() // чтение разрешено многим одновременно
	defer r.mu.RUnlock()
	user, ok := r.users[id]
	if !ok {
		return nil, domain.ErrUserNotFound
	}
	return user, nil
}

func (r *InMemoryUserRepo) Save(user *domain.User) error {
	if user == nil {
		return errors.New("user must not be nil")
	}
	r.mu.Lock() // эксклюзивная блокировка для записи
	defer r.mu.Unlock()
	r.users[user.ID] = user
	return nil
}
