//go:build integration

package persistence_test

import (
	"context"
	"testing"
	"time"

	"github.com/rasparac/rekreativko-api/identity/internal/application"
	"github.com/rasparac/rekreativko-api/identity/internal/infrastructure/persistence"
	"github.com/rasparac/rekreativko-api/shared/domainevent"
	testutil "github.com/rasparac/rekreativko-api/shared/testing"
	"github.com/rasparac/rekreativko-api/shared/token"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"
)

// With ACCOUNT_VERIFICATION_REQUIRED on, a new account is pending and gets a
// code; off, it is active at once, has no code, can log in, and its verified
// event says the address was never confirmed.
func TestRegister_AccountVerificationRequired(t *testing.T) {
	for name, required := range map[string]bool{"required": true, "switched off": false} {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			db := testutil.NewTestDatabase(t)
			db.RunMigrationsForService()

			txManager := db.CreateTransactionManager()
			logger := testutil.CreateLogger()
			hasher := token.NewPasswordHasher(bcrypt.MinCost)

			svc := application.NewService(
				persistence.NewAccountManager(db.Pool, txManager, logger),
				persistence.NewVerificationCodeManager(db.Pool, txManager, logger),
				persistence.NewRefreshTokenManager(db.Pool, txManager, logger),
				txManager,
				domainevent.NewDomainEventManager(txManager, logger),
				token.NewGenerator([]byte("test-secret-test-secret-test-secret"), time.Minute, time.Hour),
				logger,
				token.NewVerificationCodeGenerator(6),
				hasher,
				testMetrics(),
				false,
				required,
			)

			const email = "new.player@example.com"
			accountID, err := svc.Register(ctx, application.RegistrationParams{Email: email, Password: correctPassword})
			require.NoError(t, err)

			var status string
			require.NoError(t, db.Pool.QueryRow(ctx,
				`SELECT status FROM identity.accounts WHERE id = $1`, accountID).Scan(&status))

			var codes int
			require.NoError(t, db.Pool.QueryRow(ctx,
				`SELECT COUNT(*) FROM identity.verification_codes WHERE account_id = $1`, accountID).Scan(&codes))

			eventTypes := map[string]int{}
			rows, err := db.Pool.Query(ctx,
				`SELECT event_type FROM identity.event_outbox WHERE aggregate_id = $1`, accountID)
			require.NoError(t, err)
			defer rows.Close()
			for rows.Next() {
				var eventType string
				require.NoError(t, rows.Scan(&eventType))
				eventTypes[eventType]++
			}
			require.NoError(t, rows.Err())

			_, loginErr := svc.Login(ctx, application.LoginParams{Email: email, Password: correctPassword})

			if required {
				assert.Equal(t, "pending", status)
				assert.Equal(t, 1, codes)
				assert.Zero(t, eventTypes["identity.account.verified"], "nothing is verified yet")
				assert.Error(t, loginErr, "a pending account cannot log in")
				return
			}

			assert.Equal(t, "active", status)
			assert.Zero(t, codes, "no code is created")
			assert.Equal(t, 1, eventTypes["identity.account.verified"], "account-profile still builds the profile from it")
			assert.Equal(t, 1, eventTypes["identity.account.activated"])
			assert.NoError(t, loginErr, "an active account logs in right away")

			var unconfirmed bool
			require.NoError(t, db.Pool.QueryRow(ctx,
				`SELECT COALESCE((payload->>'unconfirmed')::boolean, false) FROM identity.event_outbox
				 WHERE aggregate_id = $1 AND event_type = 'identity.account.verified'`, accountID).Scan(&unconfirmed))
			assert.True(t, unconfirmed)
		})
	}
}
