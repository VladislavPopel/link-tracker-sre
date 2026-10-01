package client

import (
	"context"
	"fmt"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

type TelegramClient struct {
	bot     *tgbotapi.BotAPI
	timeout int
}

func NewTelegramClient(token string, timeout int, debug bool) (*TelegramClient, error) {
	bot, err := tgbotapi.NewBotAPI(token)
	if err != nil {
		return nil, fmt.Errorf("create bot api: %w", err)
	}
	bot.Debug = debug
	return &TelegramClient{bot: bot, timeout: timeout}, nil
}

// Username возвращает имя бота
func (c *TelegramClient) Username() string {
	return c.bot.Self.UserName
}

// Send отправляет сообщение, возвращает ID отправленного сообщения
func (c *TelegramClient) Send(msg tgbotapi.MessageConfig) (int, error) {
	resp, err := c.bot.Send(msg)
	if err != nil {
		return 0, fmt.Errorf("send message: %w", err)
	}
	return resp.MessageID, nil
}

// SetCommands регистрирует команды в меню бота
func (c *TelegramClient) SetCommands(commands []tgbotapi.BotCommand) error {
	_, err := c.bot.Request(tgbotapi.NewSetMyCommands(commands...))
	if err != nil {
		return fmt.Errorf("set commands: %w", err)
	}
	return nil
}

// Updates запускает получение обновлений и возвращает канал
func (c *TelegramClient) Updates(_ context.Context) tgbotapi.UpdatesChannel {
	u := tgbotapi.NewUpdate(updateOffset) // константа
	u.Timeout = c.timeout
	return c.bot.GetUpdatesChan(u)
}

const updateOffset = 0
