package persistence

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/rasparac/rekreativko-api/internal/account-profile/domain"
	"github.com/rasparac/rekreativko-api/internal/shared/logger"
	"github.com/rasparac/rekreativko-api/internal/shared/store/postgres"
)

type (
	accountProfile struct {
		accountID               uuid.UUID
		nickname                sql.NullString
		fullName                sql.NullString
		profilePictureURL       sql.NullString
		profilePictureUpdatedAt sql.NullTime
		dateOfBirth             sql.NullTime
		bio                     sql.NullString
		locationCity            sql.NullString
		locationCountry         sql.NullString
		locationLatitude        sql.NullFloat64
		locationLongitude       sql.NullFloat64
		createdAt               time.Time
		updatedAt               time.Time
		deletedAt               sql.NullTime

		activityInterests []accountProfileActivityInterest
	}

	accountProfileActivityInterest struct {
		ActivityType  string `json:"activity_type"`
		ActivityLevel string `json:"activity_level"`
	}

	AccountProfileFilter struct {
		AccountID *uuid.UUID
		Nickname  *string
		Limit     *int
		Offset    *int
	}

	AccountProfilesFilter struct {
		ByAccounIDs       []uuid.UUID
		ByNicknames       []string
		ByLocationCountry *string
		ByLocationCity    *string
		DateOfBirthOver   *time.Time
		DateOfBirthUnder  *time.Time
		Limit             *int
		Offset            *int

		IncludeDeleted *bool

		SortBy    *string
		SortOrder *string
	}

	AccountProfileReaderWriter interface {
		CreateAccountProfile(ctx context.Context, profile *domain.AccountProfile) error
		UpdateAccountProfile(ctx context.Context, profile *domain.AccountProfile) error
		DeleteAccountProfile(ctx context.Context, id uuid.UUID) error
		FindBy(ctx context.Context, filter AccountProfileFilter) (*domain.AccountProfile, error)
		FindAllBy(ctx context.Context, filter AccountProfilesFilter) ([]*domain.AccountProfile, error)
	}

	accountProfileManager struct {
		tx     *postgres.TransactionManager
		logger *logger.Logger
	}
)

// -- group by PK; other columns functionally dependent (PostgreSQL)
// this query is used for both FindBy and FindAllBy
const baseAccountProfileSelectBlueprint = `
	SELECT
		upp.account_id,
		upp.nickname,
		upp.full_name,
		upp.profile_picture_url,
		upp.profile_picture_uploaded_at,
		upp.date_of_birth,
		upp.bio,
		upp.location_city,
		upp.location_country,
		upp.location_latitude,
		upp.location_longitude,
		upp.created_at,
		upp.updated_at,
		upp.deleted_at,
		COALESCE(
			json_agg(
				json_build_object(
					'id', uai.id,
					'activity_type', uai.activity_type,
					'activity_level', uai.activity_level
				)
			) FILTER (WHERE uai.id IS NOT NULL),
			'[]'
		) AS activity_interests
	FROM account_profile.profiles upp
	LEFT JOIN account_profile.activity_interests uai
		ON upp.account_id = uai.account_id

	-- filters
	WHERE
	%s	
	GROUP BY upp.account_id
	--limiting and ordering
	%s
`

func NewAccountProfileManager(
	tx *postgres.TransactionManager,
	logger *logger.Logger,
) *accountProfileManager {
	return &accountProfileManager{
		tx:     tx,
		logger: logger,
	}
}

func (m *accountProfileManager) CreateAccountProfile(
	ctx context.Context,
	profile *domain.AccountProfile,
) error {
	var (
		query = `INSERT INTO account_profile.profiles
			(
				account_id,
				nickname,
				full_name,
				profile_picture_url,
				profile_picture_uploaded_at,
				date_of_birth,
				bio,
				location_city,
				location_country,
				location_latitude,
				location_longitude,
				created_at,
				updated_at
			)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)`
		model = toAccountProfileModel(profile)
		tx    = m.tx.Querier(ctx)
	)

	_, err := tx.Exec(
		ctx,
		query,
		model.accountID,
		model.nickname,
		model.fullName,
		model.profilePictureURL,
		model.profilePictureUpdatedAt,
		model.dateOfBirth,
		model.bio,
		model.locationCity,
		model.locationCountry,
		model.locationLatitude,
		model.locationLongitude,
		model.createdAt,
		model.updatedAt,
	)
	if err != nil {
		return err
	}

	return nil
}

