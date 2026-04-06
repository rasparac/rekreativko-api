package persistance

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rasparac/rekreativko-api/internal/identity/domain"
	"github.com/rasparac/rekreativko-api/internal/shared/logger"
	"github.com/rasparac/rekreativko-api/internal/shared/store/postgres"
)

type (
	RefreshTokenReaderWriter interface {
		CreateRefreshToken(ctx context.Context, token *domain.RefreshToken) error
		GetTokenBy(ctx context.Context, filter RefreshTokenFilter) (*domain.RefreshToken, error)
		Revoke(ctx context.Context, uuid uuid.UUID) error
		RevokeAll(ctx context.Context, accountID uuid.UUID) error
		DeleteExpired(ctx context.Context) error
	}

	refreshTokenModel struct {
		ID        uuid.UUID
		AccountID uuid.UUID
		Token     string
		ExpiresAt time.Time
		CreatedAt time.Time
		RevokedAt sql.NullTime
	}

	RefreshTokenFilter struct {
		AccountID uuid.UUID
		Token     string
	}

	refreshTokenManager struct {
		db     *pgxpool.Pool
		tx     *postgres.TransactionManager
		logger *logger.Logger
	}
)

func (rtf RefreshTokenFilter) Validate() error {
	if rtf.AccountID == uuid.Nil && rtf.Token == "" {
		return errors.New("missing filter")
	}

	return nil
}

func NewRefreshTokenManager(
	db *pgxpool.Pool,
	tx *postgres.TransactionManager,
	logger *logger.Logger,
) *refreshTokenManager {
	return &refreshTokenManager{
		db:     db,
		tx:     tx,
		logger: logger,
	}
}

func (m *refreshTokenManager) CreateRefreshToken(ctx context.Context, token *domain.RefreshToken) error {
	var (
		model = toRefreshTokenModel(token)
		query = `
		INSERT INTO identity.refresh_tokens
			(id, account_id, token, expires_at, created_at, revoked_at)
		VALUES
			($1, $2, $3, $4, $5, $6)
		`
	)

	_, err := m.tx.Querier(ctx).Exec(
		ctx,
		query,
		model.ID,
		model.AccountID,
		model.Token,
		model.ExpiresAt,
		model.CreatedAt,
		model.RevokedAt,
	)

	return err
}

func (m *refreshTokenManager) GetTokenBy(ctx context.Context, filter RefreshTokenFilter) (*domain.RefreshToken, error) {
	if err := filter.Validate(); err != nil {
		return nil, err
	}

	var (
		baseQuery = `SELECT
		id,
		account_id,
		token,
		expires_at,
		created_at,
		revoked_at
		FROM identity.refresh_tokens
		WHERE %s`
		filters []string
		args    []any
	)

	if filter.AccountID != uuid.Nil {
		args = append(args, filter.AccountID)
		filters = append(filters, fmt.Sprintf("account_id = $%d", len(args)))
	}

	if filter.Token != "" {
		args = append(args, filter.Token)
		filters = append(filters, fmt.Sprintf("token = $%d", len(args)))
	}

	var (
		model refreshTokenModel
		query = fmt.Sprintf(
			baseQuery,
			strings.Join(filters, " AND "),
		)
	)

	err := m.db.QueryRow(ctx, query, args...).Scan(
		&model.ID,
		&model.AccountID,
		&model.Token,
		&model.ExpiresAt,
		&model.CreatedAt,
		&model.RevokedAt,
	)

	return toDomainRefreshToken(model), err
}

func (m *refreshTokenManager) Revoke(ctx context.Context, uuid uuid.UUID) error {
	query := `
	UPDATE
		identity.refresh_tokens
	SET
		revoked_at = NOW()
	WHERE id = $1 AND revoked_at IS NULL
	`

	cmdTag, err := m.tx.Querier(ctx).Exec(ctx, query, uuid)

	if cmdTag.RowsAffected() == 0 {
		return domain.ErrRefreshTokenNotFound
	}

	return err
}

func (m *refreshTokenManager) RevokeAll(ctx context.Context, accountID uuid.UUID) error {
	query := `
	UPDATE
		identity.refresh_tokens
	SET
		revoked_at = NOW()
	WHERE account_id = $1 AND revoked_at IS NULL
	`

	_, err := m.tx.Querier(ctx).Exec(ctx, query, accountID)

	return err
}

func (m *refreshTokenManager) DeleteExpired(ctx context.Context) error {
	query := `
	DELETE FROM
		identity.refresh_tokens
	WHERE expires_at < NOW()
	`

	_, err := m.tx.Querier(ctx).Exec(ctx, query)

	return err
}

func toRefreshTokenModel(refreshToken *domain.RefreshToken) *refreshTokenModel {
	var revokedAt time.Time
	if refreshToken.RevokedAt() != nil {
		revokedAt = *refreshToken.RevokedAt()
	}

	return &refreshTokenModel{
		ID:        refreshToken.ID(),
		AccountID: refreshToken.AccountID(),
		Token:     refreshToken.Token(),
		ExpiresAt: refreshToken.ExpiresAt(),
		CreatedAt: refreshToken.CreatedAt(),
		RevokedAt: sql.NullTime{
			Time:  revokedAt,
			Valid: !revokedAt.IsZero(),
		},
	}
}

func toDomainRefreshToken(refreshToken refreshTokenModel) *domain.RefreshToken {
	return domain.ReconstructRefreshToken(
		refreshToken.ID,
		refreshToken.AccountID,
		refreshToken.Token,
		refreshToken.ExpiresAt,
		refreshToken.CreatedAt,
		&refreshToken.RevokedAt.Time,
	)
}

var _ RefreshTokenReaderWriter = (*refreshTokenManager)(nil)
