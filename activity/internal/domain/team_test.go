package domain

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func intPtr(v int) *int {
	return &v
}

func newTestTeamSession(t *testing.T, config *TeamConfig) *Session {
	t.Helper()

	title, err := NewTitle("Pickup basketball")
	require.NoError(t, err)

	location, err := NewSessionLocation("Belgrade", "RS", "", 44.8, 20.4)
	require.NoError(t, err)

	schedule, err := NewSessionSchedule(time.Now().Add(time.Hour), nil)
	require.NoError(t, err)

	visibility := SessionVisibilityPublic
	session, _, err := NewSession(SessionInput{
		CreatedByID:     uuid.New(),
		Title:           title,
		ActivityType:    ActivityTypeBasketball,
		DifficultyLevel: DifficultyLevelBeginner,
		Location:        location,
		Schedule:        schedule,
		Visibility:      &visibility,
		TeamConfig:      config,
	})
	require.NoError(t, err)

	return session
}

func newTestTeamConfig(t *testing.T, playersPerTeam *int) *TeamConfig {
	t.Helper()

	config, err := NewTeamConfig(nil, playersPerTeam, nil)
	require.NoError(t, err)

	return &config
}

func newGoingAttendee(t *testing.T, session *Session) *Attendee {
	t.Helper()

	attendee, err := NewRSVPManualAttendee(session, session.ActivityGroupID(), uuid.New(), AttendeeStatusGoing, 0)
	require.NoError(t, err)
	require.Equal(t, AttendeeStatusGoing, attendee.Status())
	attendee.ClearEvents()

	return attendee
}

func attendeeEventTypes(a *Attendee) []string {
	types := make([]string, 0, len(a.Events()))
	for _, e := range a.Events() {
		types = append(types, e.GetEventType())
	}
	return types
}

func TestNewTeamConfig(t *testing.T) {
	tests := []struct {
		name           string
		teamCount      *int
		playersPerTeam *int
		colors         []string
		wantErr        error
		wantCount      int
		wantColors     []string
	}{
		{name: "defaults to two teams", wantCount: 2},
		{name: "explicit count", teamCount: intPtr(4), wantCount: 4},
		{name: "count below two", teamCount: intPtr(1), wantErr: ErrInvalidTeamCount},
		{name: "count above max", teamCount: intPtr(MaxTeamCount + 1), wantErr: ErrInvalidTeamCount},
		{name: "players per team optional limit", playersPerTeam: intPtr(5), wantCount: 2},
		{name: "players per team zero", playersPerTeam: intPtr(0), wantErr: ErrInvalidPlayersPerTeam},
		{name: "colors one per team, normalized", colors: []string{"#ff0000", "#0000FF"}, wantCount: 2, wantColors: []string{"#FF0000", "#0000FF"}},
		{name: "colors count mismatch", colors: []string{"#FF0000"}, wantErr: ErrInvalidTeamColors},
		{name: "color not hex", colors: []string{"red", "#0000FF"}, wantErr: ErrInvalidTeamColors},
		{name: "short hex rejected", colors: []string{"#F00", "#00F"}, wantErr: ErrInvalidTeamColors},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config, err := NewTeamConfig(tt.teamCount, tt.playersPerTeam, tt.colors)
			if tt.wantErr != nil {
				assert.ErrorIs(t, err, tt.wantErr)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.wantCount, config.TeamCount())
			assert.Equal(t, tt.playersPerTeam, config.PlayersPerTeam())
			if tt.wantColors != nil {
				assert.Equal(t, tt.wantColors, config.Colors())
			} else {
				assert.Empty(t, config.Colors())
			}
		})
	}
}

func TestNewSession_WithTeams_CreatesNamedTeams(t *testing.T) {
	config, err := NewTeamConfig(intPtr(3), nil, []string{"#FFFFFF", "#000000", "#FF0000"})
	require.NoError(t, err)

	session := newTestTeamSession(t, &config)

	require.True(t, session.HasTeams())
	require.Len(t, session.Teams(), 3)
	for i, team := range session.Teams() {
		assert.Equal(t, session.ID(), team.SessionID())
		assert.Equal(t, i, team.Position())
	}
	assert.Equal(t, "Team A", session.Teams()[0].Name())
	assert.Equal(t, "Team B", session.Teams()[1].Name())
	assert.Equal(t, "Team C", session.Teams()[2].Name())
	assert.Equal(t, "#FF0000", session.Teams()[2].Color())
}

