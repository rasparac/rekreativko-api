package persistence

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/rasparac/rekreativko-api/activity/internal/domain"
	"github.com/rasparac/rekreativko-api/shared/logger"
	"github.com/rasparac/rekreativko-api/shared/store/postgres"
)

type (
	sessionTemplateManager struct {
		tx     *postgres.TransactionManager
		logger *logger.Logger
	}

	SessionTemplateFilter struct {
		ActivityGroupID *uuid.UUID
		CreatedByID     *uuid.UUID
		Status          *domain.SessionTemplateStatus
		IsRecurring     *bool // filter recurring vs non-recurring templates

		// For cron job: find templates that need session generation
		NeedsGeneration *bool          // templates where generated_up_to < NOW() + lookahead
		LookaheadWindow *time.Duration // how far ahead to generate (e.g., 7 days)

		// Pagination
		Limit  int
		Offset int
	}

	// Database schema reference:
	// CREATE TABLE IF NOT EXISTS activity.session_template(
	//     id uuid PRIMARY KEY DEFAULT uuid_generate_v4(),
	//     activity_group_id uuid NOT NULL REFERENCES activity.activity_group(id),
	//     created_by_id uuid NOT NULL,
	//     title varchar(200) NOT NULL,
	//     description text DEFAULT NULL,
	//     capacity int DEFAULT NULL CHECK (capacity IS NULL OR capacity > 0),
	//     location_city varchar(100) DEFAULT NULL,
	//     location_country varchar(100) DEFAULT NULL,
	//     generated_up_to timestamptz DEFAULT NULL,
	//     status varchar(50) NOT NULL DEFAULT 'active',
	//     recurrence_frequency varchar(50) DEFAULT NULL,
	//     recurrence_interval smallint DEFAULT NULL,
	//     recurrence_day_of_week smallint DEFAULT NULL,
	//     recurrence_day_of_month smallint DEFAULT NULL,
	//     recurrence_time_hour smallint DEFAULT NULL CHECK (recurrence_time_hour BETWEEN 0 AND 23),
	//     recurrence_time_minute smallint DEFAULT NULL CHECK (recurrence_time_minute BETWEEN 0 AND 59),
	//     recurrence_ends_at timestamptz DEFAULT NULL,
	//     created_at timestamptz NOT NULL DEFAULT NOW(),
	//     updated_at timestamptz NOT NULL DEFAULT NOW(),
	//     deleted_at timestamptz DEFAULT NULL
	// );

	sessionTemplateModel struct {
		id              uuid.UUID
		activityGroupID uuid.UUID
		createdByID     uuid.UUID

		title           string
		description     sql.NullString
		status          string
		capacity        sql.NullInt32
		locationCity    sql.NullString
		locationCountry sql.NullString
		generatedUpTo   sql.NullTime

		// Recurrence fields
		recurrenceFrequency  sql.NullString
		recurrenceInterval   sql.NullInt32
		recurrenceDayOfWeek  sql.NullInt32
		recurrenceDayOfMonth sql.NullInt32
		recurrenceTimeHour   sql.NullInt32
		recurrenceTimeMinute sql.NullInt32
		recurrenceEndsAt     sql.NullTime

		createdAt time.Time
		updatedAt time.Time
		deletedAt sql.NullTime
	}
)

func NewSessionTemplateManager(
	tx *postgres.TransactionManager,
	logger *logger.Logger,
) *sessionTemplateManager {
	return &sessionTemplateManager{
		tx:     tx,
		logger: logger,
	}
}

