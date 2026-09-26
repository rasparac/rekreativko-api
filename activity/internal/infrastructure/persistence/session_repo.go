package persistence

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/rasparac/rekreativko-api/activity/internal/domain"
	"github.com/rasparac/rekreativko-api/shared/logger"
	"github.com/rasparac/rekreativko-api/shared/store/postgres"
)

type sessionModel struct {
	id                uuid.UUID
	activityGroupID   uuid.NullUUID
	createdByID       uuid.UUID
	sessionTemplateID sql.NullString // uuid as string, nullable
	title             string
	activityType      string
	difficultyLevel   string
	locationCity      string
	locationCountry   string
	locationStreet    sql.NullString
	locationLat       float64
	locationLng       float64
	startTime         sql.NullTime
	endTime           sql.NullTime
	capacity          sql.NullInt32
	status            string
	visibility        string
	requiresApproval  bool
	isRecurring       bool
	teamCount         sql.NullInt32 // NULL = session has no teams
	minPlayersPerTeam sql.NullInt32
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
	ListSessions(ctx context.Context, filter SessionFilter) ([]*domain.Session, string, error)
	DiscoverSessions(ctx context.Context, filter DiscoverSessionsFilter) ([]SessionWithDistance, string, error)
	DeleteSession(ctx context.Context, id uuid.UUID) error
	FindSessionsPastEndTime(ctx context.Context) ([]*domain.Session, error)
	ReplaceTeams(ctx context.Context, session *domain.Session) error
}

// SessionFilter defines query filters for listing sessions
type SessionFilter struct {
	ActivityGroupID   *uuid.UUID
	SessionTemplateID *uuid.UUID
	CreatedByID       *uuid.UUID
	Status            *domain.SessionStatus
	Visibility        *domain.SessionVisibility
	ActivityType      *domain.ActivityType
	DifficultyLevel   *domain.DifficultyLevel
	IsRecurring       *bool
	StartTimeFrom     *sql.NullTime
	StartTimeTo       *sql.NullTime
	// AttendeeID filters to sessions this account has RSVP'd to. AttendeeStatuses,
	// when non-empty, further restricts to those RSVP statuses (e.g. "going"/
	// "promoted" for "currently holds a spot"); empty means any status.
	AttendeeID       *uuid.UUID
	AttendeeStatuses []domain.AttendeeStatus
	// RequesterID scopes results to what this caller may actually see: public
	// sessions, plus any private session they created, are an attendee of, or
	// belong to a group they're a confirmed member of. Mirrors
	// ActivityGroupFilter.RequesterID - applied unconditionally, regardless of
	// which other filters (ActivityGroupID, CreatedByID, AttendeeID, ...) are
	// also set, so a private session an unrelated caller has no relationship
	// to is excluded from every listing shape, not just direct fetch.
	RequesterID uuid.UUID
	Limit       int
	PageToken   string
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
			start_time, end_time, capacity, status, visibility, is_recurring, note,
			created_at, updated_at, title, activity_type, difficulty_level, location_street,
			requires_approval, team_count, min_players_per_team
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, $22, $23, $24
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
		model.visibility,
		model.isRecurring,
		model.note,
		model.createdAt,
		model.updatedAt,
		model.title,
		model.activityType,
		model.difficultyLevel,
		model.locationStreet,
		model.requiresApproval,
		model.teamCount,
		model.minPlayersPerTeam,
	)

	if err != nil {
		return fmt.Errorf("failed to create session: %w", err)
	}

	return nil
}

// ReplaceTeams persists the session's current teams, replacing any existing
// ones: every attendee's team_id is cleared (soft-deleted rows included, so no
// row still references an old team), the old teams are deleted, the new ones
// inserted and the session's team config updated. Must run in a transaction.
func (m *sessionManager) ReplaceTeams(ctx context.Context, session *domain.Session) error {
	q := m.tx.Querier(ctx)

	if _, err := q.Exec(ctx,
		`UPDATE activity.session_attendee SET team_id = NULL WHERE session_id = $1 AND team_id IS NOT NULL`,
		session.ID(),
	); err != nil {
		return fmt.Errorf("failed to clear attendee teams: %w", err)
	}

	if _, err := q.Exec(ctx, `DELETE FROM activity.session_team WHERE session_id = $1`, session.ID()); err != nil {
		return fmt.Errorf("failed to delete session teams: %w", err)
	}

	for _, team := range session.Teams() {
		if err := m.createTeam(ctx, team); err != nil {
			return err
		}
	}

	model := sessionModelFromDomain(session)
	result, err := q.Exec(ctx,
		`UPDATE activity.session SET team_count = $2, min_players_per_team = $3, updated_at = $4 WHERE id = $1`,
		model.id,
		model.teamCount,
		model.minPlayersPerTeam,
		model.updatedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to update session team config: %w", err)
	}

	if result.RowsAffected() == 0 {
		return domain.ErrSessionNotFound
	}

	return nil
}

