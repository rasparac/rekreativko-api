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
	"github.com/rasparac/rekreativko-api/activity/internal/domain"
	"github.com/rasparac/rekreativko-api/shared/logger"
	"github.com/rasparac/rekreativko-api/shared/store/postgres"
)

const sessionInviteColumns = `
			id,
			session_id,
			invited_user_id,
			invited_by_user_id,
			status,
			created_at,
			expires_at,
			responded_at`

type sessionInviteModel struct {
	id              uuid.UUID
	sessionID       uuid.UUID
	invitedUserID   uuid.UUID
	invitedByUserID uuid.UUID
	status          string
	createdAt       time.Time
	expiresAt       time.Time
	respondedAt     sql.NullTime
}

// SessionInviteRepository defines the interface for session invite persistence
type SessionInviteRepository interface {
	CreateInvite(ctx context.Context, invite *domain.SessionInvite) error
	UpdateInvite(ctx context.Context, invite *domain.SessionInvite) error
	GetInviteByID(ctx context.Context, id uuid.UUID) (*domain.SessionInvite, error)
	GetPendingInviteBySessionAndUser(ctx context.Context, sessionID, invitedUserID uuid.UUID) (*domain.SessionInvite, error)
	ListPendingInvitesForUser(ctx context.Context, filter ListPendingInvitesFilter) ([]*domain.SessionInvite, string, error)
	// FindExpiredPendingInvites returns every still-pending invite whose expiry has
	// passed. Unbounded on purpose - only ever called from the cron sweep.
	FindExpiredPendingInvites(ctx context.Context) ([]*domain.SessionInvite, error)
}

type sessionInviteManager struct {
	tx     *postgres.TransactionManager
	logger *logger.Logger
}

// NewSessionInviteRepository creates a new session invite repository
func NewSessionInviteRepository(
	tx *postgres.TransactionManager,
	logger *logger.Logger,
) SessionInviteRepository {
	return &sessionInviteManager{
		tx:     tx,
		logger: logger,
	}
}

func (m *sessionInviteManager) CreateInvite(ctx context.Context, invite *domain.SessionInvite) error {
	model := sessionInviteModelFromDomain(invite)

	query := `
		INSERT INTO activity.session_invites (
			id,
			session_id,
			invited_user_id,
			invited_by_user_id,
			status,
			expires_at,
			created_at,
			responded_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`

	q := m.tx.Querier(ctx)

	_, err := q.Exec(
		ctx,
		query,
		model.id,
		model.sessionID,
		model.invitedUserID,
		model.invitedByUserID,
		model.status,
		model.expiresAt,
		model.createdAt,
		model.respondedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to create session invite: %w", err)
	}

	return nil
}

func (m *sessionInviteManager) UpdateInvite(ctx context.Context, invite *domain.SessionInvite) error {
	model := sessionInviteModelFromDomain(invite)

	query := `
		UPDATE activity.session_invites
		SET
			status = $1,
			responded_at = $2
		WHERE id = $3
	`

	q := m.tx.Querier(ctx)

	_, err := q.Exec(
		ctx,
		query,
		model.status,
		model.respondedAt,
		model.id,
	)
	if err != nil {
		return fmt.Errorf("failed to update session invite: %w", err)
	}

	return nil
}

func (m *sessionInviteManager) GetInviteByID(ctx context.Context, id uuid.UUID) (*domain.SessionInvite, error) {
	query := `SELECT` + sessionInviteColumns + `
		FROM activity.session_invites
		WHERE id = $1
	`

	q := m.tx.Querier(ctx)

	model, err := scanSessionInvite(q.QueryRow(ctx, query, id))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrInviteNotFound
		}
		return nil, fmt.Errorf("failed to get session invite: %w", err)
	}

	return sessionInviteModelToDomain(model)
}

func (m *sessionInviteManager) GetPendingInviteBySessionAndUser(
	ctx context.Context,
	sessionID, invitedUserID uuid.UUID,
) (*domain.SessionInvite, error) {
	query := `SELECT` + sessionInviteColumns + `
		FROM activity.session_invites
		WHERE session_id = $1
		  AND invited_user_id = $2
		  AND status = 'pending'
	`

	q := m.tx.Querier(ctx)

	model, err := scanSessionInvite(q.QueryRow(ctx, query, sessionID, invitedUserID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrInviteNotFound
		}
		return nil, fmt.Errorf("failed to get pending session invite: %w", err)
	}

	return sessionInviteModelToDomain(model)
}

