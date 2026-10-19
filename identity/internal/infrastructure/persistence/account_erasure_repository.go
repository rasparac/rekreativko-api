package persistence

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rasparac/rekreativko-api/shared/logger"
	"github.com/rasparac/rekreativko-api/shared/store/postgres"
)

type (
	AccountErasureReaderWriter interface {
		RecordCompleted(ctx context.Context, accountID uuid.UUID, context string, completedAt time.Time) error
		CountIncomplete(ctx context.Context, contexts []string, erasedBefore time.Time) (int64, error)
	}

	// AccountErasureManager tracks which bounded contexts have erased their
	// data of an erased account (identity.account_erasure).
	AccountErasureManager struct {
		db     *pgxpool.Pool
		tx     *postgres.TransactionManager
		logger *logger.Logger
	}
)

func NewAccountErasureManager(
	db *pgxpool.Pool,
	tx *postgres.TransactionManager,
	logger *logger.Logger,
) *AccountErasureManager {
	return &AccountErasureManager{
		db:     db,
		tx:     tx,
		logger: logger,
	}
}

// RecordCompleted records that context has erased its data of the account. A
// repeated confirmation (events are delivered at least once) keeps the first
// completed_at.
func (m *AccountErasureManager) RecordCompleted(
	ctx context.Context,
	accountID uuid.UUID,
	context string,
	completedAt time.Time,
) error {
	query := `
	INSERT INTO identity.account_erasure (account_id, context, completed_at)
	VALUES ($1, $2, $3)
	ON CONFLICT (account_id, context) DO NOTHING
	`

	_, err := m.tx.Querier(ctx).Exec(ctx, query, accountID, context, completedAt)

	return err
}

// CountIncomplete counts the accounts erased before erasedBefore that some of
// contexts have not confirmed yet.
func (m *AccountErasureManager) CountIncomplete(
	ctx context.Context,
	contexts []string,
	erasedBefore time.Time,
) (int64, error) {
	query := `
	SELECT COUNT(*)
	FROM identity.accounts a
	WHERE
		a.erased_at IS NOT NULL
		AND a.erased_at < $1
		AND (
			SELECT COUNT(*)
			FROM identity.account_erasure e
			WHERE e.account_id = a.id AND e.context = ANY($2)
		) < cardinality($2::text[])
	`

	var count int64
	err := m.tx.Querier(ctx).QueryRow(ctx, query, erasedBefore, contexts).Scan(&count)

	return count, err
}

var _ AccountErasureReaderWriter = (*AccountErasureManager)(nil)
