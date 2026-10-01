package bot

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"gitlab.education.tbank.ru/backend-academy-go-2025/homeworks/link-tracker/internal/httphelper"
)

// LinkUpdate - тело POST /updates
type LinkUpdate struct {
	ID          int64   `json:"id"`
	URL         string  `json:"url"`
	Description string  `json:"description"`
	TgChatIDs   []int64 `json:"tgChatIds"`
}

// MessageSender позволяет отправлять сообщения в Telegram
type MessageSender interface {
	Send(msg tgbotapi.MessageConfig) (int, error)
}

// UpdateServer обрабатывает входящие обновления от Scrapper
type UpdateServer struct {
	sender MessageSender
}

func NewUpdateServer(sender MessageSender) *UpdateServer {
	return &UpdateServer{sender: sender}
}

func (s *UpdateServer) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /updates", s.handleUpdates)
	return mux
}

func (s *UpdateServer) handleUpdates(w http.ResponseWriter, r *http.Request) {
	var update LinkUpdate
	if err := json.NewDecoder(r.Body).Decode(&update); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if update.URL == "" || len(update.TgChatIDs) == 0 {
		writeError(w, http.StatusBadRequest, "url and tgChatIds are required")
		return
	}

	text := fmt.Sprintf("Обновление!\nСсылка: %s\n%s", update.URL, update.Description)
	for _, chatID := range update.TgChatIDs {
		msg := tgbotapi.NewMessage(chatID, text)
		if _, err := s.sender.Send(msg); err != nil {
			slog.Error("failed to send update notification",
				"chat_id", chatID,
				"url", update.URL,
				"err", err,
			)
		}
	}

	w.WriteHeader(http.StatusOK)
}

func writeError(w http.ResponseWriter, status int, desc string) {
	w.Header().Set(httphelper.HeaderContentType, httphelper.ContentTypeJSON)
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(struct {
		Description string `json:"description"`
		Code        string `json:"code"`
	}{
		Description: desc,
		Code:        strconv.Itoa(status),
	}); err != nil {
		slog.Error("failed to encode error response", "err", err)
	}
}
