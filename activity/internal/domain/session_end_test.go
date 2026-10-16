package domain

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSessionSchedule_EffectiveEndTime(t *testing.T) {
	start := time.Now().UTC().Add(time.Hour)
	end := start.Add(90 * time.Minute)

	withEnd, err := NewSessionSchedule(start, &end)
	require.NoError(t, err)
	assert.Equal(t, end, withEnd.EffectiveEndTime())

	withoutEnd, err := NewSessionSchedule(start, nil)
	require.NoError(t, err)
	assert.Equal(t, start.Add(DefaultSessionDuration), withoutEnd.EffectiveEndTime())
}

func TestSession_ExpireSchedule_WithoutEndTimeUsesTheDefaultDuration(t *testing.T) {
	session := newTestSessionWithCapacity(t, 5)
	require.Nil(t, session.Schedule().EndTime())
	start := session.Schedule().StartTime()

	// Still inside start + DefaultSessionDuration: not over yet.
	assert.ErrorIs(t, session.ExpireSchedule(start.Add(DefaultSessionDuration-time.Minute)), ErrSessionNotExpiredYet)
	assert.Equal(t, SessionStatusScheduled, session.Status())

	require.NoError(t, session.ExpireSchedule(start.Add(DefaultSessionDuration+time.Minute)))
	assert.Equal(t, SessionStatusCompleted, session.Status())
}
