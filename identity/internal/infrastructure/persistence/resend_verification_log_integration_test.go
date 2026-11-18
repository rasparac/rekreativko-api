//go:build integration

package persistence_test

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/rasparac/rekreativko-api/identity/internal/application"
	"github.com/rasparac/rekreativko-api/identity/internal/infrastructure/persistence"
	"github.com/rasparac/rekreativko-api/shared/domainevent"
	"github.com/rasparac/rekreativko-api/shared/logger"
	testutil "github.com/rasparac/rekreativko-api/shared/testing"
	"github.com/rasparac/rekreativko-api/shared/token"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"
)

// The identity a client types to ask for a new code is an email or phone number:
// it must never reach a log line (Loki keeps it 14 days), whether the account
// exists or not. Once the account is found, its id is logged instead.
func TestResendVerificationCode_LogsAccountIDNeverTheIdentity(t *testing.T) {
	ctx := context.Background()
	db := testutil.NewTestDatabase(t)
	db.RunMigrationsForService()

	var (
		logs      bytes.Buffer
		txManager = db.CreateTransactionManager()
		log       = logger.NewWithWriter("debug", "json", &logs)
	)

	svc := application.NewService(
		persistence.NewAccountManager(db.Pool, txManager, log),
		persistence.NewVerificationCodeManager(db.Pool, txManager, log),
		persistence.NewRefreshTokenManager(db.Pool, txManager, log),
		txManager,
		domainevent.MustNewEventWriter(txManager, log, "identity"),
		token.NewGenerator([]byte("test-secret-test-secret-test-secret"), time.Minute, time.Hour),
		log,
		token.NewVerificationCodeGenerator(6),
		token.NewPasswordHasher(bcrypt.MinCost),
		testMetrics(),
		false,
		true,
		0,
	)

	const email = "log.check@example.com"
	accountID, err := svc.Register(ctx, application.RegistrationParams{Email: email, Password: correctPassword})
	require.NoError(t, err)

	logs.Reset()
	_, err = svc.ResendVerificationCode(ctx, application.ResendVerificationCodeParams{Type: "email", Identity: email})
	require.NoError(t, err)

	assert.NotContains(t, logs.String(), email)
	assert.Contains(t, logs.String(), accountID.String(), "the found account is identified by its id")

	logs.Reset()
	const unknown = "nobody@example.com"
	_, err = svc.ResendVerificationCode(ctx, application.ResendVerificationCodeParams{Type: "email", Identity: unknown})
	require.Error(t, err)

	assert.NotEmpty(t, logs.String(), "the failure is logged")
	assert.NotContains(t, logs.String(), unknown)
}
