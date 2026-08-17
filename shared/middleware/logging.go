package middleware

import (
	"net/http"
	"time"

	"github.com/rasparac/rekreativko-api/shared/logger"
)

func Logging(log *logger.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()

			rw := newResponseWriter(w)

			next.ServeHTTP(rw, r)

			duration := time.Since(start)

			ctx := r.Context()

			// Log based on status code severity
			// 5xx: ERROR level (server errors)
			// 4xx: WARN level (client errors)
			// 2xx-3xx: INFO level (successful requests)
			if rw.statusCode >= 500 {
				log.Error(ctx, "HTTP Request",
					"http_method", r.Method,
					"path", r.URL.Path,
					"status", rw.statusCode,
					"duration_ms", duration.Milliseconds(),
					"written", rw.written,
				)
			} else if rw.statusCode >= 400 {
				log.Warn(ctx, "HTTP Request",
					"http_method", r.Method,
					"path", r.URL.Path,
					"status", rw.statusCode,
					"duration_ms", duration.Milliseconds(),
					"written", rw.written,
				)
			} else {
				log.Info(ctx, "HTTP Request",
					"http_method", r.Method,
					"path", r.URL.Path,
					"status", rw.statusCode,
					"duration_ms", duration.Milliseconds(),
					"written", rw.written,
				)
			}

		})
	}
}
