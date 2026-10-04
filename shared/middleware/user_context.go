package middleware

import (
	"net/http"

	"github.com/rasparac/rekreativko-api/shared/authcontext"
)

func ExtractUserContext(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		accountID := authcontext.GetAccountIDFromHeader(r.Header)

		ctx = authcontext.WithAccountID(ctx, accountID)

		if expiresAt := authcontext.GetTokenExpiresAtFromHeader(r.Header); !expiresAt.IsZero() {
			ctx = authcontext.WithTokenExpiresAt(ctx, expiresAt)
		}

		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
