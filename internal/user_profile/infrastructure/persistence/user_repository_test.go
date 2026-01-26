package persistence

import (
	"database/sql"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/rasparac/rekreativko-api/internal/user_profile/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_buildSingleProfileWhereQuery(t *testing.T) {
	accountUUID := uuid.New()
	testCase := []struct {
		name          string
		filter        UserProfileFilter
		expectedQuery string
		expectedArgs  []any
	}{
		{
			name: "only nickname",
			filter: UserProfileFilter{
				Nickname: ptr("john_doe"),
			},
			expectedQuery: "upp.nickname = $1 AND upp.deleted_at IS NULL",
			expectedArgs:  []any{"john_doe"},
		},
		{
			name: "only accountID",
			filter: UserProfileFilter{
				AccountID: &accountUUID,
			},
			expectedQuery: "upp.id = $1 AND upp.deleted_at IS NULL",
			expectedArgs:  []any{accountUUID.String()},
		},
		{
			name: "nickname and accountID",
			filter: UserProfileFilter{
				Nickname:  ptr("john_doe"),
				AccountID: &accountUUID,
			},
			expectedQuery: "upp.id = $1 AND upp.nickname = $2 AND upp.deleted_at IS NULL",
			expectedArgs:  []any{accountUUID.String(), "john_doe"},
		},
	}

	for _, tc := range testCase {
		t.Run(tc.name, func(t *testing.T) {
			query, args := buildSingleProfileWhereQuery(tc.filter)

			assert.Equal(t, tc.expectedQuery, query)
			assert.Equal(t, tc.expectedArgs, args)
		})
	}
}

func Test_buildMulitpleProfilesWhereQuery(t *testing.T) {
	accountUUID1 := uuid.New()
	accountUUID2 := uuid.New()

	date := time.Date(2024, 3, 15, 10, 0, 0, 0, time.UTC)

	testCase := []struct {
		name          string
		filter        UsersProfilesFilter
		expectedQuery string
		expectedArgs  []any
	}{
		{
			name: "only byIDs",
			filter: UsersProfilesFilter{
				ByIDs: []uuid.UUID{
					accountUUID1,
					accountUUID2,
				},
			},
			expectedQuery: "upp.id IN ($1, $2)",
			expectedArgs:  []any{accountUUID1.String(), accountUUID2.String()},
		},
		{
			name: "only byNicknames",
			filter: UsersProfilesFilter{
				ByNicknames: []string{
					"john_doe",
					"jane_doe",
				},
			},
			expectedQuery: "upp.nickname IN ($1, $2)",
			expectedArgs:  []any{"john_doe", "jane_doe"},
		},
		{
			name: "byIDs and byNicknames",
			filter: UsersProfilesFilter{
				ByIDs: []uuid.UUID{
					accountUUID1,
					accountUUID2,
				},
				ByNicknames: []string{
					"john_doe",
					"jane_doe",
				},
			},
			expectedQuery: "upp.id IN ($1, $2) AND upp.nickname IN ($3, $4)",
			expectedArgs:  []any{accountUUID1.String(), accountUUID2.String(), "john_doe", "jane_doe"},
		},
		{
			name: "byLocationCountry",
			filter: UsersProfilesFilter{
				ByLocationCountry: ptr("DE"),
			},
			expectedQuery: "upp.location_country = $1",
			expectedArgs:  []any{"DE"},
		},
		{
			name: "byLocationCity",
			filter: UsersProfilesFilter{
				ByLocationCity: ptr("Berlin"),
			},
			expectedQuery: "upp.location_city = $1",
			expectedArgs:  []any{"Berlin"},
		},
		{
			name: "by DateOfBirthOver",
			filter: UsersProfilesFilter{
				DateOfBirthOver: ptr(date.AddDate(-20, 0, 0)),
			},
			expectedQuery: "upp.date_of_birth > $1",
			expectedArgs:  []any{date.AddDate(-20, 0, 0)},
		},
	}

	for _, tc := range testCase {
		t.Run(tc.name, func(t *testing.T) {
			query, args := buildMulitpleProfilesWhereQuery(tc.filter)

			assert.Equal(t, tc.expectedQuery, query)
			assert.Equal(t, tc.expectedArgs, args)
		})
	}
}

func TestToUserProfileModel_FullProfile(t *testing.T) {
	id := uuid.New()
	now := time.Now().UTC()

	nickname, _ := domain.NewNickname("john_doe")
	fullName, _ := domain.NewFullName("John Doe")
	dob, _ := domain.NewDateOfBirth(time.Now().AddDate(-20, 0, 0))
	pic, _ := domain.NewProfilePicture("https://img.com/pic.jpg", now)
	loc, _ := domain.NewLocation("Berlin", "DE", ptr(52.52), ptr(13.405))

	up := domain.ReconstructUserProfile(
		id,
		nickname,
		fullName,
		pic,
		dob,
		"bio text",
		loc,
		nil,
		now,
		now,
		nil,
	)

	model := toUserProfileModel(up)

	assert.Equal(t, id, model.accountID)
	assert.True(t, model.nickname.Valid)
	assert.Equal(t, "john_doe", model.nickname.String)
	assert.True(t, model.profilePictureURL.Valid)
	assert.Equal(t, "https://img.com/pic.jpg", model.profilePictureURL.String)
	assert.True(t, model.locationLatitude.Valid)
	assert.Equal(t, 52.52, model.locationLatitude.Float64)
}

func TestToUserProfileModel_EmptyOptionalFields(t *testing.T) {
	id := uuid.New()
	now := time.Now().UTC()

	up := domain.ReconstructUserProfile(
		id,
		nil,
		nil,
		nil,
		nil,
		"",
		nil,
		nil,
		now,
		now,
		nil,
	)

	model := toUserProfileModel(up)

	assert.False(t, model.nickname.Valid)
	assert.False(t, model.fullName.Valid)
	assert.False(t, model.profilePictureURL.Valid)
	assert.False(t, model.dateOfBirth.Valid)
	assert.False(t, model.locationCity.Valid)
	assert.False(t, model.deletedAt.Valid)
}

func TestToDomainUserProfile_FullModel(t *testing.T) {
	id := uuid.New()
	now := time.Now().UTC()

	model := userProfile{
		accountID:       id,
		nickname:        sql.NullString{String: "john_doe", Valid: true},
		fullName:        sql.NullString{String: "John Doe", Valid: true},
		bio:             sql.NullString{String: "bio", Valid: true},
		locationCity:    sql.NullString{String: "Berlin", Valid: true},
		locationCountry: sql.NullString{String: "DE", Valid: true},
		createdAt:       now,
		updatedAt:       now,
		deletedAt:       sql.NullTime{},
	}

	interests := []userProfileActivityInterest{
		{
			activityType:  string(domain.ActivityTypeRunning),
			activityLevel: "beginner",
		},
	}

	up, err := toDomainUserProfile(model, interests)
	require.NoError(t, err)

	assert.Equal(t, id, up.ID())
	assert.Equal(t, "john_doe", up.Nickname().Value())
	assert.Equal(t, "Berlin", up.Location().City())
	assert.True(t, up.HasActivityInterest(domain.ActivityTypeRunning))
}

func TestToDomainUserProfile_InvalidNickname(t *testing.T) {
	model := userProfile{
		accountID: uuid.New(),
		nickname:  sql.NullString{String: "!!", Valid: true},
		createdAt: time.Now(),
		updatedAt: time.Now(),
	}

	_, err := toDomainUserProfile(model, nil)

	require.Error(t, err)
}

func TestToActivityInterestModels(t *testing.T) {
	id := uuid.New()

	interest, _ := domain.NewActivityInterest(domain.ActivityTypeRunning, domain.ActivityLevelBeginner)

	models := toActivityInterestModels(id, []*domain.ActivityInterest{interest})

	require.Len(t, models, 1)
	assert.Equal(t, id, models[0].accountID)
	assert.Equal(t, "running", models[0].activityType)
	assert.Equal(t, "beginner", models[0].activityLevel)
}

func TestToDomainActivityInterests_InvalidType(t *testing.T) {
	models := []userProfileActivityInterest{
		{
			activityType:  "invalid",
			activityLevel: "beginner",
		},
	}

	_, err := toDomainActivityInterests(models)

	require.Error(t, err)
}

func ptr[T any](v T) *T {
	return &v
}
