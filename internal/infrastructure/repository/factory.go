package repository

import (
	"database/sql"
	"fmt"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"gitlab.education.tbank.ru/backend-academy-go-2025/homeworks/link-tracker/internal/domain"
	ormrepo "gitlab.education.tbank.ru/backend-academy-go-2025/homeworks/link-tracker/internal/infrastructure/repository/orm"
	sqlrepo "gitlab.education.tbank.ru/backend-academy-go-2025/homeworks/link-tracker/internal/infrastructure/repository/sql"
)

// AccessType определяет способ работы с БД.
type AccessType string

const (
	AccessTypeSQL AccessType = "SQL"
	AccessTypeORM AccessType = "ORM"
)

// New возвращает реализацию LinkRepository в зависимости от access-type
func New(db *sql.DB, dsn string, accessType AccessType) (domain.LinkRepository, error) {
	switch accessType {
	case AccessTypeSQL:
		return sqlrepo.NewLinkRepo(db), nil
	case AccessTypeORM:
		gdb, err := newGORM(dsn)
		if err != nil {
			return nil, err
		}
		return ormrepo.NewLinkRepo(gdb), nil
	default:
		return nil, fmt.Errorf("unknown access-type %q, use SQL or ORM", accessType)
	}
}

func newGORM(dsn string) (*gorm.DB, error) {
	gdb, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		return nil, fmt.Errorf("gorm open: %w", err)
	}
	return gdb, nil
}
