package scrapper

import (
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"gitlab.education.tbank.ru/backend-academy-go-2025/homeworks/link-tracker/internal/metrics"
)

// statusRecorder запоминает код ответа, который записал обработчик
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

// Unwrap нужен http.ResponseController, чтобы обёртка не прятала возможности исходного writer'а
func (r *statusRecorder) Unwrap() http.ResponseWriter {
	return r.ResponseWriter
}

// LoggingMiddleware на каждый HTTP-запрос пишет одну структурированную запись в лог
// и обновляет HTTP-метрики.
func LoggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}

		next.ServeHTTP(rec, r)

		elapsed := time.Since(start)

		route := r.Pattern
		if route == "" {
			route = r.URL.Path
		}

		metricRoute := r.Pattern
		if metricRoute == "" {
			metricRoute = "unmatched"
		}
		metrics.HTTPRequests.WithLabelValues(r.Method, metricRoute, strconv.Itoa(rec.status)).Inc()
		metrics.HTTPDuration.WithLabelValues(r.Method, metricRoute).Observe(elapsed.Seconds())

		level := slog.LevelInfo
		switch {
		case rec.status >= http.StatusInternalServerError:
			level = slog.LevelError
		case rec.status >= http.StatusBadRequest:
			level = slog.LevelWarn
		case r.URL.Path == "/updates":
			level = slog.LevelDebug
		}

		slog.Log(r.Context(), level, "http request",
			"method", r.Method,
			"route", route,
			"status", rec.status,
			"duration_ms", float64(elapsed.Microseconds())/1000,
		)
	})
}
