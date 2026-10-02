package scrapper

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"gitlab.education.tbank.ru/backend-academy-go-2025/homeworks/link-tracker/internal/domain"
	"gitlab.education.tbank.ru/backend-academy-go-2025/homeworks/link-tracker/internal/httphelper"
)

const (
	defaultPageLimit = 50
	maxPageLimit     = 200
)

type addLinkRequest struct {
	Link     string   `json:"link"`
	Tags     []string `json:"tags"`
	Filters  []string `json:"filters"`
	TgChatID int64    `json:"tgChatId"`
}

type updateLinkRequest struct {
	TgChatID int64    `json:"tgChatId"`
	Tags     []string `json:"tags"`
	Filters  []string `json:"filters"`
}

type removeLinkRequest struct {
	Link     string `json:"link"`
	TgChatID int64  `json:"tgChatId"`
}

type linkResponse struct {
	ID      int64    `json:"id"`
	URL     string   `json:"url"`
	Tags    []string `json:"tags"`
	Filters []string `json:"filters"`
}

type listLinksResponse struct {
	Links []*linkResponse `json:"links"`
	Size  int             `json:"size"`
}

type chatResponse struct {
	ID        int64     `json:"id"`
	CreatedAt time.Time `json:"createdAt"`
}

type listChatsResponse struct {
	Chats []*chatResponse `json:"chats"`
	Size  int             `json:"size"`
}

type apiError struct {
	Description   string   `json:"description"`
	Code          string   `json:"code"`
	ExceptionName string   `json:"exceptionName"`
	Stacktrace    []string `json:"stacktrace"`
}

// Handler предоставляет HTTP API сервиса Scrapper
type Handler struct {
	repo    domain.LinkRepository
	updates domain.UpdateRepository
}

func NewHandler(repo domain.LinkRepository) *Handler {
	return &Handler{repo: repo}
}

func (h *Handler) newMux() *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /tg-chat", h.listChats)
	mux.HandleFunc("/tg-chat/", h.handleChat)
	mux.HandleFunc("/links", h.handleLinks)
	mux.HandleFunc("PUT /links/{id}", h.updateLink)
	if h.updates != nil {
		mux.HandleFunc("GET /updates", h.listUpdates)
	}
	return mux
}

func (h *Handler) Router() http.Handler {
	return h.newMux()
}

// RouterWithStatic - API плюс раздача статики фронтенда
func (h *Handler) RouterWithStatic(static fs.FS) http.Handler {
	mux := h.newMux()
	mux.Handle("/", http.FileServerFS(static))
	return mux
}

// --- chats ---

func (h *Handler) listChats(w http.ResponseWriter, r *http.Request) {
	page, err := parsePage(r)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, err.Error(), "BadRequest")
		return
	}

	chats, err := h.repo.GetChats(r.Context(), page)
	if err != nil {
		slog.Error("list chats failed", "err", err)
		writeAPIError(w, http.StatusInternalServerError, err.Error(), "InternalError")
		return
	}

	resp := listChatsResponse{
		Chats: make([]*chatResponse, 0, len(chats)),
		Size:  len(chats),
	}
	for _, c := range chats {
		resp.Chats = append(resp.Chats, &chatResponse{ID: c.ID, CreatedAt: c.CreatedAt})
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *Handler) handleChat(w http.ResponseWriter, r *http.Request) {
	idStr := strings.TrimPrefix(r.URL.Path, "/tg-chat/")
	chatID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || chatID <= 0 {
		writeAPIError(w, http.StatusBadRequest, "invalid chat id", "BadRequest")
		return
	}

	switch r.Method {
	case http.MethodPost:
		h.registerChat(r.Context(), w, chatID)
	case http.MethodDelete:
		h.deleteChat(r.Context(), w, chatID)
	default:
		writeAPIError(w, http.StatusMethodNotAllowed, "method not allowed", "MethodNotAllowed")
	}
}

