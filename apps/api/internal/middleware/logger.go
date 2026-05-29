package middleware

import (
	"log/slog"
	"net/http"
	"time"
)

type responseWriter struct {
	http.ResponseWriter
	status int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.status = code
	rw.ResponseWriter.WriteHeader(code)
}

// Unwrap exposes the underlying ResponseWriter so http.NewResponseController
// (Go 1.20+) can walk through to find the Flusher / Hijacker / etc. Required
// for SSE streams that need explicit flush.
func (rw *responseWriter) Unwrap() http.ResponseWriter { return rw.ResponseWriter }

// Logger logs each request with method, path, status, duration, and request_id.
func Logger(logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rw := &responseWriter{ResponseWriter: w, status: http.StatusOK}

		next.ServeHTTP(rw, r)

		logger.Info("request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", rw.status,
			"duration_ms", time.Since(start).Milliseconds(),
			"request_id", GetRequestID(r.Context()),
		)
	})
}
