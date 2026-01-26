package middleware

import (
	"context"
	"net/http"
	"runtime/debug"

	"github.com/rasparac/rekreativko-api/internal/shared/api"
	"github.com/rasparac/rekreativko-api/internal/shared/logger"
)

func Recover(log *logger.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if err := recover(); err != nil {
					log.Error(
						context.Background(),
						"recovered from panic",
						"error", err,
						"stack", string(debug.Stack()),
						"path", r.URL.Path,
						"http_method", r.Method,
					)
					api.WriteInternalServerErrorResponse(w)
				}
			}()

			next.ServeHTTP(w, r)
		})
	}
}
