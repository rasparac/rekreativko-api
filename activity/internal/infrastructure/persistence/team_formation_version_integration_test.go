//go:build integration

package persistence_test

import (
	"testing"

	"github.com/rasparac/rekreativko-api/activity/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestTeamFormationSnapshot_EveryChangeRaisesTheVersion walks a session
// through every kind of team-formation write - including resets, which delete
// rows - and checks the snapshot version the database triggers maintain goes
// up each time, so clients never drop a snapshot of newer state as stale.
func TestTeamFormationSnapshot_EveryChangeRaisesTheVersion(t *testing.T) {
	env := setupSessionTeamTest(t)
	session, u := env.draftSession(t, 4)
	creator := session.CreatedByID()

	last := int64(-1)
	requireNewerVersion := func(t *testing.T, step string) {
		t.Helper()

		snapshot, err := env.formation.Snapshot(env.ctx, session.ID())
		require.NoError(t, err, step)
		assert.Greaterf(t, snapshot.Version, last, "after %s", step)
		last = snapshot.Version

		again, err := env.formation.Snapshot(env.ctx, session.ID())
		require.NoError(t, err, step)
		assert.Equalf(t, last, again.Version, "nothing changed since %s", step)
	}

	requireNewerVersion(t, "people joined")

	env.addGoingAttendee(t, session)
	requireNewerVersion(t, "someone else joined")

	withTeams, err := env.createTeams(session, nil, nil, nil)
	require.NoError(t, err)
	requireNewerVersion(t, "teams created")

	require.NoError(t, env.assign(session, u[0], withTeams.Teams()[0].ID()))
	requireNewerVersion(t, "someone assigned")

	require.NoError(t, env.resetTeams(session, creator, ""))
	requireNewerVersion(t, "teams reset (teams deleted)")

	_, err = env.startDraft(session, u[0], u[1], nil)
	require.NoError(t, err)
	requireNewerVersion(t, "draft started")

	_, err = env.pick(session, u[0], u[2])
	require.NoError(t, err)
	requireNewerVersion(t, "player picked")

	require.NoError(t, env.resetTeams(session, creator, ""))
	requireNewerVersion(t, "draft reset (draft and picks deleted)")

	// everyone going, including the one who joined after draftSession
	going, err := env.formation.Snapshot(env.ctx, session.ID())
	require.NoError(t, err)
	half := len(going.GoingUserIDs) / 2
	state, err := env.propose(session, u[0], going.GoingUserIDs[:half], going.GoingUserIDs[half:])
	require.NoError(t, err)
	requireNewerVersion(t, "voting opened")

	require.NoError(t, env.vote(session, u[1], state.Round.Proposals()[0].ID()))
	requireNewerVersion(t, "vote cast")

	_, err = env.closeVoting(session, nil)
	require.NoError(t, err)
	requireNewerVersion(t, "voting closed (proposals and votes deleted, teams replaced)")

	require.NoError(t, env.sessionSvc.CancelSession(env.ctx, session.ID(), creator, "", ""))
	requireNewerVersion(t, "session cancelled")

	assert.Equal(t, domain.SessionStatusCanceled, env.sessionStatus(t, session))
}
