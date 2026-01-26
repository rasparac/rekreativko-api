package middleware

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/rasparac/rekreativko-api/internal/shared/api"
	"github.com/rasparac/rekreativko-api/internal/shared/logger"
	"go.opentelemetry.io/otel/trace"
)

type (
	contextKey string

	auth interface {
		ValidateAccessToken(ctx context.Context, token string) (uuid.UUID, error)
	}

	AuthMiddleware struct {
		auth   auth
		logger *logger.Logger
	}
)

var (
	ErrMissingAuthHeader = errors.New("missing authorization header")
	ErrInvalidAuthHeader = errors.New("invalid authorization header")
)

const (
	AccountIDContextKey contextKey = "accountID"
)

func NewAuthMiddleware(auth auth, logger *logger.Logger) *AuthMiddleware {
	return &AuthMiddleware{
		auth:   auth,
		logger: logger,
	}
}

func (m *AuthMiddleware) RequireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var (
			ctx  = r.Context()
			span = trace.SpanFromContext(ctx)
		)

		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			span.RecordError(ErrMissingAuthHeader)
			m.logger.Error(ctx, "missing authorization header")
			api.WriteUnauthorizedResponse(w, "unauthorized", "missing authorization header")
			return
		}

		parts := strings.Split(authHeader, " ")
		if len(parts) != 2 || parts[0] != "Bearer" {
			span.RecordError(ErrInvalidAuthHeader)
			m.logger.Error(ctx, "invalid authorization header")
			api.WriteUnauthorizedResponse(w, "unauthorized", "invalid authorization header")
			return
		}

		token := parts[1]
		accountID, err := m.auth.ValidateAccessToken(ctx, token)
		if err != nil {
			span.RecordError(err)
			m.logger.Error(ctx, "invalid token", "error", err)
			api.WriteUnauthorizedResponse(w, "unauthorized", "invalid token")
			return
		}

		ctx = context.WithValue(ctx, AccountIDContextKey, accountID)

		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func GetAccountID(r *http.Request) uuid.UUID {
	accountID, ok := r.Context().Value(AccountIDContextKey).(uuid.UUID)
	if !ok {
		return uuid.Nil
	}
	return accountID
}
