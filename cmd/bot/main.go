package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"gitlab.education.tbank.ru/backend-academy-go-2025/homeworks/link-tracker/internal/application"
	"gitlab.education.tbank.ru/backend-academy-go-2025/homeworks/link-tracker/internal/bot"
	"gitlab.education.tbank.ru/backend-academy-go-2025/homeworks/link-tracker/internal/client"
	"gitlab.education.tbank.ru/backend-academy-go-2025/homeworks/link-tracker/internal/config"
	"gitlab.education.tbank.ru/backend-academy-go-2025/homeworks/link-tracker/internal/infrastructure"
)

const (
	shutdownTimeout = 5 * time.Second
)

func main() {
	if err := run(); err != nil {
		slog.Error("bot failed", "err", err)
		os.Exit(1)
	}
}

func run() error {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	cfg, err := config.LoadBot() // BotConfig
	if err != nil {
		slog.Error("failed to load config", "err", err)
		return fmt.Errorf("load bot config: %w", err)
	}

	tgClient, err := client.NewTelegramClient(cfg.TelegramToken, cfg.UpdatesTimeout, cfg.Debug)
	if err != nil {
		slog.Error("failed to create telegram client", "err", err)
		return fmt.Errorf("create telegram client: %w", err)
	}

	commander, err := initCommander(cfg, tgClient)
	if err != nil {
		slog.Error("failed to init commander", "err", err)
		return fmt.Errorf("init commander: %w", err)
	}

	httpServer := initHTTPServer(cfg, tgClient)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		sig := <-sigCh
		slog.Info("received signal, shutting down", "signal", sig)
		cancel()
	}()

	go func() {
		<-ctx.Done()
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer shutdownCancel()
		if shutdownErr := httpServer.Shutdown(shutdownCtx); shutdownErr != nil {
			slog.Error("http server shutdown error", "err", shutdownErr)
		}
	}()

	go func() {
		slog.Info("bot update server listening", "addr", cfg.BotListenAddr)
		if serveErr := httpServer.ListenAndServe(); !errors.Is(serveErr, http.ErrServerClosed) {
			slog.Error("bot http server error", "err", serveErr)
		}
	}()

	slog.Info("bot started", "username", tgClient.Username())
	updates := tgClient.Updates(ctx)

	for {
		select {
		case <-ctx.Done():
			slog.Info("context cancelled, stopping bot")
			return nil
		case update, ok := <-updates:
			if !ok {
				return nil
			}
			commander.Handle(ctx, update)
		}
	}
}

func initCommander(cfg *config.BotConfig, tgClient *client.TelegramClient) (*application.Commander, error) {
	scrapperClient := client.NewScrapperHTTPClient(cfg.ScrapperURL)
	userRepo := infrastructure.NewInMemoryUserRepo()

	commander, err := application.NewCommander(tgClient, userRepo, scrapperClient)
	if err != nil {
		slog.Error("failed to create commander", "err", err)
		return nil, fmt.Errorf("create commander: %w", err)
	}
	if err = commander.Init(); err != nil {
		return nil, fmt.Errorf("init commander: %w", err)
	}
	return commander, nil
}

func initHTTPServer(cfg *config.BotConfig, sender bot.MessageSender) *http.Server {
	updateServer := bot.NewUpdateServer(sender)
	return &http.Server{
		Addr:    cfg.BotListenAddr,
		Handler: updateServer.Handler(),
	}
}
