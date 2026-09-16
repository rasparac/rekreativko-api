package domain

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestSessionWithCapacity(t *testing.T, capacity int) *Session {
	t.Helper()

	title, err := NewTitle("Test Session")
	require.NoError(t, err)

	location, err := NewSessionLocation("Belgrade", "RS", "", 44.8, 20.4)
	require.NoError(t, err)

	schedule, err := NewSessionSchedule(time.Now().Add(time.Hour), nil)
	require.NoError(t, err)

	visibility := SessionVisibilityPublic
	session, _, err := NewSession(SessionInput{
		CreatedByID:     uuid.New(),
		Title:           title,
		ActivityType:    ActivityTypeRunning,
		DifficultyLevel: DifficultyLevelBeginner,
		Location:        location,
		Schedule:        schedule,
		Capacity:        &capacity,
		Visibility:      &visibility,
	})
	require.NoError(t, err)

	return session
}

// Regression test for rekreativko-api-2o2: NewRSVPManualAttendee(status=Going)
// must itself enforce capacity, since UpdateRSVP's capacity check is a no-op
// when called immediately afterwards with the same status the attendee was
// just constructed with (newStatus == a.status short-circuits to "no change").
func TestNewRSVPManualAttendee_GoingOverCapacity_DowngradesToPending(t *testing.T) {
	session := newTestSessionWithCapacity(t, 1)

	attendee, err := NewRSVPManualAttendee(session, session.ActivityGroupID(), uuid.New(), AttendeeStatusGoing, 1)
	require.NoError(t, err)

	assert.Equal(t, AttendeeStatusPending, attendee.Status())
}

func TestNewRSVPManualAttendee_GoingUnderCapacity_StaysGoing(t *testing.T) {
	session := newTestSessionWithCapacity(t, 1)

	attendee, err := NewRSVPManualAttendee(session, session.ActivityGroupID(), uuid.New(), AttendeeStatusGoing, 0)
	require.NoError(t, err)

	assert.Equal(t, AttendeeStatusGoing, attendee.Status())
}

func TestNewRSVPManualAttendee_GoingUnlimitedCapacity_StaysGoing(t *testing.T) {
	session := newTestSessionWithCapacity(t, 1)
	// nil capacity means unlimited - simulate by passing a huge confirmed count
	// against a session whose capacity is nil.
	session.capacity = nil

	attendee, err := NewRSVPManualAttendee(session, session.ActivityGroupID(), uuid.New(), AttendeeStatusGoing, 1000)
	require.NoError(t, err)

	assert.Equal(t, AttendeeStatusGoing, attendee.Status())
}

func TestNewRSVPManualAttendee_NotGoingOrMaybe_IgnoresCapacity(t *testing.T) {
	session := newTestSessionWithCapacity(t, 1)

	notGoing, err := NewRSVPManualAttendee(session, session.ActivityGroupID(), uuid.New(), AttendeeStatusNotGoing, 1)
	require.NoError(t, err)
	assert.Equal(t, AttendeeStatusNotGoing, notGoing.Status())

	maybe, err := NewRSVPManualAttendee(session, session.ActivityGroupID(), uuid.New(), AttendeeStatusMaybe, 1)
	require.NoError(t, err)
	assert.Equal(t, AttendeeStatusMaybe, maybe.Status())
}