func TestNewSession_WithoutTeams_Unchanged(t *testing.T) {
	session := newTestTeamSession(t, nil)

	assert.False(t, session.HasTeams())
	assert.Nil(t, session.TeamConfig())
	assert.Empty(t, session.Teams())
}

func TestNewSession_TeamsOnNonTeamSport_Rejected(t *testing.T) {
	title, err := NewTitle("Morning run")
	require.NoError(t, err)
	location, err := NewSessionLocation("Belgrade", "RS", "", 44.8, 20.4)
	require.NoError(t, err)
	schedule, err := NewSessionSchedule(time.Now().Add(time.Hour), nil)
	require.NoError(t, err)

	_, _, err = NewSession(SessionInput{
		CreatedByID:     uuid.New(),
		Title:           title,
		ActivityType:    ActivityTypeRunning,
		DifficultyLevel: DifficultyLevelBeginner,
		Location:        location,
		Schedule:        schedule,
		TeamConfig:      newTestTeamConfig(t, nil),
	})

	assert.ErrorIs(t, err, ErrTeamsNotSupported)
}

func TestAttendee_AssignToTeam(t *testing.T) {
	session := newTestTeamSession(t, newTestTeamConfig(t, intPtr(2)))
	teamA := session.Teams()[0].ID()
	teamB := session.Teams()[1].ID()
	creator := session.CreatedByID()

	t.Run("assigns unassigned attendee and emits team_assigned", func(t *testing.T) {
		a := newGoingAttendee(t, session)

		require.NoError(t, a.AssignToTeam(session, teamA, creator, "", 0))

		require.NotNil(t, a.TeamID())
		assert.Equal(t, teamA, *a.TeamID())
		assert.Equal(t, []string{EventActivitySessionAttendeeTeamAssigned}, attendeeEventTypes(a))
	})

	t.Run("moving to another team emits team_changed", func(t *testing.T) {
		a := newGoingAttendee(t, session)
		require.NoError(t, a.AssignToTeam(session, teamA, creator, "", 0))
		a.ClearEvents()

		require.NoError(t, a.AssignToTeam(session, teamB, creator, "", 0))

		assert.Equal(t, teamB, *a.TeamID())
		require.Len(t, a.Events(), 1)
		changed, ok := a.Events()[0].(*AttendeeTeamChangedEvent)
		require.True(t, ok)
		assert.Equal(t, teamA, changed.PreviousTeamID)
		assert.Equal(t, teamB, changed.TeamID)
	})

	t.Run("same team is a no-op even when full", func(t *testing.T) {
		a := newGoingAttendee(t, session)
		require.NoError(t, a.AssignToTeam(session, teamA, creator, "", 0))
		a.ClearEvents()

		require.NoError(t, a.AssignToTeam(session, teamA, creator, "", 2))
		assert.Empty(t, a.Events())
	})

	t.Run("full team rejected", func(t *testing.T) {
		a := newGoingAttendee(t, session)

		err := a.AssignToTeam(session, teamA, creator, "", 2)

		assert.ErrorIs(t, err, ErrTeamFull)
		assert.Nil(t, a.TeamID())
	})

	t.Run("group admin may assign", func(t *testing.T) {
		a := newGoingAttendee(t, session)
		assert.NoError(t, a.AssignToTeam(session, teamA, uuid.New(), MemberRoleAdmin, 0))
	})

	t.Run("regular member rejected", func(t *testing.T) {
		a := newGoingAttendee(t, session)
		err := a.AssignToTeam(session, teamA, uuid.New(), MemberRoleMember, 0)
		assert.ErrorIs(t, err, ErrUnauthorized)
	})

	t.Run("unknown team rejected", func(t *testing.T) {
		a := newGoingAttendee(t, session)
		err := a.AssignToTeam(session, uuid.New(), creator, "", 0)
		assert.ErrorIs(t, err, ErrTeamNotFound)
	})

	t.Run("unconfirmed attendee rejected", func(t *testing.T) {
		a, err := NewRSVPManualAttendee(session, nil, uuid.New(), AttendeeStatusMaybe, 0)
		require.NoError(t, err)

		err = a.AssignToTeam(session, teamA, creator, "", 0)
		assert.ErrorIs(t, err, ErrAttendeeNotGoing)
	})
}

