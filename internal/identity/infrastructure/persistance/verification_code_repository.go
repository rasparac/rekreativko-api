package persistance

import (
	"context"
	"database/sql"
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
	VerificationCodeFilter struct {
		AccountID uuid.UUID
		Code      string
	}

	VerificationCodeReaderWriter interface {
		CreateVerificationCode(ctx context.Context, code *domain.VerificationCode) error
		GetVerificationCodesBy(ctx context.Context, filter VerificationCodeFilter) ([]*domain.VerificationCode, error)
		MarkAsUsed(ctx context.Context, code string) error
		DeleteExpiredVerificationCodes(ctx context.Context) error
	}

	verificationCodeManager struct {
		db     *pgxpool.Pool
		tx     *postgres.TransactionManager
		logger *logger.Logger
	}

	verificationCodeModel struct {
		ID        uuid.UUID
		AccountID uuid.UUID
		Code      string
		CodeType  string
		ExpiresAt time.Time
		CreatedAt time.Time
		UsedAt    sql.Null[time.Time]
	}
)

func NewVerificationCodeManager(
	db *pgxpool.Pool,
	tx *postgres.TransactionManager,
	logger *logger.Logger,
) *verificationCodeManager {
	return &verificationCodeManager{
		db:     db,
		tx:     tx,
		logger: logger,
	}
}

func (m *verificationCodeManager) CreateVerificationCode(ctx context.Context, code *domain.VerificationCode) error {
	var (
		query = `
		INSERT INTO identity.verification_codes (
			id,
			account_id,
			code,
			type,
			expires_at,
			created_at
		) VALUES ($1, $2, $3, $4, $5, $6)
		`
		model = toVerificationCodeModel(code)
	)

	_, err := m.tx.Querier(ctx).Exec(
		ctx,
		query,
		model.ID,
		model.AccountID,
		model.Code,
		model.CodeType,
		model.ExpiresAt,
		model.CreatedAt,
	)

	return err
}

func (m *verificationCodeManager) GetVerificationCodesBy(ctx context.Context, filter VerificationCodeFilter) ([]*domain.VerificationCode, error) {
	var (
		query = `
		SELECT
			id,
			account_id,
			code,
			type,
			expires_at,
			created_at,
			used_at
		FROM identity.verification_codes
		WHERE %s
		ORDER BY created_at DESC
	`
		args   = []any{}
		filtes = []string{}
	)

	if filter.AccountID != uuid.Nil {
		args = append(args, filter.AccountID)
		filtes = append(filtes, fmt.Sprintf("account_id = $%d", len(args)))
	}

	if filter.Code != "" {
		args = append(args, filter.Code)
		filtes = append(filtes, fmt.Sprintf("code = $%d", len(args)))
	}

	query = fmt.Sprintf(query, strings.Join(filtes, " AND "))

	rows, err := m.tx.Querier(ctx).Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}

	result, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (*domain.VerificationCode, error) {
		var code verificationCodeModel
		if err := rows.Scan(
			&code.ID,
			&code.AccountID,
			&code.Code,
			&code.CodeType,
			&code.ExpiresAt,
			&code.CreatedAt,
			&code.UsedAt,
		); err != nil {
			return nil, err
		}

		return toDomainVerificationCode(code), nil
	})
	if err != nil {
		return nil, err
	}

	return result, nil
}

func (m *verificationCodeManager) MarkAsUsed(ctx context.Context, code string) error {
	query := `
	UPDATE
		identity.verification_codes
	SET
		used_at = NOW()
	WHERE code = $1 AND used_at IS NULL
	`

	cmdTag, err := m.tx.Querier(ctx).Exec(ctx, query, code)
	if err != nil {
		return err
	}

	if cmdTag.RowsAffected() == 0 {
		return domain.ErrVerificationCodeNotFound
	}

	return nil
}

func (m *verificationCodeManager) DeleteExpiredVerificationCodes(ctx context.Context) error {

	query := `
	DELETE FROM
		identity.verification_codes
	WHERE expires_at < NOW()
	`

	_, err := m.tx.Querier(ctx).Exec(ctx, query)
	return err
}

func toVerificationCodeModel(verificationCode *domain.VerificationCode) *verificationCodeModel {
	var usedAt time.Time
	if verificationCode.UsedAt() != nil {
		usedAt = *verificationCode.UsedAt()
	}

	return &verificationCodeModel{
		ID:        verificationCode.ID(),
		AccountID: verificationCode.AccountID(),
		Code:      verificationCode.Code(),
		CodeType:  verificationCode.CodeType().String(),
		ExpiresAt: verificationCode.ExpiresAt(),
		CreatedAt: verificationCode.CreatedAt(),
		UsedAt: sql.Null[time.Time]{
			V:     usedAt,
			Valid: !usedAt.IsZero(),
		},
	}
}

func toDomainVerificationCode(verificationCode verificationCodeModel) *domain.VerificationCode {
	return domain.ReconstructVerificationCode(
		verificationCode.ID,
		verificationCode.AccountID,
		verificationCode.Code,
		domain.CodeType(verificationCode.CodeType),
		verificationCode.ExpiresAt,
		verificationCode.CreatedAt,
		&verificationCode.UsedAt.V,
	)
}

var _ VerificationCodeReaderWriter = (*verificationCodeManager)(nil)