func (m *sessionManager) createTeam(ctx context.Context, team *domain.Team) error {
	query := `
		INSERT INTO activity.session_team (id, session_id, name, color, position, created_at)
		VALUES ($1, $2, $3, $4, $5, $6)
	`

	var color sql.NullString
	if team.Color() != "" {
		color = sql.NullString{String: team.Color(), Valid: true}
	}

	_, err := m.tx.Querier(ctx).Exec(
		ctx,
		query,
		team.ID(),
		team.SessionID(),
		team.Name(),
		color,
		team.Position(),
		team.CreatedAt(),
	)
	if err != nil {
		return fmt.Errorf("failed to create session team: %w", err)
	}

	return nil
}

// listTeams loads a session's teams in position order.
func (m *sessionManager) listTeams(ctx context.Context, sessionID uuid.UUID) ([]*domain.Team, error) {
	query := `
		SELECT id, session_id, name, color, position, created_at
		FROM activity.session_team
		WHERE session_id = $1
		ORDER BY position ASC
	`

	rows, err := m.tx.Querier(ctx).Query(ctx, query, sessionID)
	if err != nil {
		return nil, fmt.Errorf("failed to list session teams: %w", err)
	}
	defer rows.Close()

	var teams []*domain.Team
	for rows.Next() {
		var (
			id, teamSessionID uuid.UUID
			name              string
			color             sql.NullString
			position          int
			createdAt         time.Time
		)
		if err := rows.Scan(&id, &teamSessionID, &name, &color, &position, &createdAt); err != nil {
			return nil, fmt.Errorf("failed to scan session team: %w", err)
		}
		teams = append(teams, domain.ReconstructTeam(id, teamSessionID, name, color.String, position, createdAt))
	}

	if rows.Err() != nil {
		return nil, fmt.Errorf("error iterating session teams: %w", rows.Err())
	}

	return teams, nil
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
			visibility = $10,
			note = $11,
			updated_at = $12,
			cancelled_at = $13,
			started_at = $14,
			completed_at = $15,
			location_street = $16,
			requires_approval = $17
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
		model.visibility,
		model.note,
		model.updatedAt,
		model.cancelledAt,
		model.startedAt,
		model.completedAt,
		model.locationStreet,
		model.requiresApproval,
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
			start_time, end_time, capacity, status, visibility, is_recurring, note,
			created_at, updated_at, cancelled_at, started_at, completed_at,
			title, activity_type, difficulty_level, location_street, requires_approval,
			team_count, min_players_per_team
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
		&model.visibility,
		&model.isRecurring,
		&model.note,
		&model.createdAt,
		&model.updatedAt,
		&model.cancelledAt,
		&model.startedAt,
		&model.completedAt,
		&model.title,
		&model.activityType,
		&model.difficultyLevel,
		&model.locationStreet,
		&model.requiresApproval,
		&model.teamCount,
		&model.minPlayersPerTeam,
	)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrSessionNotFound
		}
		return nil, fmt.Errorf("failed to get session: %w", err)
	}

	var teams []*domain.Team
	if model.teamCount.Valid {
		teams, err = m.listTeams(ctx, model.id)
		if err != nil {
			return nil, err
		}
	}

	return sessionModelToDomain(&model, teams)
}

