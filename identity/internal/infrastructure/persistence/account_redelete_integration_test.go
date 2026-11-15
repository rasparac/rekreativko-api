//go:build integration

package persistence_test

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/rasparac/rekreativko-api/identity/internal/application"
	"github.com/rasparac/rekreativko-api/identity/internal/domain"
	"github.com/rasparac/rekreativko-api/identity/internal/infrastructure/persistence"
	"github.com/rasparac/rekreativko-api/shared/domainevent"
	testutil "github.com/rasparac/rekreativko-api/shared/testing"
	"github.com/rasparac/rekreativko-api/shared/token"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// rekreativko-api-5s3: an account deleted after a backup's time is active
// again after the restore; deleting it again keeps the user's request time.
func TestRedoAccountDeletion_DeletesAsOfTheOriginalRequest(t *testing.T) {
	env := setupErasureTest(t)
	const grace = 30 * 24 * time.Hour

	txManager := env.db.CreateTransactionManager()
	logger := testutil.CreateLogger()
	svc := application.NewService(
		env.accounts,
		persistence.NewVerificationCodeManager(env.db.Pool, txManager, logger),
		persistence.NewRefreshTokenManager(env.db.Pool, txManager, logger),
		txManager, domainevent.MustNewEventWriter(txManager, logger, "identity"),
		token.NewGenerator([]byte("test-secret-test-secret-test-secret"), time.Minute, time.Hour),
		logger, token.NewVerificationCodeGenerator(6), env.hasher, testMetrics(),
		false, true, grace,
	)

	restored := env.activeAccount(t, "restored@example.com", "+38591000001")
	asked := time.Now().UTC().Add(-3 * 24 * time.Hour).Truncate(time.Microsecond)

	require.NoError(t, svc.RedoAccountDeletion(env.ctx, restored, asked))

	t.Run("deleted like DELETE /me, its erasure still due grace after the request", func(t *testing.T) {
		stored := env.stored(t, restored)
		assert.Equal(t, string(domain.AccountStatusDeleted), stored.status)
		require.NotNil(t, stored.eraseAfter)
		assert.True(t, asked.Add(grace).Equal(*stored.eraseAfter), "erase_after %s", stored.eraseAfter)
		assert.Nil(t, stored.erased)
		assert.NotNil(t, stored.email, "the data is kept until the erase job runs")

		var deletedAt time.Time
		var liveTokens int
		require.NoError(t, env.db.Pool.QueryRow(env.ctx, `
			SELECT deleted_at,
				(SELECT COUNT(*) FROM identity.refresh_tokens WHERE account_id = a.id AND revoked_at IS NULL)
			FROM identity.accounts a WHERE id = $1`, restored,
		).Scan(&deletedAt, &liveTokens))
		assert.True(t, asked.Equal(deletedAt), "deleted_at %s", deletedAt)
		assert.Zero(t, liveTokens, "every session is revoked")

		var deletedEvents int
		require.NoError(t, env.db.Pool.QueryRow(env.ctx, `SELECT COUNT(*) FROM identity.event_outbox
			WHERE event_type = $1 AND aggregate_id = $2`, domain.IdentityAccountDeletedEvent, restored,
		).Scan(&deletedEvents))
		assert.Equal(t, 1, deletedEvents)
	})

	t.Run("running it again changes nothing", func(t *testing.T) {
		assert.ErrorIs(t, svc.RedoAccountDeletion(env.ctx, restored, time.Now().UTC()), domain.ErrAccountDeleted)
		assert.True(t, asked.Add(grace).Equal(*env.stored(t, restored).eraseAfter))
	})

	t.Run("an unknown account is not found", func(t *testing.T) {
		assert.ErrorIs(t, svc.RedoAccountDeletion(env.ctx, uuid.New(), asked), domain.ErrAccountNotFound)
	})
}
