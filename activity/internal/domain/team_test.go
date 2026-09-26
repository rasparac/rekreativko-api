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

func newTestSessionOfType(t *testing.T, activityType ActivityType) *Session {
	t.Helper()

	title, err := NewTitle("Pickup game")
	require.NoError(t, err)

	location, err := NewSessionLocation("Belgrade", "RS", "", 44.8, 20.4)
	require.NoError(t, err)

	schedule, err := NewSessionSchedule(time.Now().Add(time.Hour), nil)
	require.NoError(t, err)

	visibility := SessionVisibilityPublic
	session, _, err := NewSession(SessionInput{
		CreatedByID:     uuid.New(),
		Title:           title,
		ActivityType:    activityType,
		DifficultyLevel: DifficultyLevelBeginner,
		Location:        location,
		Schedule:        schedule,
		Visibility:      &visibility,
	})
	require.NoError(t, err)

	return session
}

// newTestTeamSession creates a basketball session and, when config is set,
// splits it into teams the way the creator would after people joined.
func newTestTeamSession(t *testing.T, config *TeamConfig) *Session {
	t.Helper()

	session := newTestSessionOfType(t, ActivityTypeBasketball)
	if config != nil {
		require.NoError(t, session.CreateTeams(session.CreatedByID(), "", *config))
		session.ClearEvents()
	}

	return session
}

func newTestTeamConfig(t *testing.T, minPlayersPerTeam *int) *TeamConfig {
	t.Helper()

	config, err := NewTeamConfig(nil, minPlayersPerTeam, nil)
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
		name              string
		teamCount         *int
		minPlayersPerTeam *int
		colors            []string
		wantErr           error
		wantCount         int
		wantColors        []string
	}{
		{name: "defaults to two teams", wantCount: 2},
		{name: "explicit count", teamCount: intPtr(4), wantCount: 4},
		{name: "count below two", teamCount: intPtr(1), wantErr: ErrInvalidTeamCount},
		{name: "count above max", teamCount: intPtr(MaxTeamCount + 1), wantErr: ErrInvalidTeamCount},
		{name: "optional minimum per team", minPlayersPerTeam: intPtr(5), wantCount: 2},
		{name: "minimum zero", minPlayersPerTeam: intPtr(0), wantErr: ErrInvalidMinPlayersPerTeam},
		{name: "colors one per team, normalized", colors: []string{"#ff0000", "#0000FF"}, wantCount: 2, wantColors: []string{"#FF0000", "#0000FF"}},
		{name: "colors count mismatch", colors: []string{"#FF0000"}, wantErr: ErrInvalidTeamColors},
		{name: "color not hex", colors: []string{"red", "#0000FF"}, wantErr: ErrInvalidTeamColors},
		{name: "short hex rejected", colors: []string{"#F00", "#00F"}, wantErr: ErrInvalidTeamColors},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config, err := NewTeamConfig(tt.teamCount, tt.minPlayersPerTeam, tt.colors)
			if tt.wantErr != nil {
				assert.ErrorIs(t, err, tt.wantErr)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.wantCount, config.TeamCount())
			assert.Equal(t, tt.minPlayersPerTeam, config.MinPlayersPerTeam())
			if tt.wantColors != nil {
				assert.Equal(t, tt.wantColors, config.Colors())
			} else {
				assert.Empty(t, config.Colors())
			}
		})
	}
}

func TestNewSession_HasNoTeams(t *testing.T) {
	session := newTestSessionOfType(t, ActivityTypeBasketball)

	assert.False(t, session.HasTeams())
	assert.Nil(t, session.TeamConfig())
	assert.Empty(t, session.Teams())
}

func TestSession_CreateTeams_CreatesNamedTeams(t *testing.T) {
	config, err := NewTeamConfig(intPtr(3), nil, []string{"#FFFFFF", "#000000", "#FF0000"})
	require.NoError(t, err)

	session := newTestSessionOfType(t, ActivityTypeBasketball)
	session.ClearEvents()

	require.NoError(t, session.CreateTeams(session.CreatedByID(), "", config))

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

	require.Len(t, session.Events(), 1)
	created, ok := session.Events()[0].(*SessionTeamsCreatedEvent)
	require.True(t, ok)
	assert.False(t, created.Replaced)
	assert.Len(t, created.Teams, 3)
}

