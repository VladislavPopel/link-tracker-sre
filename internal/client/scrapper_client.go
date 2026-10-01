package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"gitlab.education.tbank.ru/backend-academy-go-2025/homeworks/link-tracker/internal/domain"
	"gitlab.education.tbank.ru/backend-academy-go-2025/homeworks/link-tracker/internal/httphelper"
)

// ScrapperHTTPClient реализует application.ScrapperClient через HTTP
type ScrapperHTTPClient struct {
	baseURL    string
	httpClient *http.Client
}

func NewScrapperHTTPClient(baseURL string) *ScrapperHTTPClient {
	return &ScrapperHTTPClient{
		baseURL:    baseURL,
		httpClient: &http.Client{Timeout: httpTimeout},
	}
}

type scrapperAddLinkReq struct {
	Link     string   `json:"link"`
	Tags     []string `json:"tags"`
	Filters  []string `json:"filters"`
	TgChatID int64    `json:"tgChatId"`
}

type scrapperRemoveLinkReq struct {
	Link     string `json:"link"`
	TgChatID int64  `json:"tgChatId"`
}

type scrapperLinkResp struct {
	ID      int64    `json:"id"`
	URL     string   `json:"url"`
	Tags    []string `json:"tags"`
	Filters []string `json:"filters"`
}

type scrapperListResp struct {
	Links []*scrapperLinkResp `json:"links"`
	Size  int                 `json:"size"`
}

func (c *ScrapperHTTPClient) RegisterChat(ctx context.Context, chatID int64) error {
	url := fmt.Sprintf("%s/tg-chat/%d", c.baseURL, chatID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, nil)
	if err != nil {
		return fmt.Errorf("create register chat request: %w", err)
	}
	req.Header.Set(httphelper.HeaderContentType, httphelper.ContentTypeJSON)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("register chat request: %w", err)
	}
	//nolint:errcheck // не проверяем ошибку
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("register chat: unexpected status %d", resp.StatusCode)
	}
	return nil
}

func (c *ScrapperHTTPClient) AddLink(ctx context.Context, chatID int64, rawURL string, tags []string, filters []string) (*domain.Link, error) {
	body, err := json.Marshal(scrapperAddLinkReq{
		Link: rawURL, Tags: tags, Filters: filters, TgChatID: chatID,
	})
	if err != nil {
		return nil, fmt.Errorf("marshal add link request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/links", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create add link request: %w", err)
	}
	req.Header.Set(httphelper.HeaderContentType, httphelper.ContentTypeJSON)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("add link request: %w", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode == http.StatusConflict {
		return nil, domain.ErrLinkExists
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("add link: unexpected status %d", resp.StatusCode)
	}

	var r scrapperLinkResp
	if err = json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return nil, fmt.Errorf("decode add link response: %w", err)
	}
	return &domain.Link{ID: r.ID, URL: r.URL, Tags: r.Tags, Filters: r.Filters, ChatID: chatID}, nil
}

func (c *ScrapperHTTPClient) RemoveLink(ctx context.Context, chatID int64, rawURL string) (*domain.Link, error) {
	body, err := json.Marshal(scrapperRemoveLinkReq{Link: rawURL, TgChatID: chatID})
	if err != nil {
		return nil, fmt.Errorf("marshal remove link request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, c.baseURL+"/links", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create delete request: %w", err)
	}
	req.Header.Set(httphelper.HeaderContentType, httphelper.ContentTypeJSON)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("remove link request: %w", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode == http.StatusNotFound {
		return nil, domain.ErrLinkNotFound
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("remove link: unexpected status %d", resp.StatusCode)
	}

	var r scrapperLinkResp
	if err = json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return nil, fmt.Errorf("decode remove link response: %w", err)
	}
	return &domain.Link{ID: r.ID, URL: r.URL, Tags: r.Tags, Filters: r.Filters, ChatID: chatID}, nil
}

func (c *ScrapperHTTPClient) GetLinks(ctx context.Context, chatID int64) ([]*domain.Link, error) {
	url := fmt.Sprintf("%s/links?tgChatId=%d", c.baseURL, chatID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("create get links request: %w", err)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("get links request: %w", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("get links: unexpected status %d", resp.StatusCode)
	}

	var r scrapperListResp
	if err = json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return nil, fmt.Errorf("decode get links response: %w", err)
	}

	links := make([]*domain.Link, 0, len(r.Links))
	for _, l := range r.Links {
		links = append(links, &domain.Link{ID: l.ID, URL: l.URL, Tags: l.Tags, Filters: l.Filters, ChatID: chatID})
	}
	return links, nil
}
