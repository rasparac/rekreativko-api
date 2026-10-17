//go:build integration

package persistence_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/rasparac/rekreativko-api/identity/internal/application"
	"github.com/rasparac/rekreativko-api/identity/internal/domain"
	"github.com/rasparac/rekreativko-api/identity/internal/infrastructure/persistence"
	"github.com/rasparac/rekreativko-api/identity/internal/metrics"
	"github.com/rasparac/rekreativko-api/shared/domainerror"
	"github.com/rasparac/rekreativko-api/shared/domainevent"
	testutil "github.com/rasparac/rekreativko-api/shared/testing"
	"github.com/rasparac/rekreativko-api/shared/token"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"
)

var (
	sharedTestMetricsOnce sync.Once
	sharedTestMetrics     *metrics.Metrics
)

// metrics.New registers with the default registry, which panics when done twice.
func testMetrics() *metrics.Metrics {
	sharedTestMetricsOnce.Do(func() {
		sharedTestMetrics = metrics.New()
	})
	return sharedTestMetrics
}

const correctPassword = "Correct-Horse-1"

type loginTestEnv struct {
	ctx       context.Context
	db        *testutil.TestDatabase
	svc       *application.Service
	email     string
	accountID uuid.UUID
}

// setupLoginTest builds the real Service on a real database with one active account.
func setupLoginTest(t *testing.T) *loginTestEnv {
	t.Helper()

	db := testutil.NewTestDatabase(t)
	db.RunMigrationsForService()

	txManager := db.CreateTransactionManager()
	logger := testutil.CreateLogger()
	hasher := token.NewPasswordHasher(bcrypt.MinCost)

	accountRepo := persistence.NewAccountManager(db.Pool, txManager, logger)
	svc := application.NewService(
		accountRepo,
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
	)

	hash, err := hasher.Hash(correctPassword)
	require.NoError(t, err)

	const email = "player@example.com"
	emailVO, err := domain.NewEmail(email)
	require.NoError(t, err)
	phoneVO, err := domain.NewPhoneNumber("")
	require.NoError(t, err)

	now := time.Now().UTC()
	account := domain.ReconstructAccount(
		uuid.New(), emailVO, phoneVO, domain.NewPasswordFromHash(hash),
		domain.AccountStatusActive, 0, nil, now, now, nil,
	)
	require.NoError(t, accountRepo.CreateAccount(context.Background(), account))

	return &loginTestEnv{ctx: context.Background(), db: db, svc: svc, email: email, accountID: account.ID()}
}

func (e *loginTestEnv) login(password string) error {
	_, err := e.svc.Login(e.ctx, application.LoginParams{Email: e.email, Password: password})
	return err
}

// state is what is stored for the account: the failed-attempt counter and the lock.
func (e *loginTestEnv) state(t *testing.T) (attempts int, lockedUntil *time.Time) {
	t.Helper()

	err := e.db.Pool.QueryRow(e.ctx,
		`SELECT failed_login_attempts, locked_until FROM identity.accounts WHERE id = $1`, e.accountID,
	).Scan(&attempts, &lockedUntil)
	require.NoError(t, err)
	return attempts, lockedUntil
}

func (e *loginTestEnv) outboxCount(t *testing.T, eventType string) int {
	t.Helper()

	var count int
	err := e.db.Pool.QueryRow(e.ctx,
		`SELECT COUNT(*) FROM identity.event_outbox WHERE event_type = $1 AND aggregate_id = $2`, eventType, e.accountID,
	).Scan(&count)
	require.NoError(t, err)
	return count
}

func requireCode(t *testing.T, err error, code string) {
	t.Helper()

	var appErr *domainerror.AppError
	require.True(t, errors.As(err, &appErr), "expected an AppError, got %v", err)
	assert.Equal(t, code, appErr.Code)
}

// A wrong password has to be remembered: the counter, the login.failed events
// and, at 5 attempts, the lock all used to be rolled back along with the
// failed login, so an account could never be locked.
func TestLogin_FiveWrongPasswordsLockTheAccount(t *testing.T) {
	env := setupLoginTest(t)

	for attempt := 1; attempt <= 5; attempt++ {
		requireCode(t, env.login("wrong-password"), "identity_invalid_credentials")

		attempts, lockedUntil := env.state(t)
		assert.Equal(t, attempt, attempts, "the counter is saved after every wrong password")
		if attempt < 5 {
			assert.Nil(t, lockedUntil)
		}
	}

	_, lockedUntil := env.state(t)
	require.NotNil(t, lockedUntil, "the 5th wrong password locks the account")
	assert.WithinDuration(t, time.Now().Add(15*time.Minute), *lockedUntil, time.Minute)

	assert.Equal(t, 5, env.outboxCount(t, domain.IdentityAccountLoginFailedEvent))
	assert.Equal(t, 1, env.outboxCount(t, domain.IdentityAccountLockedEvent))

	// Locked means locked: even the right password is refused now.
	requireCode(t, env.login(correctPassword), "identity_account_locked")
	assert.Zero(t, env.outboxCount(t, domain.IdentityAccountLoginSucceededEvent))
}

