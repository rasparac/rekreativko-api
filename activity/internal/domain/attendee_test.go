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

func TestNewRequestedAttendee(t *testing.T) {
	session := newTestSessionWithCapacity(t, 5)
	user, manager := uuid.New(), uuid.New()

	a := NewRequestedAttendee(session, session.ActivityGroupID(), user, []uuid.UUID{manager})

	assert.Equal(t, AttendeeStatusPending, a.Status(), "never going until a manager approves")
	assert.Equal(t, AttendeeSourceRequested, a.Source())
	assert.Equal(t, user, a.UserID())
	assert.Equal(t, session.ID(), a.SessionID())
	assert.Equal(t, []string{EventActivitySessionAttendeeJoinRequested}, attendeeEventTypes(a))
}

func TestAttendee_Promote(t *testing.T) {
	session := newTestSessionWithCapacity(t, 1)

	t.Run("a waiting attendee is promoted", func(t *testing.T) {
		a, err := NewRSVPManualAttendee(session, nil, uuid.New(), AttendeeStatusGoing, 1) // full: pending
		require.NoError(t, err)
		require.Equal(t, AttendeeStatusPending, a.Status())
		a.ClearEvents()

		require.NoError(t, a.Promote())

		assert.Equal(t, AttendeeStatusPromoted, a.Status())
		assert.True(t, a.Status().IsConfirmed())
		assert.Equal(t, []string{EventActivitySessionAttendeePromoted}, attendeeEventTypes(a))
	})

	t.Run("only someone who is waiting", func(t *testing.T) {
		for _, status := range []AttendeeStatus{AttendeeStatusGoing, AttendeeStatusMaybe, AttendeeStatusNotGoing, AttendeeStatusPromoted} {
			a, err := NewRSVPManualAttendee(session, nil, uuid.New(), status, 0)
			require.NoError(t, err)

			assert.ErrorIs(t, a.Promote(), ErrInvalidAttendeeStatus, status)
		}
	})
}

func TestAttendee_ApproveAndReject_AreOnlyForJoinRequests(t *testing.T) {
	session := newTestSessionWithCapacity(t, 5)
	creator := session.CreatedByID()

	t.Run("a request is approved or rejected by a manager", func(t *testing.T) {
		approved := NewRequestedAttendee(session, nil, uuid.New(), nil)
		require.NoError(t, approved.Approve(session, creator, ""))
		assert.Equal(t, AttendeeStatusGoing, approved.Status())

		rejected := NewRequestedAttendee(session, nil, uuid.New(), nil)
		require.NoError(t, rejected.Reject(session, creator, ""))
		assert.Equal(t, AttendeeStatusNotGoing, rejected.Status())
	})

	t.Run("a person who is merely on the waitlist is not a request", func(t *testing.T) {
		waiting, err := NewRSVPManualAttendee(session, nil, uuid.New(), AttendeeStatusGoing, 5)
		require.NoError(t, err)
		require.Equal(t, AttendeeStatusPending, waiting.Status())

		assert.ErrorIs(t, waiting.Approve(session, creator, ""), ErrAttendeeNotAwaitingApproval)
		assert.ErrorIs(t, waiting.Reject(session, creator, ""), ErrAttendeeNotAwaitingApproval)
	})

	t.Run("not by someone who cannot manage the session", func(t *testing.T) {
		request := NewRequestedAttendee(session, nil, uuid.New(), nil)

		assert.ErrorIs(t, request.Approve(session, uuid.New(), MemberRoleMember), ErrUnauthorized)
		assert.ErrorIs(t, request.Reject(session, uuid.New(), ""), ErrUnauthorized)
		assert.Equal(t, AttendeeStatusPending, request.Status())
	})
}
