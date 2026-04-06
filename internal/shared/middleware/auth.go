package middleware

import (
	"context"
	"errors"
	"net/http"
	"regexp"
	"strings"

	"github.com/google/uuid"
	"github.com/rasparac/rekreativko-api/internal/shared/api"
	"github.com/rasparac/rekreativko-api/internal/shared/authcontext"
	"github.com/rasparac/rekreativko-api/internal/shared/logger"
	"github.com/rasparac/rekreativko-api/internal/shared/token"
	"go.opentelemetry.io/otel/trace"
)

type (
	auth interface {
		ValidateAccessToken(ctx context.Context, token string) (*token.Claims, error)
	}

	AuthMiddleware struct {
		auth        auth
		logger      *logger.Logger
		publicPaths []*regexp.Regexp
	}
)

var (
	ErrMissingAuthHeader = errors.New("missing authorization header")
	ErrInvalidAuthHeader = errors.New("invalid authorization header")
)

func NewAuthMiddleware(auth auth, logger *logger.Logger, publicPaths []string) *AuthMiddleware {
	compiled := make([]*regexp.Regexp, 0, len(publicPaths))
	for _, pattern := range publicPaths {
		re := regexp.MustCompile(pattern)
		compiled = append(compiled, re)
	}

	return &AuthMiddleware{
		auth:        auth,
		logger:      logger,
		publicPaths: compiled,
	}
}

func (m *AuthMiddleware) isPublicPath(path string) bool {

	m.logger.Info(context.Background(), "checking if path is public", "path", path)

	for _, re := range m.publicPaths {
		if re.MatchString(path) {
			return true
		}
	}
	return false
}

func (m *AuthMiddleware) RequireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var (
			ctx  = r.Context()
			span = trace.SpanFromContext(ctx)
		)

		if m.isPublicPath(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}

		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			span.RecordError(ErrMissingAuthHeader)
			m.logger.Error(ctx, "missing authorization header", "path", r.URL.Path)
			api.WriteUnauthorizedResponse(w, "unauthorized", "missing authorization header")
			return
		}

		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || parts[0] != "Bearer" {
			span.RecordError(ErrInvalidAuthHeader)
			m.logger.Error(ctx, "invalid authorization header", "path", r.URL.Path)
			api.WriteUnauthorizedResponse(w, "unauthorized", "invalid authorization header")
			return
		}

		token := parts[1]
		claims, err := m.auth.ValidateAccessToken(ctx, token)
		if err != nil {
			span.RecordError(err)
			m.logger.Error(ctx, "invalid token", "error", err, "path", r.URL.Path)
			api.WriteUnauthorizedResponse(w, "unauthorized", "invalid token")
			return
		}

		accountID, err := uuid.Parse(claims.Subject)
		if err != nil {
			span.RecordError(err)
			m.logger.Error(ctx, "invalid account id", "error", err, "path", r.URL.Path)
			api.WriteUnauthorizedResponse(w, "unauthorized", "invalid account id")
			return
		}

		ctx = context.WithValue(ctx, authcontext.AccountIDContextKey, accountID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
