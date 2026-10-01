package scrapper_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"gitlab.education.tbank.ru/backend-academy-go-2025/homeworks/link-tracker/internal/domain"
	"gitlab.education.tbank.ru/backend-academy-go-2025/homeworks/link-tracker/internal/httphelper"
	"gitlab.education.tbank.ru/backend-academy-go-2025/homeworks/link-tracker/internal/scrapper"
)

// helpers

type mockLinkRepo struct {
	chats map[int64]struct{}
	links map[int64][]*domain.Link
}

func newMockLinkRepo() *mockLinkRepo {
	return &mockLinkRepo{
		chats: make(map[int64]struct{}),
		links: make(map[int64][]*domain.Link),
	}
}

func (m *mockLinkRepo) AddChat(_ context.Context, chatID int64) error {
	m.chats[chatID] = struct{}{}
	return nil
}
func (m *mockLinkRepo) RemoveChat(_ context.Context, chatID int64) error {
	if _, ok := m.chats[chatID]; !ok {
		return domain.ErrChatNotFound
	}
	delete(m.chats, chatID)
	return nil
}
func (m *mockLinkRepo) AddLink(_ context.Context, link *domain.Link) (*domain.Link, error) {
	if _, ok := m.chats[link.ChatID]; !ok {
		return nil, domain.ErrChatNotFound
	}
	for _, l := range m.links[link.ChatID] {
		if l.URL == link.URL {
			return nil, domain.ErrLinkExists
		}
	}
	link.ID = int64(len(m.links[link.ChatID]) + 1)
	m.links[link.ChatID] = append(m.links[link.ChatID], link)
	return link, nil
}
func (m *mockLinkRepo) RemoveLink(_ context.Context, chatID int64, url string) (*domain.Link, error) {
	for i, l := range m.links[chatID] {
		if l.URL == url {
			m.links[chatID] = append(m.links[chatID][:i], m.links[chatID][i+1:]...)
			return l, nil
		}
	}
	return nil, domain.ErrLinkNotFound
}
func (m *mockLinkRepo) GetLinks(_ context.Context, chatID int64, _ domain.Page) ([]*domain.Link, error) {
	return m.links[chatID], nil
}
func (m *mockLinkRepo) GetAllLinks(_ context.Context, _ domain.Page) ([]*domain.Link, error) {
	var all []*domain.Link
	for _, links := range m.links {
		all = append(all, links...)
	}
	return all, nil
}

func newServer(t *testing.T) *httptest.Server {
	t.Helper()
	repo := newMockLinkRepo()
	h := scrapper.NewHandler(repo)
	return httptest.NewServer(h.Router())
}

func postJSON(t *testing.T, server *httptest.Server, path string, body any) *http.Response {
	t.Helper()
	b, _ := json.Marshal(body)
	resp, err := http.Post(server.URL+path, httphelper.ContentTypeJSON, bytes.NewReader(b))
	if err != nil {
		t.Fatalf("POST %s: %v", path, err)
	}
	return resp
}

func deleteJSON(t *testing.T, server *httptest.Server, path string, body any) *http.Response {
	t.Helper()
	b, _ := json.Marshal(body)
	req, _ := http.NewRequest(http.MethodDelete, server.URL+path, bytes.NewReader(b))
	req.Header.Set(httphelper.HeaderContentType, httphelper.ContentTypeJSON)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("DELETE %s: %v", path, err)
	}
	return resp
}

func getLinks(t *testing.T, server *httptest.Server, chatID int) *http.Response {
	t.Helper()
	resp, err := http.Get(server.URL + "/links?tgChatId=" + itoa(chatID))
	if err != nil {
		t.Fatalf("GET /links: %v", err)
	}
	return resp
}

func itoa(i int) string {
	b, _ := json.Marshal(i)
	return string(b)
}

func registerChat(t *testing.T, server *httptest.Server) {
	t.Helper()
	resp := postJSON(t, server, "/tg-chat/1", nil)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("register chat 1: expected 200, got %d", resp.StatusCode)
	}
}

type listResp struct {
	Links []struct {
		URL string `json:"url"`
	} `json:"links"`
	Size int `json:"size"`
}

func decodeList(t *testing.T, resp *http.Response) listResp {
	t.Helper()
	defer func() {
		_ = resp.Body.Close()
	}()
	var r listResp
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		t.Fatalf("decode list response: %v", err)
	}
	return r
}

func containsURL(list listResp, url string) bool {
	for _, l := range list.Links {
		if l.URL == url {
			return true
		}
	}
	return false
}

