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

type groupInviteModel struct {
	id              uuid.UUID
	activityGroupID uuid.UUID
	invitedUserID   uuid.UUID
	invitedByUserID uuid.UUID
	status          string
	createdAt       time.Time
	expiresAt       time.Time
	respondedAt     sql.NullTime
}

// ListPendingInvitesFilter contains parameters for listing a user's pending invites
type ListPendingInvitesFilter struct {
	InvitedUserID uuid.UUID
	Limit         int
	PageToken     string
}

// GroupInviteRepository defines the interface for group invite persistence
type GroupInviteRepository interface {
	CreateInvite(ctx context.Context, invite *domain.GroupInvite) error
	UpdateInvite(ctx context.Context, invite *domain.GroupInvite) error
	GetInviteByID(ctx context.Context, id uuid.UUID) (*domain.GroupInvite, error)
	GetPendingInviteByGroupAndUser(ctx context.Context, activityGroupID, invitedUserID uuid.UUID) (*domain.GroupInvite, error)
	ListPendingInvitesForUser(ctx context.Context, filter ListPendingInvitesFilter) ([]*domain.GroupInvite, string, error)
	// FindExpiredPendingInvites returns every still-pending invite whose expiry has
	// passed. Unbounded on purpose - only ever called from the cron sweep.
	FindExpiredPendingInvites(ctx context.Context) ([]*domain.GroupInvite, error)
}

type groupInviteManager struct {
	tx     *postgres.TransactionManager
	logger *logger.Logger
}

// NewGroupInviteRepository creates a new group invite repository
func NewGroupInviteRepository(
	tx *postgres.TransactionManager,
	logger *logger.Logger,
) GroupInviteRepository {
	return &groupInviteManager{
		tx:     tx,
		logger: logger,
	}
}

func (m *groupInviteManager) CreateInvite(ctx context.Context, invite *domain.GroupInvite) error {
	model := groupInviteModelFromDomain(invite)

	query := `
		INSERT INTO activity.group_invites (
			id,
			activity_group_id,
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
		model.activityGroupID,
		model.invitedUserID,
		model.invitedByUserID,
		model.status,
		model.expiresAt,
		model.createdAt,
		model.respondedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to create invite: %w", err)
	}

	return nil
}

func (m *groupInviteManager) UpdateInvite(ctx context.Context, invite *domain.GroupInvite) error {
	model := groupInviteModelFromDomain(invite)

	query := `
		UPDATE activity.group_invites
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
		return fmt.Errorf("failed to update invite: %w", err)
	}

	return nil
}

func (m *groupInviteManager) GetInviteByID(ctx context.Context, id uuid.UUID) (*domain.GroupInvite, error) {
	query := `
		SELECT
			id,
			activity_group_id,
			invited_user_id,
			invited_by_user_id,
			status,
			created_at,
			expires_at,
			responded_at
		FROM activity.group_invites
		WHERE id = $1
	`

	q := m.tx.Querier(ctx)

	var model groupInviteModel
	err := q.QueryRow(ctx, query, id).Scan(
		&model.id,
		&model.activityGroupID,
		&model.invitedUserID,
		&model.invitedByUserID,
		&model.status,
		&model.createdAt,
		&model.expiresAt,
		&model.respondedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrInviteNotFound
		}
		return nil, fmt.Errorf("failed to get invite: %w", err)
	}

	return groupInviteModelToDomain(&model)
}

func (m *groupInviteManager) GetPendingInviteByGroupAndUser(
	ctx context.Context,
	activityGroupID, invitedUserID uuid.UUID,
) (*domain.GroupInvite, error) {
	query := `
		SELECT
			id,
			activity_group_id,
			invited_user_id,
			invited_by_user_id,
			status,
			created_at,
			expires_at,
			responded_at
		FROM activity.group_invites
		WHERE activity_group_id = $1
		  AND invited_user_id = $2
		  AND status = 'pending'
	`

	q := m.tx.Querier(ctx)

	var model groupInviteModel
	err := q.QueryRow(ctx, query, activityGroupID, invitedUserID).Scan(
		&model.id,
		&model.activityGroupID,
		&model.invitedUserID,
		&model.invitedByUserID,
		&model.status,
		&model.createdAt,
		&model.expiresAt,
		&model.respondedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrInviteNotFound
		}
		return nil, fmt.Errorf("failed to get pending invite: %w", err)
	}

	return groupInviteModelToDomain(&model)
}

