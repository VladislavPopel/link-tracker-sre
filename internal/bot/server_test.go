package bot_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"gitlab.education.tbank.ru/backend-academy-go-2025/homeworks/link-tracker/internal/bot"
)

type mockSender struct {
	sent []tgbotapi.MessageConfig
}

func (m *mockSender) Send(msg tgbotapi.MessageConfig) (int, error) {
	m.sent = append(m.sent, msg)
	return len(m.sent), nil
}

func newTestServer(sender bot.MessageSender) http.Handler {
	return bot.NewUpdateServer(sender).Handler()
}

func TestHandleUpdates_ValidRequest(t *testing.T) {
	sender := &mockSender{}
	srv := newTestServer(sender)

	update := bot.LinkUpdate{
		ID:          1,
		URL:         "https://github.com/user/repo",
		Description: "New commit pushed",
		TgChatIDs:   []int64{100, 200},
	}
	body, _ := json.Marshal(update)

	req := httptest.NewRequest(http.MethodPost, "/updates", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("want 200, got %d (body: %s)", w.Code, w.Body.String())
	}
	if len(sender.sent) != 2 {
		t.Errorf("want 2 messages sent (one per chat), got %d", len(sender.sent))
	}
	if sender.sent[0].ChatID != 100 {
		t.Errorf("want first message to chat 100, got %d", sender.sent[0].ChatID)
	}
}

func TestHandleUpdates_InvalidJSON(t *testing.T) {
	srv := newTestServer(&mockSender{})
	req := httptest.NewRequest(http.MethodPost, "/updates", bytes.NewReader([]byte("{invalid json")))
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code == http.StatusOK {
		t.Errorf("want non-200 for invalid JSON, got 200")
	}
}

func TestHandleUpdates_MissingURL(t *testing.T) {
	srv := newTestServer(&mockSender{})
	update := map[string]interface{}{
		"id":          1,
		"description": "test",
		"tgChatIds":   []int64{123},
		// url отсутствует
	}
	body, _ := json.Marshal(update)
	req := httptest.NewRequest(http.MethodPost, "/updates", bytes.NewReader(body))
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("want 400 for missing url, got %d", w.Code)
	}
}

func TestHandleUpdates_EmptyChatIDs(t *testing.T) {
	srv := newTestServer(&mockSender{})
	update := bot.LinkUpdate{
		ID:          1,
		URL:         "https://github.com/user/repo",
		Description: "test",
		TgChatIDs:   nil, // пустой список
	}
	body, _ := json.Marshal(update)
	req := httptest.NewRequest(http.MethodPost, "/updates", bytes.NewReader(body))
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("want 400 for empty tgChatIds, got %d", w.Code)
	}
}

func TestHandleUpdates_SendsCorrectMessage(t *testing.T) {
	sender := &mockSender{}
	srv := newTestServer(sender)

	update := bot.LinkUpdate{
		ID:          42,
		URL:         "https://stackoverflow.com/questions/12345",
		Description: "New answer posted",
		TgChatIDs:   []int64{777},
	}
	body, _ := json.Marshal(update)
	req := httptest.NewRequest(http.MethodPost, "/updates", bytes.NewReader(body))
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", w.Code)
	}
	if len(sender.sent) != 1 {
		t.Fatalf("want 1 message, got %d", len(sender.sent))
	}
	// Проверяем, что сообщение содержит URL
	msgText := sender.sent[0].Text
	if !containsStr(msgText, update.URL) {
		t.Errorf("expected URL in message, got: %s", msgText)
	}
}

func containsStr(s, substr string) bool {
	return len(s) >= len(substr) && func() bool {
		for i := 0; i <= len(s)-len(substr); i++ {
			if s[i:i+len(substr)] == substr {
				return true
			}
		}
		return false
	}()
}
