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

type memberModel struct {
	id              uuid.UUID
	activityGroupID uuid.UUID
	userID          uuid.UUID
	role            string
	status          string
	isPriority      bool
	joinedAt        time.Time
	decidedAt       sql.NullTime
	leftAt          sql.NullTime
}

// MemberRepository defines the interface for member persistence
type MemberRepository interface {
	CreateMember(ctx context.Context, member *domain.Member) error
	UpdateMember(ctx context.Context, member *domain.Member) error
	GetMemberByID(ctx context.Context, id uuid.UUID) (*domain.Member, error)
	GetMemberByGroupAndUser(ctx context.Context, activityGroupID, userID uuid.UUID) (*domain.Member, error)
	ListMembers(ctx context.Context, filter MemberFilter) ([]*domain.Member, error)
	DeleteMember(ctx context.Context, id uuid.UUID) error
}

// MemberFilter defines query filters for listing members
type MemberFilter struct {
	ActivityGroupID *uuid.UUID
	UserID          *uuid.UUID
	Status          *domain.MemberStatus
	Role            *domain.MemberRole
	Limit           int
	Offset          int
}

type memberManager struct {
	tx     *postgres.TransactionManager
	logger *logger.Logger
}

// NewMemberRepository creates a new member repository
func NewMemberRepository(
	tx *postgres.TransactionManager,
	logger *logger.Logger,
) MemberRepository {
	return &memberManager{
		tx:     tx,
		logger: logger,
	}
}

func (m *memberManager) CreateMember(ctx context.Context, member *domain.Member) error {
	model := memberModelFromDomain(member)

	query := `
		INSERT INTO activity.member (
			id,
			activity_group_id,
			account_id,
			role,
			status,
			is_priority,
			joined_at,
			decided_at,
			left_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`

	q := m.tx.Querier(ctx)

	_, err := q.Exec(
		ctx,
		query,
		model.id,
		model.activityGroupID,
		model.userID,
		model.role,
		model.status,
		model.isPriority,
		model.joinedAt,
		model.decidedAt,
		model.leftAt,
	)
	if err != nil {
		return fmt.Errorf("failed to create member: %w", err)
	}

	return nil
}

func (m *memberManager) UpdateMember(ctx context.Context, member *domain.Member) error {
	model := memberModelFromDomain(member)

	query := `
		UPDATE activity.member
		SET
			role = $1,
			status = $2,
			is_priority = $3,
			decided_at = $4,
			left_at = $5
		WHERE id = $6
	`

	q := m.tx.Querier(ctx)

	_, err := q.Exec(
		ctx,
		query,
		model.role,
		model.status,
		model.isPriority,
		model.decidedAt,
		model.leftAt,
		model.id,
	)
	if err != nil {
		return fmt.Errorf("failed to update member: %w", err)
	}

	return nil
}

func (m *memberManager) GetMemberByID(ctx context.Context, id uuid.UUID) (*domain.Member, error) {
	query := `
		SELECT
			id,
			activity_group_id,
			account_id,
			role,
			status,
			is_priority,
			joined_at,
			decided_at,
			left_at
		FROM activity.member
		WHERE id = $1 AND deleted_at IS NULL
	`

	q := m.tx.Querier(ctx)

	var model memberModel
	err := q.QueryRow(ctx, query, id).Scan(
		&model.id,
		&model.activityGroupID,
		&model.userID,
		&model.role,
		&model.status,
		&model.isPriority,
		&model.joinedAt,
		&model.decidedAt,
		&model.leftAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("member not found: %w", err)
		}
		return nil, fmt.Errorf("failed to get member: %w", err)
	}

	return memberModelToDomain(&model)
}

func (m *memberManager) GetMemberByGroupAndUser(
	ctx context.Context,
	activityGroupID, userID uuid.UUID,
) (*domain.Member, error) {
	query := `
		SELECT
			id,
			activity_group_id,
			account_id,
			role,
			status,
			is_priority,
			joined_at,
			decided_at,
			left_at
		FROM activity.member
		WHERE activity_group_id = $1
		  AND account_id = $2
		  AND deleted_at IS NULL
	`

	q := m.tx.Querier(ctx)

	var model memberModel
	err := q.QueryRow(ctx, query, activityGroupID, userID).Scan(
		&model.id,
		&model.activityGroupID,
		&model.userID,
		&model.role,
		&model.status,
		&model.isPriority,
		&model.joinedAt,
		&model.decidedAt,
		&model.leftAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("member not found: %w", err)
		}
		return nil, fmt.Errorf("failed to get member: %w", err)
	}

	return memberModelToDomain(&model)
}

