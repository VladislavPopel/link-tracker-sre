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

const (
	pgUniqueViolation = "23505"

	insertTagSQL = `INSERT INTO link_tags (chat_id, link_id, tag) VALUES ($1, $2, $3)
		ON CONFLICT DO NOTHING`
	insertFilterSQL = `INSERT INTO link_filters (chat_id, link_id, filter) VALUES ($1, $2, $3)
		ON CONFLICT DO NOTHING`

	// Теги и фильтры собираются коррелированными подзапросами, а не JOIN'ами:
	// два LEFT JOIN по разным таблицам дали бы декартово произведение (дубли в массивах).
	getLinksSQL = `
SELECT l.id,
       l.url,
       COALESCE((SELECT array_agg(t.tag ORDER BY t.tag)
                 FROM link_tags t
                 WHERE t.chat_id = cl.chat_id AND t.link_id = l.id), '{}'),
       COALESCE((SELECT array_agg(f.filter ORDER BY f.filter)
                 FROM link_filters f
                 WHERE f.chat_id = cl.chat_id AND f.link_id = l.id), '{}')
FROM links l
JOIN chat_links cl ON cl.link_id = l.id
WHERE cl.chat_id = $1
ORDER BY l.id
LIMIT $2 OFFSET $3`
)

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

// RemoveChat удаляет чат и каскадно все его ссылки, теги и фильтры.
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

// GetChats возвращает зарегистрированные чаты с пагинацией.
func (r *LinkRepo) GetChats(ctx context.Context, page domain.Page) ([]*domain.Chat, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, created_at FROM chats ORDER BY id LIMIT $1 OFFSET $2`,
		page.Limit, page.Offset,
	)
	if err != nil {
		return nil, fmt.Errorf("sql get chats: %w", err)
	}
	defer func() { _ = rows.Close() }()

	chats := make([]*domain.Chat, 0)
	for rows.Next() {
		var c domain.Chat
		if scanErr := rows.Scan(&c.ID, &c.CreatedAt); scanErr != nil {
			return nil, fmt.Errorf("sql scan chat: %w", scanErr)
		}
		chats = append(chats, &c)
	}
	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("sql rows error: %w", err)
	}
	return chats, nil
}

// AddLink добавляет ссылку для чата в транзакции.
func (r *LinkRepo) AddLink(ctx context.Context, link *domain.Link) (*domain.Link, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("sql begin tx: %w", err)
	}
	// Rollback после успешного Commit — no-op (вернёт ErrTxDone, он нам не важен).
	defer func() { _ = tx.Rollback() }()

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

	if err = insertValues(ctx, tx, insertTagSQL, link.ChatID, linkID, link.Tags); err != nil {
		return nil, fmt.Errorf("sql insert tags: %w", err)
	}
	if err = insertValues(ctx, tx, insertFilterSQL, link.ChatID, linkID, link.Filters); err != nil {
		return nil, fmt.Errorf("sql insert filters: %w", err)
	}

	if err = tx.Commit(); err != nil {
		return nil, fmt.Errorf("sql commit: %w", err)
	}

	link.ID = linkID
	return link, nil
}

// UpdateLink заменяет теги и фильтры подписки чата на ссылку (PUT-семантика:
// то, что не передано, удаляется).
func (r *LinkRepo) UpdateLink(
	ctx context.Context,
	chatID, linkID int64,
	tags, filters []string,
) (*domain.Link, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("sql begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	// Подписка должна существовать; заодно достаём url для ответа.
	var url string
	err = tx.QueryRowContext(ctx,
		`SELECT l.url
		   FROM links l
		   JOIN chat_links cl ON cl.link_id = l.id
		  WHERE cl.chat_id = $1 AND l.id = $2`,
		chatID, linkID,
	).Scan(&url)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, domain.ErrLinkNotFound
		}
		return nil, fmt.Errorf("sql find subscription: %w", err)
	}

	if _, err = tx.ExecContext(ctx,
		`DELETE FROM link_tags WHERE chat_id = $1 AND link_id = $2`, chatID, linkID); err != nil {
		return nil, fmt.Errorf("sql delete tags: %w", err)
	}
	if _, err = tx.ExecContext(ctx,
		`DELETE FROM link_filters WHERE chat_id = $1 AND link_id = $2`, chatID, linkID); err != nil {
		return nil, fmt.Errorf("sql delete filters: %w", err)
	}

	if err = insertValues(ctx, tx, insertTagSQL, chatID, linkID, tags); err != nil {
		return nil, fmt.Errorf("sql insert tags: %w", err)
	}
	if err = insertValues(ctx, tx, insertFilterSQL, chatID, linkID, filters); err != nil {
		return nil, fmt.Errorf("sql insert filters: %w", err)
	}

	if err = tx.Commit(); err != nil {
		return nil, fmt.Errorf("sql commit: %w", err)
	}

	return &domain.Link{ID: linkID, URL: url, Tags: tags, Filters: filters, ChatID: chatID}, nil
}

// RemoveLink удаляет подписку чата на ссылку в транзакции.
func (r *LinkRepo) RemoveLink(ctx context.Context, chatID int64, url string) (*domain.Link, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("sql begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	// Находим ссылку
	var linkID int64
	err = tx.QueryRowContext(ctx, `SELECT id FROM links WHERE url = $1`, url).Scan(&linkID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, domain.ErrLinkNotFound
		}
		return nil, fmt.Errorf("sql find link: %w", err)
	}

	// Удаляем теги и фильтры подписки
	_, err = tx.ExecContext(ctx,
		`DELETE FROM link_tags WHERE chat_id = $1 AND link_id = $2`,
		chatID, linkID,
	)
	if err != nil {
		return nil, fmt.Errorf("sql delete tags: %w", err)
	}
	_, err = tx.ExecContext(ctx,
		`DELETE FROM link_filters WHERE chat_id = $1 AND link_id = $2`,
		chatID, linkID,
	)
	if err != nil {
		return nil, fmt.Errorf("sql delete filters: %w", err)
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

// GetLinks возвращает ссылки чата с тегами и фильтрами, с пагинацией.
func (r *LinkRepo) GetLinks(ctx context.Context, chatID int64, page domain.Page) ([]*domain.Link, error) {
	rows, err := r.db.QueryContext(ctx, getLinksSQL, chatID, page.Limit, page.Offset)
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
			l       domain.Link
			tags    []string
			filters []string
		)
		if err := rows.Scan(&l.ID, &l.URL, pq.Array(&tags), pq.Array(&filters)); err != nil {
			return nil, fmt.Errorf("sql scan link: %w", err)
		}
		l.ChatID = chatID
		l.Tags = tags
		l.Filters = filters
		links = append(links, &l)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("sql rows error: %w", err)
	}
	return links, nil
}

// insertValues вставляет значения (теги или фильтры) для подписки чата на ссылку.
// query — одна из констант insertTagSQL / insertFilterSQL.
func insertValues(ctx context.Context, tx *sql.Tx, query string, chatID, linkID int64, values []string) error {
	for _, v := range values {
		if _, err := tx.ExecContext(ctx, query, chatID, linkID, v); err != nil {
			return err
		}
	}
	return nil
}
