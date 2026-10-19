//go:build integration

package persistence_test

import (
	"context"
	"encoding/json"
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
	"golang.org/x/crypto/bcrypt"
)

type erasureTestEnv struct {
	ctx      context.Context
	db       *testutil.TestDatabase
	svc      *application.Service
	erasure  *application.ErasureService
	erasures *persistence.AccountErasureManager
	accounts *persistence.AccountManager
	hasher   *token.PasswordHasher
}

// setupErasureTest builds the real Service (grace 0: a deleted account is due
// right away) and ErasureService on a real database.
func setupErasureTest(t *testing.T) *erasureTestEnv {
	t.Helper()

	db := testutil.NewTestDatabase(t)
	db.RunMigrationsForService()

	txManager := db.CreateTransactionManager()
	logger := testutil.CreateLogger()
	hasher := token.NewPasswordHasher(bcrypt.MinCost)
	events := domainevent.NewDomainEventManager(txManager, logger)

	accounts := persistence.NewAccountManager(db.Pool, txManager, logger)
	codes := persistence.NewVerificationCodeManager(db.Pool, txManager, logger)
	tokens := persistence.NewRefreshTokenManager(db.Pool, txManager, logger)
	erasures := persistence.NewAccountErasureManager(db.Pool, txManager, logger)

	svc := application.NewService(
		accounts, codes, tokens, txManager, events,
		token.NewGenerator([]byte("test-secret-test-secret-test-secret"), time.Minute, time.Hour),
		logger, token.NewVerificationCodeGenerator(6), hasher, testMetrics(),
		false, true, 0,
	)

	erasure, err := application.NewErasureService(
		accounts, tokens, codes, erasures, txManager, events, logger, testMetrics(),
		application.ErasurePolicy{Batch: 10, Contexts: []string{"activity", "notifications"}, IncompleteAfter: time.Hour},
	)
	require.NoError(t, err)

	return &erasureTestEnv{
		ctx: context.Background(), db: db, svc: svc, erasure: erasure,
		erasures: erasures, accounts: accounts, hasher: hasher,
	}
}

// activeAccount creates an active account with an email, a phone, a
// verification code and a refresh token (from logging in).
func (e *erasureTestEnv) activeAccount(t *testing.T, email, phone string) uuid.UUID {
	t.Helper()

	hash, err := e.hasher.Hash(correctPassword)
	require.NoError(t, err)
	emailVO, err := domain.NewEmail(email)
	require.NoError(t, err)
	phoneVO, err := domain.NewPhoneNumber(phone)
	require.NoError(t, err)

	now := time.Now().UTC()
	account := domain.ReconstructAccount(
		uuid.New(), emailVO, phoneVO, domain.NewPasswordFromHash(hash),
		domain.AccountStatusActive, 0, nil, now, now, nil, nil, nil,
	)
	require.NoError(t, e.accounts.CreateAccount(e.ctx, account))

	e.db.RunSQL(`INSERT INTO identity.verification_codes (account_id, code, type, expires_at)
		VALUES ($1, substr(md5(random()::text), 1, 6), 'email', NOW() + interval '1 hour')`, account.ID())

	_, err = e.svc.Login(e.ctx, application.LoginParams{Email: email, Password: correctPassword})
	require.NoError(t, err)

	return account.ID()
}

type storedAccount struct {
	email, phone       *string
	password, status   string
	eraseAfter, erased *time.Time
	codes, tokens      int
}

func (e *erasureTestEnv) stored(t *testing.T, id uuid.UUID) storedAccount {
	t.Helper()

	var a storedAccount
	require.NoError(t, e.db.Pool.QueryRow(e.ctx, `
		SELECT email, phone_number, password, status, erase_after, erased_at,
			(SELECT COUNT(*) FROM identity.verification_codes WHERE account_id = a.id),
			(SELECT COUNT(*) FROM identity.refresh_tokens WHERE account_id = a.id)
		FROM identity.accounts a WHERE id = $1`, id,
	).Scan(&a.email, &a.phone, &a.password, &a.status, &a.eraseAfter, &a.erased, &a.codes, &a.tokens))
	return a
}

func (e *erasureTestEnv) erasedEvents(t *testing.T, id uuid.UUID) []json.RawMessage {
	t.Helper()

	rows, err := e.db.Pool.Query(e.ctx, `SELECT payload FROM identity.event_outbox
		WHERE event_type = $1 AND aggregate_id = $2`, domain.IdentityAccountErasedEvent, id)
	require.NoError(t, err)
	defer rows.Close()

	var payloads []json.RawMessage
	for rows.Next() {
		var p json.RawMessage
		require.NoError(t, rows.Scan(&p))
		payloads = append(payloads, p)
	}
	require.NoError(t, rows.Err())
	return payloads
}