func (m *accountProfileManager) UpdateAccountProfile(
	ctx context.Context,
	profile *domain.AccountProfile,
) error {
	var (
		query = `UPDATE account_profile.profiles
			SET
				nickname = $1,
				full_name = $2,
				profile_picture_url = $3,
				profile_picture_uploaded_at = $4,
				date_of_birth = $5,
				bio = $6,
				location_city = $7,
				location_country = $8,
				location_latitude = $9,
				location_longitude = $10,
				updated_at = $11
			WHERE account_id = $12`
		model = toAccountProfileModel(profile)
		tx    = m.tx.Querier(ctx)
	)

	_, err := tx.Exec(
		ctx,
		query,
		model.nickname,
		model.fullName,
		model.profilePictureURL,
		model.profilePictureUpdatedAt,
		model.dateOfBirth,
		model.bio,
		model.locationCity,
		model.locationCountry,
		model.locationLatitude,
		model.locationLongitude,
		model.updatedAt,
		model.accountID,
	)
	if err != nil {
		return err
	}

	err = m.replaceActivityInterests(
		ctx,
		tx,
		model.accountID,
		profile.ActivityInterests(),
	)
	if err != nil {
		return err
	}
	return nil
}

func (m *accountProfileManager) DeleteAccountProfile(
	ctx context.Context,
	id uuid.UUID,
) error {
	var (
		query = `UPDATE account_profile
			SET
				deleted_at = $2
				updated_at = $2
			WHERE id = $1 AND deleted_at IS NULL`
		tx = m.tx.Querier(ctx)
	)

	_, err := tx.Exec(ctx, query, id, time.Now().UTC())
	return err
}

func (m *accountProfileManager) FindBy(
	ctx context.Context,
	filter AccountProfileFilter,
) (*domain.AccountProfile, error) {
	var (
		whereClause, args = buildSingleProfileWhereQuery(filter)
		query             = fmt.Sprintf(
			baseAccountProfileSelectBlueprint,
			whereClause,
			"LIMIT 1",
		)

		model                 accountProfile
		tx                    = m.tx.Querier(ctx)
		activityInterestsJSON sql.NullString
	)
	err := tx.QueryRow(ctx, query, args...).Scan(
		&model.accountID,
		&model.nickname,
		&model.fullName,
		&model.profilePictureURL,
		&model.profilePictureUpdatedAt,
		&model.dateOfBirth,
		&model.bio,
		&model.locationCity,
		&model.locationCountry,
		&model.locationLatitude,
		&model.locationLongitude,
		&model.createdAt,
		&model.updatedAt,
		&model.deletedAt,
		&activityInterestsJSON,
	)
	if err != nil {
		return nil, err
	}

	var activityInterests []accountProfileActivityInterest
	if activityInterestsJSON.Valid {
		err = json.Unmarshal([]byte(activityInterestsJSON.String), &activityInterests)
		if err != nil {
			return nil, err
		}
	}

	dm, err := toDomainAccountProfile(model, activityInterests)
	if err != nil {
		return nil, err
	}
	return dm, nil
}

