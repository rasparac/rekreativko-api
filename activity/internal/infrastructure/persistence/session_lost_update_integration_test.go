//go:build integration

package persistence_test

import (
	"context"
	"testing"
	"time"

	"github.com/rasparac/rekreativko-api/activity/internal/application"
	"github.com/rasparac/rekreativko-api/activity/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Starting, updating or changing the visibility of a session writes the whole
// row back. One that waits while a cancel commits must not put the status it
// read before back: the session stays cancelled.
func TestSession_ACommandWaitingWhileTheSessionIsCancelledKeepsItCancelled(t *testing.T) {
	commands := map[string]func(env *sessionTeamTestEnv, session *domain.Session) error{
		"start": func(env *sessionTeamTestEnv, session *domain.Session) error {
			return env.sessionSvc.StartSession(env.ctx, session.ID(), session.CreatedByID(), "")
		},
		"update": func(env *sessionTeamTestEnv, session *domain.Session) error {
			return env.sessionSvc.UpdateSession(env.ctx, session.ID(), application.UpdateSessionParams{
				RequesterID:     session.CreatedByID(),
				LocationCity:    "Novi Sad",
				LocationCountry: "RS",
				LocationLat:     45.25,
				LocationLng:     19.84,
				StartTime:       time.Now().Add(48 * time.Hour),
				Visibility:      string(domain.SessionVisibilityPublic),
			})
		},
		"set visibility": func(env *sessionTeamTestEnv, session *domain.Session) error {
			return env.sessionSvc.SetSessionVisibility(env.ctx, session.ID(), session.CreatedByID(), "",
				string(domain.SessionVisibilityPrivate))
		},
	}

	for name, command := range commands {
		t.Run(name, func(t *testing.T) {
			env := setupSessionTeamTest(t)
			session := env.createSession(t, domain.ActivityTypeBasketball)

			lockHeld := make(chan struct{})
			release := make(chan struct{})
			holderDone := make(chan error, 1)

			// Cancels the session the way CancelSession does - under the
			// session lock - and only commits once the command is waiting.
			go func() {
				holderDone <- env.txManager.WithTransaction(env.ctx, func(tCtx context.Context) error {
					q := env.txManager.Querier(tCtx)
					if _, err := q.Exec(tCtx, "SELECT pg_advisory_xact_lock($1, hashtext($2))", sessionCapacityLockNamespace, session.ID().String()); err != nil {
						return err
					}
					if _, err := q.Exec(tCtx, "UPDATE activity.session SET status = $1, cancelled_at = NOW() WHERE id = $2", string(domain.SessionStatusCanceled), session.ID()); err != nil {
						return err
					}

					close(lockHeld)
					<-release

					return nil
				})
			}()
			<-lockHeld

			commandErr := make(chan error, 1)
			go func() {
				commandErr <- command(env, session)
			}()

			// Waiting on the session lock (or, without it, on the row lock).
			require.Eventually(t, func() bool {
				var waiting int
				err := env.txManager.Querier(env.ctx).QueryRow(env.ctx,
					"SELECT count(*) FROM pg_locks WHERE NOT granted").Scan(&waiting)
				return err == nil && waiting > 0
			}, 10*time.Second, 20*time.Millisecond, "the command never started waiting")

			close(release)
			require.NoError(t, <-holderDone)

			select {
			case <-commandErr:
				// refused or not, what matters is the status it leaves behind
			case <-time.After(10 * time.Second):
				t.Fatal("the command did not finish after the cancel committed")
			}

			assert.Equal(t, domain.SessionStatusCanceled, env.sessionStatus(t, session))
		})
	}
}
