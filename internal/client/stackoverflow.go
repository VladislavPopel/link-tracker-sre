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

type soQuestion struct {
	LastActivityDate int64 `json:"last_activity_date"`
}

type soResponse struct {
	Items []soQuestion `json:"items"`
}

// SOAnswer представляет ответ на вопрос StackOverflow
type SOAnswer struct {
	AnswerID     int64  `json:"answer_id"`
	CreationDate int64  `json:"creation_date"`
	Body         string `json:"body"` // HTML; требует filter=withbody
	Owner        struct {
		DisplayName string `json:"display_name"`
	} `json:"owner"`
}

type soAnswersResponse struct {
	Items []SOAnswer `json:"items"`
}

type StackOverflowClient struct {
	httpClient *http.Client
	key        string
	baseURL    string
}

func NewStackOverflowClient(key string) *StackOverflowClient {
	return &StackOverflowClient{
		httpClient: &http.Client{Timeout: httpTimeout},
		key:        key,
		baseURL:    "https://api.stackexchange.com/2.3",
	}
}

// GetLastUpdated возвращает дату последней активности,
// URL должен быть вида https://stackoverflow.com/questions/{id}/...
func (c *StackOverflowClient) GetLastUpdated(ctx context.Context, questionURL string) (time.Time, error) {
	questionID, err := parseStackOverflowURL(questionURL)
	if err != nil {
		return time.Time{}, err
	}

	apiURL := fmt.Sprintf("%s/questions/%s?site=stackoverflow", c.baseURL, questionID)
	if c.key != "" {
		apiURL += "&key=" + c.key
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return time.Time{}, fmt.Errorf("create request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return time.Time{}, fmt.Errorf("do request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return time.Time{}, fmt.Errorf("stackoverflow API returned status %d", resp.StatusCode)
	}

	var result soResponse
	if err = json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return time.Time{}, fmt.Errorf("decode response: %w", err)
	}

	if len(result.Items) == 0 {
		return time.Time{}, fmt.Errorf("question not found: %s", questionID)
	}

	return time.Unix(result.Items[0].LastActivityDate, 0), nil
}

// GetLatestAnswer возвращает последний ответ на вопрос
func (c *StackOverflowClient) GetLatestAnswer(ctx context.Context, questionURL string) (*SOAnswer, error) {
	questionID, err := parseStackOverflowURL(questionURL)
	if err != nil {
		return nil, err
	}

	// filter=withbody включает поле body в ответе (HTML)
	apiURL := fmt.Sprintf(
		"%s/questions/%s/answers?site=stackoverflow&sort=creation&order=desc&pagesize=1&filter=withbody",
		c.baseURL, questionID,
	)
	if c.key != "" {
		apiURL += "&key=" + c.key
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("do request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("stackoverflow API returned status %d", resp.StatusCode)
	}

	var result soAnswersResponse
	if err = json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	if len(result.Items) == 0 {
		return nil, ErrNoUpdates
	}
	return &result.Items[0], nil
}

// parseStackOverflowURL извлекает ID вопроса из URL
func parseStackOverflowURL(rawURL string) (string, error) {
	for _, prefix := range []string{
		"https://stackoverflow.com/questions/",
		"http://stackoverflow.com/questions/",
	} {
		rawURL = strings.TrimPrefix(rawURL, prefix)
	}
	parts := strings.SplitN(rawURL, "/", urlSplitN)
	if len(parts) == 0 || parts[0] == "" {
		return "", errors.New("invalid StackOverflow URL: expected https://stackoverflow.com/questions/{id}")
	}
	return parts[0], nil
}

// SetBaseURL подменяет базовый URL StackOverflow API(для тестов)
func (c *StackOverflowClient) SetBaseURL(url string) {
	c.baseURL = url
}
