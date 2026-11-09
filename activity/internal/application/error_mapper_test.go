package application

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/rasparac/rekreativko-api/activity/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMapErrToAppError_WeeklyRuleWithoutADayOfWeek(t *testing.T) {
	appErr := MapErrToAppError(domain.ErrRecurrenceRuleMissingDayOfWeek)

	require.NotNil(t, appErr)
	assert.Equal(t, "missing_day_of_week", appErr.Code)
}

func TestMapErrToAppError_ProposalLimitsSayWhichAndHowMany(t *testing.T) {
	tests := []struct {
		err   error
		code  string
		limit int
	}{
		{domain.ErrRoundProposalLimit, "round_proposal_limit", domain.MaxProposalsPerRound},
		{domain.ErrAuthorProposalLimit, "author_proposal_limit", domain.MaxProposalsPerAuthor},
	}

	for _, tt := range tests {
		t.Run(tt.code, func(t *testing.T) {
			appErr := MapErrToAppError(fmt.Errorf("propose: %w", tt.err))

			require.NotNil(t, appErr)
			assert.Equal(t, tt.code, appErr.Code)
			assert.Equal(t, http.StatusConflict, appErr.StatusCode)
			assert.Equal(t, tt.limit, appErr.Details["limit"])
		})
	}
}
