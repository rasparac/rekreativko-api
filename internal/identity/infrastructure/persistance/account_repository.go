package persistance

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
	"github.com/rasparac/rekreativko-api/internal/identity/domain"
	"github.com/rasparac/rekreativko-api/internal/shared/logger"
	"github.com/rasparac/rekreativko-api/internal/shared/store/postgres"
)

type (
	AccountReaderWriter interface {
		CreateAccount(ctx context.Context, account *domain.Account) error
		GetBy(ctx context.Context, filter AccountFilter) (*domain.Account, error)
		UpdateAccount(ctx context.Context, account *domain.Account) error
		DeleteAccount(ctx context.Context, UUID uuid.UUID) error
	}

	AccountFilter struct {
		PhoneNumber *string
		UUID        *uuid.UUID
		Email       *string
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
	}
)

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
		baseQuery = `SELECT
		id,
		email,
		phone_number,
		password,
		status,
		failed_login_attempts,
		locked_until,
		created_at,
		updated_at
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

	baseQuery = fmt.Sprintf(baseQuery, strings.Join(filters, " AND "))

	var model accoutModel
	err = am.db.QueryRow(ctx, baseQuery, args...).Scan(
		&model.ID,
		&model.Email,
		&model.PhoneNumber,
		&model.PasswordHash,
		&model.Status,
		&model.FailedLoginAttempts,
		&model.LockedUntil,
		&model.CreatedAt,
		&model.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrAccountNotFound
	} else if err != nil {
		return nil, err
	}

	return toDomainAccount(model)
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
				updated_at = $7
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
	)
	if err != nil {
		return err
	}

	if cmdTag.RowsAffected() == 0 {
		return domain.ErrAccountNotFound
	}

	return nil
}

func (am *AccountManager) DeleteAccount(ctx context.Context, UUID uuid.UUID) error {
	query := `
	UPDATE
		identity.accounts
	SET
		status = 'deleted',
		updated_at = NOW()
	WHERE id = $1`

	querier := am.tx.Querier(ctx)

	_, err := querier.Exec(ctx, query, UUID)
	if err != nil {
		return err
	}

	return nil
}

func toAccountModel(account *domain.Account) *accoutModel {
	var lockedIn time.Time
	if account.LockedUntil() != nil {
		lockedIn = *account.LockedUntil()
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
	}
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
	), nil
}

var _ AccountReaderWriter = (*AccountManager)(nil)
