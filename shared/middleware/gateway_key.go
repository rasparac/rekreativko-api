package middleware

import (
	"crypto/subtle"
	"net/http"
	"strings"

	"github.com/rasparac/rekreativko-api/shared/logger"
)

const (
	gatewayKey = "X-Gateway-Key"
)

func CheckGatewayKey(logger *logger.Logger, expectedKey string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()
			keys := r.Header.Get(gatewayKey)
			if keys == "" {
				logger.Warn(ctx, "gateway key header is missing")
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}

			// Check each key using constant-time comparison to prevent timing attacks
			for key := range strings.SplitSeq(keys, ",") {
				key = strings.TrimSpace(key)
				// subtle.ConstantTimeCompare returns 1 if equal, 0 otherwise
				if subtle.ConstantTimeCompare([]byte(key), []byte(expectedKey)) == 1 {
					next.ServeHTTP(w, r)
					return
				}
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
