package ormrepo

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgconn"
	"gitlab.education.tbank.ru/backend-academy-go-2025/homeworks/link-tracker/internal/domain"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// GORM models

type Chat struct {
	ID int64 `gorm:"primaryKey"`
}

type Link struct {
	ID  int64  `gorm:"primaryKey;autoIncrement"`
	URL string `gorm:"uniqueIndex;not null"`
}

type ChatLink struct {
	ChatID int64 `gorm:"primaryKey"`
	LinkID int64 `gorm:"primaryKey"`
}

type LinkTag struct {
	ID     int64  `gorm:"primaryKey;autoIncrement"`
	ChatID int64  `gorm:"uniqueIndex:idx_chat_link_tag"`
	LinkID int64  `gorm:"uniqueIndex:idx_chat_link_tag"`
	Tag    string `gorm:"uniqueIndex:idx_chat_link_tag"`
}

// Repository

// LinkRepo - реализация domain.LinkRepository через GORM.
type LinkRepo struct {
	db *gorm.DB
}

func NewLinkRepo(db *gorm.DB) *LinkRepo {
	return &LinkRepo{db: db}
}

func (r *LinkRepo) AddChat(ctx context.Context, chatID int64) error {
	result := r.db.WithContext(ctx).
		Clauses(clause.OnConflict{DoNothing: true}).
		Create(&Chat{ID: chatID})
	if result.Error != nil {
		return fmt.Errorf("orm add chat: %w", result.Error)
	}
	return nil
}

func (r *LinkRepo) RemoveChat(ctx context.Context, chatID int64) error {
	result := r.db.WithContext(ctx).Delete(&Chat{}, chatID)
	if result.Error != nil {
		return fmt.Errorf("orm remove chat: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return domain.ErrChatNotFound
	}
	return nil
}

func (r *LinkRepo) AddLink(ctx context.Context, link *domain.Link) (*domain.Link, error) {
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Проверяем чат
		var chat Chat
		if err := tx.First(&chat, link.ChatID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return domain.ErrChatNotFound
			}
			return fmt.Errorf("orm find chat: %w", err)
		}

		// Upsert ссылки
		gLink := Link{URL: link.URL}
		if err := tx.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "url"}},
			DoUpdates: clause.AssignmentColumns([]string{"url"}),
		}).Create(&gLink).Error; err != nil {
			return fmt.Errorf("orm upsert link: %w", err)
		}
		link.ID = gLink.ID

		// Связь чат-ссылка
		chatLink := ChatLink{ChatID: link.ChatID, LinkID: gLink.ID}
		if err := tx.Create(&chatLink).Error; err != nil {
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) && pgErr.Code == "23505" {
				return domain.ErrLinkExists
			}
			return fmt.Errorf("orm insert chat_link: %w", err)
		}

		// Теги
		for _, tag := range link.Tags {
			lt := LinkTag{ChatID: link.ChatID, LinkID: gLink.ID, Tag: tag}
			if err := tx.Clauses(clause.OnConflict{DoNothing: true}).
				Create(&lt).Error; err != nil {
				return fmt.Errorf("orm insert tag: %w", err)
			}
		}

		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("orm add link transaction: %w", err)
	}
	return link, nil
}

func (r *LinkRepo) RemoveLink(ctx context.Context, chatID int64, url string) (*domain.Link, error) {
	var result *domain.Link

	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var gLink Link
		if err := tx.Where("url = ?", url).First(&gLink).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return domain.ErrLinkNotFound
			}
			return fmt.Errorf("orm find link: %w", err)
		}

		// Удаляем теги
		if err := tx.Where("chat_id = ? AND link_id = ?", chatID, gLink.ID).
			Delete(&LinkTag{}).Error; err != nil {
			return fmt.Errorf("orm delete tags: %w", err)
		}

		// Удаляем подписку
		res := tx.Where("chat_id = ? AND link_id = ?", chatID, gLink.ID).
			Delete(&ChatLink{})
		if res.Error != nil {
			return fmt.Errorf("orm delete chat_link: %w", res.Error)
		}
		if res.RowsAffected == 0 {
			return domain.ErrLinkNotFound
		}

		result = &domain.Link{ID: gLink.ID, URL: gLink.URL, ChatID: chatID}
		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("orm remove link transaction: %w", err)
	}
	return result, nil
}

func (r *LinkRepo) GetLinks(ctx context.Context, chatID int64, page domain.Page) ([]*domain.Link, error) {
	type row struct {
		ID  int64
		URL string
		Tag *string
	}

	var rows []row
	err := r.db.WithContext(ctx).
		Model(&Link{}).
		Select("links.id, links.url, link_tags.tag").
		Joins("JOIN chat_links ON chat_links.link_id = links.id").
		Joins("LEFT JOIN link_tags ON link_tags.link_id = links.id AND link_tags.chat_id = chat_links.chat_id").
		Where("chat_links.chat_id = ?", chatID).
		Order("links.id").
		Limit(page.Limit).
		Offset(page.Offset).
		Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("orm get links: %w", err)
	}

	// Группировка тегов
	grouped := make(map[int64]*domain.Link)
	var order []int64
	for _, row := range rows {
		if _, ok := grouped[row.ID]; !ok {
			grouped[row.ID] = &domain.Link{ID: row.ID, URL: row.URL, ChatID: chatID}
			order = append(order, row.ID)
		}
		if row.Tag != nil {
			grouped[row.ID].Tags = append(grouped[row.ID].Tags, *row.Tag)
		}
	}

	links := make([]*domain.Link, 0, len(order))
	for _, id := range order {
		links = append(links, grouped[id])
	}
	return links, nil
}

func (r *LinkRepo) GetAllLinks(ctx context.Context, page domain.Page) ([]*domain.Link, error) {
	type row struct {
		ID     int64
		URL    string
		ChatID int64
		Tag    *string
	}

	var rows []row
	err := r.db.WithContext(ctx).
		Model(&Link{}).
		Select("links.id, links.url, chat_links.chat_id, link_tags.tag").
		Joins("JOIN chat_links ON chat_links.link_id = links.id").
		Joins("LEFT JOIN link_tags ON link_tags.link_id = links.id AND link_tags.chat_id = chat_links.chat_id").
		Order("links.id").
		Limit(page.Limit).
		Offset(page.Offset).
		Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("orm get all links: %w", err)
	}

	// Группируем теги по ссылке
	type key struct{ linkID, chatID int64 }
	grouped := make(map[key]*domain.Link)
	var order []key

	for _, r := range rows {
		k := key{r.ID, r.ChatID}
		if _, ok := grouped[k]; !ok {
			grouped[k] = &domain.Link{ID: r.ID, URL: r.URL, ChatID: r.ChatID}
			order = append(order, k)
		}
		if r.Tag != nil {
			grouped[k].Tags = append(grouped[k].Tags, *r.Tag)
		}
	}

	links := make([]*domain.Link, 0, len(order))
	for _, k := range order {
		links = append(links, grouped[k])
	}
	return links, nil
}
