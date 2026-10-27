//go:build integration

package persistence_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/rasparac/rekreativko-api/activity/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// sessionCapacityLockNamespace mirrors the unexported constant in the
// application package (attendee_service.go).
const sessionCapacityLockNamespace = 78233

// A proposal that waits for the session lock must act on the session as it is
// once the lock is theirs: the transaction that held the lock may have called
// the session off in the meantime.
func TestTeamVoting_ProposeWaitingOnTheLockSeesTheSessionCancelledMeanwhile(t *testing.T) {
	env := setupSessionTeamTest(t)
	session, u := env.draftSession(t, 4)

	lockHeld := make(chan struct{})
	release := make(chan struct{})
	holderDone := make(chan error, 1)

	// Holds the session lock, cancels the session and only then commits.
	go func() {
		holderDone <- env.txManager.WithTransaction(env.ctx, func(tCtx context.Context) error {
			q := env.txManager.Querier(tCtx)
			if _, err := q.Exec(tCtx, "SELECT pg_advisory_xact_lock($1, hashtext($2))", sessionCapacityLockNamespace, session.ID().String()); err != nil {
				return err
			}
			if _, err := q.Exec(tCtx, "UPDATE activity.session SET status = $1 WHERE id = $2", string(domain.SessionStatusCanceled), session.ID()); err != nil {
				return err
			}

			close(lockHeld)
			<-release

			return nil
		})
	}()
	<-lockHeld

	proposeErr := make(chan error, 1)
	go func() {
		_, err := env.propose(session, u[0], []uuid.UUID{u[0], u[1]}, []uuid.UUID{u[2], u[3]})
		proposeErr <- err
	}()

	// Propose has loaded the session (still scheduled for everyone else) and is
	// blocked on the lock.
	require.Eventually(t, func() bool {
		var waiting int
		err := env.txManager.Querier(env.ctx).QueryRow(env.ctx,
			"SELECT count(*) FROM pg_locks WHERE locktype = 'advisory' AND NOT granted").Scan(&waiting)
		return err == nil && waiting > 0
	}, 10*time.Second, 20*time.Millisecond, "Propose never started waiting for the session lock")

	close(release)
	require.NoError(t, <-holderDone)

	select {
	case err := <-proposeErr:
		assert.Error(t, err, "the session was cancelled before Propose got the lock, so it must be refused")
	case <-time.After(10 * time.Second):
		t.Fatal("Propose did not finish after the lock was released")
	}

	got := env.voting(t, session)
	assert.True(t, got.Round == nil || !got.Round.IsOpen(), "no voting round may be opened on a cancelled session")
}
