package application

import (
	"context"
	"errors"
	"strings"
	"testing"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"gitlab.education.tbank.ru/backend-academy-go-2025/homeworks/link-tracker/internal/domain"
	"gitlab.education.tbank.ru/backend-academy-go-2025/homeworks/link-tracker/internal/infrastructure"
)

func makeUpdate(text string, chatID int64, userID int64) tgbotapi.Update {
	return tgbotapi.Update{
		Message: &tgbotapi.Message{
			Text: text,
			Chat: &tgbotapi.Chat{ID: chatID},
			From: &tgbotapi.User{ID: userID, UserName: "testuser"},
		},
	}
}

func newTestCommander(t *testing.T, sc ScrapperClient) (*Commander, *mockBotClient) {
	t.Helper()
	mock := &mockBotClient{}
	repo := infrastructure.NewInMemoryUserRepo()
	c, err := NewCommander(mock, repo, sc)
	if err != nil {
		t.Fatalf("NewCommander: %v", err)
	}
	if err = c.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	return c, mock
}

func TestCommandResponses(t *testing.T) {
	tests := []struct {
		name          string
		command       string
		expectedReply string
	}{
		{
			name:          "Positive: start command",
			command:       "/start",
			expectedReply: StartMessage,
		},
		{
			name:          "Positive: help command",
			command:       "/help",
			expectedReply: HelpMessage,
		},
		{
			name:          "Negative: unknown command",
			command:       "/unknown",
			expectedReply: UnknownMessage,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, _ := newTestCommander(t, &mockScrapperClient{})
			msg := c.dispatch(context.Background(), makeUpdate(tt.command, 123, 1))
			if msg.Text != tt.expectedReply {
				t.Errorf("Для команды %s ожидался ответ %q, но получен %q", tt.command, tt.expectedReply, msg.Text)
			}
		})
	}
}

func TestNewCommander_NilBot(t *testing.T) {
	_, err := NewCommander(nil, infrastructure.NewInMemoryUserRepo(), nil)
	if err == nil {
		t.Error("expected error for nil bot, got nil")
	}
}

func TestHandle_NilMessage(t *testing.T) {
	commander, mock := newTestCommander(t, &mockScrapperClient{})
	commander.Handle(context.Background(), tgbotapi.Update{Message: nil})
	if len(mock.sentMessages) != 0 {
		t.Error("expected no messages sent for nil message")
	}
}

func TestHandleStart_SavesUser(t *testing.T) {
	mock := &mockBotClient{}
	repo := infrastructure.NewInMemoryUserRepo()
	commander, _ := NewCommander(mock, repo, &mockScrapperClient{})
	_ = commander.Init()

	update := makeUpdate("/start", 123, 42)
	commander.Handle(context.Background(), update)

	user, err := repo.Get(42)
	if err != nil {
		t.Fatalf("user not saved: %v", err)
	}
	if user.ID != 42 {
		t.Errorf("want user ID 42, got %d", user.ID)
	}
}

func TestSaveNilUser(t *testing.T) {
	repo := infrastructure.NewInMemoryUserRepo()
	err := repo.Save(nil)
	if err == nil {
		t.Error("expected error when saving nil user")
	}
}

func TestGetNotFound(t *testing.T) {
	repo := infrastructure.NewInMemoryUserRepo()
	_, err := repo.Get(999)
	if !errors.Is(err, domain.ErrUserNotFound) {
		t.Errorf("want ErrUserNotFound, got %v", err)
	}
}

func TestTrack_HappyPath(t *testing.T) {
	sc := &mockScrapperClient{}
	c, _ := newTestCommander(t, sc)

	// /track
	msg := c.dispatch(context.Background(), makeUpdate("/track", 1, 1))
	if msg.Text != TrackAskURL {
		t.Fatalf("step1: want %q, got %q", TrackAskURL, msg.Text)
	}

	// valid URL
	msg = c.dispatch(context.Background(), makeUpdate("https://github.com/user/repo", 1, 1))
	if msg.Text != TrackAskTags {
		t.Fatalf("step2: want %q, got %q", TrackAskTags, msg.Text)
	}

	// tags
	msg = c.dispatch(context.Background(), makeUpdate("work, hobby", 1, 1))
	if msg.Text != TrackSuccess {
		t.Fatalf("step3: want %q, got %q", TrackSuccess, msg.Text)
	}

	if len(sc.addedLinks) != 1 {
		t.Fatalf("want 1 link, got %d", len(sc.addedLinks))
	}
	if sc.addedLinks[0].URL != "https://github.com/user/repo" {
		t.Errorf("wrong URL: %s", sc.addedLinks[0].URL)
	}
	if len(sc.addedLinks[0].Tags) != 2 {
		t.Errorf("want 2 tags, got %v", sc.addedLinks[0].Tags)
	}
}

func TestTrack_InvalidURL(t *testing.T) {
	c, _ := newTestCommander(t, &mockScrapperClient{})
	c.dispatch(context.Background(), makeUpdate("/track", 1, 1))

	msg := c.dispatch(context.Background(), makeUpdate("tbank://invalid", 1, 1))
	if msg.Text != TrackInvalidURL {
		t.Errorf("want %q, got %q", TrackInvalidURL, msg.Text)
	}
	// State должен остаться StateTrackWaitURL
	if c.sessions.Get(1).State != StateTrackWaitURL {
		t.Error("want StateTrackWaitURL after invalid URL")
	}
}

