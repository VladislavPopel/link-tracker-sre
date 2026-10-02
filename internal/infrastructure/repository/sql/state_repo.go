package sqlrepo

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"gitlab.education.tbank.ru/backend-academy-go-2025/homeworks/link-tracker/internal/domain"
)

// Проверка на этапе компиляции: StateRepo действительно реализует интерфейс.
var _ domain.LinkStateRepository = (*StateRepo)(nil)

// StateRepo - реализация domain.LinkStateRepository на raw SQL
type StateRepo struct {
	db *sql.DB
}

func NewStateRepo(db *sql.DB) *StateRepo {
	return &StateRepo{db: db}
}

// GetLastSeen возвращает last_seen_at ссылки; для неизвестной или ещё не проверенной ссылки — нулевое время.
func (r *StateRepo) GetLastSeen(ctx context.Context, url string) (time.Time, error) {
	var t sql.NullTime
	err := r.db.QueryRowContext(ctx,
		`SELECT last_seen_at FROM links WHERE url = $1`,
		url,
	).Scan(&t)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return time.Time{}, nil
		}
		return time.Time{}, fmt.Errorf("sql get last seen: %w", err)
	}
	if !t.Valid {
		return time.Time{}, nil
	}
	return t.Time, nil
}

// AdvanceLastSeen — compare-and-swap. IS NOT DISTINCT FROM (в отличие от "=")
// корректно сравнивает и NULL с NULL: так работает самая первая проверка ссылки.
func (r *StateRepo) AdvanceLastSeen(ctx context.Context, url string, expected, next time.Time) (bool, error) {
	exp := sql.NullTime{Time: expected, Valid: !expected.IsZero()}

	res, err := r.db.ExecContext(ctx,
		`UPDATE links
		    SET last_seen_at = $3::timestamptz
		  WHERE url = $1
		    AND last_seen_at IS NOT DISTINCT FROM $2::timestamptz`,
		url, exp, next,
	)
	if err != nil {
		return false, fmt.Errorf("sql advance last seen: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("sql advance last seen rows affected: %w", err)
	}
	return n == 1, nil
}
