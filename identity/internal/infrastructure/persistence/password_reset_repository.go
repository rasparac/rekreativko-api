package persistence

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/rasparac/rekreativko-api/identity/internal/domain"
	"github.com/rasparac/rekreativko-api/shared/logger"
	"github.com/rasparac/rekreativko-api/shared/store/postgres"
)

// passwordResetManager stores identity.password_resets: at most one
// outstanding forgot-password code per account.
type passwordResetManager struct {
	tx     *postgres.TransactionManager
	logger *logger.Logger
}

func NewPasswordResetManager(tx *postgres.TransactionManager, logger *logger.Logger) *passwordResetManager {
	return &passwordResetManager{tx: tx, logger: logger}
}

// GetForUpdate returns the account's code, locked until the transaction ends
// so concurrent guesses are counted one after another, or nil if it has none.
func (m *passwordResetManager) GetForUpdate(ctx context.Context, accountID uuid.UUID) (*domain.PasswordReset, error) {
	var (
		code, codeType       string
		failedAttempts       int
		expiresAt, createdAt time.Time
	)

	err := m.tx.Querier(ctx).QueryRow(ctx, `
		SELECT code, code_type, failed_attempts, expires_at, created_at
		FROM identity.password_resets
		WHERE account_id = $1
		FOR UPDATE`, accountID,
	).Scan(&code, &codeType, &failedAttempts, &expiresAt, &createdAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	return domain.ReconstructPasswordReset(
		accountID, code, domain.CodeType(codeType), failedAttempts, expiresAt, createdAt,
	), nil
}

// Save writes the account's code, replacing the one it had.
func (m *passwordResetManager) Save(ctx context.Context, reset *domain.PasswordReset) error {
	_, err := m.tx.Querier(ctx).Exec(ctx, `
		INSERT INTO identity.password_resets (account_id, code, code_type, failed_attempts, expires_at, created_at)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (account_id) DO UPDATE SET
			code = EXCLUDED.code,
			code_type = EXCLUDED.code_type,
			failed_attempts = EXCLUDED.failed_attempts,
			expires_at = EXCLUDED.expires_at,
			created_at = EXCLUDED.created_at`,
		reset.AccountID(), reset.Code(), string(reset.CodeType()), reset.FailedAttempts(), reset.ExpiresAt(), reset.CreatedAt(),
	)

	return err
}

// DeleteAllForAccount removes the account's code: once used, burned, or when
// the account is erased.
func (m *passwordResetManager) DeleteAllForAccount(ctx context.Context, accountID uuid.UUID) error {
	_, err := m.tx.Querier(ctx).Exec(ctx, `DELETE FROM identity.password_resets WHERE account_id = $1`, accountID)
	return err
}
