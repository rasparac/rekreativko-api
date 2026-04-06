package middleware

import (
	"net/http"

	"github.com/rasparac/rekreativko-api/internal/shared/authcontext"
)

func ExtractUserContext(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		accountID := authcontext.GetAccountIDFromHeader(r.Header)

		ctx = authcontext.WithAccountID(ctx, accountID)

		roles := authcontext.GetRolesFromHeader(r.Header)
		ctx = authcontext.WithRoles(ctx, roles)

		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
