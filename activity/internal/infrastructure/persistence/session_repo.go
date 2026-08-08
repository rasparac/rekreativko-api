package persistence

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/rasparac/rekreativko-api/activity/internal/domain"
	"github.com/rasparac/rekreativko-api/shared/logger"
	"github.com/rasparac/rekreativko-api/shared/store/postgres"
)

type sessionModel struct {
	id                uuid.UUID
	activityGroupID   uuid.UUID
	createdByID       uuid.UUID
	sessionTemplateID sql.NullString // uuid as string, nullable
	locationCity      string
	locationCountry   string
	locationLat       float64
	locationLng       float64
	startTime         sql.NullTime
	endTime           sql.NullTime
	capacity          sql.NullInt32
	status            string
	isRecurring       bool
	note              sql.NullString
	createdAt         sql.NullTime
	updatedAt         sql.NullTime
	cancelledAt       sql.NullTime
	startedAt         sql.NullTime
	completedAt       sql.NullTime
}

// SessionRepository defines the interface for session persistence
type SessionRepository interface {
	CreateSession(ctx context.Context, session *domain.Session) error
	UpdateSession(ctx context.Context, session *domain.Session) error
	GetSessionByID(ctx context.Context, id uuid.UUID) (*domain.Session, error)
	ListSessions(ctx context.Context, filter SessionFilter) ([]*domain.Session, error)
	DeleteSession(ctx context.Context, id uuid.UUID) error
}

// SessionFilter defines query filters for listing sessions
type SessionFilter struct {
	ActivityGroupID   *uuid.UUID
	SessionTemplateID *uuid.UUID
	Status            *domain.SessionStatus
	IsRecurring       *bool
	StartTimeFrom     *sql.NullTime
	StartTimeTo       *sql.NullTime
	Limit             int
	Offset            int
}

type sessionManager struct {
	tx     *postgres.TransactionManager
	logger *logger.Logger
}

// NewSessionManager creates a new session repository
func NewSessionManager(
	tx *postgres.TransactionManager,
	logger *logger.Logger,
) SessionRepository {
	return &sessionManager{
		tx:     tx,
		logger: logger,
	}
}

func (m *sessionManager) CreateSession(ctx context.Context, session *domain.Session) error {
	model := sessionModelFromDomain(session)

	query := `
		INSERT INTO activity.session (
			id, activity_group_id, created_by_id, session_template_id,
			location_city, location_country, location_lat, location_lng,
			start_time, end_time, capacity, status, is_recurring, note,
			created_at, updated_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16
		)
	`

	q := m.tx.Querier(ctx)

	_, err := q.Exec(
		ctx,
		query,
		model.id,
		model.activityGroupID,
		model.createdByID,
		model.sessionTemplateID,
		model.locationCity,
		model.locationCountry,
		model.locationLat,
		model.locationLng,
		model.startTime,
		model.endTime,
		model.capacity,
		model.status,
		model.isRecurring,
		model.note,
		model.createdAt,
		model.updatedAt,
	)

	if err != nil {
		return fmt.Errorf("failed to create session: %w", err)
	}

	return nil
}

func (m *sessionManager) UpdateSession(ctx context.Context, session *domain.Session) error {
	model := sessionModelFromDomain(session)

	query := `
		UPDATE activity.session
		SET
			location_city = $2,
			location_country = $3,
			location_lat = $4,
			location_lng = $5,
			start_time = $6,
			end_time = $7,
			capacity = $8,
			status = $9,
			note = $10,
			updated_at = $11,
			cancelled_at = $12,
			started_at = $13,
			completed_at = $14
		WHERE id = $1
	`

	q := m.tx.Querier(ctx)

	result, err := q.Exec(
		ctx,
		query,
		model.id,
		model.locationCity,
		model.locationCountry,
		model.locationLat,
		model.locationLng,
		model.startTime,
		model.endTime,
		model.capacity,
		model.status,
		model.note,
		model.updatedAt,
		model.cancelledAt,
		model.startedAt,
		model.completedAt,
	)

	if err != nil {
		return fmt.Errorf("failed to update session: %w", err)
	}

	if result.RowsAffected() == 0 {
		return domain.ErrSessionNotFound
	}

	return nil
}

