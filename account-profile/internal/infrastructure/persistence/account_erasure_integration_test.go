//go:build integration

package persistence_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/rasparac/rekreativko-api/account-profile/internal/application"
	"github.com/rasparac/rekreativko-api/account-profile/internal/domain"
	"github.com/rasparac/rekreativko-api/account-profile/internal/infrastructure/persistence"
	"github.com/rasparac/rekreativko-api/account-profile/internal/interfaces/events"
	"github.com/rasparac/rekreativko-api/shared/domainevent"
	sharedevents "github.com/rasparac/rekreativko-api/shared/events"
	testutil "github.com/rasparac/rekreativko-api/shared/testing"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type handler interface {
	Handle(ctx context.Context, payload []byte) error
}

type accountErasureTestEnv struct {
	ctx      context.Context
	db       *testutil.TestDatabase
	verified handler
	erased   handler
}

func setupAccountErasureTest(t *testing.T) *accountErasureTestEnv {
	t.Helper()

	db := testutil.NewTestDatabase(t)
	db.RunMigrationsForService()

	txManager := db.CreateTransactionManager()
	logger := testutil.CreateLogger()
	eventWriter := domainevent.NewDomainEventManager(txManager, logger)
	erasures := persistence.NewAccountErasureManager(txManager)

	profileSvc := application.NewService(logger, txManager, persistence.NewAccountProfileManager(txManager, logger), eventWriter, testMetrics())
	settingsSvc := application.NewAccountSettingsService(persistence.NewAccountProfileSettingsManager(txManager), txManager, logger, testMetrics())
	erasureSvc := application.NewAccountErasureService(erasures, txManager, eventWriter, logger, testMetrics())

	return &accountErasureTestEnv{
		ctx:      context.Background(),
		db:       db,
		verified: events.NewCreateProfileEventHandler(txManager, profileSvc, settingsSvc, erasures, logger),
		erased:   events.NewAccountErasedEventHandler(erasureSvc, logger),
	}
}

// fullAccount creates an account the way identity.account.verified does and
// fills every table that holds its data.
func (e *accountErasureTestEnv) fullAccount(t *testing.T) uuid.UUID {
	t.Helper()

	accountID := uuid.New()
	require.NoError(t, e.verified.Handle(e.ctx, e.verifiedPayload(t, accountID)))

	e.db.RunSQL(`UPDATE account_profile.profiles SET nickname = $2, full_name = 'Ana Horvat',
		date_of_birth = '1990-05-01', bio = 'runner', location_city = 'Zagreb', location_country = 'HR',
		location_latitude = 45.81, location_longitude = 15.98, profile_picture_url = 'https://example.com/a.png'
		WHERE account_id = $1`, accountID, "ana_"+accountID.String()[:8])
	e.db.RunSQL(`INSERT INTO account_profile.activity_interests (account_id, activity_type, activity_level)
		VALUES ($1, 'running', 'advanced')`, accountID)
	e.db.RunSQL(`INSERT INTO account_profile.profile_statistics
		(account_id, activities_joined, activities_by_type, monthly_breakdown, achievements)
		VALUES ($1, 3, '{"running": 3}', '{}', '[]')`, accountID)

	return accountID
}

func (e *accountErasureTestEnv) verifiedPayload(t *testing.T, accountID uuid.UUID) []byte {
	t.Helper()

	payload, err := json.Marshal(events.AccountVerifiedEvent{EventID: uuid.New(), AccountID: accountID, DeliveryType: "email"})
	require.NoError(t, err)
	return payload
}

func (e *accountErasureTestEnv) erasedPayload(t *testing.T, accountID uuid.UUID) []byte {
	t.Helper()

	payload, err := json.Marshal(events.AccountErasedEvent{EventID: uuid.New(), AccountID: accountID})
	require.NoError(t, err)
	return payload
}