func (m *groupInviteManager) ListPendingInvitesForUser(
	ctx context.Context,
	filter ListPendingInvitesFilter,
) ([]*domain.GroupInvite, string, error) {
	cursor, err := postgres.DecodePageToken(filter.PageToken)
	if err != nil {
		return nil, "", err
	}

	conditions := []string{"invited_user_id = $1", "status = 'pending'"}
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

	query := `
		SELECT
			id,
			activity_group_id,
			invited_user_id,
			invited_by_user_id,
			status,
			created_at,
			expires_at,
			responded_at
		FROM activity.group_invites
		WHERE ` + strings.Join(conditions, " AND ") + `
		ORDER BY created_at DESC, id DESC`

	if filter.Limit > 0 {
		query += fmt.Sprintf(" LIMIT $%d", argIndex)
		args = append(args, filter.Limit+1)
		argIndex++
	}

	q := m.tx.Querier(ctx)

	rows, err := q.Query(ctx, query, args...)
	if err != nil {
		return nil, "", fmt.Errorf("failed to list invites: %w", err)
	}
	defer rows.Close()

	var invites []*domain.GroupInvite
	for rows.Next() {
		var model groupInviteModel
		err := rows.Scan(
			&model.id,
			&model.activityGroupID,
			&model.invitedUserID,
			&model.invitedByUserID,
			&model.status,
			&model.createdAt,
			&model.expiresAt,
			&model.respondedAt,
		)
		if err != nil {
			return nil, "", fmt.Errorf("failed to scan invite: %w", err)
		}

		invite, err := groupInviteModelToDomain(&model)
		if err != nil {
			return nil, "", fmt.Errorf("failed to convert invite: %w", err)
		}

		invites = append(invites, invite)
	}

	if rows.Err() != nil {
		return nil, "", fmt.Errorf("error iterating invites: %w", rows.Err())
	}

	page, nextPageToken := postgres.BuildPage(invites, filter.Limit, func(invite *domain.GroupInvite) (string, uuid.UUID) {
		return invite.CreatedAt().UTC().Format(time.RFC3339Nano), invite.ID()
	})

	return page, nextPageToken, nil
}

func (m *groupInviteManager) FindExpiredPendingInvites(
	ctx context.Context,
) ([]*domain.GroupInvite, error) {
	query := `
		SELECT
			id,
			activity_group_id,
			invited_user_id,
			invited_by_user_id,
			status,
			created_at,
			expires_at,
			responded_at
		FROM activity.group_invites
		WHERE status = 'pending' AND expires_at < NOW()
	`

	q := m.tx.Querier(ctx)

	rows, err := q.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to find expired invites: %w", err)
	}
	defer rows.Close()

	var invites []*domain.GroupInvite
	for rows.Next() {
		var model groupInviteModel
		err := rows.Scan(
			&model.id,
			&model.activityGroupID,
			&model.invitedUserID,
			&model.invitedByUserID,
			&model.status,
			&model.createdAt,
			&model.expiresAt,
			&model.respondedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan invite: %w", err)
		}

		invite, err := groupInviteModelToDomain(&model)
		if err != nil {
			return nil, fmt.Errorf("failed to convert invite: %w", err)
		}

		invites = append(invites, invite)
	}

	if rows.Err() != nil {
		return nil, fmt.Errorf("error iterating invites: %w", rows.Err())
	}

	return invites, nil
}

func groupInviteModelFromDomain(invite *domain.GroupInvite) *groupInviteModel {
	model := &groupInviteModel{
		id:              invite.ID(),
		activityGroupID: invite.ActivityGroupID(),
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

func groupInviteModelToDomain(model *groupInviteModel) (*domain.GroupInvite, error) {
	status := domain.InviteStatus(model.status)
	if !status.IsValid() {
		return nil, fmt.Errorf("invalid invite status: %s", model.status)
	}

	var respondedAt *time.Time
	if model.respondedAt.Valid {
		respondedAt = &model.respondedAt.Time
	}

	return domain.ReconstructGroupInvite(
		model.id,
		model.activityGroupID,
		model.invitedUserID,
		model.invitedByUserID,
		status,
		model.createdAt,
		model.expiresAt,
		respondedAt,
	), nil
}