func (m *sessionTemplateManager) CreateSessionTemplate(
	ctx context.Context,
	in *domain.SessionTemplate,
) error {
	model := sessionTemplateModelFromDomain(in)

	query := `
		INSERT INTO activity.session_template (
			id,
			activity_group_id,
			created_by_id,
			title,
			description,
			status,
			capacity,
			location_city,
			location_country,
			generated_up_to,
			recurrence_frequency,
			recurrence_interval,
			recurrence_day_of_week,
			recurrence_day_of_month,
			recurrence_time_hour,
			recurrence_time_minute,
			recurrence_ends_at,
			created_at,
			updated_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10,
			$11, $12, $13, $14, $15, $16, $17, $18, $19
		)
	`

	q := m.tx.Querier(ctx)

	_, err := q.Exec(ctx, query,
		model.id,
		model.activityGroupID,
		model.createdByID,
		model.title,
		model.description,
		model.status,
		model.capacity,
		model.locationCity,
		model.locationCountry,
		model.generatedUpTo,
		model.recurrenceFrequency,
		model.recurrenceInterval,
		model.recurrenceDayOfWeek,
		model.recurrenceDayOfMonth,
		model.recurrenceTimeHour,
		model.recurrenceTimeMinute,
		model.recurrenceEndsAt,
		model.createdAt,
		model.updatedAt,
	)

	if err != nil {
		return fmt.Errorf("failed to create session template (id=%s, group=%s): %w",
			model.id, model.activityGroupID, err)
	}

	return nil
}

func (m *sessionTemplateManager) UpdateSessionTemplate(
	ctx context.Context,
	in *domain.SessionTemplate,
) error {
	model := sessionTemplateModelFromDomain(in)

	query := `
		UPDATE activity.session_template
		SET
			title = $2,
			description = $3,
			status = $4,
			capacity = $5,
			location_city = $6,
			location_country = $7,
			recurrence_frequency = $8,
			recurrence_interval = $9,
			recurrence_day_of_week = $10,
			recurrence_day_of_month = $11,
			recurrence_time_hour = $12,
			recurrence_time_minute = $13,
			recurrence_ends_at = $14,
			updated_at = NOW()
		WHERE id = $1 AND deleted_at IS NULL
	`

	q := m.tx.Querier(ctx)

	_, err := q.Exec(ctx, query,
		model.id,
		model.title,
		model.description,
		model.status,
		model.capacity,
		model.locationCity,
		model.locationCountry,
		model.recurrenceFrequency,
		model.recurrenceInterval,
		model.recurrenceDayOfWeek,
		model.recurrenceDayOfMonth,
		model.recurrenceTimeHour,
		model.recurrenceTimeMinute,
		model.recurrenceEndsAt,
	)

	if err != nil {
		return fmt.Errorf("failed to update session template (id=%s): %w", model.id, err)
	}

	return nil
}

func (m *sessionTemplateManager) UpdateGeneratedUpTo(
	ctx context.Context,
	templateID uuid.UUID,
	generatedUpTo time.Time,
) error {
	query := `
		UPDATE activity.session_template
		SET
			generated_up_to = $2,
			updated_at = NOW()
		WHERE id = $1 AND deleted_at IS NULL
	`

	q := m.tx.Querier(ctx)

	_, err := q.Exec(ctx, query, templateID, generatedUpTo)
	if err != nil {
		return fmt.Errorf("failed to update generated_up_to for session template (id=%s): %w", templateID, err)
	}

	return nil
}

func (m *sessionTemplateManager) DeleteSessionTemplate(
	ctx context.Context,
	templateID uuid.UUID,
) error {
	query := `
		UPDATE activity.session_template
		SET
			deleted_at = NOW(),
			updated_at = NOW()
		WHERE id = $1 AND deleted_at IS NULL
	`

	q := m.tx.Querier(ctx)

	_, err := q.Exec(ctx, query, templateID)
	if err != nil {
		return fmt.Errorf("failed to delete session template (id=%s): %w", templateID, err)
	}

	return nil
}

