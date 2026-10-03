package authcontext

import (
	"context"
	"net/http"

	"github.com/google/uuid"
)

type (
	contextKey string
)

const (
	AccountIDContextKey contextKey = "accountID"
	XUserIDHeader                  = "X-User-ID"
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
