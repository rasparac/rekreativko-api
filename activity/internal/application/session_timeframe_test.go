package application

import (
	"testing"

	"github.com/rasparac/rekreativko-api/activity/internal/domain"
	"github.com/stretchr/testify/assert"
)

func TestParseSessionTimeframe(t *testing.T) {
	for raw, want := range map[string]SessionTimeframe{
		"":         SessionTimeframeUpcoming,
		"upcoming": SessionTimeframeUpcoming,
		"past":     SessionTimeframePast,
		"all":      SessionTimeframeAll,
	} {
		got, err := parseSessionTimeframe(raw)
		assert.NoError(t, err, raw)
		assert.Equal(t, want, got, raw)
	}

	_, err := parseSessionTimeframe("yesterday")
	assert.ErrorIs(t, err, domain.ErrInvalidSessionTimeframe)
}
