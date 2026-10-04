package authcontext

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"
)

type (
	contextKey string
)

const (
	AccountIDContextKey      contextKey = "accountID"
	TokenExpiresAtContextKey contextKey = "tokenExpiresAt"
	XUserIDHeader                       = "X-User-ID"
	// XTokenExpiresAtHeader is when the caller's access token expires (unix
	// seconds). Like X-User-ID only the gateway sets it, so long-lived
	// responses (SSE) can end when the token does.
	XTokenExpiresAtHeader = "X-Token-Expires-At"
)

func GetAccountID(ctx context.Context) uuid.UUID {
	accountID, ok := ctx.Value(AccountIDContextKey).(uuid.UUID)
	if !ok {
		return uuid.Nil
	}
	return accountID
}

func GetAccountIDFromHeader(header http.Header) uuid.UUID {
	accountID, err := uuid.Parse(header.Get(XUserIDHeader))
	if err != nil {
		return uuid.Nil
	}
	return accountID
}

func WithAccountID(ctx context.Context, accountID uuid.UUID) context.Context {
	return context.WithValue(ctx, AccountIDContextKey, accountID)
}

// GetTokenExpiresAt returns when the caller's access token expires, or the
// zero time when it is unknown.
func GetTokenExpiresAt(ctx context.Context) time.Time {
	expiresAt, _ := ctx.Value(TokenExpiresAtContextKey).(time.Time)
	return expiresAt
}

func WithTokenExpiresAt(ctx context.Context, expiresAt time.Time) context.Context {
	return context.WithValue(ctx, TokenExpiresAtContextKey, expiresAt)
}

// GetTokenExpiresAtFromHeader parses XTokenExpiresAtHeader; the zero time
// when it is missing or malformed.
func GetTokenExpiresAtFromHeader(header http.Header) time.Time {
	seconds, err := strconv.ParseInt(header.Get(XTokenExpiresAtHeader), 10, 64)
	if err != nil {
		return time.Time{}
	}
	return time.Unix(seconds, 0)
}
