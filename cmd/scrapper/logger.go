package main

import (
	"log/slog"
	"os"
	"strings"
)

const serviceName = "scrapper"

// newLogger создаёт JSON-логгер со схемой:
// timestamp, level, message, service.name
func newLogger() *slog.Logger {
	handler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
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
	return slog.New(handler).With("service.name", serviceName)
}
