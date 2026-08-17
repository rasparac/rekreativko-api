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

type activityGroupManager struct {
	tx     *postgres.TransactionManager
	logger *logger.Logger
}

type (
	ActivityGroupFilter struct {
		ID        uuid.UUID
		CreatorID uuid.UUID
		Title     string
		Status    domain.ActivityGroupStatus
		Limit     int
		Offset    int
	}

	DiscoveryFilter struct {
		City         string
		Country      string
		ActivityType domain.ActivityType
		Limit        int
		Offset       int
	}

	activityGroupModel struct {
		ID              uuid.UUID
		CreatorID       uuid.UUID
		Title           string
		Description     string
		ActivityType    domain.ActivityType
		DifficultyLevel domain.DifficultyLevel
		Visibility      domain.ActivityGroupVisibility
		Status          domain.ActivityGroupStatus
		LocationCity    string
		LocationCountry string
		Timezone        string
		DefaultCapacity sql.NullInt32
		CreatedAt       time.Time
		UpdatedAt       time.Time
		CancelledAt     sql.NullTime
	}
)

func NewActivityGroupRepository(
	tx *postgres.TransactionManager,
	logger *logger.Logger,
) *activityGroupManager {
	return &activityGroupManager{
		tx:     tx,
		logger: logger,
	}
}

// CREATE TABLE IF NOT EXISTS activity.activity_group(
//
//	id uuid PRIMARY KEY DEFAULT uuid_generate_v4(),
//	creator_id uuid NOT NULL,
//	title text NOT NULL,
//	description text NOT NULL,
//	activity_type varchar(200) NOT NULL,
//	difficulty_level varchar(50) NOT NULL DEFAULT 'beginner',
//	visibility varchar(50) NOT NULL DEFAULT 'public',
//	status varchar(50) NOT NULL DEFAULT 'draft',
//	-- location (city/country for discovery, no coordinates)
//	location_city varchar(100) DEFAULT NULL,
//	location_country varchar(100) DEFAULT NULL,
//	timezone varchar(100) NOT NULL DEFAULT 'UTC',
//	default_capacity int DEFAULT NULL, -- NULL means no limit
//	created_at timestamptz NOT NULL DEFAULT NOW(),
//	updated_at timestamptz NOT NULL DEFAULT NOW(),
//	deleted_at timestamptz DEFAULT NULL
//
// );
func (agm *activityGroupManager) CreateActivityGroup(
	ctx context.Context,
	aq *domain.ActivityGroup,
) error {
	var (
		q     = agm.tx.Querier(ctx)
		query = `
		INSERT INTO activity.activity_group (
			id,
			creator_id,
			title,
			description,
			activity_type,
			difficulty_level,
			visibility,
			status,
			location_city,
			location_country,
			timezone,
			default_capacity,
			created_at,
			updated_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14
		)
	`
		model = mapDomainActivityGroupToModel(aq)
	)
	_, err := q.Exec(
		ctx,
		query,
		model.ID,
		model.CreatorID,
		model.Title,
		model.Description,
		model.ActivityType,
		model.DifficultyLevel,
		model.Visibility,
		model.Status,
		model.LocationCity,
		model.LocationCountry,
		model.Timezone,
		model.DefaultCapacity,
		model.CreatedAt,
		model.UpdatedAt,
	)
	if err != nil {
		return err
	}

	return nil
}

func (agm *activityGroupManager) UpdateActivityGroup(
	ctx context.Context,
	ag *domain.ActivityGroup,
) error {
	var (
		q     = agm.tx.Querier(ctx)
		query = `
		UPDATE activity.activity_group SET
			title = $1,
			description = $2,
			activity_type = $3,
			difficulty_level = $4,
			visibility = $5,
			status = $6,
			location_city = $7,
			location_country = $8,
			timezone = $9,
			default_capacity = $10,
			updated_at = $11
		WHERE id = $12
	`
		model = mapDomainActivityGroupToModel(ag)
	)
	_, err := q.Exec(
		ctx,
		query,
		model.Title,
		model.Description,
		model.ActivityType,
		model.DifficultyLevel,
		model.Visibility,
		model.Status,
		model.LocationCity,
		model.LocationCountry,
		model.Timezone,
		model.DefaultCapacity,
		model.UpdatedAt,
		model.ID,
	)
	if err != nil {
		return err
	}

	return nil
}

