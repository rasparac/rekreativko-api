package persistence

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/rasparac/rekreativko-api/shared/store/postgres"
)

type (
	AccountStateRepository interface {
		MarkDeleted(ctx context.Context, accountID uuid.UUID, deletedAt time.Time) error
		Erase(ctx context.Context, accountID uuid.UUID, erasedAt time.Time) (bool, error)
		IsInactive(ctx context.Context, accountID uuid.UUID) (bool, error)
	}

	// AccountStateManager records accounts deleted or erased in identity
	// (notifications.deleted_account, notifications.erased_account).
	AccountStateManager struct {
		tx *postgres.TransactionManager
	}
)

func NewAccountStateManager(tx *postgres.TransactionManager) *AccountStateManager {
	return &AccountStateManager{tx: tx}
}

// MarkDeleted records that the account was deleted. A repeat keeps the first
// deleted_at.
func (m *AccountStateManager) MarkDeleted(ctx context.Context, accountID uuid.UUID, deletedAt time.Time) error {
	_, err := m.tx.Querier(ctx).Exec(ctx, `
	INSERT INTO notifications.deleted_account (account_id, deleted_at)
	VALUES ($1, $2)
	ON CONFLICT (account_id) DO NOTHING`, accountID, deletedAt)

	return err
}

// Erase records the erasure and deletes every notification the account
// received. It must run in the caller's transaction. It returns false,
// deleting nothing, if the account was already erased: the erased_account row
// is inserted first, so of two concurrent deliveries of the same event the
// second waits for the first and then finds it erased. Notifications of other
// accounts that name this one in data (an actor's id) stay - the id alone
// identifies nobody once identity has erased the account.
func (m *AccountStateManager) Erase(ctx context.Context, accountID uuid.UUID, erasedAt time.Time) (bool, error) {
	tx := m.tx.Querier(ctx)

	tag, err := tx.Exec(ctx, `
	INSERT INTO notifications.erased_account (account_id, erased_at)
	VALUES ($1, $2)
	ON CONFLICT (account_id) DO NOTHING`, accountID, erasedAt)
	if err != nil {
		return false, fmt.Errorf("record erasure: %w", err)
	}

	if tag.RowsAffected() == 0 {
		return false, nil
	}

	_, err = tx.Exec(ctx, `DELETE FROM notifications.notification WHERE recipient_account_id = $1`, accountID)
	if err != nil {
		return false, fmt.Errorf("delete notifications: %w", err)
	}

	return true, nil
}

// IsInactive reports whether the account was deleted or erased: nothing is
// delivered to it and no notification is created for it any more.
func (m *AccountStateManager) IsInactive(ctx context.Context, accountID uuid.UUID) (bool, error) {
	var inactive bool
	err := m.tx.Querier(ctx).QueryRow(ctx, `
	SELECT EXISTS (SELECT 1 FROM notifications.deleted_account WHERE account_id = $1)
		OR EXISTS (SELECT 1 FROM notifications.erased_account WHERE account_id = $1)`,
		accountID,
	).Scan(&inactive)

	return inactive, err
}