func TestSession_CreateTeams_ReplacesExistingTeams(t *testing.T) {
	session := newTestTeamSession(t, newTestTeamConfig(t, nil))
	oldTeamA := session.Teams()[0].ID()

	config, err := NewTeamConfig(intPtr(3), intPtr(4), nil)
	require.NoError(t, err)
	require.NoError(t, session.CreateTeams(session.CreatedByID(), "", config))

	require.Len(t, session.Teams(), 3)
	_, stillThere := session.Team(oldTeamA)
	assert.False(t, stillThere, "old teams must be gone")
	assert.Equal(t, intPtr(4), session.TeamConfig().MinPlayersPerTeam())

	created := session.Events()[0].(*SessionTeamsCreatedEvent)
	assert.True(t, created.Replaced)
}

func TestSession_CreateTeams_Rejections(t *testing.T) {
	config := *newTestTeamConfig(t, nil)

	t.Run("not a team sport", func(t *testing.T) {
		session := newTestSessionOfType(t, ActivityTypeRunning)
		err := session.CreateTeams(session.CreatedByID(), "", config)
		assert.ErrorIs(t, err, ErrTeamsNotSupported)
		assert.False(t, session.HasTeams())
	})

	t.Run("regular member", func(t *testing.T) {
		session := newTestSessionOfType(t, ActivityTypeFootball)
		err := session.CreateTeams(uuid.New(), MemberRoleMember, config)
		assert.ErrorIs(t, err, ErrUnauthorized)
	})

	t.Run("group admin allowed", func(t *testing.T) {
		session := newTestSessionOfType(t, ActivityTypeVolleyball)
		assert.NoError(t, session.CreateTeams(uuid.New(), MemberRoleAdmin, config))
	})

	t.Run("canceled session", func(t *testing.T) {
		session := newTestSessionOfType(t, ActivityTypeBasketball)
		require.NoError(t, session.Cancel(session.CreatedByID(), "", "rain", nil))
		err := session.CreateTeams(session.CreatedByID(), "", config)
		assert.ErrorIs(t, err, ErrSessionCanceled)
	})

	t.Run("started session allowed", func(t *testing.T) {
		session := newTestSessionOfType(t, ActivityTypeBasketball)
		require.NoError(t, session.Start(session.CreatedByID(), ""))
		assert.NoError(t, session.CreateTeams(session.CreatedByID(), "", config))
	})
}

func TestAttendee_UnassignForReplacedTeams(t *testing.T) {
	session := newTestTeamSession(t, newTestTeamConfig(t, nil))
	a := newGoingAttendee(t, session)
	require.NoError(t, a.AssignToTeam(session, session.Teams()[0].ID(), session.CreatedByID(), ""))
	a.ClearEvents()

	a.UnassignForReplacedTeams(session.CreatedByID())

	assert.Nil(t, a.TeamID())
	require.Len(t, a.Events(), 1)
	assert.Equal(t, TeamUnassignReasonTeamsReplaced, a.Events()[0].(*AttendeeTeamUnassignedEvent).Reason)
}

