//go:build integration

package persistence_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/rasparac/rekreativko-api/activity/internal/domain"
	"github.com/rasparac/rekreativko-api/activity/internal/interfaces/http/mapper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTeamFormationQuery_NothingYet(t *testing.T) {
	env := setupSessionTeamTest(t)
	session := env.createSession(t, domain.ActivityTypeBasketball)

	snapshot, err := env.formation.Snapshot(env.ctx, session.ID())
	require.NoError(t, err)

	resp := mapper.TeamFormationSnapshotToResponse(snapshot, uuid.New())
	assert.Equal(t, session.ID(), resp.SessionID)
	assert.Positive(t, resp.Version)
	assert.Nil(t, resp.TeamConfig)
	assert.Empty(t, resp.Teams)
	assert.NotNil(t, resp.Teams, "teams is [] not null")
	assert.Nil(t, resp.Draft)
	require.NotNil(t, resp.Voting)
	assert.Equal(t, "none", resp.Voting.VotingStatus)
}

func TestTeamFormationQuery_TeamsAndRunningDraft(t *testing.T) {
	env := setupSessionTeamTest(t)
	session, u := env.draftSession(t, 4)

	withTeams, err := env.createTeams(session, nil, nil, nil)
	require.NoError(t, err)
	teamA := withTeams.Teams()[0].ID()
	require.NoError(t, env.assign(session, u[2], teamA))

	_, err = env.startDraft(session, u[0], u[1], nil)
	require.NoError(t, err)
	_, err = env.pick(session, u[0], u[3])
	require.NoError(t, err)

	first, err := env.formation.Snapshot(env.ctx, session.ID())
	require.NoError(t, err)
	resp := mapper.TeamFormationSnapshotToResponse(first, u[0])

	require.NotNil(t, resp.TeamConfig)
	assert.Equal(t, 2, resp.TeamConfig.TeamCount)
	require.Len(t, resp.Teams, 2)
	assert.Equal(t, []uuid.UUID{u[2]}, resp.Teams[0].MemberUserIDs, "the draft has not replaced the teams yet")

	require.NotNil(t, resp.Draft)
	assert.Equal(t, "active", resp.Draft.Status)
	assert.Contains(t, resp.Draft.Teams[0].UserIDs, u[3])
	assert.Equal(t, "none", resp.Voting.VotingStatus)

	second, err := env.formation.Snapshot(env.ctx, session.ID())
	require.NoError(t, err)
	assert.Greater(t, second.Version, first.Version, "a later snapshot has a higher version")
}

func TestTeamFormationQuery_VotingIsPersonalisedPerViewer(t *testing.T) {
	env := setupSessionTeamTest(t)
	session, u := env.draftSession(t, 4)

	state, err := env.propose(session, u[0], []uuid.UUID{u[0], u[1]}, []uuid.UUID{u[2], u[3]})
	require.NoError(t, err)
	proposalID := state.Round.Proposals()[0].ID()
	require.NoError(t, env.vote(session, u[1], proposalID))

	snapshot, err := env.formation.Snapshot(env.ctx, session.ID())
	require.NoError(t, err)

	voter := mapper.TeamFormationSnapshotToResponse(snapshot, u[1])
	other := mapper.TeamFormationSnapshotToResponse(snapshot, u[2])

	assert.Equal(t, "open", voter.Voting.VotingStatus)
	require.NotNil(t, voter.Voting.MyVote)
	assert.Nil(t, other.Voting.MyVote, "my_vote is the viewer's own vote")
	assert.Equal(t, voter.Version, other.Version, "one snapshot, personalised per viewer")
}

func TestTeamFormationQuery_UnknownSession(t *testing.T) {
	env := setupSessionTeamTest(t)

	_, err := env.formation.Snapshot(env.ctx, uuid.New())
	requireAppErrorCode(t, err, "session_not_found")
}

// Only people going are in the snapshot; the waitlist, maybes and decliners
// are not.
func TestTeamFormationSnapshot_ListsOnlyThoseGoing(t *testing.T) {
	env := setupSessionTeamTest(t)
	session := env.createSession(t, domain.ActivityTypeBasketball)

	going := env.addGoingAttendee(t, session)
	promoted := uuid.New()
	for status, userID := range map[domain.AttendeeStatus]uuid.UUID{
		domain.AttendeeStatusPromoted: promoted,
		domain.AttendeeStatusPending:  uuid.New(),
		domain.AttendeeStatusMaybe:    uuid.New(),
		domain.AttendeeStatusNotGoing: uuid.New(),
	} {
		attendee, err := domain.NewRSVPManualAttendee(session, session.ActivityGroupID(), userID, status, 0)
		require.NoError(t, err)
		require.NoError(t, env.attendeeRepo.CreateAttendee(env.ctx, attendee))
	}

	snapshot, err := env.formation.Snapshot(env.ctx, session.ID())
	require.NoError(t, err)

	assert.ElementsMatch(t, []uuid.UUID{going, promoted}, snapshot.GoingUserIDs)
}