func (agm *activityGroupManager) GetActivityGroupByID(
	ctx context.Context,
	ID uuid.UUID,
) (*domain.ActivityGroup, error) {
	var (
		q     = agm.tx.Querier(ctx)
		query = `
		SELECT
			id,
			creator_id,
			title,
			description,
			activity_type,
			difficulty_level,
			visibility,
			status,
			location_city,
			location_country,
			timezone,
			default_capacity,
			created_at,
			updated_at,
			cancelled_at
		FROM activity.activity_group
		WHERE id = $1
	`
	)
	row := q.QueryRow(ctx, query, ID)
	model, err := scanActivityGroupModel(row)
	if err != nil {
		return nil, err
	}

	return mapActivityGroupModelToDomain(model)
}

func (agm *activityGroupManager) CancelActivityGroup(
	ctx context.Context,
	ag *domain.ActivityGroup,
) error {
	var (
		q     = agm.tx.Querier(ctx)
		query = `
		UPDATE activity.activity_group
		SET
			status = $1,
			cancelled_at = $2,
			updated_at = $3
		WHERE id = $4
	`
	)
	_, err := q.Exec(
		ctx,
		query,
		ag.Status(),
		ag.CancelledAt(),
		ag.UpdatedAt(),
		ag.ID(),
	)
	if err != nil {
		return err
	}

	return nil
}

func (agm *activityGroupManager) ListActivityGroups(
	ctx context.Context,
	filter ActivityGroupFilter,
) ([]*domain.ActivityGroup, error) {
	query, args := buildActivityGroupQuery(filter)

	q := agm.tx.Querier(ctx)
	rows, err := q.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var activityGroups []*domain.ActivityGroup
	for rows.Next() {
		model, err := scanActivityGroupModel(rows)
		if err != nil {
			return nil, err
		}
		ag, err := mapActivityGroupModelToDomain(model)
		if err != nil {
			return nil, err
		}

		activityGroups = append(activityGroups, ag)
	}

	return activityGroups, nil
}

func (agm *activityGroupManager) DiscoverGroups(
	ctx context.Context,
	filter DiscoveryFilter,
) ([]*domain.ActivityGroup, error) {
	query, args := buildDiscoveryQuery(filter)

	q := agm.tx.Querier(ctx)
	rows, err := q.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var activityGroups []*domain.ActivityGroup
	for rows.Next() {
		model, err := scanActivityGroupModel(rows)
		if err != nil {
			return nil, err
		}
		ag, err := mapActivityGroupModelToDomain(model)
		if err != nil {
			return nil, err
		}

		activityGroups = append(activityGroups, ag)
	}

	return activityGroups, nil
}