func (m *sessionManager) GetSessionByID(ctx context.Context, id uuid.UUID) (*domain.Session, error) {
	query := `
		SELECT
			id, activity_group_id, created_by_id, session_template_id,
			location_city, location_country, location_lat, location_lng,
			start_time, end_time, capacity, status, is_recurring, note,
			created_at, updated_at, cancelled_at, started_at, completed_at
		FROM activity.session
		WHERE id = $1
	`

	q := m.tx.Querier(ctx)

	var model sessionModel
	err := q.QueryRow(ctx, query, id).Scan(
		&model.id,
		&model.activityGroupID,
		&model.createdByID,
		&model.sessionTemplateID,
		&model.locationCity,
		&model.locationCountry,
		&model.locationLat,
		&model.locationLng,
		&model.startTime,
		&model.endTime,
		&model.capacity,
		&model.status,
		&model.isRecurring,
		&model.note,
		&model.createdAt,
		&model.updatedAt,
		&model.cancelledAt,
		&model.startedAt,
		&model.completedAt,
	)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrSessionNotFound
		}
		return nil, fmt.Errorf("failed to get session: %w", err)
	}

	return sessionModelToDomain(&model)
}

func (m *sessionManager) ListSessions(ctx context.Context, filter SessionFilter) ([]*domain.Session, error) {
	qb := &postgres.QueryBuilder{
		BaseQuery: `
		SELECT
			id,
			activity_group_id,
			created_by_id,
			session_template_id,
			location_city,
			location_country,
			location_lat,
			location_lng,
			start_time,
			end_time,
			capacity,
			status,
			is_recurring,
			note,
			created_at,
			updated_at,
			cancelled_at,
			started_at,
			completed_at
		FROM activity.session
		WHERE 1=1`,
		Args: make([]any, 0),
	}

	// Apply filters
	if filter.ActivityGroupID != nil {
		qb.AddCondition("activity_group_id = ", *filter.ActivityGroupID)
	}

	if filter.SessionTemplateID != nil {
		qb.AddCondition("session_template_id = ", *filter.SessionTemplateID)
	}

	if filter.Status != nil {
		qb.AddCondition("status = ", filter.Status.String())
	}

	if filter.IsRecurring != nil {
		qb.AddCondition("is_recurring = ", *filter.IsRecurring)
	}

	if filter.StartTimeFrom != nil && filter.StartTimeFrom.Valid {
		qb.AddCondition("start_time >= ", filter.StartTimeFrom.Time)
	}

	if filter.StartTimeTo != nil && filter.StartTimeTo.Valid {
		qb.AddCondition("start_time <= ", filter.StartTimeTo.Time)
	}

	// Ordering
	qb.BaseQuery += " ORDER BY start_time ASC"

	// Pagination
	if filter.Limit > 0 {
		qb.ParamCount++
		qb.BaseQuery += fmt.Sprintf(" LIMIT $%d", qb.ParamCount)
		qb.Args = append(qb.Args, filter.Limit)
	}
	if filter.Offset > 0 {
		qb.ParamCount++
		qb.BaseQuery += fmt.Sprintf(" OFFSET $%d", qb.ParamCount)
		qb.Args = append(qb.Args, filter.Offset)
	}

	query, args := qb.BaseQuery, qb.Args

	q := m.tx.Querier(ctx)

	rows, err := q.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to list sessions: %w", err)
	}
	defer rows.Close()

	var sessions []*domain.Session
	for rows.Next() {
		var model sessionModel
		err := rows.Scan(
			&model.id,
			&model.activityGroupID,
			&model.createdByID,
			&model.sessionTemplateID,
			&model.locationCity,
			&model.locationCountry,
			&model.locationLat,
			&model.locationLng,
			&model.startTime,
			&model.endTime,
			&model.capacity,
			&model.status,
			&model.isRecurring,
			&model.note,
			&model.createdAt,
			&model.updatedAt,
			&model.cancelledAt,
			&model.startedAt,
			&model.completedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan session: %w", err)
		}

		session, err := sessionModelToDomain(&model)
		if err != nil {
			m.logger.Error(ctx, "failed to convert session model to domain", "error", err, "session_id", model.id)
			continue
		}

		sessions = append(sessions, session)
	}

	if rows.Err() != nil {
		return nil, fmt.Errorf("error iterating sessions: %w", rows.Err())
	}

	return sessions, nil
}

