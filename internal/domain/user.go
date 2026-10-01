package domain

import "errors"

var ErrUserNotFound = errors.New("user not found")

// User - пользователь в системе
type User struct {
	ID int64
}

type UserRepository interface {
	Get(id int64) (*User, error)
	Save(user *User) error
}
