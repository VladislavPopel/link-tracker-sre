package sqlrepo

import (
	"context"
	"database/sql"
	"fmt"

	"gitlab.education.tbank.ru/backend-academy-go-2025/homeworks/link-tracker/internal/domain"
)

// Внутренний запрос берёт самые свежие limit строк (ORDER BY id DESC LIMIT),
// внешний возвращает их в хронологическом порядке.
// Так при первой загрузке (afterId=0) фронт получает последние уведомления,
// а не самые старые, а при опросе — только то, что появилось после afterId.
const getUpdatesSQL = `
SELECT id, chat_id, url, description, created_at
FROM (
    SELECT id, chat_id, url, description, created_at
    FROM updates
    WHERE chat_id = $1 AND id > $2
    ORDER BY id DESC
    LIMIT $3
) latest
ORDER BY id`

// UpdateRepo - реализация domain.UpdateRepository на raw SQL
type UpdateRepo struct {
	db *sql.DB
}

func NewUpdateRepo(db *sql.DB) *UpdateRepo {
	return &UpdateRepo{db: db}
}

// AddUpdates записывает уведомление для каждого существующего чата в одной транзакции.
func (r *UpdateRepo) AddUpdates(ctx context.Context, chatIDs []int64, url, description string) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("sql begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	for _, chatID := range chatIDs {
		// INSERT ... SELECT ... FROM chats: если чат уже удалён, строк не будет
		// и внешний ключ не нарушится. Касты нужны, чтобы Postgres понял типы параметров.
		_, err = tx.ExecContext(ctx,
			`INSERT INTO updates (chat_id, url, description)
			 SELECT id, $2::text, $3::text FROM chats WHERE id = $1`,
			chatID, url, description,
		)
		if err != nil {
			return fmt.Errorf("sql insert update for chat %d: %w", chatID, err)
		}
	}

	if err = tx.Commit(); err != nil {
		return fmt.Errorf("sql commit: %w", err)
	}
	return nil
}

// GetUpdates возвращает уведомления чата, появившиеся после afterID.
func (r *UpdateRepo) GetUpdates(ctx context.Context, chatID, afterID int64, limit int) ([]*domain.Update, error) {
	rows, err := r.db.QueryContext(ctx, getUpdatesSQL, chatID, afterID, limit)
	if err != nil {
		return nil, fmt.Errorf("sql get updates: %w", err)
	}
	defer func() { _ = rows.Close() }()

	updates := make([]*domain.Update, 0)
	for rows.Next() {
		var u domain.Update
		if scanErr := rows.Scan(&u.ID, &u.ChatID, &u.URL, &u.Description, &u.CreatedAt); scanErr != nil {
			return nil, fmt.Errorf("sql scan update: %w", scanErr)
		}
		updates = append(updates, &u)
	}
	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("sql rows error: %w", err)
	}
	return updates, nil
}