func (m *sessionTemplateManager) GetSessionTemplateByID(
	ctx context.Context,
	templateID uuid.UUID,
) (*domain.SessionTemplate, error) {
	query := `
		SELECT
			id,
			activity_group_id,
			created_by_id,
			title,
			description,
			status,
			capacity,
			location_city,
			location_country,
			generated_up_to,
			recurrence_frequency,
			recurrence_interval,
			recurrence_day_of_week,
			recurrence_day_of_month,
			recurrence_time_hour,
			recurrence_time_minute,
			recurrence_ends_at,
			created_at,
			updated_at
		FROM activity.session_template
		WHERE id = $1 AND deleted_at IS NULL
	`

	q := m.tx.Querier(ctx)

	var model sessionTemplateModel

	err := q.QueryRow(ctx, query, templateID).Scan(
		&model.id,
		&model.activityGroupID,
		&model.createdByID,
		&model.title,
		&model.description,
		&model.status,
		&model.capacity,
		&model.locationCity,
		&model.locationCountry,
		&model.generatedUpTo,
		&model.recurrenceFrequency,
		&model.recurrenceInterval,
		&model.recurrenceDayOfWeek,
		&model.recurrenceDayOfMonth,
		&model.recurrenceTimeHour,
		&model.recurrenceTimeMinute,
		&model.recurrenceEndsAt,
		&model.createdAt,
		&model.updatedAt,
	)

	if err != nil {
		return nil, fmt.Errorf("failed to get session template (id=%s): %w", templateID, err)
	}

	template, err := sessionTemplateToDomain(&model)
	if err != nil {
		return nil, fmt.Errorf("failed to convert session template to domain (id=%s): %w", templateID, err)
	}

	return template, nil
}

func (m *sessionTemplateManager) ListSessionTemplates(
	ctx context.Context,
	filter SessionTemplateFilter,
) ([]*domain.SessionTemplate, error) {
	query, args := buildSessionTemplateQuery(filter)

	q := m.tx.Querier(ctx)

	rows, err := q.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query session templates: %w", err)
	}
	defer rows.Close()

	// Pre-allocate slice based on limit or reasonable default
	var templates []*domain.SessionTemplate
	if filter.Limit > 0 {
		templates = make([]*domain.SessionTemplate, 0, filter.Limit)
	} else {
		templates = make([]*domain.SessionTemplate, 0, 100) // reasonable default
	}

	for rows.Next() {
		var model sessionTemplateModel

		err := rows.Scan(
			&model.id,
			&model.activityGroupID,
			&model.createdByID,
			&model.title,
			&model.description,
			&model.status,
			&model.capacity,
			&model.locationCity,
			&model.locationCountry,
			&model.generatedUpTo,
			&model.recurrenceFrequency,
			&model.recurrenceInterval,
			&model.recurrenceDayOfWeek,
			&model.recurrenceDayOfMonth,
			&model.recurrenceTimeHour,
			&model.recurrenceTimeMinute,
			&model.recurrenceEndsAt,
			&model.createdAt,
			&model.updatedAt,
		)

		if err != nil {
			return nil, fmt.Errorf("failed to scan session template row: %w", err)
		}

		template, err := sessionTemplateToDomain(&model)
		if err != nil {
			return nil, fmt.Errorf("failed to convert session template to domain: %w", err)
		}

		templates = append(templates, template)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating session template rows: %w", err)
	}

	return templates, nil
}

// buildSessionTemplateQuery constructs the SQL query and arguments for filtering session templates
func buildSessionTemplateQuery(filter SessionTemplateFilter) (string, []interface{}) {
	qb := &postgres.QueryBuilder{
		BaseQuery: `
		SELECT
			id,
			activity_group_id,
			created_by_id,
			title,
			description,
			status,
			capacity,
			location_city,
			location_country,
			generated_up_to,
			recurrence_frequency,
			recurrence_interval,
			recurrence_day_of_week,
			recurrence_day_of_month,
			recurrence_time_hour,
			recurrence_time_minute,
			recurrence_ends_at,
			created_at,
			updated_at
		FROM activity.session_template
		WHERE deleted_at IS NULL`,
		Args: make([]any, 0),
	}

	// Filter by activity group
	if filter.ActivityGroupID != nil {
		qb.AddCondition("activity_group_id = ", *filter.ActivityGroupID)
	}

	// Filter by creator
	if filter.CreatedByID != nil {
		qb.AddCondition("created_by_id = ", *filter.CreatedByID)
	}

	// Filter by status
	if filter.Status != nil {
		qb.AddCondition("status = ", filter.Status.String())
	}

	// Filter by recurring/non-recurring
	if filter.IsRecurring != nil {
		if *filter.IsRecurring {
			qb.AddRawCondition("recurrence_frequency IS NOT NULL")
		} else {
			qb.AddRawCondition("recurrence_frequency IS NULL")
		}
	}

	// Filter for cron job: templates that need session generation
	if filter.NeedsGeneration != nil && *filter.NeedsGeneration {
		// Only active recurring templates
		qb.AddRawCondition("status = 'active'")
		qb.AddRawCondition("recurrence_frequency IS NOT NULL")

		// Templates where:
		// - generated_up_to IS NULL (never generated yet)
		// - OR generated_up_to < NOW() + lookahead_window
		if filter.LookaheadWindow != nil {
			qb.AddCondition("(generated_up_to IS NULL OR generated_up_to < NOW() + ", *filter.LookaheadWindow)
			qb.BaseQuery += ")"
		} else {
			// Default lookahead of 7 days if not specified
			qb.AddRawCondition("(generated_up_to IS NULL OR generated_up_to < NOW() + INTERVAL '7 days')")
		}
	}

	// Order by created_at for consistent results
	qb.BaseQuery += ` ORDER BY created_at DESC`

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

	return qb.Build()
}