func buildActivityGroupQuery(filter ActivityGroupFilter) (string, []interface{}) {
	qb := &postgres.QueryBuilder{
		BaseQuery: `
		SELECT
			id,
			creator_id,
			title,
			description,
			activity_type,
			difficulty_level,
			visibility,
			status,
			location_city,
			location_country,
			timezone,
			default_capacity,
			created_at,
			updated_at,
			cancelled_at
		FROM activity.activity_group
		WHERE 1=1`,
		Args: make([]any, 0),
	}

	if filter.ID != uuid.Nil {
		qb.AddCondition("id = ", filter.ID)
	}

	if filter.CreatorID != uuid.Nil {
		qb.AddCondition("creator_id = ", filter.CreatorID)
	}

	if filter.Title != "" {
		qb.AddLikeCondition("title ILIKE ", filter.Title)
	}

	if filter.Status != "" {
		qb.AddCondition("status = ", filter.Status)
	}

	qb.BaseQuery += ` ORDER BY created_at DESC`

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

func buildDiscoveryQuery(filter DiscoveryFilter) (string, []interface{}) {
	qb := &postgres.QueryBuilder{
		BaseQuery: `
		SELECT
			id,
			creator_id,
			title,
			description,
			activity_type,
			difficulty_level,
			visibility,
			status,
			location_city,
			location_country,
			timezone,
			default_capacity,
			created_at,
			updated_at,
			cancelled_at
		FROM activity.activity_group
		WHERE visibility = 'public' AND status = 'active'`,
		Args: make([]any, 0),
	}

	if filter.City != "" {
		qb.AddLikeCondition("location_city ILIKE ", filter.City)
	}

	if filter.Country != "" {
		qb.AddLikeCondition("location_country ILIKE ", filter.Country)
	}

	if filter.ActivityType != "" {
		qb.AddCondition("activity_type = ", filter.ActivityType)
	}

	qb.BaseQuery += ` ORDER BY created_at DESC`

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

func mapActivityGroupModelToDomain(
	model *activityGroupModel,
) (*domain.ActivityGroup, error) {
	location, err := domain.NewActivityGroupLocation(
		model.LocationCity,
		model.LocationCountry,
	)
	if err != nil {
		return nil, err
	}

	title, err := domain.NewTitle(model.Title)
	if err != nil {
		return nil, err
	}

	var capacity *domain.Capacity
	if model.DefaultCapacity.Valid {
		c, err := domain.NewCapacity(int(model.DefaultCapacity.Int32))
		if err != nil {
			return nil, err
		}
		capacity = &c
	}

	var cancelledAt *time.Time
	if model.CancelledAt.Valid {
		cancelledAt = &model.CancelledAt.Time
	}

	return domain.ReconstructActivityGroup(
		model.ID,
		model.CreatorID,
		title,
		model.Description,
		model.ActivityType,
		model.DifficultyLevel,
		model.Visibility,
		model.Status,
		location,
		model.Timezone,
		capacity,
		model.CreatedAt,
		model.UpdatedAt,
		cancelledAt,
	), nil
}

func mapDomainActivityGroupToModel(
	domain *domain.ActivityGroup,
) *activityGroupModel {
	var defaultCapacity sql.NullInt32
	if domain.DefaultCapacity() != nil {
		v := domain.DefaultCapacity().Capacity()
		defaultCapacity = sql.NullInt32{
			Int32: int32(v),
			Valid: true,
		}
	} else {
		defaultCapacity = sql.NullInt32{
			Valid: false,
		}
	}

	var cancelledAt sql.NullTime
	if domain.CancelledAt() != nil {
		cancelledAt = sql.NullTime{
			Time:  *domain.CancelledAt(),
			Valid: true,
		}
	} else {
		cancelledAt = sql.NullTime{
			Valid: false,
		}
	}

	return &activityGroupModel{
		ID:              domain.ID(),
		CreatorID:       domain.CreatorID(),
		Title:           domain.Title().Value(),
		Description:     domain.Description(),
		ActivityType:    domain.ActivityType(),
		DifficultyLevel: domain.DifficultyLevel(),
		Visibility:      domain.Visibility(),
		Status:          domain.Status(),
		LocationCity:    domain.Location().City(),
		LocationCountry: domain.Location().Country(),
		Timezone:        domain.Timezone(),
		DefaultCapacity: defaultCapacity,
		CreatedAt:       domain.CreatedAt(),
		UpdatedAt:       domain.UpdatedAt(),
		CancelledAt:     cancelledAt,
	}
}

type scanner interface {
	Scan(dest ...any) error
}

func scanActivityGroupModel(
	scanner scanner,
) (*activityGroupModel, error) {
	var model activityGroupModel
	err := scanner.Scan(
		&model.ID,
		&model.CreatorID,
		&model.Title,
		&model.Description,
		&model.ActivityType,
		&model.DifficultyLevel,
		&model.Visibility,
		&model.Status,
		&model.LocationCity,
		&model.LocationCountry,
		&model.Timezone,
		&model.DefaultCapacity,
		&model.CreatedAt,
		&model.UpdatedAt,
		&model.CancelledAt,
	)
	return &model, err
}
