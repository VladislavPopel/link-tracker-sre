package scrapper

import (
	"context"

	"gitlab.education.tbank.ru/backend-academy-go-2025/homeworks/link-tracker/internal/client"
)

// SetClientBaseURL  позволяет подменить базовый URL GitHub API (для тестов с httptest)
func (c *GitHubChecker) SetClientBaseURL(url string) {
	c.inner.SetBaseURL(url)
}

// SetClientBaseURL  позволяет подменить базовый URL StackOverflow API (для тестов)
func (c *StackOverflowChecker) SetClientBaseURL(url string) {
	c.inner.SetBaseURL(url)
}

// NewGitHubCheckerWithClient создаёт GitHubChecker с явно переданным клиентом
// Используется в тестах для инжекции mock-клиента вместо реального
func NewGitHubCheckerWithClient(c *client.GitHubClient) *GitHubChecker {
	return &GitHubChecker{inner: c}
}

// NewStackOverflowCheckerWithClient создаёт StackOverflowChecker с явно переданным клиентом
func NewStackOverflowCheckerWithClient(c *client.StackOverflowClient) *StackOverflowChecker {
	return &StackOverflowChecker{inner: c}
}

// RunOnce вызывает один цикл проверки синхронно (без планировщика)
// Используется в тестах вместо запуска по таймеру
func (s *Scheduler) RunOnce(ctx context.Context) {
	s.checkAll(ctx)
}
