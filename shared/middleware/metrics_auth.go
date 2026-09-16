package middleware

import (
	"crypto/subtle"
	"net/http"
	"strings"

	"github.com/rasparac/rekreativko-api/shared/logger"
)

// RequireMetricsToken protects a /metrics endpoint with a bearer token, so it can sit
// on a publicly reachable listener without leaking operational data to anyone who can
// reach the port. Prometheus scrape_configs support "authorization: credentials"
// natively, so no custom scrape client is needed on the collector side.
func RequireMetricsToken(logger *logger.Logger, expectedToken string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()

			token, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
			if !ok || subtle.ConstantTimeCompare([]byte(token), []byte(expectedToken)) != 1 {
				logger.Warn(ctx, "invalid or missing metrics token")
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