func TestAttendee_AssignToTeam_NoPlayerLimit(t *testing.T) {
	session := newTestTeamSession(t, newTestTeamConfig(t, nil))
	a := newGoingAttendee(t, session)

	err := a.AssignToTeam(session, session.Teams()[0].ID(), session.CreatedByID(), "", 1000)

	assert.NoError(t, err)
}

func TestAttendee_AssignToTeam_SessionWithoutTeams(t *testing.T) {
	session := newTestTeamSession(t, nil)
	a := newGoingAttendee(t, session)

	err := a.AssignToTeam(session, uuid.New(), session.CreatedByID(), "", 0)

	assert.ErrorIs(t, err, ErrSessionHasNoTeams)
}

func TestAttendee_AssignToTeam_TerminalSession(t *testing.T) {
	session := newTestTeamSession(t, newTestTeamConfig(t, nil))
	a := newGoingAttendee(t, session)
	require.NoError(t, session.Cancel(session.CreatedByID(), "", "rain", nil))

	err := a.AssignToTeam(session, session.Teams()[0].ID(), session.CreatedByID(), "", 0)

	assert.ErrorIs(t, err, ErrSessionCanceled)
}

func TestAttendee_UnassignFromTeam(t *testing.T) {
	session := newTestTeamSession(t, newTestTeamConfig(t, nil))
	a := newGoingAttendee(t, session)
	teamA := session.Teams()[0].ID()
	require.NoError(t, a.AssignToTeam(session, teamA, session.CreatedByID(), "", 0))
	a.ClearEvents()

	require.NoError(t, a.UnassignFromTeam(session, session.CreatedByID(), ""))

	assert.Nil(t, a.TeamID())
	require.Len(t, a.Events(), 1)
	unassigned, ok := a.Events()[0].(*AttendeeTeamUnassignedEvent)
	require.True(t, ok)
	assert.Equal(t, teamA, unassigned.TeamID)
	assert.Equal(t, TeamUnassignReasonManual, unassigned.Reason)

	// already unassigned: no-op
	a.ClearEvents()
	require.NoError(t, a.UnassignFromTeam(session, session.CreatedByID(), ""))
	assert.Empty(t, a.Events())
}

func TestAttendee_LeavingSpotFreesTeamSlot(t *testing.T) {
	for _, newStatus := range []AttendeeStatus{AttendeeStatusNotGoing, AttendeeStatusMaybe} {
		t.Run(string(newStatus), func(t *testing.T) {
			session := newTestTeamSession(t, newTestTeamConfig(t, nil))
			a := newGoingAttendee(t, session)
			require.NoError(t, a.AssignToTeam(session, session.Teams()[0].ID(), session.CreatedByID(), "", 0))
			a.ClearEvents()

			require.NoError(t, a.UpdateRSVP(newStatus, session, 1))

			assert.Nil(t, a.TeamID())
			assert.Contains(t, attendeeEventTypes(a), EventActivitySessionAttendeeTeamUnassigned)
		})
	}
}

func TestAttendee_RemoveFreesTeamSlot(t *testing.T) {
	session := newTestTeamSession(t, newTestTeamConfig(t, nil))
	a := newGoingAttendee(t, session)
	require.NoError(t, a.AssignToTeam(session, session.Teams()[0].ID(), session.CreatedByID(), "", 0))
	a.ClearEvents()

	require.NoError(t, a.Remove(session, session.CreatedByID(), ""))

	assert.Nil(t, a.TeamID())
	assert.Equal(t,
		[]string{EventActivitySessionAttendeeRemoved, EventActivitySessionAttendeeTeamUnassigned},
		attendeeEventTypes(a),
	)
	unassigned := a.Events()[1].(*AttendeeTeamUnassignedEvent)
	assert.Equal(t, TeamUnassignReasonRemoved, unassigned.Reason)
}

func TestAttendee_LeaveTeam(t *testing.T) {
	session := newTestTeamSession(t, newTestTeamConfig(t, nil))
	a := newGoingAttendee(t, session)

	// not on a team: no event
	a.LeaveTeam()
	assert.Empty(t, a.Events())

	require.NoError(t, a.AssignToTeam(session, session.Teams()[0].ID(), session.CreatedByID(), "", 0))
	a.ClearEvents()

	a.LeaveTeam()

	assert.Nil(t, a.TeamID())
	assert.Equal(t, []string{EventActivitySessionAttendeeTeamUnassigned}, attendeeEventTypes(a))
}
