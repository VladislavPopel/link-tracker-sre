package scrapper

import (
	"log/slog"
	"net/http"
	"time"
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

func (r *statusRecorder) Unwrap() http.ResponseWriter {
	return r.ResponseWriter
}

// LoggingMiddleware пишет одну структурированнную запись на каждый HTTP запрос
func LoggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}

		next.ServeHTTP(rec, r)

		route := r.Pattern
		if route == "" {
			route = r.URL.Path
		}

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
			"duration_ms", float64(time.Since(start).Microseconds())/1000,
		)
	})
}
