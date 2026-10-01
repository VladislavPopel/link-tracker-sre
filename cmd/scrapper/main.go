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

	"gitlab.education.tbank.ru/backend-academy-go-2025/homeworks/link-tracker/internal/config"
	"gitlab.education.tbank.ru/backend-academy-go-2025/homeworks/link-tracker/internal/domain"
	"gitlab.education.tbank.ru/backend-academy-go-2025/homeworks/link-tracker/internal/infrastructure/db"
	"gitlab.education.tbank.ru/backend-academy-go-2025/homeworks/link-tracker/internal/infrastructure/repository"
	"gitlab.education.tbank.ru/backend-academy-go-2025/homeworks/link-tracker/internal/scrapper"
)

const (
	shutdownTimeout = 5 * time.Second
)

func main() {
	if err := run(); err != nil {
		slog.Error("scrapper failed", "err", err)
		os.Exit(1)
	}
}

func run() error {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	cfg, err := config.LoadScrapper()
	if err != nil {
		return fmt.Errorf("load scrapper config: %w", err)
	}
	// База данных
	linkRepo, err := initRepository(cfg)
	if err != nil {
		return err
	}

	sched, err := initScheduler(cfg, linkRepo)
	if err != nil {
		return err
	}

	return startServer(cfg, linkRepo, sched)
}

func initRepository(cfg *config.ScrapperConfig) (domain.LinkRepository, error) {
	sqlDB, err := db.Open(cfg.DatabaseDSN)
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}

	if err = db.Ping(context.Background(), sqlDB); err != nil {
		return nil, fmt.Errorf("ping db: %w", err)
	}
	// Применяем миграции при старте
	if err = db.RunMigrations(sqlDB); err != nil {
		return nil, fmt.Errorf("run migrations: %w", err)
	}
	slog.Info("migrations applied")

	repo, err := repository.New(sqlDB, cfg.DatabaseDSN, repository.AccessType(cfg.DatabaseAccessType))
	if err != nil {
		return nil, fmt.Errorf("create repository: %w", err)
	}
	slog.Info("repository initialized", "access-type", cfg.DatabaseAccessType)
	return repo, nil
}

func initScheduler(cfg *config.ScrapperConfig, repo domain.LinkRepository) (*scrapper.Scheduler, error) {
	// Link checkers
	checkers := []scrapper.LinkChecker{
		scrapper.NewGitHubChecker(cfg.GitHubToken),
		scrapper.NewStackOverflowChecker(cfg.StackOverflowAPIKey),
	}
	// Sheduler
	// MessageSender
	sender := scrapper.NewHTTPMessageSender(cfg.BotURL)

	interval := time.Duration(cfg.ScheduleIntervalSec) * time.Second

	sched, err := scrapper.NewScheduler(
		repo,
		checkers,
		sender,
		interval,
		cfg.BatchSize,
		cfg.WorkerCount,
	)
	if err != nil {
		return nil, fmt.Errorf("create scheduler: %w", err)
	}
	return sched, nil
}

func startServer(cfg *config.ScrapperConfig, repo domain.LinkRepository, sched *scrapper.Scheduler) error {
	handler := scrapper.NewHandler(repo)
	httpServer := &http.Server{
		Addr:    cfg.ListenAddr,
		Handler: handler.Router(),
	}
	// Graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig := <-sigCh
		slog.Info("received signal, shutting down", "signal", sig)
		cancel()

		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer shutdownCancel()
		if shutdownErr := httpServer.Shutdown(shutdownCtx); shutdownErr != nil {
			slog.Error("http server shutdown error", "err", shutdownErr)
		}
		if stopErr := sched.Stop(); stopErr != nil {
			slog.Error("scheduler stop error", "err", stopErr)
		}
	}()

	sched.Start()
	slog.Info("scrapper listening",
		"addr", cfg.ListenAddr,
		"batchSize", cfg.BatchSize,
		"workers", cfg.WorkerCount,
	)

	if listenErr := httpServer.ListenAndServe(); !errors.Is(listenErr, http.ErrServerClosed) {
		return fmt.Errorf("http server ListenAndServe: %w", listenErr)
	}

	<-ctx.Done()
	slog.Info("scrapper stopped")
	return nil
}

/*
func run() error {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	cfg, err := config.LoadScrapper()
	if err != nil {
		slog.Error("failed to load config", "err", err)
		return fmt.Errorf("load scrapper config: %w", err)
	}

	// База данных
	sqlDB, err := db.Open(cfg.DatabaseDSN)
	if err != nil {
		return fmt.Errorf("open db: %w", err)
	}
	defer func() { _ = sqlDB.Close() }()

	ctx := context.Background()
	if err = db.Ping(ctx, sqlDB); err != nil {
		return fmt.Errorf("ping db: %w", err)
	}

	// Применяем миграции при старте
	if err = db.RunMigrations(sqlDB); err != nil {
		return fmt.Errorf("run migrations: %w", err)
	}
	slog.Info("migrations applied")

	// linkRepo := infrastructure.NewInMemoryLinkRepo()
	linkRepo, err := repository.New(sqlDB, cfg.DatabaseDSN, repository.AccessType(cfg.DatabaseAccessType))
	if err != nil {
		return fmt.Errorf("create repository: %w", err)
	}
	slog.Info("repository initialized", "access-type", cfg.DatabaseAccessType)

	// HTTP handler
	handler := scrapper.NewHandler(linkRepo)
	httpServer := &http.Server{
		Addr:    cfg.ListenAddr,
		Handler: handler.Router(),
	}

	// Link checkers
	checkers := []scrapper.LinkChecker{
		scrapper.NewGitHubChecker(cfg.GitHubToken),
		scrapper.NewStackOverflowChecker(cfg.StackOverflowAPIKey),
	}

	// Sheduler
	interval := time.Duration(cfg.ScheduleIntervalSec) * time.Second
	stateStore := scrapper.NewLinkStateStore()
	sched, err := scrapper.NewScheduler(linkRepo, checkers, cfg.BotURL, interval, stateStore)
	if err != nil {
		slog.Error("failed to create scheduler", "err", err)
		return fmt.Errorf("create scheduler: %w", err)
	}

	// Graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig := <-sigCh
		slog.Info("received signal, shutting down", "signal", sig)
		cancel()

		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer shutdownCancel()

		if shutdownErr := httpServer.Shutdown(shutdownCtx); shutdownErr != nil {
			slog.Error("http server shutdown error", "err", shutdownErr)
		}

		if stopErr := sched.Stop(); stopErr != nil {
			slog.Error("scheduler stop error", "err", stopErr)
		}
	}()

	sched.Start()

	slog.Info("scrapper listening", "addr", cfg.ListenAddr)
	if err = httpServer.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
		slog.Error("scrapper http server error", "err", err)
	}

	<-ctx.Done()
	slog.Info("scrapper stopped")
	return nil
} */
