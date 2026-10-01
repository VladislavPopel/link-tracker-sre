package application

import (
	"context"

	"gitlab.education.tbank.ru/backend-academy-go-2025/homeworks/link-tracker/internal/domain"
)

type mockScrapperClient struct {
	links      []*domain.Link
	addErr     error
	removeErr  error
	getErr     error
	regErr     error
	addedLinks []*domain.Link
}

func (m *mockScrapperClient) RegisterChat(_ context.Context, _ int64) error { return m.regErr }

func (m *mockScrapperClient) AddLink(_ context.Context, chatID int64, rawURL string, tags []string, filters []string) (*domain.Link, error) {
	if m.addErr != nil {
		return nil, m.addErr
	}
	l := &domain.Link{ID: int64(len(m.addedLinks) + 1), URL: rawURL, Tags: tags, Filters: filters, ChatID: chatID}
	m.addedLinks = append(m.addedLinks, l)
	return l, nil
}

func (m *mockScrapperClient) RemoveLink(_ context.Context, chatID int64, rawURL string) (*domain.Link, error) {
	if m.removeErr != nil {
		return nil, m.removeErr
	}
	return &domain.Link{ID: 1, URL: rawURL, ChatID: chatID}, nil
}

func (m *mockScrapperClient) GetLinks(_ context.Context, _ int64) ([]*domain.Link, error) {
	if m.getErr != nil {
		return nil, m.getErr
	}
	return m.links, nil
}
