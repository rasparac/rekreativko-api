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
		ID              uuid.UUID
		CreatorID       uuid.UUID
		Title           string
		Status          domain.ActivityGroupStatus
		ActivityType    domain.ActivityType
		DifficultyLevel domain.DifficultyLevel
		// MemberID filters to groups this account has a membership row in.
		// MemberStatuses restricts which membership statuses count (e.g.
		// "pending" for outstanding join requests); the application layer
		// defaults it to ["confirmed"] when not otherwise specified, so
		// MemberID alone means "groups this account is a confirmed member
		// of" (includes ones they created, since the creator is auto-added
		// as a confirmed member) - existing callers see no behavior change.
		MemberID       uuid.UUID
		MemberStatuses []domain.MemberStatus
		// RequesterID scopes results to what this caller may actually see:
		// public groups, plus any private group they created or are a
		// confirmed member of. A private group they have no relationship to
		// is excluded entirely, not just access-denied on direct fetch. Note
		// this means a private group can still surface via someone else's
		// MemberID/CreatorID if the requester happens to independently be a
		// member of that same group too - Visibility below is the harder cap
		// the application layer uses to rule that out for profile-view.
		RequesterID uuid.UUID
		// Visibility, when set, restricts to that visibility only - used by
		// the application layer to force "public only" when MemberID or
		// CreatorID points at someone other than the requester, the same cap
		// SessionFilter applies for a session's AttendeeID/CreatedByID.
		Visibility domain.ActivityGroupVisibility
		Limit      int
		PageToken  string
	}

	DiscoveryFilter struct {
		City      string
		Country   string
		Interests []postgres.InterestPair
		Limit     int
		PageToken string
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
		DeletedAt       sql.NullTime
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
			cancelled_at,
			deleted_at
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

func (agm *activityGroupManager) DeleteActivityGroup(
	ctx context.Context,
	ag *domain.ActivityGroup,
) error {
	var (
		q     = agm.tx.Querier(ctx)
		query = `
		UPDATE activity.activity_group
		SET
			deleted_at = $1,
			updated_at = $2
		WHERE id = $3
	`
	)
	_, err := q.Exec(
		ctx,
		query,
		ag.DeletedAt(),
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
) ([]*domain.ActivityGroup, string, error) {
	query, args, err := buildActivityGroupQuery(filter)
	if err != nil {
		return nil, "", err
	}

	q := agm.tx.Querier(ctx)
	rows, err := q.Query(ctx, query, args...)
	if err != nil {
		return nil, "", err
	}
	defer rows.Close()

	var activityGroups []*domain.ActivityGroup
	for rows.Next() {
		model, err := scanActivityGroupModel(rows)
		if err != nil {
			return nil, "", err
		}
		ag, err := mapActivityGroupModelToDomain(model)
		if err != nil {
			return nil, "", err
		}

		activityGroups = append(activityGroups, ag)
	}

	page, nextPageToken := postgres.BuildPage(activityGroups, filter.Limit, func(ag *domain.ActivityGroup) (string, uuid.UUID) {
		return ag.CreatedAt().UTC().Format(time.RFC3339Nano), ag.ID()
	})

	return page, nextPageToken, nil
}

func (agm *activityGroupManager) DiscoverGroups(
	ctx context.Context,
	filter DiscoveryFilter,
) ([]*domain.ActivityGroup, string, error) {
	query, args, err := buildDiscoveryQuery(filter)
	if err != nil {
		return nil, "", err
	}

	q := agm.tx.Querier(ctx)
	rows, err := q.Query(ctx, query, args...)
	if err != nil {
		return nil, "", err
	}
	defer rows.Close()

	var activityGroups []*domain.ActivityGroup
	for rows.Next() {
		model, err := scanActivityGroupModel(rows)
		if err != nil {
			return nil, "", err
		}
		ag, err := mapActivityGroupModelToDomain(model)
		if err != nil {
			return nil, "", err
		}

		activityGroups = append(activityGroups, ag)
	}

	page, nextPageToken := postgres.BuildPage(activityGroups, filter.Limit, func(ag *domain.ActivityGroup) (string, uuid.UUID) {
		return ag.CreatedAt().UTC().Format(time.RFC3339Nano), ag.ID()
	})

	return page, nextPageToken, nil
}

func buildActivityGroupQuery(filter ActivityGroupFilter) (string, []interface{}, error) {
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
			cancelled_at,
			deleted_at
		FROM activity.activity_group
		WHERE deleted_at IS NULL`,
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

	if filter.ActivityType != "" {
		qb.AddCondition("activity_type = ", filter.ActivityType)
	}

	if filter.DifficultyLevel != "" {
		qb.AddCondition("difficulty_level = ", filter.DifficultyLevel)
	}

	if filter.Visibility != "" {
		qb.AddCondition("visibility = ", filter.Visibility)
	}

	if filter.MemberID != uuid.Nil {
		qb.ParamCount++
		memberParam := qb.ParamCount
		condition := fmt.Sprintf(
			" AND id IN (SELECT activity_group_id FROM activity.member WHERE account_id = $%d",
			memberParam,
		)
		qb.Args = append(qb.Args, filter.MemberID)

		if len(filter.MemberStatuses) > 0 {
			statuses := make([]string, len(filter.MemberStatuses))
			for i, st := range filter.MemberStatuses {
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
		// Public groups are visible to everyone; a private group only to its
		// creator or a confirmed member - anyone else must not see it in
		// listings at all, not just be denied on direct fetch.
		qb.ParamCount++
		requesterParam := qb.ParamCount
		qb.BaseQuery += fmt.Sprintf(
			` AND (visibility = 'public' OR creator_id = $%d OR id IN (SELECT activity_group_id FROM activity.member WHERE account_id = $%d AND status = 'confirmed' AND deleted_at IS NULL))`,
			requesterParam, requesterParam,
		)
		qb.Args = append(qb.Args, filter.RequesterID)
	}

	cursor, err := postgres.DecodePageToken(filter.PageToken)
	if err != nil {
		return "", nil, err
	}
	if cursor != nil {
		sortValue, err := time.Parse(time.RFC3339Nano, cursor.SortValue)
		if err != nil {
			return "", nil, postgres.ErrInvalidPageToken
		}
		qb.AddKeysetCondition("created_at", "DESC", sortValue, cursor.ID)
	}

	qb.BaseQuery += ` ORDER BY created_at DESC, id DESC`

	if filter.Limit > 0 {
		qb.ParamCount++
		qb.BaseQuery += fmt.Sprintf(" LIMIT $%d", qb.ParamCount)
		qb.Args = append(qb.Args, filter.Limit+1)
	}

	query, args := qb.Build()
	return query, args, nil
}

func buildDiscoveryQuery(filter DiscoveryFilter) (string, []interface{}, error) {
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
			cancelled_at,
			deleted_at
		FROM activity.activity_group
		WHERE visibility = 'public' AND status = 'active' AND deleted_at IS NULL`,
		Args: make([]any, 0),
	}

	if filter.City != "" {
		qb.AddLikeCondition("location_city ILIKE ", filter.City)
	}

	if filter.Country != "" {
		qb.AddLikeCondition("location_country ILIKE ", filter.Country)
	}

	qb.AddInterestsCondition("activity_type", "difficulty_level", filter.Interests)

	cursor, err := postgres.DecodePageToken(filter.PageToken)
	if err != nil {
		return "", nil, err
	}
	if cursor != nil {
		sortValue, err := time.Parse(time.RFC3339Nano, cursor.SortValue)
		if err != nil {
			return "", nil, postgres.ErrInvalidPageToken
		}
		qb.AddKeysetCondition("created_at", "DESC", sortValue, cursor.ID)
	}

	qb.BaseQuery += ` ORDER BY created_at DESC, id DESC`

	if filter.Limit > 0 {
		qb.ParamCount++
		qb.BaseQuery += fmt.Sprintf(" LIMIT $%d", qb.ParamCount)
		qb.Args = append(qb.Args, filter.Limit+1)
	}

	query, args := qb.Build()
	return query, args, nil
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

	var deletedAt *time.Time
	if model.DeletedAt.Valid {
		deletedAt = &model.DeletedAt.Time
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
		deletedAt,
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

	var deletedAt sql.NullTime
	if domain.DeletedAt() != nil {
		deletedAt = sql.NullTime{
			Time:  *domain.DeletedAt(),
			Valid: true,
		}
	} else {
		deletedAt = sql.NullTime{
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
		DeletedAt:       deletedAt,
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
		&model.DeletedAt,
	)
	return &model, err
}
