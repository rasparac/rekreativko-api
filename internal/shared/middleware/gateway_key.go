package middleware

import (
	"net/http"
	"slices"
	"strings"

	"github.com/rasparac/rekreativko-api/internal/shared/logger"
)

const (
	gatewayKey = "X-Gateway-Key"
)

func CheckGatewayKey(logger *logger.Logger, value string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()
			keys := r.Header.Get(gatewayKey)
			if keys == "" {
				logger.Warn(ctx, "gateway key header is missing")
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}

			splittedKey := strings.Split(keys, ",")
			if slices.Contains(splittedKey, value) {
				next.ServeHTTP(w, r)
				return
			}

			logger.Warn(ctx, "invalid gateway key")
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
		})
	}
}

func AddGatewayKey(value string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			r.Header.Add(gatewayKey, value)

			next.ServeHTTP(w, r)
		})
	}
}
