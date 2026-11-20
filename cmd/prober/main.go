// Prober - синтетический клиент: раз в несколько секунд обращается к сервису
// снаружи (как пользователь) и считает успешные и неуспешные проверки.
// Результаты отдаёт в формате OpenMetrics на /metrics.
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/caarlos0/env/v11"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

const (
	resultSuccess   = "success"
	resultFailure   = "failure"
	shutdownTimeout = 5 * time.Second
)

type config struct {
	TargetURL  string        `env:"PROBE_TARGET_URL,required"`
	Interval   time.Duration `env:"PROBE_INTERVAL"    envDefault:"5s"`
	Timeout    time.Duration `env:"PROBE_TIMEOUT"     envDefault:"2s"`
	ListenAddr string        `env:"PROBE_LISTEN_ADDR" envDefault:":9091"`
}

// probe - одна проверка: GET <TargetURL><path> должен вернуть 200 за время Timeout.
type probe struct {
	name string
	path string
}

var probes = []probe{
	{name: "frontend", path: "/"},
	{name: "api", path: "/tg-chat?limit=1"}, // проходит через бэкенд до базы данных
}

var probeRequests = promauto.NewCounterVec(prometheus.CounterOpts{
	Name: "linktracker_probe_requests_total",
	Help: "Total number of synthetic probe requests by probe and result.",
}, []string{"probe", "result"})

func init() {
	// Все комбинации меток существуют со значения 0, чтобы в запросах не было "дыр"
	for _, p := range probes {
		for _, r := range []string{resultSuccess, resultFailure} {
			probeRequests.WithLabelValues(p.name, r)
		}
	}
}

func main() {
	if err := run(); err != nil {
		slog.Error("prober failed", "err", err)
		os.Exit(1)
	}
}

func run() error {
	slog.SetDefault(newLogger())

	var cfg config
	if err := env.Parse(&cfg); err != nil {
		return fmt.Errorf("parse config: %w", err)
	}
	baseURL := strings.TrimRight(cfg.TargetURL, "/")

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())
	srv := &http.Server{Addr: cfg.ListenAddr, Handler: mux}

	srvErr := make(chan error, 1)
	go func() {
		if err := srv.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
			srvErr <- err
		}
	}()

	client := &http.Client{Timeout: cfg.Timeout}
	ticker := time.NewTicker(cfg.Interval)
	defer ticker.Stop()

	slog.Info("prober started",
		"target", baseURL,
		"interval", cfg.Interval.String(),
		"timeout", cfg.Timeout.String(),
		"metricsAddr", cfg.ListenAddr,
	)

	for {
		select {
		case <-ctx.Done():
			slog.Info("prober shutting down")
			shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
			defer cancel()
			return srv.Shutdown(shutdownCtx)
		case err := <-srvErr:
			return fmt.Errorf("metrics server: %w", err)
		case <-ticker.C:
			for _, p := range probes {
				runProbe(ctx, client, baseURL, p)
			}
		}
	}
}

func runProbe(ctx context.Context, client *http.Client, baseURL string, p probe) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+p.path, nil)
	if err != nil {
		slog.Error("build probe request", "probe", p.name, "err", err)
		return
	}

	start := time.Now()
	resp, err := client.Do(req)
	if ctx.Err() != nil {
		return // процесс останавливается: это не сбой сервиса, не считаем
	}

	result := resultFailure
	status := 0
	if err == nil {
		status = resp.StatusCode
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
		if status == http.StatusOK {
			result = resultSuccess
		}
	}
	probeRequests.WithLabelValues(p.name, result).Inc()

	if result == resultFailure {
		slog.Warn("probe failed",
			"probe", p.name,
			"status", status,
			"duration_ms", float64(time.Since(start).Microseconds())/1000,
			"err", err,
		)
	}
}

// newLogger - JSON-логи в той же схеме полей, что и у scrapper
// (timestamp, level, message, service.name), чтобы их разбирал Vector.
func newLogger() *slog.Logger {
	handler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			if len(groups) > 0 {
				return a
			}
			switch a.Key {
			case slog.TimeKey:
				a.Key = "timestamp"
			case slog.MessageKey:
				a.Key = "message"
			case slog.LevelKey:
				if level, ok := a.Value.Any().(slog.Level); ok {
					a.Value = slog.StringValue(strings.ToLower(level.String()))
				}
			}
			return a
		},
	})
	return slog.New(handler).With("service.name", "prober")
}