// FindSessionsPastEndTime returns every non-terminal session whose end time has
// already passed - meant to be run periodically by a cron job, mirroring
// FindExpiredPendingInvites.
func (m *sessionManager) FindSessionsPastEndTime(ctx context.Context) ([]*domain.Session, error) {
	query := `
		SELECT
			id, activity_group_id, created_by_id, session_template_id,
			location_city, location_country, location_lat, location_lng,
			start_time, end_time, capacity, status, visibility, is_recurring, note,
			created_at, updated_at, cancelled_at, started_at, completed_at,
			title, activity_type, difficulty_level, location_street, requires_approval,
			team_count, min_players_per_team
		FROM activity.session
		WHERE status IN ('scheduled', 'started') AND end_time IS NOT NULL AND end_time < now()
	`

	q := m.tx.Querier(ctx)

	rows, err := q.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to find sessions past end time: %w", err)
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
			&model.visibility,
			&model.isRecurring,
			&model.note,
			&model.createdAt,
			&model.updatedAt,
			&model.cancelledAt,
			&model.startedAt,
			&model.completedAt,
			&model.title,
			&model.activityType,
			&model.difficultyLevel,
			&model.locationStreet,
			&model.requiresApproval,
			&model.teamCount,
			&model.minPlayersPerTeam,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan session: %w", err)
		}

		session, err := sessionModelToDomain(&model, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to build session from model: %w", err)
		}

		sessions = append(sessions, session)
	}

	if rows.Err() != nil {
		return nil, fmt.Errorf("error iterating sessions: %w", rows.Err())
	}

	return sessions, nil
}

func (m *sessionManager) ListSessions(ctx context.Context, filter SessionFilter) ([]*domain.Session, string, error) {
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
			visibility,
			is_recurring,
			note,
			created_at,
			updated_at,
			cancelled_at,
			started_at,
			completed_at,
			title,
			activity_type,
			difficulty_level,
			location_street,
			requires_approval,
			team_count,
			min_players_per_team
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

	if filter.CreatedByID != nil {
		qb.AddCondition("created_by_id = ", *filter.CreatedByID)
	}

	if filter.Status != nil {
		qb.AddCondition("status = ", filter.Status.String())
	}

	if filter.Visibility != nil {
		qb.AddCondition("visibility = ", filter.Visibility.String())
	}

	if filter.ActivityType != nil {
		qb.AddCondition("activity_type = ", filter.ActivityType.String())
	}

	if filter.DifficultyLevel != nil {
		qb.AddCondition("difficulty_level = ", filter.DifficultyLevel.String())
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

	if filter.AttendeeID != nil {
		qb.ParamCount++
		attendeeParam := qb.ParamCount
		condition := fmt.Sprintf(
			" AND id IN (SELECT session_id FROM activity.session_attendee WHERE account_id = $%d",
			attendeeParam,
		)
		qb.Args = append(qb.Args, *filter.AttendeeID)

		if len(filter.AttendeeStatuses) > 0 {
			statuses := make([]string, len(filter.AttendeeStatuses))
			for i, st := range filter.AttendeeStatuses {
				statuses[i] = string(st)
			}
			qb.ParamCount++
			condition += fmt.Sprintf(" AND status = ANY($%d)", qb.ParamCount)
			qb.Args = append(qb.Args, statuses)
		}

		condition += " AND deleted_at IS NULL)"
		qb.BaseQuery += condition
	}

	if filter.RequesterID != uuid.Nil {
		// Public sessions are visible to everyone; a private one only to its
		// creator, one of its attendees, or a confirmed member of the group
		// it belongs to (moot for a standalone session, which has no group) -
		// anyone else must not see it in listings at all, not just be denied
		// on direct fetch.
		qb.ParamCount++
		requesterParam := qb.ParamCount
		qb.BaseQuery += fmt.Sprintf(
			` AND (
				visibility = 'public'
				OR created_by_id = $%d
				OR id IN (SELECT session_id FROM activity.session_attendee WHERE account_id = $%d AND deleted_at IS NULL)
				OR activity_group_id IN (SELECT activity_group_id FROM activity.member WHERE account_id = $%d AND status = 'confirmed' AND deleted_at IS NULL)
			)`,
			requesterParam, requesterParam, requesterParam,
		)
		qb.Args = append(qb.Args, filter.RequesterID)
	}

	// Resume from the previous page's cursor, if any
	cursor, err := postgres.DecodePageToken(filter.PageToken)
	if err != nil {
		return nil, "", err
	}
	if cursor != nil {
		sortValue, err := time.Parse(time.RFC3339Nano, cursor.SortValue)
		if err != nil {
			return nil, "", postgres.ErrInvalidPageToken
		}
		qb.AddKeysetCondition("start_time", "ASC", sortValue, cursor.ID)
	}

	// Ordering - id is a tiebreaker for rows with an identical start_time
	qb.BaseQuery += " ORDER BY start_time ASC, id ASC"

	// Fetch one extra row so we can tell whether there's a next page
	fetchLimit := filter.Limit
	if fetchLimit > 0 {
		qb.ParamCount++
		qb.BaseQuery += fmt.Sprintf(" LIMIT $%d", qb.ParamCount)
		qb.Args = append(qb.Args, fetchLimit+1)
	}

	query, args := qb.BaseQuery, qb.Args

	q := m.tx.Querier(ctx)

	rows, err := q.Query(ctx, query, args...)
	if err != nil {
		return nil, "", fmt.Errorf("failed to list sessions: %w", err)
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
			&model.visibility,
			&model.isRecurring,
			&model.note,
			&model.createdAt,
			&model.updatedAt,
			&model.cancelledAt,
			&model.startedAt,
			&model.completedAt,
			&model.title,
			&model.activityType,
			&model.difficultyLevel,
			&model.locationStreet,
			&model.requiresApproval,
			&model.teamCount,
			&model.minPlayersPerTeam,
		)
		if err != nil {
			return nil, "", fmt.Errorf("failed to scan session: %w", err)
		}

		session, err := sessionModelToDomain(&model, nil)
		if err != nil {
			m.logger.Error(ctx, "failed to convert session model to domain", "error", err, "session_id", model.id)
			continue
		}

		sessions = append(sessions, session)
	}

	if rows.Err() != nil {
		return nil, "", fmt.Errorf("error iterating sessions: %w", rows.Err())
	}

	page, nextPageToken := postgres.BuildPage(sessions, filter.Limit, func(s *domain.Session) (string, uuid.UUID) {
		return s.Schedule().StartTime().UTC().Format(time.RFC3339Nano), s.ID()
	})

	return page, nextPageToken, nil
}