// ListSessionTemplatesByGroup is a convenience method to get all templates for an activity group
func (m *sessionTemplateManager) ListSessionTemplatesByGroup(
	ctx context.Context,
	activityGroupID uuid.UUID,
) ([]*domain.SessionTemplate, error) {
	return m.ListSessionTemplates(ctx, SessionTemplateFilter{
		ActivityGroupID: &activityGroupID,
	})
}

// FindRecurringTemplatesToGenerate finds active recurring templates that need session generation
// This is used by the cron job to generate upcoming sessions
func (m *sessionTemplateManager) FindRecurringTemplatesToGenerate(
	ctx context.Context,
	lookaheadWindow time.Duration,
) ([]*domain.SessionTemplate, error) {
	needsGeneration := true
	return m.ListSessionTemplates(ctx, SessionTemplateFilter{
		NeedsGeneration: &needsGeneration,
		LookaheadWindow: &lookaheadWindow,
	})
}

func sessionTemplateModelFromDomain(st *domain.SessionTemplate) *sessionTemplateModel {
	model := &sessionTemplateModel{
		id:              st.ID(),
		activityGroupID: st.ActivityGroupID(),
		createdByID:     st.CreatedByID(),
		title:           st.Title(),
		status:          string(st.Status()),
		createdAt:       st.CreatedAt(),
		updatedAt:       st.UpdatedAt(),
	}

	// Description
	if desc := st.Description(); desc != "" {
		model.description = sql.NullString{String: desc, Valid: true}
	}

	// Capacity
	if cap := st.DefaultCapacity(); cap != nil {
		model.capacity = sql.NullInt32{Int32: int32(*cap), Valid: true}
	}

	// Location
	if city := st.LocationCity(); city != "" {
		model.locationCity = sql.NullString{String: city, Valid: true}
	}
	if country := st.LocationCountry(); country != "" {
		model.locationCountry = sql.NullString{String: country, Valid: true}
	}

	// Generated up to
	if genUpTo := st.GeneratedUpTo(); genUpTo != nil {
		model.generatedUpTo = sql.NullTime{Time: *genUpTo, Valid: true}
	}

	// Recurrence fields - check if frequency is set to determine if recurring
	if freq := st.RecurrenceFrequency(); freq != "" {
		model.recurrenceFrequency = sql.NullString{String: string(freq), Valid: true}

		// If frequency is set, persist other recurrence fields
		// Interval - 0 is invalid, must be >= 1
		if interval := st.RecurrenceInterval(); interval > 0 {
			model.recurrenceInterval = sql.NullInt32{Int32: int32(interval), Valid: true}
		}

		// Day of week - 0 is Sunday (valid!), -1 is our invalid marker
		if dow := st.RecurrenceDayOfWeek(); dow >= 0 {
			model.recurrenceDayOfWeek = sql.NullInt32{Int32: int32(dow), Valid: true}
		}

		// Day of month - 1-31, 0 means not set
		if dom := st.RecurrenceDayOfMonth(); dom > 0 {
			model.recurrenceDayOfMonth = sql.NullInt32{Int32: int32(dom), Valid: true}
		}

		// Time hour - 0 is midnight (valid!), -1 is our invalid marker
		if hour := st.RecurrenceTimeHour(); hour >= 0 && hour <= 23 {
			model.recurrenceTimeHour = sql.NullInt32{Int32: int32(hour), Valid: true}
		}

		// Time minute - 0 is valid (start of hour)!, -1 is our invalid marker
		if minute := st.RecurrenceTimeMinute(); minute >= 0 && minute <= 59 {
			model.recurrenceTimeMinute = sql.NullInt32{Int32: int32(minute), Valid: true}
		}

		// Recurrence ends at
		if endsAt := st.RecurrenceEndsAt(); endsAt != nil {
			model.recurrenceEndsAt = sql.NullTime{Time: *endsAt, Valid: true}
		}
	}

	return model
}

