//go:build integration

package persistence_test

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/rasparac/rekreativko-api/identity/internal/application"
	"github.com/rasparac/rekreativko-api/identity/internal/domain"
	"github.com/rasparac/rekreativko-api/identity/internal/infrastructure/persistence"
	testutil "github.com/rasparac/rekreativko-api/shared/testing"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const newPassword = "Brand-New-Pass-2"

type passwordTestEnv struct {
	*erasureTestEnv
	passwords *application.PasswordService
}

func setupPasswordTest(t *testing.T) *passwordTestEnv {
	t.Helper()

	env := setupErasureTest(t)
	resets := persistence.NewPasswordResetManager(env.db.CreateTransactionManager(), testutil.CreateLogger())

	return &passwordTestEnv{erasureTestEnv: env, passwords: application.NewPasswordService(env.svc, resets)}
}

func (e *passwordTestEnv) forgot(t *testing.T, email string) {
	t.Helper()
	require.NoError(t, e.passwords.ForgotPassword(e.ctx, application.ForgotPasswordParams{Type: "email", Identity: email}))
}

func (e *passwordTestEnv) reset(email, code, password string) error {
	return e.passwords.ResetPassword(e.ctx, application.ResetPasswordParams{
		Type: "email", Identity: email, Code: code, NewPassword: password,
	})
}

// sentCodes are the codes identity.password_reset.requested carried for the
// account, oldest first - what notifications would have sent.
func (e *passwordTestEnv) sentCodes(t *testing.T, id uuid.UUID) []string {
	t.Helper()

	rows, err := e.db.Pool.Query(e.ctx, `SELECT payload->>'code' FROM identity.event_outbox
		WHERE event_type = $1 AND aggregate_id = $2 ORDER BY created_at`, domain.IdentityPasswordResetRequestedEvent, id)
	require.NoError(t, err)
	defer rows.Close()

	var codes []string
	for rows.Next() {
		var code string
		require.NoError(t, rows.Scan(&code))
		codes = append(codes, code)
	}
	require.NoError(t, rows.Err())
	return codes
}

func (e *passwordTestEnv) outboxCount(t *testing.T, eventType string, id uuid.UUID) int {
	t.Helper()

	var n int
	require.NoError(t, e.db.Pool.QueryRow(e.ctx, `SELECT COUNT(*) FROM identity.event_outbox
		WHERE event_type = $1 AND aggregate_id = $2`, eventType, id).Scan(&n))
	return n
}

func (e *passwordTestEnv) liveTokens(t *testing.T, id uuid.UUID) int {
	t.Helper()

	var n int
	require.NoError(t, e.db.Pool.QueryRow(e.ctx, `SELECT COUNT(*) FROM identity.refresh_tokens
		WHERE account_id = $1 AND revoked_at IS NULL`, id).Scan(&n))
	return n
}

func (e *passwordTestEnv) canLogin(email, password string) bool {
	_, err := e.svc.Login(e.ctx, application.LoginParams{Email: email, Password: password})
	return err == nil
}

func TestForgotPassword(t *testing.T) {
	env := setupPasswordTest(t)

	t.Run("an unknown address gets the same answer and nothing is sent", func(t *testing.T) {
		env.forgot(t, "nobody@example.com")

		var resets int
		require.NoError(t, env.db.Pool.QueryRow(env.ctx, `SELECT COUNT(*) FROM identity.password_resets`).Scan(&resets))
		assert.Zero(t, resets)
	})

	t.Run("an active account is sent one code a minute", func(t *testing.T) {
		id := env.activeAccount(t, "forgetful@example.com", "+38591000101")

		env.forgot(t, "forgetful@example.com")
		env.forgot(t, "forgetful@example.com")

		codes := env.sentCodes(t, id)
		require.Len(t, codes, 1, "the second request came too soon")
		assert.Len(t, codes[0], 6)
	})

	t.Run("the address is found however the user capitalized it", func(t *testing.T) {
		id := env.activeAccount(t, "capitals@example.com", "+38591000103")

		env.forgot(t, " Capitals@Example.COM ")

		assert.Len(t, env.sentCodes(t, id), 1)
		assert.True(t, env.canLogin("Capitals@Example.com", correctPassword))
	})

	t.Run("a deleted account is sent nothing", func(t *testing.T) {
		id := env.activeAccount(t, "gone@example.com", "+38591000102")
		require.NoError(t, env.svc.DeleteAccount(env.ctx, id))

		env.forgot(t, "gone@example.com")
		assert.Empty(t, env.sentCodes(t, id))
	})
}