func (m *memberManager) ListMembers(ctx context.Context, filter MemberFilter) ([]*domain.Member, error) {
	var (
		conditions []string
		args       []interface{}
		argIndex   = 1
	)

	// Base query
	query := `
		SELECT
			id,
			activity_group_id,
			account_id,
			role,
			status,
			is_priority,
			joined_at,
			decided_at,
			left_at
		FROM activity.member
		WHERE deleted_at IS NULL
	`

	// Add filters
	if filter.ActivityGroupID != nil {
		conditions = append(conditions, fmt.Sprintf("activity_group_id = $%d", argIndex))
		args = append(args, *filter.ActivityGroupID)
		argIndex++
	}

	if filter.UserID != nil {
		conditions = append(conditions, fmt.Sprintf("account_id = $%d", argIndex))
		args = append(args, *filter.UserID)
		argIndex++
	}

	if filter.Status != nil {
		conditions = append(conditions, fmt.Sprintf("status = $%d", argIndex))
		args = append(args, filter.Status.String())
		argIndex++
	}

	if filter.Role != nil {
		conditions = append(conditions, fmt.Sprintf("role = $%d", argIndex))
		args = append(args, filter.Role.String())
		argIndex++
	}

	// Build WHERE clause
	if len(conditions) > 0 {
		query += " AND " + strings.Join(conditions, " AND ")
	}

	// Add ordering
	query += " ORDER BY joined_at DESC"

	// Add pagination
	if filter.Limit > 0 {
		query += fmt.Sprintf(" LIMIT $%d", argIndex)
		args = append(args, filter.Limit)
		argIndex++
	}

	if filter.Offset > 0 {
		query += fmt.Sprintf(" OFFSET $%d", argIndex)
		args = append(args, filter.Offset)
		argIndex++
	}

	q := m.tx.Querier(ctx)

	rows, err := q.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to list members: %w", err)
	}
	defer rows.Close()

	var members []*domain.Member
	for rows.Next() {
		var model memberModel
		err := rows.Scan(
			&model.id,
			&model.activityGroupID,
			&model.userID,
			&model.role,
			&model.status,
			&model.isPriority,
			&model.joinedAt,
			&model.decidedAt,
			&model.leftAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan member: %w", err)
		}

		member, err := memberModelToDomain(&model)
		if err != nil {
			return nil, fmt.Errorf("failed to convert member: %w", err)
		}

		members = append(members, member)
	}

	if rows.Err() != nil {
		return nil, fmt.Errorf("error iterating members: %w", rows.Err())
	}

	return members, nil
}

func (m *memberManager) DeleteMember(ctx context.Context, id uuid.UUID) error {
	query := `
		UPDATE activity.member
		SET deleted_at = NOW()
		WHERE id = $1
	`

	q := m.tx.Querier(ctx)

	_, err := q.Exec(ctx, query, id)
	if err != nil {
		return fmt.Errorf("failed to delete member: %w", err)
	}

	return nil
}

// memberModelFromDomain converts domain Member to database model
func memberModelFromDomain(member *domain.Member) *memberModel {
	model := &memberModel{
		id:              member.ID(),
		activityGroupID: member.ActivityGroupID(),
		userID:          member.UserID(),
		role:            member.Role().String(),
		status:          member.Status().String(),
		isPriority:      member.IsPriority(),
		joinedAt:        member.JoinedAt(),
	}

	if decidedAt := member.DecidedAt(); decidedAt != nil {
		model.decidedAt = sql.NullTime{Time: *decidedAt, Valid: true}
	}

	// Note: leftAt is not exposed in domain.Member, so we leave it as zero value
	// The domain uses DecidedAt for status transitions

	return model
}

// memberModelToDomain converts database model to domain Member
func memberModelToDomain(model *memberModel) (*domain.Member, error) {
	role := domain.MemberRole(model.role)
	if !role.IsValid() {
		return nil, fmt.Errorf("invalid member role: %s", model.role)
	}

	status := domain.MemberStatus(model.status)
	if !status.IsValid() {
		return nil, fmt.Errorf("invalid member status: %s", model.status)
	}

	var decidedAt *time.Time
	if model.decidedAt.Valid {
		decidedAt = &model.decidedAt.Time
	}

	var leftAt *time.Time
	if model.leftAt.Valid {
		leftAt = &model.leftAt.Time
	}

	return domain.ReconstructMember(
		model.id,
		model.activityGroupID,
		model.userID,
		role,
		status,
		model.isPriority,
		model.joinedAt,
		decidedAt,
		leftAt,
	), nil
}