func (m *sessionInviteManager) ListPendingInvitesForUser(
	ctx context.Context,
	filter ListPendingInvitesFilter,
) ([]*domain.SessionInvite, string, error) {
	cursor, err := postgres.DecodePageToken(filter.PageToken)
	if err != nil {
		return nil, "", err
	}

	conditions := []string{"invited_user_id = $1", "status = 'pending'", "expires_at > NOW()"}
	args := []interface{}{filter.InvitedUserID}
	argIndex := 2

	if cursor != nil {
		sortValue, err := time.Parse(time.RFC3339Nano, cursor.SortValue)
		if err != nil {
			return nil, "", postgres.ErrInvalidPageToken
		}
		conditions = append(conditions, fmt.Sprintf("(created_at, id) < ($%d, $%d)", argIndex, argIndex+1))
		args = append(args, sortValue, cursor.ID)
		argIndex += 2
	}

	query := `SELECT` + sessionInviteColumns + `
		FROM activity.session_invites
		WHERE ` + strings.Join(conditions, " AND ") + `
		ORDER BY created_at DESC, id DESC`

	if filter.Limit > 0 {
		query += fmt.Sprintf(" LIMIT $%d", argIndex)
		args = append(args, filter.Limit+1)
	}

	q := m.tx.Querier(ctx)

	rows, err := q.Query(ctx, query, args...)
	if err != nil {
		return nil, "", fmt.Errorf("failed to list session invites: %w", err)
	}
	defer rows.Close()

	invites, err := collectSessionInvites(rows)
	if err != nil {
		return nil, "", err
	}

	page, nextPageToken := postgres.BuildPage(invites, filter.Limit, func(invite *domain.SessionInvite) (string, uuid.UUID) {
		return invite.CreatedAt().UTC().Format(time.RFC3339Nano), invite.ID()
	})

	return page, nextPageToken, nil
}

func (m *sessionInviteManager) FindExpiredPendingInvites(
	ctx context.Context,
) ([]*domain.SessionInvite, error) {
	query := `SELECT` + sessionInviteColumns + `
		FROM activity.session_invites
		WHERE status = 'pending' AND expires_at < NOW()
	`

	q := m.tx.Querier(ctx)

	rows, err := q.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to find expired session invites: %w", err)
	}
	defer rows.Close()

	return collectSessionInvites(rows)
}

func collectSessionInvites(rows pgx.Rows) ([]*domain.SessionInvite, error) {
	var invites []*domain.SessionInvite
	for rows.Next() {
		model, err := scanSessionInvite(rows)
		if err != nil {
			return nil, fmt.Errorf("failed to scan session invite: %w", err)
		}

		invite, err := sessionInviteModelToDomain(model)
		if err != nil {
			return nil, fmt.Errorf("failed to convert session invite: %w", err)
		}

		invites = append(invites, invite)
	}

	if rows.Err() != nil {
		return nil, fmt.Errorf("error iterating session invites: %w", rows.Err())
	}

	return invites, nil
}

func scanSessionInvite(row pgx.Row) (*sessionInviteModel, error) {
	var model sessionInviteModel
	err := row.Scan(
		&model.id,
		&model.sessionID,
		&model.invitedUserID,
		&model.invitedByUserID,
		&model.status,
		&model.createdAt,
		&model.expiresAt,
		&model.respondedAt,
	)
	if err != nil {
		return nil, err
	}

	return &model, nil
}

func sessionInviteModelFromDomain(invite *domain.SessionInvite) *sessionInviteModel {
	model := &sessionInviteModel{
		id:              invite.ID(),
		sessionID:       invite.SessionID(),
		invitedUserID:   invite.InvitedUserID(),
		invitedByUserID: invite.InvitedByID(),
		status:          invite.Status().String(),
		createdAt:       invite.CreatedAt(),
		expiresAt:       invite.ExpiresAt(),
	}

	if respondedAt := invite.RespondedAt(); respondedAt != nil {
		model.respondedAt = sql.NullTime{Time: *respondedAt, Valid: true}
	}

	return model
}

func sessionInviteModelToDomain(model *sessionInviteModel) (*domain.SessionInvite, error) {
	status := domain.InviteStatus(model.status)
	if !status.IsValid() {
		return nil, fmt.Errorf("invalid session invite status: %s", model.status)
	}

	var respondedAt *time.Time
	if model.respondedAt.Valid {
		respondedAt = &model.respondedAt.Time
	}

	return domain.ReconstructSessionInvite(
		model.id,
		model.sessionID,
		model.invitedUserID,
		model.invitedByUserID,
		status,
		model.createdAt,
		model.expiresAt,
		respondedAt,
	), nil
}