func TestLogin_SuccessResetsTheCounter(t *testing.T) {
	env := setupLoginTest(t)

	for range 3 {
		requireCode(t, env.login("wrong-password"), "identity_invalid_credentials")
	}
	attempts, _ := env.state(t)
	require.Equal(t, 3, attempts)

	require.NoError(t, env.login(correctPassword))

	attempts, lockedUntil := env.state(t)
	assert.Zero(t, attempts)
	assert.Nil(t, lockedUntil)
	assert.Equal(t, 1, env.outboxCount(t, domain.IdentityAccountLoginSucceededEvent))
}

func TestLogin_AnExpiredLockNoLongerBlocksLogin(t *testing.T) {
	env := setupLoginTest(t)

	_, err := env.db.Pool.Exec(env.ctx,
		`UPDATE identity.accounts SET failed_login_attempts = 5, locked_until = now() - interval '1 minute' WHERE id = $1`, env.accountID)
	require.NoError(t, err)

	require.NoError(t, env.login(correctPassword))

	attempts, lockedUntil := env.state(t)
	assert.Zero(t, attempts)
	assert.Nil(t, lockedUntil)
}

// Parallel wrong passwords must all count (no lost updates) and, once the
// account is locked, must not push it towards the 10-attempt, year-long lock.
func TestLogin_ParallelWrongPasswordsAreAllCounted(t *testing.T) {
	t.Run("below the threshold every one counts", func(t *testing.T) {
		env := setupLoginTest(t)

		var wg sync.WaitGroup
		for range 4 {
			wg.Go(func() { _ = env.login("wrong-password") })
		}
		wg.Wait()

		attempts, lockedUntil := env.state(t)
		assert.Equal(t, 4, attempts)
		assert.Nil(t, lockedUntil)
		assert.Equal(t, 4, env.outboxCount(t, domain.IdentityAccountLoginFailedEvent))
	})

	t.Run("past the threshold it stays one 15-minute lock", func(t *testing.T) {
		env := setupLoginTest(t)

		var wg sync.WaitGroup
		for range 12 {
			wg.Go(func() { _ = env.login("wrong-password") })
		}
		wg.Wait()

		attempts, lockedUntil := env.state(t)
		assert.Equal(t, 5, attempts, "attempts after the lock are not counted")
		require.NotNil(t, lockedUntil)
		assert.WithinDuration(t, time.Now().Add(15*time.Minute), *lockedUntil, time.Minute)
		assert.Equal(t, 1, env.outboxCount(t, domain.IdentityAccountLockedEvent))
	})
}

// A lock that has run out is followed by a longer one on the next wrong
// password, never by a permanent one.
func TestLogin_LocksBackOffAfterAnExpiredLock(t *testing.T) {
	env := setupLoginTest(t)

	for _, tc := range []struct {
		attemptsBefore int
		wantLock       time.Duration
	}{
		{attemptsBefore: 4, wantLock: 15 * time.Minute},
		{attemptsBefore: 9, wantLock: time.Hour},
		{attemptsBefore: 14, wantLock: 24 * time.Hour},
		{attemptsBefore: 15, wantLock: 24 * time.Hour}, // the count stops here, so does the back-off
	} {
		_, err := env.db.Pool.Exec(env.ctx,
			`UPDATE identity.accounts SET failed_login_attempts = $2, locked_until = now() - interval '1 minute' WHERE id = $1`,
			env.accountID, tc.attemptsBefore)
		require.NoError(t, err)

		requireCode(t, env.login("wrong-password"), "identity_invalid_credentials")

		attempts, lockedUntil := env.state(t)
		assert.Equal(t, min(tc.attemptsBefore+1, 15), attempts)
		require.NotNil(t, lockedUntil, "after %d failures", tc.attemptsBefore+1)
		assert.WithinDuration(t, time.Now().Add(tc.wantLock), *lockedUntil, time.Minute, "after %d failures", tc.attemptsBefore+1)
	}
}
