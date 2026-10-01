package application

import (
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

type mockBotClient struct {
	sentMessages []tgbotapi.MessageConfig
	sendErr      error
}

func (m *mockBotClient) Send(msg tgbotapi.MessageConfig) (int, error) {
	m.sentMessages = append(m.sentMessages, msg)
	return len(m.sentMessages), m.sendErr
}

func (m *mockBotClient) SetCommands(_ []tgbotapi.BotCommand) error {
	return nil
}
