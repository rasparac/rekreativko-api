package middleware

import (
	"net/http"
	"time"

	"github.com/rasparac/rekreativko-api/internal/shared/logger"
)

type responseWriter struct {
	http.ResponseWriter
	statusCode int
	written    int64
}

func (w *responseWriter) WriteHeader(statusCode int) {
	w.statusCode = statusCode
	w.ResponseWriter.WriteHeader(statusCode)
}

func (w *responseWriter) Write(b []byte) (int, error) {
	n, err := w.ResponseWriter.Write(b)
	w.written += int64(n)
	return n, err
}

func Logging(log *logger.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()

			rw := &responseWriter{
				ResponseWriter: w,
				statusCode:     http.StatusOK,
			}

			next.ServeHTTP(rw, r)

			duration := time.Since(start)

			ctx := r.Context()

			log.Info(
				ctx,
				"HTTP Request",
				"http_method", r.Method,
				"path", r.URL.Path,
				"status", rw.statusCode,
				"duration_ms", duration,
				"written", rw.written,
			)

		})
	}
}
