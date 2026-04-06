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
	RolesContextKey     contextKey = "roles"
	XUserIDHeader                  = "X-User-ID"
	XUserRolesHeader               = "X-User-Roles"
)

func GetAccountID(ctx context.Context) uuid.UUID {
	accountID, ok := ctx.Value(AccountIDContextKey).(uuid.UUID)
	if !ok {
		return uuid.Nil
	}
	return accountID
}

func GetRoles(ctx context.Context) []string {
	roles, ok := ctx.Value(RolesContextKey).([]string)
	if !ok {
		return []string{}
	}
	return roles
}

func GetAccountIDFromHeader(header http.Header) uuid.UUID {
	accountID, err := uuid.Parse(header.Get(XUserIDHeader))
	if err != nil {
		return uuid.Nil
	}
	return accountID
}

func GetRolesFromHeader(header http.Header) []string {
	return header.Values(XUserRolesHeader)
}

func WithAccountID(ctx context.Context, accountID uuid.UUID) context.Context {
	return context.WithValue(ctx, AccountIDContextKey, accountID)
}

func WithRoles(ctx context.Context, roles []string) context.Context {
	return context.WithValue(ctx, RolesContextKey, roles)
}
