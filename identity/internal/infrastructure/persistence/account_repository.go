package persistence

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rasparac/rekreativko-api/identity/internal/domain"
	"github.com/rasparac/rekreativko-api/shared/logger"
	"github.com/rasparac/rekreativko-api/shared/store/postgres"
)

type (
	AccountReaderWriter interface {
		CreateAccount(ctx context.Context, account *domain.Account) error
		GetBy(ctx context.Context, filter AccountFilter) (*domain.Account, error)
		UpdateAccount(ctx context.Context, account *domain.Account) error
		ListDueForErasure(ctx context.Context, now time.Time, limit int) ([]*domain.Account, error)
	}

	// AccountFilter selects one account. By email or phone number it finds
	// only an account that is not deleted: a deleted account keeps its
	// address until the erasure, but the address is free to register again
	// (the unique indexes ignore deleted rows), so it would be ambiguous. By
	// id it finds a deleted account too.
	AccountFilter struct {
		PhoneNumber *string
		UUID        *uuid.UUID
		Email       *string

		// ForUpdate locks the row until the surrounding transaction ends, so
		// concurrent read-modify-write cycles on one account (counting failed
		// logins) queue up instead of overwriting each other. Only meaningful
		// inside a transaction.
		ForUpdate bool
	}

	AccountManager struct {
		db     *pgxpool.Pool
		tx     *postgres.TransactionManager
		logger *logger.Logger
	}

	accoutModel struct {
		ID                  uuid.UUID
		PhoneNumber         sql.NullString
		Email               sql.NullString
		PasswordHash        string
		Status              string
		FailedLoginAttempts int
		LockedUntil         sql.NullTime
		CreatedAt           time.Time
		UpdatedAt           time.Time
		DeletedAt           sql.NullTime
		EraseAfter          sql.NullTime
		ErasedAt            sql.NullTime
	}
)

// accountColumns are the columns scanAccount reads, in its order.
const accountColumns = `
		id,
		email,
		phone_number,
		password,
		status,
		failed_login_attempts,
		locked_until,
		created_at,
		updated_at,
		deleted_at,
		erase_after,
		erased_at`

func (f AccountFilter) Validate() error {
	if f.PhoneNumber == nil && f.Email == nil && f.UUID == nil {
		return errors.New("missing filter")
	}

	return nil
}

func NewAccountManager(
	db *pgxpool.Pool,
	tx *postgres.TransactionManager,
	logger *logger.Logger,
) *AccountManager {
	return &AccountManager{
		db:     db,
		tx:     tx,
		logger: logger,
	}
}

func (am *AccountManager) GetBy(ctx context.Context, filter AccountFilter) (*domain.Account, error) {
	err := filter.Validate()
	if err != nil {
		return nil, err
	}

	var (
		baseQuery = `SELECT` + accountColumns + `
	FROM identity.accounts WHERE %s`
		args    = []any{}
		filters = []string{}
	)

	if filter.PhoneNumber != nil {
		args = append(args, *filter.PhoneNumber)
		filters = append(
			filters,
			fmt.Sprintf("phone_number = $%d", len(args)),
		)
	}

	if filter.Email != nil {
		args = append(args, *filter.Email)

		filters = append(
			filters,
			fmt.Sprintf("email = $%d", len(args)),
		)
	}

	if filter.UUID != nil {
		args = append(args, *filter.UUID)
		filters = append(
			filters,
			fmt.Sprintf("id = $%d", len(args)),
		)
	}

	if filter.PhoneNumber != nil || filter.Email != nil {
		filters = append(filters, "deleted_at IS NULL")
	}

	baseQuery = fmt.Sprintf(baseQuery, strings.Join(filters, " AND "))
	if filter.ForUpdate {
		baseQuery += " FOR UPDATE"
	}

	// The transaction's querier when there is one, so a ForUpdate lock is taken
	// in (and held by) the caller's transaction, not on a separate connection.
	model, err := scanAccount(am.tx.Querier(ctx).QueryRow(ctx, baseQuery, args...))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrAccountNotFound
	} else if err != nil {
		return nil, err
	}

	return toDomainAccount(model)
}

