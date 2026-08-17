package persistence

import (
	"context"
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

type attendeeModel struct {
	id              uuid.UUID
	sessionID       uuid.UUID
	activityGroupID uuid.UUID
	userID          uuid.UUID
	status          string
	source          string
	createdAt       time.Time
	updatedAt       time.Time
}

// AttendeeRepository defines the interface for attendee persistence
type AttendeeRepository interface {
	CreateAttendee(ctx context.Context, attendee *domain.Attendee) error
	UpdateAttendee(ctx context.Context, attendee *domain.Attendee) error
	GetAttendeeByID(ctx context.Context, id uuid.UUID) (*domain.Attendee, error)
	GetAttendeeBySessionAndUser(ctx context.Context, sessionID, userID uuid.UUID) (*domain.Attendee, error)
	ListAttendees(ctx context.Context, filter AttendeeFilter) ([]*domain.Attendee, error)
	DeleteAttendee(ctx context.Context, id uuid.UUID) error
	GetFirstPendingAttendee(ctx context.Context, sessionID uuid.UUID) (*domain.Attendee, error)
	CountConfirmedAttendees(ctx context.Context, sessionID uuid.UUID) (int, error)
}

// AttendeeFilter defines query filters for listing attendees
type AttendeeFilter struct {
	SessionID       *uuid.UUID
	ActivityGroupID *uuid.UUID
	UserID          *uuid.UUID
	Status          *domain.AttendeeStatus
	Source          *domain.AttendeeSource
	Limit           int
	Offset          int
}

type attendeeManager struct {
	tx     *postgres.TransactionManager
	logger *logger.Logger
}

// NewAttendeeRepository creates a new attendee repository
func NewAttendeeRepository(
	tx *postgres.TransactionManager,
	logger *logger.Logger,
) AttendeeRepository {
	return &attendeeManager{
		tx:     tx,
		logger: logger,
	}
}

func (a *attendeeManager) CreateAttendee(ctx context.Context, attendee *domain.Attendee) error {
	model := attendeeModelFromDomain(attendee)

	query := `
		INSERT INTO activity.session_attendee (
			id,
			session_id,
			activity_group_id,
			account_id,
			status,
			source,
			created_at,
			updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`

	q := a.tx.Querier(ctx)

	_, err := q.Exec(
		ctx,
		query,
		model.id,
		model.sessionID,
		model.activityGroupID,
		model.userID,
		model.status,
		model.source,
		model.createdAt,
		model.updatedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to create attendee: %w", err)
	}

	return nil
}

func (a *attendeeManager) UpdateAttendee(ctx context.Context, attendee *domain.Attendee) error {
	model := attendeeModelFromDomain(attendee)

	query := `
		UPDATE activity.session_attendee
		SET
			status = $1,
			updated_at = $2
		WHERE id = $3
	`

	q := a.tx.Querier(ctx)

	_, err := q.Exec(
		ctx,
		query,
		model.status,
		model.updatedAt,
		model.id,
	)
	if err != nil {
		return fmt.Errorf("failed to update attendee: %w", err)
	}

	return nil
}

func (a *attendeeManager) GetAttendeeByID(ctx context.Context, id uuid.UUID) (*domain.Attendee, error) {
	query := `
		SELECT
			id,
			session_id,
			activity_group_id,
			account_id,
			status,
			source,
			created_at,
			updated_at
		FROM activity.session_attendee
		WHERE id = $1 AND deleted_at IS NULL
	`

	q := a.tx.Querier(ctx)

	var model attendeeModel
	err := q.QueryRow(ctx, query, id).Scan(
		&model.id,
		&model.sessionID,
		&model.activityGroupID,
		&model.userID,
		&model.status,
		&model.source,
		&model.createdAt,
		&model.updatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("attendee not found: %w", err)
		}
		return nil, fmt.Errorf("failed to get attendee: %w", err)
	}

	return attendeeModelToDomain(&model)
}

func (a *attendeeManager) GetAttendeeBySessionAndUser(
	ctx context.Context,
	sessionID, userID uuid.UUID,
) (*domain.Attendee, error) {
	query := `
		SELECT
			id,
			session_id,
			activity_group_id,
			account_id,
			status,
			source,
			created_at,
			updated_at
		FROM activity.session_attendee
		WHERE session_id = $1
		  AND account_id = $2
		  AND deleted_at IS NULL
	`

	q := a.tx.Querier(ctx)

	var model attendeeModel
	err := q.QueryRow(ctx, query, sessionID, userID).Scan(
		&model.id,
		&model.sessionID,
		&model.activityGroupID,
		&model.userID,
		&model.status,
		&model.source,
		&model.createdAt,
		&model.updatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("attendee not found: %w", err)
		}
		return nil, fmt.Errorf("failed to get attendee: %w", err)
	}

	return attendeeModelToDomain(&model)
}

