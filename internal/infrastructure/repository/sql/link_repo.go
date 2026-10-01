package sqlrepo

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	sq "github.com/Masterminds/squirrel"

	"github.com/lib/pq"

	"github.com/jackc/pgx/v5/pgconn"
	"gitlab.education.tbank.ru/backend-academy-go-2025/homeworks/link-tracker/internal/domain"
)

const pgUniqueViolation = "23505"

// LinkRepo - реализация domain.LinkRepository на raw SQL
type LinkRepo struct {
	db *sql.DB
}

func NewLinkRepo(db *sql.DB) *LinkRepo {
	return &LinkRepo{db: db}
}

// AddChat регистрирует чат. Повторный вызов идемпотентен
func (r *LinkRepo) AddChat(ctx context.Context, chatID int64) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO chats (id) VALUES ($1) ON CONFLICT (id) DO NOTHING`,
		chatID,
	)
	if err != nil {
		return fmt.Errorf("sql add chat: %w", err)
	}
	return nil
}

// RemoveChat удаляет чат и каскадно все его ссылки и теги.
func (r *LinkRepo) RemoveChat(ctx context.Context, chatID int64) error {
	res, err := r.db.ExecContext(ctx,
		`DELETE FROM chats WHERE id = $1`,
		chatID,
	)
	if err != nil {
		return fmt.Errorf("sql remove chat: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("sql remove chat rows affected: %w", err)
	}
	if n == 0 {
		return domain.ErrChatNotFound
	}
	return nil
}

// AddLink добавляет ссылку для чата в транзакции.
func (r *LinkRepo) AddLink(ctx context.Context, link *domain.Link) (*domain.Link, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("sql begin tx: %w", err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	// Проверяем, что чат существует
	var exists bool
	err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM chats WHERE id = $1)`, link.ChatID).Scan(&exists)
	if err != nil {
		return nil, fmt.Errorf("sql check chat exists: %w", err)
	}
	if !exists {
		return nil, domain.ErrChatNotFound
	}

	// Upsert ссылки
	var linkID int64
	err = tx.QueryRowContext(ctx,
		`INSERT INTO links (url) VALUES ($1)
		 ON CONFLICT (url) DO UPDATE SET url = EXCLUDED.url
		 RETURNING id`,
		link.URL,
	).Scan(&linkID)
	if err != nil {
		return nil, fmt.Errorf("sql upsert link: %w", err)
	}

	// Связываем чат со ссылкой
	_, err = tx.ExecContext(ctx,
		`INSERT INTO chat_links (chat_id, link_id) VALUES ($1, $2)`,
		link.ChatID, linkID,
	)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == pgUniqueViolation {
			return nil, domain.ErrLinkExists
		}
		return nil, fmt.Errorf("sql insert chat_link: %w", err)
	}

	// Вставляем теги
	for _, tag := range link.Tags {
		_, err = tx.ExecContext(ctx,
			`INSERT INTO link_tags (chat_id, link_id, tag) VALUES ($1, $2, $3)
			 ON CONFLICT DO NOTHING`,
			link.ChatID, linkID, tag,
		)
		if err != nil {
			return nil, fmt.Errorf("sql insert tag: %w", err)
		}
	}

	if err = tx.Commit(); err != nil {
		return nil, fmt.Errorf("sql commit: %w", err)
	}

	link.ID = linkID
	return link, nil
}