// accountRows counts the account's rows in every account_profile table with an
// account_id column - read from the catalog, so a table added later is
// checked too - except erased_account, which records the erasure.
func (e *accountErasureTestEnv) accountRows(t *testing.T, accountID uuid.UUID) map[string]int {
	t.Helper()

	rows, err := e.db.Pool.Query(e.ctx, `SELECT table_name FROM information_schema.columns
		WHERE table_schema = 'account_profile' AND column_name = 'account_id' AND table_name <> 'erased_account'`)
	require.NoError(t, err)
	tables, err := pgx.CollectRows(rows, pgx.RowTo[string])
	require.NoError(t, err)
	require.NotEmpty(t, tables)

	counts := map[string]int{}
	for _, table := range tables {
		var n int
		require.NoError(t, e.db.Pool.QueryRow(e.ctx,
			`SELECT count(*) FROM account_profile.`+table+` WHERE account_id = $1`, accountID).Scan(&n))
		counts[table] = n
	}
	return counts
}

func (e *accountErasureTestEnv) erasedEvents(t *testing.T, accountID uuid.UUID) []json.RawMessage {
	t.Helper()

	rows, err := e.db.Pool.Query(e.ctx, `SELECT payload FROM account_profile.event_outbox
		WHERE event_type = $1 AND aggregate_id = $2`, domain.EventAccountErased, accountID)
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

func (e *accountErasureTestEnv) isErased(t *testing.T, accountID uuid.UUID) bool {
	t.Helper()

	var erased bool
	require.NoError(t, e.db.Pool.QueryRow(e.ctx,
		`SELECT EXISTS (SELECT 1 FROM account_profile.erased_account WHERE account_id = $1)`, accountID).Scan(&erased))
	return erased
}

func TestAccountErasure_ErasesEveryRowOfTheAccountAndConfirmsOnce(t *testing.T) {
	env := setupAccountErasureTest(t)

	erased := env.fullAccount(t)
	staying := env.fullAccount(t)

	before := env.accountRows(t, erased)
	for table, n := range before {
		require.Positive(t, n, "the test fills %s", table)
	}

	require.NoError(t, env.erased.Handle(env.ctx, env.erasedPayload(t, erased)))

	for table, n := range env.accountRows(t, erased) {
		assert.Zero(t, n, "%s erased", table)
	}
	assert.True(t, env.isErased(t, erased))
	assert.Equal(t, before, env.accountRows(t, staying), "another account is untouched")
	assert.False(t, env.isErased(t, staying))

	payloads := env.erasedEvents(t, erased)
	require.Len(t, payloads, 1, "account_profile.account.erased in the outbox")
	var payload map[string]any
	require.NoError(t, json.Unmarshal(payloads[0], &payload))
	assert.Equal(t, erased.String(), payload["account_id"])
	assert.NotContains(t, string(payloads[0]), "Ana Horvat")

	// At-least-once delivery: the same event again is a no-op, not a second
	// confirmation.
	require.NoError(t, env.erased.Handle(env.ctx, env.erasedPayload(t, erased)))
	assert.Len(t, env.erasedEvents(t, erased), 1)

	// A late identity.account.verified does not bring the profile back.
	require.NoError(t, env.verified.Handle(env.ctx, env.verifiedPayload(t, erased)))
	for table, n := range env.accountRows(t, erased) {
		assert.Zero(t, n, "%s not re-created", table)
	}
}

func TestAccountErasure_AnAccountWithNoProfileIsStillConfirmed(t *testing.T) {
	env := setupAccountErasureTest(t)
	neverVerified := uuid.New()

	require.NoError(t, env.erased.Handle(env.ctx, env.erasedPayload(t, neverVerified)))

	assert.True(t, env.isErased(t, neverVerified))
	assert.Len(t, env.erasedEvents(t, neverVerified), 1, "identity waits for the confirmation all the same")
}

func TestAccountErasure_ABadEventIsPermanent(t *testing.T) {
	env := setupAccountErasureTest(t)

	for name, payload := range map[string]string{
		"not json":           `{`,
		"missing account_id": `{"event_id":"` + uuid.NewString() + `"}`,
	} {
		err := env.erased.Handle(env.ctx, []byte(payload))
		assert.ErrorIs(t, err, sharedevents.ErrPermanent, name)
	}
}