func TestResetPassword(t *testing.T) {
	env := setupPasswordTest(t)
	const email = "locked-out@example.com"
	id := env.activeAccount(t, email, "+38591000201")
	env.forgot(t, email)
	code := env.sentCodes(t, id)[0]
	wrong := "000000"
	if code == wrong {
		wrong = "111111"
	}

	t.Run("a weak password is refused before the code is looked at", func(t *testing.T) {
		requireCode(t, env.reset(email, wrong, "short"), "identity_weak_password")
	})

	t.Run("an unknown address and a wrong code are the same error", func(t *testing.T) {
		requireCode(t, env.reset("nobody@example.com", code, newPassword), "identity_invalid_verification_code")
		requireCode(t, env.reset(email, wrong, newPassword), "identity_invalid_verification_code")
	})

	t.Run("the right code sets the password and logs every session out", func(t *testing.T) {
		require.Equal(t, 1, env.liveTokens(t, id))

		require.NoError(t, env.reset(email, code, newPassword))

		assert.Zero(t, env.liveTokens(t, id))
		assert.Equal(t, 1, env.outboxCount(t, domain.IdentityAccountPasswordChangedEvent, id))
		assert.False(t, env.canLogin(email, correctPassword))
		assert.True(t, env.canLogin(email, newPassword))
	})

	t.Run("a used code can't be used again", func(t *testing.T) {
		requireCode(t, env.reset(email, code, "Another-Pass-3"), "identity_invalid_verification_code")
	})
}

func TestResetPassword_FiveWrongCodesBurnIt(t *testing.T) {
	env := setupPasswordTest(t)
	const email = "guessed@example.com"
	id := env.activeAccount(t, email, "+38591000301")
	env.forgot(t, email)
	code := env.sentCodes(t, id)[0]

	guesses := []string{"000000", "111111", "222222", "333333", "444444", "555555"}
	var wrong []string
	for _, g := range guesses {
		if g != code {
			wrong = append(wrong, g)
		}
	}

	for range 4 {
		requireCode(t, env.reset(email, wrong[0], newPassword), "identity_invalid_verification_code")
	}
	requireCode(t, env.reset(email, wrong[1], newPassword), "identity_too_many_verification_attempts")

	// each failure was kept although the request failed: the right code is no use now
	requireCode(t, env.reset(email, code, newPassword), "identity_invalid_verification_code")
	assert.True(t, env.canLogin(email, correctPassword))
}

func TestResetPassword_LiftsALoginLock(t *testing.T) {
	env := setupPasswordTest(t)
	const email = "attacked@example.com"
	id := env.activeAccount(t, email, "+38591000401")
	for range 5 {
		_, err := env.svc.Login(env.ctx, application.LoginParams{Email: email, Password: "Wrong-Guess-9"})
		require.Error(t, err)
	}
	require.False(t, env.canLogin(email, correctPassword), "locked")

	env.forgot(t, email)
	require.NoError(t, env.reset(email, env.sentCodes(t, id)[0], newPassword))

	assert.True(t, env.canLogin(email, newPassword))
}

func TestChangePassword(t *testing.T) {
	env := setupPasswordTest(t)
	const email = "careful@example.com"
	id := env.activeAccount(t, email, "+38591000501")
	change := func(old, new string) (*domain.RefreshToken, error) {
		return env.passwords.ChangePassword(env.ctx, application.ChangePasswordParams{AccountID: id, OldPassword: old, NewPassword: new})
	}

	t.Run("a wrong current password is a 400 that counts as a failed login", func(t *testing.T) {
		_, err := change("Wrong-Guess-9", newPassword)
		requireCode(t, err, "identity_wrong_password")

		var failed int
		require.NoError(t, env.db.Pool.QueryRow(env.ctx,
			`SELECT failed_login_attempts FROM identity.accounts WHERE id = $1`, id).Scan(&failed))
		assert.Equal(t, 1, failed)
	})

	t.Run("the same password again is refused", func(t *testing.T) {
		_, err := change(correctPassword, correctPassword)
		requireCode(t, err, "identity_password_unchanged")
	})

	t.Run("a weak new password is refused", func(t *testing.T) {
		_, err := change(correctPassword, "weakpass")
		requireCode(t, err, "identity_weak_password")
	})

	t.Run("the change logs other sessions out and hands this one new tokens", func(t *testing.T) {
		token, err := change(correctPassword, newPassword)
		require.NoError(t, err)

		require.NotEmpty(t, token.Token())
		require.NotEmpty(t, token.AccessToken())
		assert.Equal(t, 1, env.liveTokens(t, id), "only the new one")
		assert.Equal(t, 1, env.outboxCount(t, domain.IdentityAccountPasswordChangedEvent, id))
		assert.True(t, env.canLogin(email, newPassword))

		_, err = env.svc.RefreshToken(env.ctx, application.RefreshTokenParams{RefreshToken: token.Token()})
		assert.NoError(t, err, "the returned refresh token works")
	})
}

func TestAccountErasure_DeletesThePasswordResetCode(t *testing.T) {
	env := setupPasswordTest(t)
	const email = "leaving@example.com"
	id := env.activeAccount(t, email, "+38591000601")
	env.forgot(t, email)
	require.NoError(t, env.svc.DeleteAccount(env.ctx, id)) // grace 0: due right away

	_, err := env.erasure.EraseDueAccounts(env.ctx, time.Now().UTC().Add(time.Second))
	require.NoError(t, err)

	var resets int
	require.NoError(t, env.db.Pool.QueryRow(env.ctx,
		`SELECT COUNT(*) FROM identity.password_resets WHERE account_id = $1`, id).Scan(&resets))
	assert.Zero(t, resets)
}