func (m *sessionManager) DeleteSession(ctx context.Context, id uuid.UUID) error {
	query := `DELETE FROM activity.session WHERE id = $1`

	q := m.tx.Querier(ctx)

	result, err := q.Exec(ctx, query, id)
	if err != nil {
		return fmt.Errorf("failed to delete session: %w", err)
	}

	if result.RowsAffected() == 0 {
		return domain.ErrSessionNotFound
	}

	return nil
}

// Conversion functions

func sessionModelFromDomain(s *domain.Session) *sessionModel {
	model := &sessionModel{
		id:              s.ID(),
		activityGroupID: s.ActivityGroupID(),
		createdByID:     s.CreatedByID(),
		locationCity:    s.Location().City(),
		locationCountry: s.Location().Country(),
		locationLat:     s.Location().Latitude(),
		locationLng:     s.Location().Longitude(),
		status:          string(s.Status()),
		isRecurring:     s.IsRecurring(),
	}

	// Template ID
	if templateID := s.TemplateID(); templateID != nil {
		model.sessionTemplateID = sql.NullString{String: templateID.String(), Valid: true}
	}

	// Note
	if note := s.Note(); note != "" {
		model.note = sql.NullString{String: note, Valid: true}
	}

	// Start time
	if !s.Schedule().StartTime().IsZero() {
		model.startTime = sql.NullTime{Time: s.Schedule().StartTime(), Valid: true}
	}

	// End time
	if endTime := s.Schedule().EndTime(); endTime != nil {
		model.endTime = sql.NullTime{Time: *endTime, Valid: true}
	}

	// Capacity
	if cap := s.Capacity(); cap != nil {
		model.capacity = sql.NullInt32{Int32: int32(*cap), Valid: true}
	}

	// Created at
	if !s.CreatedAt().IsZero() {
		model.createdAt = sql.NullTime{Time: s.CreatedAt(), Valid: true}
	}

	// Updated at
	if !s.UpdatedAt().IsZero() {
		model.updatedAt = sql.NullTime{Time: s.UpdatedAt(), Valid: true}
	}

	// Cancelled at
	if cancelledAt := s.CancelledAt(); cancelledAt != nil {
		model.cancelledAt = sql.NullTime{Time: *cancelledAt, Valid: true}
	}

	// Started at
	if startedAt := s.StartedAt(); startedAt != nil {
		model.startedAt = sql.NullTime{Time: *startedAt, Valid: true}
	}

	// Completed at
	if completedAt := s.CompletedAt(); completedAt != nil {
		model.completedAt = sql.NullTime{Time: *completedAt, Valid: true}
	}

	return model
}

func sessionModelToDomain(model *sessionModel) (*domain.Session, error) {
	// TODO: This function needs a proper domain hydration method
	// The current approach uses NewSession which runs creation validations
	// (e.g., start time must be in future) which fails for historical sessions.
	// For now, we'll return an error and note that this needs to be implemented
	// when we need to read sessions from the database.
	// The session generator only creates new sessions, so this limitation doesn't affect it.

	return nil, fmt.Errorf("session reconstruction from database not yet implemented - needs domain hydration method")
}
