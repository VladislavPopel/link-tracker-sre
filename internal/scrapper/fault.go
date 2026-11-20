package scrapper

import (
	"log/slog"
	"math/rand/v2"
	"net/http"
	"time"
)

// FaultInjection - middleware для экспериментов с SLO: искусственно добавляет
// задержку и/или отвечает ошибкой 500 на долю запросов.
// При errorRate == 0 и delay == 0 (значения по умолчанию) ничего не делает.
// Подключается внутри LoggingMiddleware, поэтому внесённые сбои видны и в логах, и в метриках.
func FaultInjection(errorRate float64, delay time.Duration, next http.Handler) http.Handler {
	if errorRate <= 0 && delay <= 0 {
		return next
	}
	slog.Warn("fault injection enabled", "error_rate", errorRate, "delay_ms", delay.Milliseconds())

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if delay > 0 {
			select {
			case <-time.After(delay):
			case <-r.Context().Done():
				return
			}
		}
		if errorRate > 0 && rand.Float64() < errorRate {
			writeAPIError(w, http.StatusInternalServerError, "injected fault", "InjectedFault")
			return
		}
		next.ServeHTTP(w, r)
	})
}