func sessionTemplateToDomain(model *sessionTemplateModel) (*domain.SessionTemplate, error) {
	// Reconstruct capacity
	var capacity *int
	if model.capacity.Valid {
		cap := int(model.capacity.Int32)
		capacity = &cap
	}

	// Reconstruct location (city/country only)
	var location *domain.Location
	if model.locationCity.Valid && model.locationCountry.Valid {
		// For templates, we only store city/country, no coordinates
		// Using 0,0 as placeholder coordinates since domain.Location requires them
		loc, err := domain.NewLocation(
			model.locationCity.String,
			model.locationCountry.String,
			0, 0, // templates don't need coordinates
		)
		if err != nil {
			return nil, err
		}
		location = &loc
	}

	// Reconstruct recurrence rule
	var recurrenceRule *domain.RecurrenceRule
	if model.recurrenceFrequency.Valid {
		freq := domain.RecurrenceFrequency(model.recurrenceFrequency.String)

		// Time of day (required if recurring)
		var hour, minute int
		if model.recurrenceTimeHour.Valid {
			hour = int(model.recurrenceTimeHour.Int32)
		}
		if model.recurrenceTimeMinute.Valid {
			minute = int(model.recurrenceTimeMinute.Int32)
		}
		timeOfDay, err := domain.NewTimeOfDay(hour, minute)
		if err != nil {
			return nil, err
		}

		// Interval
		interval := 1 // default
		if model.recurrenceInterval.Valid {
			interval = int(model.recurrenceInterval.Int32)
		}

		// Day of week (for weekly recurrence)
		var dayOfWeek *time.Weekday
		if model.recurrenceDayOfWeek.Valid {
			dow := time.Weekday(model.recurrenceDayOfWeek.Int32)
			dayOfWeek = &dow
		}

		// Day of month (for monthly recurrence)
		var dayOfMonth *int
		if model.recurrenceDayOfMonth.Valid {
			dom := int(model.recurrenceDayOfMonth.Int32)
			dayOfMonth = &dom
		}

		// Ends at
		var endsAt *time.Time
		if model.recurrenceEndsAt.Valid {
			endsAt = &model.recurrenceEndsAt.Time
		}

		rule, err := domain.NewRecurrenceRule(
			freq,
			timeOfDay,
			interval,
			dayOfWeek,
			dayOfMonth,
			endsAt,
		)
		if err != nil {
			return nil, err
		}
		recurrenceRule = &rule
	}

	// Reconstruct generated up to
	var generatedUpTo *time.Time
	if model.generatedUpTo.Valid {
		generatedUpTo = &model.generatedUpTo.Time
	}

	// Reconstruct deleted at
	var deletedAt *time.Time
	if model.deletedAt.Valid {
		deletedAt = &model.deletedAt.Time
	}

	// Parse status
	status := domain.SessionTemplateStatus(model.status)

	// Use the reconstruct method to create domain object
	template := domain.ReconstructSessionTemplate(
		model.id,
		model.activityGroupID,
		model.createdByID,
		model.title,
		model.description.String, // empty string if null
		status,
		recurrenceRule,
		capacity,
		location,
		generatedUpTo,
		model.createdAt,
		model.updatedAt,
		deletedAt,
	)

	return template, nil
}
