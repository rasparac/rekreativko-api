package application

import (
	"testing"

	"github.com/rasparac/rekreativko-api/activity/internal/domain"
	"github.com/rasparac/rekreativko-api/activity/internal/infrastructure/persistence"
	"github.com/stretchr/testify/assert"
)

func TestParseSessionTimeframe(t *testing.T) {
	for raw, want := range map[string]persistence.SessionTimeframe{
		"":         persistence.SessionTimeframeUpcoming,
		"upcoming": persistence.SessionTimeframeUpcoming,
		"past":     persistence.SessionTimeframePast,
		"all":      persistence.SessionTimeframeAll,
	} {
		got, err := parseSessionTimeframe(raw)
		assert.NoError(t, err, raw)
		assert.Equal(t, want, got, raw)
	}

	_, err := parseSessionTimeframe("yesterday")
	assert.ErrorIs(t, err, domain.ErrInvalidSessionTimeframe)
}