func TestTrack_LinkAlreadyExists(t *testing.T) {
	sc := &mockScrapperClient{addErr: domain.ErrLinkExists}
	c, _ := newTestCommander(t, sc)

	c.dispatch(context.Background(), makeUpdate("/track", 1, 1))
	c.dispatch(context.Background(), makeUpdate("https://github.com/user/repo", 1, 1))
	msg := c.dispatch(context.Background(), makeUpdate("/skip", 1, 1))

	if msg.Text != TrackExists {
		t.Errorf("want %q, got %q", TrackExists, msg.Text)
	}
}

func TestTrack_SkipTags(t *testing.T) {
	sc := &mockScrapperClient{}
	c, _ := newTestCommander(t, sc)

	c.dispatch(context.Background(), makeUpdate("/track", 1, 1))
	c.dispatch(context.Background(), makeUpdate("https://github.com/user/repo", 1, 1))
	msg := c.dispatch(context.Background(), makeUpdate("/skip", 1, 1))

	if msg.Text != TrackSuccess {
		t.Errorf("want %q, got %q", TrackSuccess, msg.Text)
	}
	if len(sc.addedLinks[0].Tags) != 0 {
		t.Error("expected empty tags after /skip")
	}
}

func TestTrack_CancelMidFlow(t *testing.T) {
	c, _ := newTestCommander(t, &mockScrapperClient{})

	c.dispatch(context.Background(), makeUpdate("/track", 1, 1))
	msg := c.dispatch(context.Background(), makeUpdate("/cancel", 1, 1))

	if msg.Text != TrackCancelled {
		t.Errorf("want %q, got %q", TrackCancelled, msg.Text)
	}
	if c.sessions.Get(1).State != StateIdle {
		t.Error("want StateIdle after /cancel")
	}
}

func TestTrack_AnotherCommandCancelsFlow(t *testing.T) {
	c, _ := newTestCommander(t, &mockScrapperClient{})

	c.dispatch(context.Background(), makeUpdate("/track", 1, 1))
	// Отправляем другую команду в середине диалога
	msg := c.dispatch(context.Background(), makeUpdate("/help", 1, 1))

	if msg.Text != HelpMessage {
		t.Errorf("want HelpMessage, got %q", msg.Text)
	}
	if c.sessions.Get(1).State != StateIdle {
		t.Error("want StateIdle after command during dialog")
	}
}

// /untrack

func TestUntrack_HappyPath(t *testing.T) {
	c, _ := newTestCommander(t, &mockScrapperClient{})

	c.dispatch(context.Background(), makeUpdate("/untrack", 1, 1))
	msg := c.dispatch(context.Background(), makeUpdate("https://github.com/user/repo", 1, 1))

	if msg.Text != UntrackSuccess {
		t.Errorf("want %q, got %q", UntrackSuccess, msg.Text)
	}
}

func TestUntrack_NotFound(t *testing.T) {
	sc := &mockScrapperClient{removeErr: domain.ErrLinkNotFound}
	c, _ := newTestCommander(t, sc)

	c.dispatch(context.Background(), makeUpdate("/untrack", 1, 1))
	msg := c.dispatch(context.Background(), makeUpdate("https://github.com/user/repo", 1, 1))

	if msg.Text != UntrackNotFound {
		t.Errorf("want %q, got %q", UntrackNotFound, msg.Text)
	}
}

// /list

func TestList_EmptyList(t *testing.T) {
	c, _ := newTestCommander(t, &mockScrapperClient{})
	msg := c.dispatch(context.Background(), makeUpdate("/list", 1, 1))
	if msg.Text != ListEmpty {
		t.Errorf("want %q, got %q", ListEmpty, msg.Text)
	}
}

func TestList_WithLinks(t *testing.T) {
	sc := &mockScrapperClient{
		links: []*domain.Link{
			{ID: 1, URL: "https://github.com/user/repo", Tags: []string{"work"}},
		},
	}
	c, _ := newTestCommander(t, sc)
	msg := c.dispatch(context.Background(), makeUpdate("/list", 1, 1))
	if msg.Text == ListEmpty {
		t.Error("expected non-empty list response")
	}
	if !strings.Contains(msg.Text, "github.com") {
		t.Error("expected URL in list response")
	}
}

func TestList_FilterByTag(t *testing.T) {
	sc := &mockScrapperClient{
		links: []*domain.Link{
			{ID: 1, URL: "https://github.com/user/repo1", Tags: []string{"work"}},
			{ID: 2, URL: "https://github.com/user/repo2", Tags: []string{"hobby"}},
		},
	}
	c, _ := newTestCommander(t, sc)
	msg := c.dispatch(context.Background(), makeUpdate("/list work", 1, 1))

	if !strings.Contains(msg.Text, "repo1") {
		t.Error("expected repo1 in filtered list")
	}
	if strings.Contains(msg.Text, "repo2") {
		t.Error("repo2 should be filtered out")
	}
}

func TestList_FilterByTag_Empty(t *testing.T) {
	sc := &mockScrapperClient{
		links: []*domain.Link{
			{ID: 1, URL: "https://github.com/user/repo", Tags: []string{"work"}},
		},
	}
	c, _ := newTestCommander(t, sc)
	msg := c.dispatch(context.Background(), makeUpdate("/list nonexistenttag", 1, 1))
	if msg.Text != ListEmpty {
		t.Errorf("want ListEmpty for unmatched tag, got %q", msg.Text)
	}
}
