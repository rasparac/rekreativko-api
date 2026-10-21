package http

import (
	"context"
	"net/http"

	"github.com/google/uuid"
	"github.com/rasparac/rekreativko-api/shared/api"
	"github.com/rasparac/rekreativko-api/shared/authcontext"
	"github.com/rasparac/rekreativko-api/shared/logger"
)

type inactiveAccountChecker interface {
	IsInactive(ctx context.Context, accountID uuid.UUID) (bool, error)
}

// RejectInactiveAccounts refuses every request that changes something (any
// method but GET, HEAD and OPTIONS) from an account deleted or erased in
// identity, with 403 account_deleted. Identity revokes a deleted account's
// refresh tokens, but its access token stays valid until it expires; without
// this, it could RSVP or join a group again after it was taken out of them.
// Must run after middleware.ExtractUserContext.
func RejectInactiveAccounts(accounts inactiveAccountChecker, log *logger.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.Method {
			case http.MethodGet, http.MethodHead, http.MethodOptions:
				next.ServeHTTP(w, r)
				return
			}

			ctx := r.Context()
			accountID := authcontext.GetAccountID(ctx)
			if accountID == uuid.Nil {
				next.ServeHTTP(w, r)
				return
			}

			inactive, err := accounts.IsInactive(ctx, accountID)
			if err != nil {
				log.Error(ctx, "failed to check the account", "error", err, "account_id", accountID)
				_ = api.WriteInternalServerErrorResponse(w)
				return
			}

			if inactive {
				_ = api.WriteForbiddenResponse(w, "account_deleted", "This account has been deleted")
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
