package persistence

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/rasparac/rekreativko-api/shared/store/postgres"
)

type (
	AccountErasureReaderWriter interface {
		EraseAccount(ctx context.Context, accountID uuid.UUID, erasedAt time.Time) (bool, error)
		IsErased(ctx context.Context, accountID uuid.UUID) (bool, error)
	}

	// AccountErasureManager erases an account's data from every
	// account_profile table and records the erasure (account_profile.erased_account).
	AccountErasureManager struct {
		tx *postgres.TransactionManager
	}
)

// accountDataTables are deleted from in this order: children before
// account_profile.profiles, which they reference (no ON DELETE CASCADE, so
// what an erasure removes stays visible here). A new table holding data of an
// account belongs in this list.
var accountDataTables = []string{
	"account_profile.activity_interests",
	"account_profile.settings",
	"account_profile.profile_statistics",
	"account_profile.account_settings_meta",
	"account_profile.profiles",
}

func NewAccountErasureManager(tx *postgres.TransactionManager) *AccountErasureManager {
	return &AccountErasureManager{tx: tx}
}

// EraseAccount records the erasure and deletes the account's rows from every
// table in accountDataTables. It must run in the caller's transaction. It
// returns false, deleting nothing, if the account was already erased: the
// erased_account row is inserted first, so of two concurrent deliveries of the
// same event the second waits for the first and then finds it erased.
func (m *AccountErasureManager) EraseAccount(
	ctx context.Context,
	accountID uuid.UUID,
	erasedAt time.Time,
) (bool, error) {
	tx := m.tx.Querier(ctx)

	tag, err := tx.Exec(ctx, `
	INSERT INTO account_profile.erased_account (account_id, erased_at)
	VALUES ($1, $2)
	ON CONFLICT (account_id) DO NOTHING`, accountID, erasedAt)
	if err != nil {
		return false, fmt.Errorf("record erasure: %w", err)
	}

	if tag.RowsAffected() == 0 {
		return false, nil
	}

	for _, table := range accountDataTables {
		_, err := tx.Exec(ctx, `DELETE FROM `+table+` WHERE account_id = $1`, accountID)
		if err != nil {
			return false, fmt.Errorf("delete from %s: %w", table, err)
		}
	}

	return true, nil
}

// IsErased reports whether the account's data was erased.
func (m *AccountErasureManager) IsErased(ctx context.Context, accountID uuid.UUID) (bool, error) {
	var erased bool
	err := m.tx.Querier(ctx).QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM account_profile.erased_account WHERE account_id = $1)`,
		accountID,
	).Scan(&erased)

	return erased, err
}