// RemoveLink удаляет подписку чата на ссылку в транзакции.
func (r *LinkRepo) RemoveLink(ctx context.Context, chatID int64, url string) (*domain.Link, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("sql begin tx: %w", err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	// Находим ссылку
	var linkID int64
	err = tx.QueryRowContext(ctx, `SELECT id FROM links WHERE url = $1`, url).Scan(&linkID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, domain.ErrLinkNotFound
		}
		return nil, fmt.Errorf("sql find link: %w", err)
	}

	// Удаляем теги подписки
	_, err = tx.ExecContext(ctx,
		`DELETE FROM link_tags WHERE chat_id = $1 AND link_id = $2`,
		chatID, linkID,
	)
	if err != nil {
		return nil, fmt.Errorf("sql delete tags: %w", err)
	}

	// Удаляем подписку
	res, err := tx.ExecContext(ctx,
		`DELETE FROM chat_links WHERE chat_id = $1 AND link_id = $2`,
		chatID, linkID,
	)
	if err != nil {
		return nil, fmt.Errorf("sql delete chat_link: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return nil, fmt.Errorf("sql rows affected: %w", err)
	}
	if n == 0 {
		return nil, domain.ErrLinkNotFound
	}

	if err = tx.Commit(); err != nil {
		return nil, fmt.Errorf("sql commit: %w", err)
	}

	return &domain.Link{ID: linkID, URL: url, ChatID: chatID}, nil
}

// GetLinks возвращает ссылки чата с пагинацией.
func (r *LinkRepo) GetLinks(ctx context.Context, chatID int64, page domain.Page) ([]*domain.Link, error) {
	query, args, err := sq.
		Select("l.id", "l.url", "COALESCE(array_agg(lt.tag) FILTER (WHERE lt.tag IS NOT NULL), '{}')").
		From("links l").
		Join("chat_links cl ON cl.link_id = l.id").
		LeftJoin("link_tags lt ON lt.link_id = l.id AND lt.chat_id = cl.chat_id").
		Where(sq.Eq{"cl.chat_id": chatID}).
		GroupBy("l.id", "l.url").
		OrderBy("l.id").
		Limit(uint64(page.Limit)).
		Offset(uint64(page.Offset)).
		PlaceholderFormat(sq.Dollar).
		ToSql()
	if err != nil {
		return nil, fmt.Errorf("build query: %w", err)
	}
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("sql get links: %w", err)
	}
	defer func() { _ = rows.Close() }()

	return scanLinks(rows, chatID)
}

// GetAllLinks возвращает все ссылки всех чатов с пагинацией (для планировщика).
func (r *LinkRepo) GetAllLinks(ctx context.Context, page domain.Page) ([]*domain.Link, error) {
	query, args, err := sq.
		Select(
			"l.id",
			"l.url",
			"cl.chat_id",
			"COALESCE(array_agg(lt.tag) FILTER (WHERE lt.tag IS NOT NULL), '{}')",
		).
		From("links l").
		Join("chat_links cl ON cl.link_id = l.id").
		LeftJoin("link_tags lt ON lt.link_id = l.id AND lt.chat_id = cl.chat_id").
		GroupBy("l.id", "l.url", "cl.chat_id").
		OrderBy("l.id").
		Limit(uint64(page.Limit)).
		Offset(uint64(page.Offset)).
		PlaceholderFormat(sq.Dollar).
		ToSql()
	if err != nil {
		return nil, fmt.Errorf("build get all links query: %w", err)
	}
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("sql get all links: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var links []*domain.Link
	for rows.Next() {
		var (
			l    domain.Link
			tags []string
		)
		if scanErr := rows.Scan(&l.ID, &l.URL, &l.ChatID, pq.Array(&tags)); scanErr != nil {
			return nil, fmt.Errorf("sql scan all links: %w", scanErr)
		}
		l.Tags = tags
		links = append(links, &l)
	}
	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("sql rows error: %w", err)
	}
	return links, nil
}

func scanLinks(rows *sql.Rows, chatID int64) ([]*domain.Link, error) {
	var links []*domain.Link
	for rows.Next() {
		var (
			l    domain.Link
			tags []string
		)
		if err := rows.Scan(&l.ID, &l.URL, pq.Array(&tags)); err != nil {
			return nil, fmt.Errorf("sql scan link: %w", err)
		}
		l.ChatID = chatID
		l.Tags = tags
		links = append(links, &l)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("sql rows error: %w", err)
	}
	return links, nil
}