func TestScrapper_AddAndGetLink(t *testing.T) {
	srv := newServer(t)
	defer srv.Close()

	const (
		chatID = 1
		link   = "https://github.com/user/repo"
	)

	// POST /tg-chat/1 - 200
	registerChat(t, srv)

	// POST /links - 200
	addResp := postJSON(t, srv, "/links", map[string]any{
		"link": link, "tgChatId": chatID,
	})
	addResp.Body.Close()
	if addResp.StatusCode != http.StatusOK {
		t.Fatalf("add link: expected 200, got %d", addResp.StatusCode)
	}

	// GET /links - 200
	listResp := getLinks(t, srv, chatID)
	if listResp.StatusCode != http.StatusOK {
		t.Fatalf("get links: expected 200, got %d", listResp.StatusCode)
	}
	list := decodeList(t, listResp)
	if !containsURL(list, link) {
		t.Errorf("expected link %s in list, got %+v", link, list)
	}
}

func TestScrapper_AddAndDeleteLink(t *testing.T) {
	srv := newServer(t)
	defer srv.Close()

	const (
		chatID = 1
		link   = "https://github.com/user/repo"
	)

	registerChat(t, srv)

	addResp := postJSON(t, srv, "/links", map[string]any{"link": link, "tgChatId": chatID})
	addResp.Body.Close()

	// DELETE /links - 200
	delResp := deleteJSON(t, srv, "/links", map[string]any{"link": link, "tgChatId": chatID})
	delResp.Body.Close()
	if delResp.StatusCode != http.StatusOK {
		t.Fatalf("delete link: expected 200, got %d", delResp.StatusCode)
	}

	// GET /links - 200
	listResp := getLinks(t, srv, chatID)
	if listResp.StatusCode != http.StatusOK {
		t.Fatalf("get links after delete: expected 200, got %d", listResp.StatusCode)
	}
	list := decodeList(t, listResp)
	if containsURL(list, link) {
		t.Errorf("link %s should not be in list after delete", link)
	}
}

func TestScrapper_DeleteLinkFromUnknownChat(t *testing.T) {
	srv := newServer(t)
	defer srv.Close()

	const (
		chatID        = 1
		unknownChatID = 999
		link          = "https://github.com/user/repo"
	)

	registerChat(t, srv)
	addResp := postJSON(t, srv, "/links", map[string]any{"link": link, "tgChatId": chatID})
	addResp.Body.Close()

	delResp := deleteJSON(t, srv, "/links", map[string]any{"link": link, "tgChatId": unknownChatID})
	delResp.Body.Close()
	if delResp.StatusCode == http.StatusOK {
		t.Fatalf("delete from unknown chat: expected non-200, got 200")
	}

	listResp := getLinks(t, srv, chatID)
	if listResp.StatusCode != http.StatusOK {
		t.Fatalf("get links: expected 200, got %d", listResp.StatusCode)
	}
	list := decodeList(t, listResp)
	if !containsURL(list, link) {
		t.Errorf("link %s should still be in list for chat %d", link, chatID)
	}
}

func TestScrapper_AddLinkToUnknownChat(t *testing.T) {
	srv := newServer(t)
	defer srv.Close()

	registerChat(t, srv)

	addResp := postJSON(t, srv, "/links", map[string]any{
		"link": "https://github.com/user/repo", "tgChatId": 2,
	})
	addResp.Body.Close()
	if addResp.StatusCode == http.StatusOK {
		t.Fatalf("add link to unknown chat: expected non-200, got 200")
	}
}

func TestScrapper_DeletedChatCannotAddLinks(t *testing.T) {
	srv := newServer(t)
	defer srv.Close()

	const chatID = 1

	registerChat(t, srv)

	req, _ := http.NewRequest(http.MethodDelete, srv.URL+"/tg-chat/"+itoa(chatID), nil)
	delResp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("DELETE /tg-chat/1: %v", err)
	}
	delResp.Body.Close()
	if delResp.StatusCode != http.StatusOK {
		t.Fatalf("delete chat: expected 200, got %d", delResp.StatusCode)
	}

	addResp := postJSON(t, srv, "/links", map[string]any{
		"link": "https://github.com/user/repo", "tgChatId": chatID,
	})
	addResp.Body.Close()
	if addResp.StatusCode == http.StatusOK {
		t.Fatalf("add link to deleted chat: expected non-200, got 200")
	}
}

func TestScrapper_DeleteNonExistentChat(t *testing.T) {
	srv := newServer(t)
	defer srv.Close()

	req, _ := http.NewRequest(http.MethodDelete, srv.URL+"/tg-chat/1", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("DELETE /tg-chat/1: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("delete non-existent chat: expected 404, got %d", resp.StatusCode)
	}
}
