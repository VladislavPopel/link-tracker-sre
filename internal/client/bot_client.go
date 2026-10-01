package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"gitlab.education.tbank.ru/backend-academy-go-2025/homeworks/link-tracker/internal/httphelper"
)

// BotHTTPClient отправляет уведомления об обновлениях сервису Bot
type BotHTTPClient struct {
	baseURL    string
	httpClient *http.Client
}

func NewBotHTTPClient(baseURL string) *BotHTTPClient {
	return &BotHTTPClient{
		baseURL:    baseURL,
		httpClient: &http.Client{Timeout: httpTimeout},
	}
}

// LinkUpdate - тело POST /updates
type LinkUpdate struct {
	ID          int64   `json:"id"`
	URL         string  `json:"url"`
	Description string  `json:"description"`
	TgChatIDs   []int64 `json:"tgChatIds"`
}

// SendUpdate отправляет уведомление об обновлении ссылки боту
func (c *BotHTTPClient) SendUpdate(ctx context.Context, update LinkUpdate) error {
	body, err := json.Marshal(update)
	if err != nil {
		return fmt.Errorf("marshal send update: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/updates", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set(httphelper.HeaderContentType, httphelper.ContentTypeJSON)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("send update request: %w", err)
	}
	//nolint:errcheck // не проверяем ошибку
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, readErr := io.ReadAll(resp.Body)
		if readErr != nil {
			return fmt.Errorf("send update: unexpected status %d, read body: %w", resp.StatusCode, readErr)
		}
		return fmt.Errorf("send update: unexpected status %d, body: %s", resp.StatusCode, respBody)
	}
	return nil
}
