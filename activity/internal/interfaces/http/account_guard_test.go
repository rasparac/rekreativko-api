package http

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/rasparac/rekreativko-api/shared/authcontext"
	"github.com/rasparac/rekreativko-api/shared/logger"
	"github.com/stretchr/testify/assert"
)

type fakeInactiveAccounts struct {
	inactive map[uuid.UUID]struct{}
	err      error
}

func (f fakeInactiveAccounts) IsInactive(_ context.Context, accountID uuid.UUID) (bool, error) {
	_, ok := f.inactive[accountID]
	return ok, f.err
}

func guarded(t *testing.T, accounts fakeInactiveAccounts, method string, accountID uuid.UUID) int {
	t.Helper()

	handler := RejectInactiveAccounts(accounts, logger.New("error", "json"))(
		http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) }),
	)

	req := httptest.NewRequest(method, "/api/v1/sessions/x/rsvp", nil)
	req = req.WithContext(authcontext.WithAccountID(req.Context(), accountID))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	return rec.Code
}

func TestRejectInactiveAccounts(t *testing.T) {
	deleted, active := uuid.New(), uuid.New()
	accounts := fakeInactiveAccounts{inactive: map[uuid.UUID]struct{}{deleted: {}}}

	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete} {
		assert.Equal(t, http.StatusForbidden, guarded(t, accounts, method, deleted), method+" by a deleted account")
		assert.Equal(t, http.StatusNoContent, guarded(t, accounts, method, active), method+" by an active account")
	}
	assert.Equal(t, http.StatusNoContent, guarded(t, accounts, http.MethodGet, deleted), "reads are not checked")
	assert.Equal(t, http.StatusNoContent, guarded(t, accounts, http.MethodPost, uuid.Nil), "no account, nothing to check")

	broken := fakeInactiveAccounts{err: errors.New("connection reset")}
	assert.Equal(t, http.StatusInternalServerError, guarded(t, broken, http.MethodPost, active), "fails closed")
}