// kmPerDegreeLatitude is the approximate distance in km covered by one degree
// of latitude - used to build a cheap bounding-box pre-filter before applying
// the precise Haversine distance formula.
const kmPerDegreeLatitude = 111.045

// DiscoverSessionsFilter defines query filters for finding public sessions near a location
type DiscoverSessionsFilter struct {
	Lat       float64
	Lng       float64
	RadiusKM  float64
	Interests []postgres.InterestPair
	Limit     int
	PageToken string
}

// SessionWithDistance pairs a session with its distance from the search location
type SessionWithDistance struct {
	Session    *domain.Session
	DistanceKM float64
}

// DiscoverSessions finds scheduled, public sessions within RadiusKM of (Lat, Lng),
// ordered by distance. Uses a bounding-box pre-filter (indexed range scan on
// location_lat/location_lng) followed by the exact Haversine formula for the
// final radius filter and ordering - no PostGIS/earthdistance extension needed
// at this scale.
func (m *sessionManager) DiscoverSessions(ctx context.Context, filter DiscoverSessionsFilter) ([]SessionWithDistance, string, error) {
	latDelta := filter.RadiusKM / kmPerDegreeLatitude
	lngDelta := filter.RadiusKM / (kmPerDegreeLatitude * math.Cos(filter.Lat*math.Pi/180))

	// Resume from the previous page's cursor, if any. distance_km is a computed
	// expression, not a stored/indexed column, but Postgres can still filter on
	// it correctly here - the candidate set is already small after the bounding
	// box + radius filters, so re-evaluating the expression per row is cheap.
	var cursorDistance *float64
	var cursorID *uuid.UUID
	cursor, err := postgres.DecodePageToken(filter.PageToken)
	if err != nil {
		return nil, "", err
	}
	if cursor != nil {
		d, err := strconv.ParseFloat(cursor.SortValue, 64)
		if err != nil {
			return nil, "", postgres.ErrInvalidPageToken
		}
		cursorDistance = &d
		cursorID = &cursor.ID
	}

	// $1-$6 are the fixed lat/lng + bounding-box params below; interest pairs
	// come next, however many there are, then radius/limit/cursor - hence
	// the dynamic paramCount bookkeeping instead of fixed $7/$8/etc.
	var (
		interestClauses []string
		interestArgs    []any
		paramCount      = 6
	)
	for _, interest := range filter.Interests {
		if interest.ActivityType == "" {
			continue
		}

		paramCount++
		typeParam := paramCount

		if interest.DifficultyLevel == "" {
			interestClauses = append(interestClauses, fmt.Sprintf("activity_type = $%d", typeParam))
			interestArgs = append(interestArgs, interest.ActivityType)
			continue
		}

		paramCount++
		levelParam := paramCount
		interestClauses = append(interestClauses, fmt.Sprintf("(activity_type = $%d AND difficulty_level = $%d)", typeParam, levelParam))
		interestArgs = append(interestArgs, interest.ActivityType, interest.DifficultyLevel)
	}

	interestCondition := ""
	if len(interestClauses) > 0 {
		interestCondition = "AND (" + strings.Join(interestClauses, " OR ") + ")"
	}

	radiusParam := paramCount + 1
	limitParam := paramCount + 2
	cursorDistParam := paramCount + 3
	cursorIDParam := paramCount + 4

	query := fmt.Sprintf(`
		SELECT * FROM (
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
				visibility,
				is_recurring,
				note,
				created_at,
				updated_at,
				cancelled_at,
				started_at,
				completed_at,
				title,
				activity_type,
				difficulty_level,
				location_street,
				requires_approval,
				team_count,
				min_players_per_team,
				(6371 * acos(LEAST(1, GREATEST(-1,
					cos(radians($1)) * cos(radians(location_lat)) *
					cos(radians(location_lng) - radians($2)) +
					sin(radians($1)) * sin(radians(location_lat))
				)))) AS distance_km
			FROM activity.session
			WHERE visibility = 'public'
				AND status = 'scheduled'
				AND (end_time IS NULL OR end_time > now())
				AND location_lat BETWEEN $3 AND $4
				AND location_lng BETWEEN $5 AND $6
				%s
		) nearby
		WHERE distance_km <= $%d
			AND ($%d::float8 IS NULL OR (distance_km, id) > ($%d::float8, $%d::uuid))
		ORDER BY distance_km ASC, id ASC
		LIMIT $%d
	`, interestCondition, radiusParam, cursorDistParam, cursorDistParam, cursorIDParam, limitParam)

	args := []any{
		filter.Lat,
		filter.Lng,
		filter.Lat - latDelta,
		filter.Lat + latDelta,
		filter.Lng - lngDelta,
		filter.Lng + lngDelta,
	}
	args = append(args, interestArgs...)
	args = append(args, filter.RadiusKM, filter.Limit+1, cursorDistance, cursorID)

	q := m.tx.Querier(ctx)

	rows, err := q.Query(ctx, query, args...)
	if err != nil {
		return nil, "", fmt.Errorf("failed to discover sessions: %w", err)
	}
	defer rows.Close()

	var results []SessionWithDistance
	for rows.Next() {
		var model sessionModel
		var distanceKM float64

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
			&model.visibility,
			&model.isRecurring,
			&model.note,
			&model.createdAt,
			&model.updatedAt,
			&model.cancelledAt,
			&model.startedAt,
			&model.completedAt,
			&model.title,
			&model.activityType,
			&model.difficultyLevel,
			&model.locationStreet,
			&model.requiresApproval,
			&model.teamCount,
			&model.minPlayersPerTeam,
			&distanceKM,
		)
		if err != nil {
			return nil, "", fmt.Errorf("failed to scan nearby session: %w", err)
		}

		session, err := sessionModelToDomain(&model, nil)
		if err != nil {
			m.logger.Error(ctx, "failed to convert session model to domain", "error", err, "session_id", model.id)
			continue
		}

		results = append(results, SessionWithDistance{
			Session:    session,
			DistanceKM: distanceKM,
		})
	}

	if rows.Err() != nil {
		return nil, "", fmt.Errorf("error iterating nearby sessions: %w", rows.Err())
	}

	page, nextPageToken := postgres.BuildPage(results, filter.Limit, func(r SessionWithDistance) (string, uuid.UUID) {
		return strconv.FormatFloat(r.DistanceKM, 'f', -1, 64), r.Session.ID()
	})

	return page, nextPageToken, nil
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
	var activityGroupID uuid.NullUUID
	if groupID := s.ActivityGroupID(); groupID != nil {
		activityGroupID = uuid.NullUUID{UUID: *groupID, Valid: true}
	}

	model := &sessionModel{
		id:               s.ID(),
		activityGroupID:  activityGroupID,
		createdByID:      s.CreatedByID(),
		title:            s.Title().Value(),
		activityType:     s.ActivityType().String(),
		difficultyLevel:  s.DifficultyLevel().String(),
		locationCity:     s.Location().City(),
		locationCountry:  s.Location().Country(),
		locationLat:      s.Location().Latitude(),
		locationLng:      s.Location().Longitude(),
		status:           string(s.Status()),
		visibility:       string(s.Visibility()),
		requiresApproval: s.RequiresApproval(),
		isRecurring:      s.IsRecurring(),
	}

	// Template ID
	if templateID := s.TemplateID(); templateID != nil {
		model.sessionTemplateID = sql.NullString{String: templateID.String(), Valid: true}
	}

	// Note
	if note := s.Note(); note != "" {
		model.note = sql.NullString{String: note, Valid: true}
	}

	// Location street (optional)
	if street := s.Location().Street(); street != "" {
		model.locationStreet = sql.NullString{String: street, Valid: true}
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

	// Team config
	if tc := s.TeamConfig(); tc != nil {
		model.teamCount = sql.NullInt32{Int32: int32(tc.TeamCount()), Valid: true}
		if ppt := tc.MinPlayersPerTeam(); ppt != nil {
			model.minPlayersPerTeam = sql.NullInt32{Int32: int32(*ppt), Valid: true}
		}
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

// sessionModelToDomain converts a session row to the domain Session. teams is
// nil when they weren't loaded (list queries).
func sessionModelToDomain(model *sessionModel, teams []*domain.Team) (*domain.Session, error) {
	location, err := domain.NewSessionLocation(
		model.locationCity,
		model.locationCountry,
		model.locationStreet.String,
		model.locationLat,
		model.locationLng,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to build session location: %w", err)
	}

	var endTime *time.Time
	if model.endTime.Valid {
		endTime = &model.endTime.Time
	}

	schedule, err := domain.ReconstructSessionSchedule(model.startTime.Time, endTime)
	if err != nil {
		return nil, fmt.Errorf("failed to build session schedule: %w", err)
	}

	var templateID *uuid.UUID
	if model.sessionTemplateID.Valid {
		parsed, err := uuid.Parse(model.sessionTemplateID.String)
		if err != nil {
			return nil, fmt.Errorf("failed to parse session template ID: %w", err)
		}
		templateID = &parsed
	}

	var capacity *int
	if model.capacity.Valid {
		c := int(model.capacity.Int32)
		capacity = &c
	}

	var teamConfig *domain.TeamConfig
	if model.teamCount.Valid {
		var minPlayersPerTeam *int
		if model.minPlayersPerTeam.Valid {
			p := int(model.minPlayersPerTeam.Int32)
			minPlayersPerTeam = &p
		}

		colors := make([]string, 0, len(teams))
		for _, t := range teams {
			if t.Color() != "" {
				colors = append(colors, t.Color())
			}
		}

		tc := domain.ReconstructTeamConfig(int(model.teamCount.Int32), minPlayersPerTeam, colors)
		teamConfig = &tc
	}

	var cancelledAt *time.Time
	if model.cancelledAt.Valid {
		cancelledAt = &model.cancelledAt.Time
	}

	var startedAt *time.Time
	if model.startedAt.Valid {
		startedAt = &model.startedAt.Time
	}

	var completedAt *time.Time
	if model.completedAt.Valid {
		completedAt = &model.completedAt.Time
	}

	var activityGroupID *uuid.UUID
	if model.activityGroupID.Valid {
		activityGroupID = &model.activityGroupID.UUID
	}

	title, err := domain.NewTitle(model.title)
	if err != nil {
		return nil, fmt.Errorf("failed to build session title: %w", err)
	}

	return domain.ReconstructSession(
		model.id,
		activityGroupID,
		model.createdByID,
		templateID,
		title,
		domain.ActivityType(model.activityType),
		domain.DifficultyLevel(model.difficultyLevel),
		location,
		schedule,
		capacity,
		domain.SessionStatus(model.status),
		domain.SessionVisibility(model.visibility),
		model.requiresApproval,
		model.isRecurring,
		model.note.String,
		nil, // openAt is not persisted yet
		teamConfig,
		teams,
		model.createdAt.Time,
		model.updatedAt.Time,
		cancelledAt,
		startedAt,
		completedAt,
	), nil
}