func (h *Handler) registerChat(ctx context.Context, w http.ResponseWriter, chatID int64) {
	if err := h.repo.AddChat(ctx, chatID); err != nil {
		slog.Error("register chat failed", "chat_id", chatID, "err", err)
		writeAPIError(w, http.StatusInternalServerError, err.Error(), "InternalError")
		return
	}
	slog.Info("chat registered", "chat_id", chatID)
	w.WriteHeader(http.StatusOK)
}

func (h *Handler) deleteChat(ctx context.Context, w http.ResponseWriter, chatID int64) {
	if err := h.repo.RemoveChat(ctx, chatID); err != nil {
		if errors.Is(err, domain.ErrChatNotFound) {
			writeAPIError(w, http.StatusNotFound, "chat not found", "ChatNotFoundException")
			return
		}
		slog.Error("delete chat failed", "chat_id", chatID, "err", err)
		writeAPIError(w, http.StatusInternalServerError, err.Error(), "InternalError")
		return
	}
	slog.Info("chat deleted", "chat_id", chatID)
	w.WriteHeader(http.StatusOK)
}

// --- links ---

func (h *Handler) handleLinks(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		h.getLinks(w, r)
	case http.MethodPost:
		h.addLink(w, r)
	case http.MethodDelete:
		h.removeLink(w, r)
	default:
		writeAPIError(w, http.StatusMethodNotAllowed, "method not allowed", "MethodNotAllowed")
	}
}

func (h *Handler) getLinks(w http.ResponseWriter, r *http.Request) {
	chatIDStr := r.URL.Query().Get("tgChatId")
	chatID, err := strconv.ParseInt(chatIDStr, 10, 64)
	if err != nil || chatID <= 0 {
		writeAPIError(w, http.StatusBadRequest, "tgChatId query param is required", "BadRequest")
		return
	}

	page, err := parsePage(r)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, err.Error(), "BadRequest")
		return
	}

	links, err := h.repo.GetLinks(r.Context(), chatID, page)
	if err != nil {
		if errors.Is(err, domain.ErrChatNotFound) {
			writeAPIError(w, http.StatusNotFound, "chat not found", "ChatNotFoundException")
			return
		}
		slog.Error("get links failed", "chat_id", chatID, "err", err)
		writeAPIError(w, http.StatusInternalServerError, err.Error(), "InternalError")
		return
	}

	resp := listLinksResponse{
		Links: make([]*linkResponse, 0, len(links)),
		Size:  len(links),
	}
	for _, l := range links {
		resp.Links = append(resp.Links, linkToResp(l))
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *Handler) addLink(w http.ResponseWriter, r *http.Request) {
	var req addLinkRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeAPIError(w, http.StatusBadRequest, "invalid request body", "BadRequest")
		return
	}
	if req.Link == "" {
		writeAPIError(w, http.StatusBadRequest, "link is required", "BadRequest")
		return
	}

	link, err := h.repo.AddLink(r.Context(), &domain.Link{
		URL:     req.Link,
		Tags:    req.Tags,
		Filters: req.Filters,
		ChatID:  req.TgChatID,
	})
	if err != nil {
		if errors.Is(err, domain.ErrLinkExists) {
			writeAPIError(w, http.StatusConflict, "link already tracked", "LinkAlreadyTrackedException")
			return
		}
		if errors.Is(err, domain.ErrChatNotFound) {
			writeAPIError(w, http.StatusNotFound, "chat not found", "ChatNotFoundException")
			return
		}
		slog.Error("add link failed", "chat_id", req.TgChatID, "url", req.Link, "err", err)
		writeAPIError(w, http.StatusInternalServerError, err.Error(), "InternalError")
		return
	}

	slog.Info("link added", "chat_id", req.TgChatID, "url", req.Link)
	writeJSON(w, http.StatusOK, linkToResp(link))
}