func TestAttendee_AssignToTeam(t *testing.T) {
	session := newTestTeamSession(t, newTestTeamConfig(t, intPtr(2)))
	teamA := session.Teams()[0].ID()
	teamB := session.Teams()[1].ID()
	creator := session.CreatedByID()

	t.Run("assigns unassigned attendee and emits team_assigned", func(t *testing.T) {
		a := newGoingAttendee(t, session)

		require.NoError(t, a.AssignToTeam(session, teamA, creator, ""))

		require.NotNil(t, a.TeamID())
		assert.Equal(t, teamA, *a.TeamID())
		assert.Equal(t, []string{EventActivitySessionAttendeeTeamAssigned}, attendeeEventTypes(a))
	})

	t.Run("moving to another team emits team_changed", func(t *testing.T) {
		a := newGoingAttendee(t, session)
		require.NoError(t, a.AssignToTeam(session, teamA, creator, ""))
		a.ClearEvents()

		require.NoError(t, a.AssignToTeam(session, teamB, creator, ""))

		assert.Equal(t, teamB, *a.TeamID())
		require.Len(t, a.Events(), 1)
		changed, ok := a.Events()[0].(*AttendeeTeamChangedEvent)
		require.True(t, ok)
		assert.Equal(t, teamA, changed.PreviousTeamID)
		assert.Equal(t, teamB, changed.TeamID)
	})

	t.Run("same team is a no-op", func(t *testing.T) {
		a := newGoingAttendee(t, session)
		require.NoError(t, a.AssignToTeam(session, teamA, creator, ""))
		a.ClearEvents()

		require.NoError(t, a.AssignToTeam(session, teamA, creator, ""))
		assert.Empty(t, a.Events())
	})

	t.Run("group admin may assign", func(t *testing.T) {
		a := newGoingAttendee(t, session)
		assert.NoError(t, a.AssignToTeam(session, teamA, uuid.New(), MemberRoleAdmin))
	})

	t.Run("regular member rejected", func(t *testing.T) {
		a := newGoingAttendee(t, session)
		err := a.AssignToTeam(session, teamA, uuid.New(), MemberRoleMember)
		assert.ErrorIs(t, err, ErrUnauthorized)
	})

	t.Run("unknown team rejected", func(t *testing.T) {
		a := newGoingAttendee(t, session)
		err := a.AssignToTeam(session, uuid.New(), creator, "")
		assert.ErrorIs(t, err, ErrTeamNotFound)
	})

	t.Run("unconfirmed attendee rejected", func(t *testing.T) {
		a, err := NewRSVPManualAttendee(session, nil, uuid.New(), AttendeeStatusMaybe, 0)
		require.NoError(t, err)

		err = a.AssignToTeam(session, teamA, creator, "")
		assert.ErrorIs(t, err, ErrAttendeeNotGoing)
	})
}

// D1: the minimum is never a maximum - a team takes any number of players.
func TestAttendee_AssignToTeam_MinimumIsNotALimit(t *testing.T) {
	session := newTestTeamSession(t, newTestTeamConfig(t, intPtr(2)))
	teamA := session.Teams()[0].ID()

	for range 5 {
		a := newGoingAttendee(t, session)
		require.NoError(t, a.AssignToTeam(session, teamA, session.CreatedByID(), ""))
	}
}

func TestTeamConfig_PlayersNeeded(t *testing.T) {
	noMin, err := NewTeamConfig(intPtr(3), nil, nil)
	require.NoError(t, err)
	assert.Equal(t, 3, noMin.PlayersNeeded(), "one per team without a minimum")

	min5, err := NewTeamConfig(nil, intPtr(5), nil)
	require.NoError(t, err)
	assert.Equal(t, 10, min5.PlayersNeeded(), "2 teams x 5")
	assert.False(t, min5.HasEnoughPlayers(9))
	assert.True(t, min5.HasEnoughPlayers(10))
	assert.True(t, min5.HasEnoughPlayers(12))
}

func TestAttendee_AssignToTeam_SessionWithoutTeams(t *testing.T) {
	session := newTestTeamSession(t, nil)
	a := newGoingAttendee(t, session)

	err := a.AssignToTeam(session, uuid.New(), session.CreatedByID(), "")

	assert.ErrorIs(t, err, ErrSessionHasNoTeams)
}

func TestAttendee_AssignToTeam_TerminalSession(t *testing.T) {
	session := newTestTeamSession(t, newTestTeamConfig(t, nil))
	a := newGoingAttendee(t, session)
	require.NoError(t, session.Cancel(session.CreatedByID(), "", "rain", nil))

	err := a.AssignToTeam(session, session.Teams()[0].ID(), session.CreatedByID(), "")

	assert.ErrorIs(t, err, ErrSessionCanceled)
}

func TestAttendee_UnassignFromTeam(t *testing.T) {
	session := newTestTeamSession(t, newTestTeamConfig(t, nil))
	a := newGoingAttendee(t, session)
	teamA := session.Teams()[0].ID()
	require.NoError(t, a.AssignToTeam(session, teamA, session.CreatedByID(), ""))
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
			require.NoError(t, a.AssignToTeam(session, session.Teams()[0].ID(), session.CreatedByID(), ""))
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
	require.NoError(t, a.AssignToTeam(session, session.Teams()[0].ID(), session.CreatedByID(), ""))
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

	require.NoError(t, a.AssignToTeam(session, session.Teams()[0].ID(), session.CreatedByID(), ""))
	a.ClearEvents()

	a.LeaveTeam()

	assert.Nil(t, a.TeamID())
	assert.Equal(t, []string{EventActivitySessionAttendeeTeamUnassigned}, attendeeEventTypes(a))
}
