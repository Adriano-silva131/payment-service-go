package httpadapter

import (
	"log/slog"
	"net/http"
	"time"
)

// RequestLogger logs method, path, status and duration for every request — the Go
// equivalent of the access logging Spring Boot/Actuator gives for free.
func RequestLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}

		next.ServeHTTP(sw, r)

		slog.InfoContext(r.Context(), "http request",
			"method", r.Method, "path", r.URL.Path, "status", sw.status, "duration", time.Since(start))
	})
}

// Recoverer converts a panic in any handler into a 500 instead of crashing the process,
// mirroring Spring MVC's default exception-handling behavior.
func Recoverer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				slog.ErrorContext(r.Context(), "panic recovered", "error", rec)
				w.WriteHeader(http.StatusInternalServerError)
			}
		}()
		next.ServeHTTP(w, r)
	})
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(status int) {
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}