// updateLink: PUT /links/{id} — полная замена тегов и фильтров подписки.
func (h *Handler) updateLink(w http.ResponseWriter, r *http.Request) {
	linkID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || linkID <= 0 {
		writeAPIError(w, http.StatusBadRequest, "invalid link id", "BadRequest")
		return
	}

	var req updateLinkRequest
	if err = json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeAPIError(w, http.StatusBadRequest, "invalid request body", "BadRequest")
		return
	}
	if req.TgChatID <= 0 {
		writeAPIError(w, http.StatusBadRequest, "tgChatId is required", "BadRequest")
		return
	}

	link, err := h.repo.UpdateLink(r.Context(), req.TgChatID, linkID, req.Tags, req.Filters)
	if err != nil {
		if errors.Is(err, domain.ErrLinkNotFound) {
			writeAPIError(w, http.StatusNotFound, "link not found", "LinkNotFoundException")
			return
		}
		slog.Error("update link failed", "chat_id", req.TgChatID, "link_id", linkID, "err", err)
		writeAPIError(w, http.StatusInternalServerError, err.Error(), "InternalError")
		return
	}

	slog.Info("link updated", "chat_id", req.TgChatID, "link_id", linkID)
	writeJSON(w, http.StatusOK, linkToResp(link))
}

func (h *Handler) removeLink(w http.ResponseWriter, r *http.Request) {
	var req removeLinkRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeAPIError(w, http.StatusBadRequest, "invalid request body", "BadRequest")
		return
	}
	if req.Link == "" {
		writeAPIError(w, http.StatusBadRequest, "link is required", "BadRequest")
		return
	}

	link, err := h.repo.RemoveLink(r.Context(), req.TgChatID, req.Link)
	if err != nil {
		if errors.Is(err, domain.ErrLinkNotFound) {
			writeAPIError(w, http.StatusNotFound, "link not found", "LinkNotFoundException")
			return
		}
		if errors.Is(err, domain.ErrChatNotFound) {
			writeAPIError(w, http.StatusNotFound, "chat not found", "ChatNotFoundException")
			return
		}
		slog.Error("remove link failed", "chat_id", req.TgChatID, "url", req.Link, "err", err)
		writeAPIError(w, http.StatusInternalServerError, err.Error(), "InternalError")
		return
	}

	slog.Info("link removed", "chat_id", req.TgChatID, "url", req.Link)
	writeJSON(w, http.StatusOK, linkToResp(link))
}

// helpers

// parsePage читает ?limit= и ?offset=; при отсутствии подставляет значения по умолчанию.
func parsePage(r *http.Request) (domain.Page, error) {
	q := r.URL.Query()
	page := domain.Page{Limit: defaultPageLimit, Offset: 0}

	if s := q.Get("limit"); s != "" {
		n, err := strconv.Atoi(s)
		if err != nil || n < 1 || n > maxPageLimit {
			return domain.Page{}, fmt.Errorf("limit must be an integer between 1 and %d", maxPageLimit)
		}
		page.Limit = n
	}
	if s := q.Get("offset"); s != "" {
		n, err := strconv.Atoi(s)
		if err != nil || n < 0 {
			return domain.Page{}, errors.New("offset must be a non-negative integer")
		}
		page.Offset = n
	}
	return page, nil
}

func linkToResp(l *domain.Link) *linkResponse {
	tags := l.Tags
	if tags == nil {
		tags = []string{}
	}
	filters := l.Filters
	if filters == nil {
		filters = []string{}
	}
	return &linkResponse{ID: l.ID, URL: l.URL, Tags: tags, Filters: filters}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set(httphelper.HeaderContentType, httphelper.ContentTypeJSON)
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Error("failed to encode json response", "err", err)
	}
}

func writeAPIError(w http.ResponseWriter, status int, desc, exceptionName string) {
	writeJSON(w, status, apiError{
		Description:   desc,
		Code:          strconv.Itoa(status),
		ExceptionName: exceptionName,
		Stacktrace:    []string{},
	})
}
