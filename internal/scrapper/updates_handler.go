package scrapper

import (
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"gitlab.education.tbank.ru/backend-academy-go-2025/homeworks/link-tracker/internal/domain"
)

type updateResponse struct {
	ID          int64     `json:"id"`
	URL         string    `json:"url"`
	Description string    `json:"description"`
	CreatedAt   time.Time `json:"createdAt"`
}

type listUpdatesResponse struct {
	Updates []*updateResponse `json:"updates"`
	Size    int               `json:"size"`
}

// WithUpdates подключает хранилище уведомлений и включает эндпоинт GET /updates.
// Вызывать нужно до Router()/RouterWithStatic().
func (h *Handler) WithUpdates(repo domain.UpdateRepository) *Handler {
	h.updates = repo
	return h
}

// listUpdates: GET /updates?tgChatId=1&afterId=0&limit=20
// Возвращает уведомления чата с id > afterId (по возрастанию id).
func (h *Handler) listUpdates(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()

	chatID, err := strconv.ParseInt(q.Get("tgChatId"), 10, 64)
	if err != nil || chatID <= 0 {
		writeAPIError(w, http.StatusBadRequest, "tgChatId query param is required", "BadRequest")
		return
	}

	var afterID int64
	if s := q.Get("afterId"); s != "" {
		afterID, err = strconv.ParseInt(s, 10, 64)
		if err != nil || afterID < 0 {
			writeAPIError(w, http.StatusBadRequest, "afterId must be a non-negative integer", "BadRequest")
			return
		}
	}

	page, err := parsePage(r) // из пагинации используем только limit
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, err.Error(), "BadRequest")
		return
	}

	updates, err := h.updates.GetUpdates(r.Context(), chatID, afterID, page.Limit)
	if err != nil {
		slog.Error("get updates failed", "chat_id", chatID, "err", err)
		writeAPIError(w, http.StatusInternalServerError, err.Error(), "InternalError")
		return
	}

	resp := listUpdatesResponse{
		Updates: make([]*updateResponse, 0, len(updates)),
		Size:    len(updates),
	}
	for _, u := range updates {
		resp.Updates = append(resp.Updates, &updateResponse{
			ID: u.ID, URL: u.URL, Description: u.Description, CreatedAt: u.CreatedAt,
		})
	}
	writeJSON(w, http.StatusOK, resp)
}
