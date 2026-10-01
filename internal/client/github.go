package client

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
)

const (
	httpTimeout = 10 * time.Second
	urlSplitN   = 2
)

var ErrNoUpdates = errors.New("no new updates")

type GitHubRepo struct {
	UpdatedAt time.Time `json:"updated_at"`
	PushedAt  time.Time `json:"pushed_at"`
}

// GitHubIssue представляет Issue или PR из GitHub API)
type GitHubIssue struct {
	Title     string    `json:"title"`
	Body      string    `json:"body"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	User      struct {
		Login string `json:"login"`
	} `json:"user"`
	// PullRequest не nil, если это PR, а не обычный Issue
	PullRequest *struct{} `json:"pull_request"`
}

type GitHubClient struct {
	httpClient *http.Client
	token      string
	baseURL    string
}

func NewGitHubClient(token string) *GitHubClient {
	return &GitHubClient{
		httpClient: &http.Client{Timeout: httpTimeout},
		token:      token,
		baseURL:    "https://api.github.com",
	}
}

// GetLastUpdated возвращает время последнего обновления репозитория
// URL должен быть вида https://github.com/owner/repo
func (c *GitHubClient) GetLastUpdated(ctx context.Context, repoURL string) (time.Time, error) {
	owner, repo, err := parseGitHubURL(repoURL)
	if err != nil {
		return time.Time{}, err
	}

	apiURL := fmt.Sprintf("%s/repos/%s/%s", c.baseURL, owner, repo)
	req, err := c.newRequest(ctx, apiURL)
	if err != nil {
		return time.Time{}, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return time.Time{}, fmt.Errorf("do request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return time.Time{}, fmt.Errorf("github API returned status %d", resp.StatusCode)
	}

	var result GitHubRepo
	if err = json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return time.Time{}, fmt.Errorf("decode response: %w", err)
	}

	if result.PushedAt.After(result.UpdatedAt) {
		return result.PushedAt, nil
	}
	return result.UpdatedAt, nil
}

// GetLatestUpdate возвращает самый новый Issue или PR, обновлённый после since
func (c *GitHubClient) GetLatestUpdate(ctx context.Context, repoURL string, since time.Time) (*GitHubIssue, error) {
	owner, repo, err := parseGitHubURL(repoURL)
	if err != nil {
		return nil, err
	}

	apiURL := fmt.Sprintf(
		"%s/repos/%s/%s/issues?state=all&sort=updated&direction=desc&per_page=5&since=%s",
		c.baseURL, owner, repo,
		since.UTC().Format(time.RFC3339),
	)

	req, err := c.newRequest(ctx, apiURL)
	if err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("do request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("github API returned status %d", resp.StatusCode)
	}

	var issues []GitHubIssue
	if err = json.NewDecoder(resp.Body).Decode(&issues); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	if len(issues) == 0 {
		return nil, ErrNoUpdates
	}
	return &issues[0], nil
}

// parseGitHubURL разбирает URL вида https://github.com/owner/repo
func parseGitHubURL(rawURL string) (owner, repo string, err error) {
	rawURL = strings.TrimPrefix(rawURL, "https://github.com/")
	rawURL = strings.TrimPrefix(rawURL, "http://github.com/")
	rawURL = strings.TrimSuffix(rawURL, "/")

	parts := strings.SplitN(rawURL, "/", urlSplitN)
	if len(parts) != urlSplitN || parts[0] == "" || parts[1] == "" {
		return "", "", errors.New("invalid GitHub URL: expected https://github.com/owner/repo")
	}
	return parts[0], parts[1], nil
}

// SetBaseURL подменяет базовый URL GitHub API(для тестов)
func (c *GitHubClient) SetBaseURL(url string) {
	c.baseURL = url
}

// newRequest создаёт HTTP-запрос с нужными заголовками GitHub API
func (c *GitHubClient) newRequest(ctx context.Context, url string) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	return req, nil
}