// ListDueForErasure returns up to limit deleted accounts whose erase_after has
// passed and that are not erased yet, longest due first. The rows are locked
// with FOR UPDATE SKIP LOCKED, so instances erasing at the same time never
// pick the same account; it MUST be called within a transaction, which holds
// the locks until it ends.
func (am *AccountManager) ListDueForErasure(ctx context.Context, now time.Time, limit int) ([]*domain.Account, error) {
	query := `SELECT` + accountColumns + `
	FROM identity.accounts
	WHERE
		erase_after IS NOT NULL
		AND erased_at IS NULL
		AND erase_after <= $1
	ORDER BY erase_after ASC
	LIMIT $2
	FOR UPDATE SKIP LOCKED`

	rows, err := am.tx.Querier(ctx).Query(ctx, query, now, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var accounts []*domain.Account
	for rows.Next() {
		model, err := scanAccount(rows)
		if err != nil {
			return nil, err
		}

		account, err := toDomainAccount(model)
		if err != nil {
			return nil, err
		}
		accounts = append(accounts, account)
	}

	return accounts, rows.Err()
}

func scanAccount(row pgx.Row) (accoutModel, error) {
	var model accoutModel
	err := row.Scan(
		&model.ID,
		&model.Email,
		&model.PhoneNumber,
		&model.PasswordHash,
		&model.Status,
		&model.FailedLoginAttempts,
		&model.LockedUntil,
		&model.CreatedAt,
		&model.UpdatedAt,
		&model.DeletedAt,
		&model.EraseAfter,
		&model.ErasedAt,
	)
	return model, err
}

func (am *AccountManager) CreateAccount(ctx context.Context, account *domain.Account) error {
	var (
		model = toAccountModel(account)
		query = `
		INSERT INTO identity.accounts
			(id, email, phone_number, password, status, failed_login_attempts, locked_until, created_at, updated_at)
		VALUES
			($1, $2, $3, $4, $5, $6, $7, $8, $9)
		`
		querier = am.tx.Querier(ctx)
	)

	_, err := querier.Exec(
		ctx,
		query,
		model.ID,
		model.Email,
		model.PhoneNumber,
		model.PasswordHash,
		model.Status,
		model.FailedLoginAttempts,
		model.LockedUntil,
		model.CreatedAt,
		model.UpdatedAt,
	)
	if err != nil {
		return err
	}

	return nil
}

func (am *AccountManager) UpdateAccount(ctx context.Context, account *domain.Account) error {
	var (
		model = toAccountModel(account)
		query = `
			UPDATE identity.accounts
			SET
				email = $2,
				phone_number = $3,
				status = $4,
				failed_login_attempts = $5,
				locked_until = $6,
				updated_at = $7,
				deleted_at = $8,
				password = $9,
				erase_after = $10,
				erased_at = $11
			WHERE id = $1
		`
	)

	querier := am.tx.Querier(ctx)

	cmdTag, err := querier.Exec(
		ctx,
		query,
		model.ID,
		model.Email,
		model.PhoneNumber,
		model.Status,
		model.FailedLoginAttempts,
		model.LockedUntil,
		model.UpdatedAt,
		model.DeletedAt,
		model.PasswordHash,
		model.EraseAfter,
		model.ErasedAt,
	)
	if err != nil {
		return err
	}

	if cmdTag.RowsAffected() == 0 {
		return domain.ErrAccountNotFound
	}

	return nil
}

func toAccountModel(account *domain.Account) *accoutModel {
	var lockedIn time.Time
	if account.LockedUntil() != nil {
		lockedIn = *account.LockedUntil()
	}

	var deletedAt time.Time
	if account.DeletedAt() != nil {
		deletedAt = *account.DeletedAt()
	}

	return &accoutModel{
		ID: account.ID(),
		PhoneNumber: sql.NullString{
			String: account.PhoneNumber().String(),
			Valid:  account.PhoneNumber().String() != "",
		},
		Email: sql.NullString{
			String: account.Email().String(),
			Valid:  account.Email().String() != "",
		},
		PasswordHash:        account.Password().String(),
		Status:              account.Status().String(),
		FailedLoginAttempts: account.FailedLoginAttempts(),
		LockedUntil: sql.NullTime{
			Time:  lockedIn,
			Valid: !lockedIn.IsZero(),
		},
		CreatedAt: account.CreatedAt(),
		UpdatedAt: account.UpdatedAt(),
		DeletedAt: sql.NullTime{
			Time:  deletedAt,
			Valid: !deletedAt.IsZero(),
		},
		EraseAfter: toNullTime(account.EraseAfter()),
		ErasedAt:   toNullTime(account.ErasedAt()),
	}
}

func toNullTime(t *time.Time) sql.NullTime {
	if t == nil {
		return sql.NullTime{}
	}
	return sql.NullTime{Time: *t, Valid: true}
}

func fromNullTime(t sql.NullTime) *time.Time {
	if !t.Valid {
		return nil
	}
	return &t.Time
}

func toDomainAccount(account accoutModel) (*domain.Account, error) {
	email, err := domain.NewEmail(account.Email.String)
	if err != nil {
		return nil, err
	}

	phoneNumber, err := domain.NewPhoneNumber(account.PhoneNumber.String)
	if err != nil {
		return nil, err
	}

	password := domain.NewPasswordFromHash(account.PasswordHash)

	var deletedAt *time.Time
	if account.DeletedAt.Valid {
		deletedAt = &account.DeletedAt.Time
	}

	return domain.ReconstructAccount(
		account.ID,
		email,
		phoneNumber,
		password,
		domain.AccountStatus(account.Status),
		account.FailedLoginAttempts,
		&account.LockedUntil.Time,
		account.CreatedAt,
		account.UpdatedAt,
		deletedAt,
		fromNullTime(account.EraseAfter),
		fromNullTime(account.ErasedAt),
	), nil
}

var _ AccountReaderWriter = (*AccountManager)(nil)