func (m *accountProfileManager) FindAllBy(
	ctx context.Context,
	filter AccountProfilesFilter,
) ([]*domain.AccountProfile, error) {
	var (
		whereClause, args = buildMulitpleProfilesWhereQuery(filter)
		tx                = m.tx.Querier(ctx)
	)

	query := fmt.Sprintf(
		baseAccountProfileSelectBlueprint,
		whereClause,
		"",
	)

	if filter.SortBy != nil {
		sortOrder := "ASC"
		if filter.SortOrder != nil {
			sortOrder = *filter.SortOrder
		}
		query = fmt.Sprintf(
			"%s ORDER BY %s %s",
			query,
			*filter.SortBy,
			sortOrder,
		)
	}

	limit := 50
	if filter.Limit != nil {
		limit = *filter.Limit
	}

	query = fmt.Sprintf(
		"%s LIMIT %d",
		query,
		limit,
	)

	var activityInterestsJSON sql.NullString
	rows, err := tx.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var profiles []accountProfile
	for rows.Next() {
		var model accountProfile
		scanErr := rows.Scan(
			&model.accountID,
			&model.nickname,
			&model.fullName,
			&model.profilePictureURL,
			&model.profilePictureUpdatedAt,
			&model.dateOfBirth,
			&model.bio,
			&model.locationCity,
			&model.locationCountry,
			&model.locationLatitude,
			&model.locationLongitude,
			&model.createdAt,
			&model.updatedAt,
			&model.deletedAt,
			&activityInterestsJSON,
		)
		if scanErr != nil {
			return nil, scanErr
		}

		var activityInterests []accountProfileActivityInterest
		if activityInterestsJSON.Valid {
			err = json.Unmarshal([]byte(activityInterestsJSON.String), &activityInterests)
			if err != nil {
				return nil, err
			}
		}

		model.activityInterests = activityInterests
		profiles = append(profiles, model)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	dm := make([]*domain.AccountProfile, len(profiles))
	for i, profile := range profiles {
		dm[i], err = toDomainAccountProfile(profile, profile.activityInterests)
		if err != nil {
			return nil, err
		}
	}
	return dm, nil
}

func (m *accountProfileManager) replaceActivityInterests(
	ctx context.Context,
	querier postgres.Querier,
	accountID uuid.UUID,
	interests []*domain.ActivityInterest,
) error {
	deleteQuery := `DELETE FROM account_profile.activity_interests WHERE account_id = $1`
	_, err := querier.Exec(ctx, deleteQuery, accountID)
	if err != nil {
		return err
	}

	if len(interests) == 0 {
		return nil
	}

	insertQuery := `INSERT INTO account_profile.activity_interests
		(
			account_id,
			activity_type,
			activity_level,
			created_at
		)
		VALUES ($1, $2, $3, $4)`

	models := toActivityInterestModels(accountID, interests)
	for _, model := range models {
		_, err := querier.Exec(
			ctx,
			insertQuery,
			accountID,
			model.ActivityType,
			model.ActivityLevel,
			time.Now(),
		)
		if err != nil {
			return err
		}
	}
	return nil
}

func toAccountProfileModel(profile *domain.AccountProfile) accountProfile {
	model := accountProfile{
		accountID: profile.ID(),
		createdAt: profile.CreatedAt(),
		updatedAt: profile.UpdatedAt(),
		bio: sql.NullString{
			String: profile.Bio(),
			Valid:  profile.Bio() != "",
		},
	}

	if profile.Nickname() != nil {
		model.nickname = sql.NullString{
			String: profile.Nickname().Value(),
			Valid:  profile.Nickname().Value() != "",
		}
	}

	if profile.FullName() != nil {
		model.fullName = sql.NullString{
			String: profile.FullName().Value(),
			Valid:  profile.FullName().Value() != "",
		}
	}

	if profile.ProfilePicture() != nil {
		model.profilePictureURL = sql.NullString{
			String: profile.ProfilePicture().URL(),
			Valid:  profile.ProfilePicture().URL() != "",
		}

		model.profilePictureUpdatedAt = sql.NullTime{
			Time:  profile.ProfilePicture().UploadedAt(),
			Valid: !profile.ProfilePicture().UploadedAt().IsZero(),
		}
	}

	if profile.DateOfBirth() != nil {
		model.dateOfBirth = sql.NullTime{
			Time:  profile.DateOfBirth().Value(),
			Valid: !profile.DateOfBirth().Value().IsZero(),
		}
	}

	if profile.Location() != nil {
		model.locationCity = sql.NullString{
			String: profile.Location().City(),
			Valid:  profile.Location().City() != "",
		}
		model.locationCountry = sql.NullString{
			String: profile.Location().Country(),
			Valid:  profile.Location().Country() != "",
		}

		if profile.Location().HasCoordinates() {
			model.locationLatitude = sql.NullFloat64{
				Float64: profile.Location().Coordinates().Latitude(),
				Valid:   true,
			}
			model.locationLongitude = sql.NullFloat64{
				Float64: profile.Location().Coordinates().Longitude(),
				Valid:   true,
			}
		}
	}

	if profile.DeletedAt() != nil {
		model.deletedAt = sql.NullTime{
			Time:  *profile.DeletedAt(),
			Valid: true,
		}
	}

	return model
}

func toDomainAccountProfile(
	model accountProfile,
	interests []accountProfileActivityInterest,
) (*domain.AccountProfile, error) {
	var err error

	var nickname *domain.Nickname
	if model.nickname.Valid {
		nickname, err = domain.NewNickname(model.nickname.String)
		if err != nil {
			return nil, err
		}
	}

	var fullName *domain.FullName
	if model.fullName.Valid {
		fullName, err = domain.NewFullName(model.fullName.String)
		if err != nil {
			return nil, err
		}
	}

	var profilePicture *domain.ProfilePicture
	if model.profilePictureURL.Valid && model.profilePictureURL.String != "" {
		profilePicture, err = domain.NewProfilePicture(
			model.profilePictureURL.String,
			model.profilePictureUpdatedAt.Time,
		)
		if err != nil {
			return nil, err
		}
	}

	var dateOfBirth *domain.DateOfBirth
	if model.dateOfBirth.Valid {
		dateOfBirth, err = domain.NewDateOfBirth(model.dateOfBirth.Time)
		if err != nil {
			return nil, err
		}
	}

	var location *domain.Location
	if model.locationCity.Valid && model.locationCountry.Valid {
		var latitude, longitude *float64
		if model.locationLatitude.Valid {
			latitude = &model.locationLatitude.Float64
		}
		if model.locationLongitude.Valid {
			longitude = &model.locationLongitude.Float64
		}

		location, err = domain.NewLocation(
			model.locationCity.String,
			model.locationCountry.String,
			latitude,
			longitude,
		)
		if err != nil {
			return nil, err
		}
	}

	activityInterests, err := toDomainActivityInterests(interests)
	if err != nil {
		return nil, err
	}

	var deletedAt *time.Time
	if model.deletedAt.Valid {
		deletedAt = &model.deletedAt.Time
	}

	dm := domain.ReconstructAccountProfile(
		model.accountID,
		nickname,
		fullName,
		profilePicture,
		dateOfBirth,
		model.bio.String,
		location,
		activityInterests,
		model.createdAt,
		model.updatedAt,
		deletedAt,
	)

	return dm, nil
}

func toActivityInterestModels(_ uuid.UUID, interests []*domain.ActivityInterest) []accountProfileActivityInterest {
	models := make([]accountProfileActivityInterest, len(interests))
	for i, interest := range interests {
		models[i] = accountProfileActivityInterest{
			ActivityType:  string(interest.ActivityType()),
			ActivityLevel: string(interest.Level()),
		}
	}
	return models
}

func toDomainActivityInterests(models []accountProfileActivityInterest) ([]*domain.ActivityInterest, error) {
	interests := make([]*domain.ActivityInterest, len(models))
	for i, model := range models {
		interest, err := domain.NewActivityInterest(
			domain.ActivityType(model.ActivityType),
			domain.ActivityLevel(model.ActivityLevel),
		)
		if err != nil {
			return nil, err
		}
		interests[i] = interest
	}
	return interests, nil
}

// apply filters and limit to baseAccountProfileSelectBlueprint
func buildSingleProfileWhereQuery(filter AccountProfileFilter) (string, []any) {
	var (
		conditions []string
		args       []any
	)

	if filter.AccountID != nil {
		conditions = append(conditions, fmt.Sprintf("upp.account_id = $%d", len(args)+1))
		accountID := filter.AccountID.String()
		args = append(args, accountID)
	}

	if filter.Nickname != nil {
		conditions = append(conditions, fmt.Sprintf("upp.nickname = $%d", len(args)+1))
		args = append(args, *filter.Nickname)
	}

	conditions = append(conditions, "upp.deleted_at IS NULL")

	return strings.Join(conditions, " AND "), args
}

func buildMulitpleProfilesWhereQuery(filter AccountProfilesFilter) (string, []any) {
	var (
		conditions []string
		args       []any
	)

	if len(filter.ByAccounIDs) > 0 {
		idPlaceholders := make([]string, len(filter.ByAccounIDs))
		for i, id := range filter.ByAccounIDs {
			idPlaceholders[i] = fmt.Sprintf("$%d", len(args)+1)
			accountID := id.String()
			args = append(args, accountID)
		}
		conditions = append(conditions, fmt.Sprintf("upp.account_id IN (%s)", strings.Join(idPlaceholders, ", ")))
	}

	if len(filter.ByNicknames) > 0 {
		nicknamePlaceholders := make([]string, len(filter.ByNicknames))
		for i, nickname := range filter.ByNicknames {
			nicknamePlaceholders[i] = fmt.Sprintf("$%d", len(args)+1)
			args = append(args, nickname)
		}
		conditions = append(conditions, fmt.Sprintf("upp.nickname IN (%s)", strings.Join(nicknamePlaceholders, ", ")))
	}

	if filter.ByLocationCountry != nil {
		conditions = append(conditions, fmt.Sprintf("upp.location_country = $%d", len(args)+1))
		args = append(args, *filter.ByLocationCountry)
	}

	if filter.ByLocationCity != nil {
		conditions = append(conditions, fmt.Sprintf("upp.location_city = $%d", len(args)+1))
		args = append(args, *filter.ByLocationCity)
	}

	if filter.DateOfBirthOver != nil {
		conditions = append(conditions, fmt.Sprintf("upp.date_of_birth > $%d", len(args)+1))
		args = append(args, *filter.DateOfBirthOver)
	}

	if filter.DateOfBirthUnder != nil {
		conditions = append(conditions, fmt.Sprintf("upp.date_of_birth < $%d", len(args)+1))
		args = append(args, *filter.DateOfBirthUnder)
	}

	if filter.IncludeDeleted != nil {
		if *filter.IncludeDeleted {
			conditions = append(conditions, "upp.deleted_at IS NOT NULL")
		} else {
			conditions = append(conditions, "upp.deleted_at IS NULL")
		}
	}

	return strings.Join(conditions, " AND "), args
}
