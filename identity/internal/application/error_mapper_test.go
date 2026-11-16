package application

import (
	"errors"
	"fmt"
	"net/http"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/rasparac/rekreativko-api/identity/internal/domain"
	"github.com/stretchr/testify/assert"
)

func TestMapDomainErr(t *testing.T) {
	tests := map[string]struct {
		err        error
		wantCode   string
		wantStatus int
	}{
		"a sentinel": {
			err: domain.ErrInvalidCredentials, wantCode: "identity_invalid_credentials", wantStatus: http.StatusUnauthorized,
		},
		// The service wraps errors with what failed; that must not turn a 404 into a 500.
		"a wrapped sentinel": {
			err: fmt.Errorf("get account: %w", domain.ErrAccountNotFound), wantCode: "identity_account_not_found", wantStatus: http.StatusNotFound,
		},
		"a locked account, with the lock's end": {
			err:      fmt.Errorf("can login: %w", domain.NewAccountLockedError("too many attempts", "2026-11-16T10:00:00Z")),
			wantCode: "identity_account_locked", wantStatus: http.StatusForbidden,
		},
		"a weak password": {
			err:      &domain.PasswordRequirementError{Requirements: []string{"a digit"}},
			wantCode: "identity_weak_password", wantStatus: http.StatusBadRequest,
		},
		"no rows": {
			err: fmt.Errorf("get token: %w", pgx.ErrNoRows), wantCode: "not_found", wantStatus: http.StatusNotFound,
		},
		"anything else": {
			err: errors.New("connection reset"), wantStatus: http.StatusInternalServerError,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			got := mapDomainErr(tt.err)

			assert.Equal(t, tt.wantStatus, got.StatusCode)
			if tt.wantCode != "" {
				assert.Equal(t, tt.wantCode, got.Code)
			}
			assert.ErrorIs(t, got, tt.err, "the original error stays inside")
		})
	}
}