func TestAccountErasure_DeletedAccountIsErasedOnceItsGracePeriodHasPassed(t *testing.T) {
	e := setupErasureTest(t)

	deleted := e.activeAccount(t, "leaving@example.com", "+38591111111")
	inGrace := e.activeAccount(t, "thinking@example.com", "+38592222222")
	staying := e.activeAccount(t, "staying@example.com", "+38593333333")

	require.NoError(t, e.svc.DeleteAccount(e.ctx, deleted))
	require.NoError(t, e.svc.DeleteAccount(e.ctx, inGrace))
	e.db.RunSQL(`UPDATE identity.accounts SET erase_after = NOW() + interval '30 days' WHERE id = $1`, inGrace)

	before := e.stored(t, deleted)
	assert.Equal(t, "deleted", before.status)
	require.NotNil(t, before.eraseAfter, "deletion schedules the erasure")
	assert.Nil(t, before.erased)
	require.NotNil(t, before.email, "nothing is erased before the job runs")

	e.erasure.RunOnce(e.ctx, time.Now().UTC())

	after := e.stored(t, deleted)
	assert.Nil(t, after.email)
	assert.Nil(t, after.phone)
	assert.Equal(t, domain.ErasedPasswordHash, after.password)
	assert.Equal(t, "deleted", after.status)
	assert.NotNil(t, after.erased)
	assert.Zero(t, after.codes, "verification codes deleted")
	assert.Zero(t, after.tokens, "refresh tokens deleted")

	payloads := e.erasedEvents(t, deleted)
	require.Len(t, payloads, 1, "identity.account.erased in the outbox")
	var payload map[string]any
	require.NoError(t, json.Unmarshal(payloads[0], &payload))
	assert.Equal(t, deleted.String(), payload["account_id"])
	assert.NotContains(t, string(payloads[0]), "leaving@example.com")

	for name, id := range map[string]uuid.UUID{"still in its grace period": inGrace, "not deleted": staying} {
		a := e.stored(t, id)
		assert.NotNil(t, a.email, name)
		assert.Nil(t, a.erased, name)
		assert.NotEqual(t, domain.ErasedPasswordHash, a.password, name)
		assert.Empty(t, e.erasedEvents(t, id), name)
	}

	// A second run finds nothing more to do.
	e.erasure.RunOnce(e.ctx, time.Now().UTC())
	assert.Len(t, e.erasedEvents(t, deleted), 1)

	// The email is free for a new account.
	_, err := e.svc.Register(e.ctx, application.RegistrationParams{Email: "leaving@example.com", Password: correctPassword})
	require.NoError(t, err)
}

func TestAccountErasure_AnAccountAnotherInstanceHoldsIsSkipped(t *testing.T) {
	e := setupErasureTest(t)

	id := e.activeAccount(t, "leaving@example.com", "")
	require.NoError(t, e.svc.DeleteAccount(e.ctx, id))

	tx, err := e.db.Pool.Begin(e.ctx)
	require.NoError(t, err)
	defer func() { _ = tx.Rollback(e.ctx) }()
	_, err = tx.Exec(e.ctx, `SELECT id FROM identity.accounts WHERE id = $1 FOR UPDATE`, id)
	require.NoError(t, err)

	erased, err := e.erasure.EraseDueAccounts(e.ctx, time.Now().UTC())
	require.NoError(t, err)
	assert.Zero(t, erased, "skipped, not waited for")

	require.NoError(t, tx.Rollback(e.ctx))

	erased, err = e.erasure.EraseDueAccounts(e.ctx, time.Now().UTC())
	require.NoError(t, err)
	assert.Equal(t, 1, erased)
}

func TestAccountErasure_TrackingCountsErasuresNotConfirmedByEveryContext(t *testing.T) {
	e := setupErasureTest(t)

	id := e.activeAccount(t, "leaving@example.com", "")
	require.NoError(t, e.svc.DeleteAccount(e.ctx, id))
	e.erasure.RunOnce(e.ctx, time.Now().UTC())

	contexts := []string{"activity", "notifications"}
	incomplete := func(erasedBefore time.Time) int64 {
		count, err := e.erasures.CountIncomplete(e.ctx, contexts, erasedBefore)
		require.NoError(t, err)
		return count
	}

	assert.Zero(t, incomplete(time.Now().Add(-time.Hour)), "erased just now: not overdue yet")
	assert.EqualValues(t, 1, incomplete(time.Now().Add(time.Hour)))

	require.NoError(t, e.erasure.RecordContextErasure(e.ctx, id, "activity"))
	require.NoError(t, e.erasure.RecordContextErasure(e.ctx, id, "activity"), "a redelivered confirmation is a no-op")
	require.NoError(t, e.erasure.RecordContextErasure(e.ctx, id, "location"), "a context nobody waits for does not count")
	assert.EqualValues(t, 1, incomplete(time.Now().Add(time.Hour)), "notifications still missing")

	require.NoError(t, e.erasure.RecordContextErasure(e.ctx, id, "notifications"))
	assert.Zero(t, incomplete(time.Now().Add(time.Hour)))

	var completed int
	require.NoError(t, e.db.Pool.QueryRow(e.ctx,
		`SELECT COUNT(*) FROM identity.account_erasure WHERE account_id = $1`, id).Scan(&completed))
	assert.Equal(t, 3, completed)

	assert.Error(t, e.erasure.RecordContextErasure(e.ctx, uuid.New(), "activity"), "unknown account")
}