func (a *attendeeManager) ListAttendees(ctx context.Context, filter AttendeeFilter) ([]*domain.Attendee, error) {
	var (
		conditions []string
		args       []interface{}
		argIndex   = 1
	)

	// Base query
	query := `
		SELECT
			id,
			session_id,
			activity_group_id,
			account_id,
			status,
			source,
			created_at,
			updated_at
		FROM activity.session_attendee
		WHERE deleted_at IS NULL
	`

	// Add filters
	if filter.SessionID != nil {
		conditions = append(conditions, fmt.Sprintf("session_id = $%d", argIndex))
		args = append(args, *filter.SessionID)
		argIndex++
	}

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
		args = append(args, string(*filter.Status))
		argIndex++
	}

	if filter.Source != nil {
		conditions = append(conditions, fmt.Sprintf("source = $%d", argIndex))
		args = append(args, string(*filter.Source))
		argIndex++
	}

	// Build WHERE clause
	if len(conditions) > 0 {
		query += " AND " + strings.Join(conditions, " AND ")
	}

	// Add ordering (FIFO for waitlist)
	query += " ORDER BY created_at ASC"

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

	q := a.tx.Querier(ctx)

	rows, err := q.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to list attendees: %w", err)
	}
	defer rows.Close()

	var attendees []*domain.Attendee
	for rows.Next() {
		var model attendeeModel
		err := rows.Scan(
			&model.id,
			&model.sessionID,
			&model.activityGroupID,
			&model.userID,
			&model.status,
			&model.source,
			&model.createdAt,
			&model.updatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan attendee: %w", err)
		}

		attendee, err := attendeeModelToDomain(&model)
		if err != nil {
			return nil, fmt.Errorf("failed to convert attendee: %w", err)
		}

		attendees = append(attendees, attendee)
	}

	if rows.Err() != nil {
		return nil, fmt.Errorf("error iterating attendees: %w", rows.Err())
	}

	return attendees, nil
}

func (a *attendeeManager) DeleteAttendee(ctx context.Context, id uuid.UUID) error {
	query := `
		UPDATE activity.session_attendee
		SET deleted_at = NOW()
		WHERE id = $1
	`

	q := a.tx.Querier(ctx)

	_, err := q.Exec(ctx, query, id)
	if err != nil {
		return fmt.Errorf("failed to delete attendee: %w", err)
	}

	return nil
}

func (a *attendeeManager) GetFirstPendingAttendee(ctx context.Context, sessionID uuid.UUID) (*domain.Attendee, error) {
	query := `
		SELECT
			id,
			session_id,
			activity_group_id,
			account_id,
			status,
			source,
			created_at,
			updated_at
		FROM activity.session_attendee
		WHERE session_id = $1
		  AND status = 'pending'
		  AND deleted_at IS NULL
		ORDER BY created_at ASC
		LIMIT 1
	`

	q := a.tx.Querier(ctx)

	var model attendeeModel
	err := q.QueryRow(ctx, query, sessionID).Scan(
		&model.id,
		&model.sessionID,
		&model.activityGroupID,
		&model.userID,
		&model.status,
		&model.source,
		&model.createdAt,
		&model.updatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil // No pending attendees
		}
		return nil, fmt.Errorf("failed to get first pending attendee: %w", err)
	}

	return attendeeModelToDomain(&model)
}

func (a *attendeeManager) CountConfirmedAttendees(ctx context.Context, sessionID uuid.UUID) (int, error) {
	query := `
		SELECT COUNT(*)
		FROM activity.session_attendee
		WHERE session_id = $1
		  AND (status = 'going' OR status = 'promoted')
		  AND deleted_at IS NULL
	`

	q := a.tx.Querier(ctx)

	var count int
	err := q.QueryRow(ctx, query, sessionID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count confirmed attendees: %w", err)
	}

	return count, nil
}

// attendeeModelFromDomain converts domain Attendee to database model
func attendeeModelFromDomain(attendee *domain.Attendee) *attendeeModel {
	return &attendeeModel{
		id:              attendee.ID(),
		sessionID:       attendee.SessionID(),
		activityGroupID: attendee.ActivityID(),
		userID:          attendee.UserID(),
		status:          string(attendee.Status()),
		source:          string(attendee.Source()),
		createdAt:       attendee.CreatedAt(),
		updatedAt:       attendee.UpdatedAt(),
	}
}

// attendeeModelToDomain converts database model to domain Attendee
func attendeeModelToDomain(model *attendeeModel) (*domain.Attendee, error) {
	status := domain.AttendeeStatus(model.status)
	if !status.IsValid() {
		return nil, fmt.Errorf("invalid attendee status: %s", model.status)
	}

	source := domain.AttendeeSource(model.source)

	return domain.ReconstructAttendee(
		model.id,
		model.sessionID,
		model.activityGroupID,
		model.userID,
		status,
		source,
		model.createdAt,
		model.updatedAt,
	), nil
}
