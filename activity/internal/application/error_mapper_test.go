package application

import (
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
