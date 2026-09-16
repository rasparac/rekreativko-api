package middleware

import (
	"context"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/rasparac/rekreativko-api/shared/authcontext"
	"github.com/rasparac/rekreativko-api/shared/logger"
)

// accountIDLogKey carries a *uuid.UUID (not a plain value) into context so
// RequireAuth - which runs *after* Logging in the gateway's middleware
// chain and therefore only ever sees a downstream copy of the request - can
// still report the resolved account ID back to this line. A plain
// context.WithValue can't do this: each middleware's r.WithContext(...)
// makes a new *http.Request, so a value set deeper in the chain never
// becomes visible to an outer middleware's own request/context. A pointer
// stashed here before calling next is shared by reference, so writes to
// *ptr from deeper in the chain are visible once next.ServeHTTP returns.
type accountIDLogKey struct{}

func withAccountIDLogPointer(ctx context.Context) (context.Context, *uuid.UUID) {
	ptr := new(uuid.UUID)
	return context.WithValue(ctx, accountIDLogKey{}, ptr), ptr
}

func accountIDLogPointer(ctx context.Context) *uuid.UUID {
	ptr, _ := ctx.Value(accountIDLogKey{}).(*uuid.UUID)
	return ptr
}

func Logging(log *logger.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()

			rw := newResponseWriter(w)

			ctx, accountID := withAccountIDLogPointer(r.Context())
			r = r.WithContext(ctx)

			next.ServeHTTP(rw, r)

			duration := time.Since(start)

			// *accountID is uuid.Nil unless RequireAuth ran and resolved one
			// further down the chain - correctly absent for public paths and
			// rejected/unauthenticated requests. Set it on ctx via the
			// standard key (rather than as an explicit field below) so
			// logger.log's own automatic account_id injection picks up this
			// value instead of producing a second, empty "account_id" key.
			if *accountID != uuid.Nil {
				ctx = authcontext.WithAccountID(ctx, *accountID)
			}

			fields := []any{
				"http_method", r.Method,
				"path", r.URL.Path,
				"query_params", r.URL.RawQuery,
				"status", rw.statusCode,
				"duration_ms", duration.Milliseconds(),
				"written", rw.written,
			}

			// Log based on status code severity
			// 5xx: ERROR level (server errors)
			// 4xx: WARN level (client errors)
			// 2xx-3xx: INFO level (successful requests)
			switch {
			case rw.statusCode >= 500:
				log.Error(ctx, "HTTP Request", fields...)
			case rw.statusCode >= 400:
				log.Warn(ctx, "HTTP Request", fields...)
			default:
				log.Info(ctx, "HTTP Request", fields...)
			}
		})
	}
}
