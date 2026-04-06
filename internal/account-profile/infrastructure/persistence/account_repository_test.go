package persistence

import (
	"database/sql"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/rasparac/rekreativko-api/internal/account-profile/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_buildSingleProfileWhereQuery(t *testing.T) {
	accountUUID := uuid.New()
	testCase := []struct {
		name          string
		filter        AccountProfileFilter
		expectedQuery string
		expectedArgs  []any
	}{
		{
			name: "only nickname",
			filter: AccountProfileFilter{
				Nickname: ptr("john_doe"),
			},
			expectedQuery: "upp.nickname = $1 AND upp.deleted_at IS NULL",
			expectedArgs:  []any{"john_doe"},
		},
		{
			name: "only accountID",
			filter: AccountProfileFilter{
				AccountID: &accountUUID,
			},
			expectedQuery: "upp.id = $1 AND upp.deleted_at IS NULL",
			expectedArgs:  []any{accountUUID.String()},
		},
		{
			name: "nickname and accountID",
			filter: AccountProfileFilter{
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
		filter        AccountProfilesFilter
		expectedQuery string
		expectedArgs  []any
	}{
		{
			name: "only ByAccounIDs",
			filter: AccountProfilesFilter{
				ByAccounIDs: []uuid.UUID{
					accountUUID1,
					accountUUID2,
				},
			},
			expectedQuery: "upp.id IN ($1, $2)",
			expectedArgs:  []any{accountUUID1.String(), accountUUID2.String()},
		},
		{
			name: "only byNicknames",
			filter: AccountProfilesFilter{
				ByNicknames: []string{
					"john_doe",
					"jane_doe",
				},
			},
			expectedQuery: "upp.nickname IN ($1, $2)",
			expectedArgs:  []any{"john_doe", "jane_doe"},
		},
		{
			name: "ByAccounIDs and byNicknames",
			filter: AccountProfilesFilter{
				ByAccounIDs: []uuid.UUID{
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
			filter: AccountProfilesFilter{
				ByLocationCountry: ptr("DE"),
			},
			expectedQuery: "upp.location_country = $1",
			expectedArgs:  []any{"DE"},
		},
		{
			name: "byLocationCity",
			filter: AccountProfilesFilter{
				ByLocationCity: ptr("Berlin"),
			},
			expectedQuery: "upp.location_city = $1",
			expectedArgs:  []any{"Berlin"},
		},
		{
			name: "by DateOfBirthOver",
			filter: AccountProfilesFilter{
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

func TestToAccountProfileModel_FullProfile(t *testing.T) {
	id := uuid.New()
	now := time.Now().UTC()

	nickname, _ := domain.NewNickname("john_doe")
	fullName, _ := domain.NewFullName("John Doe")
	dob, _ := domain.NewDateOfBirth(time.Now().AddDate(-20, 0, 0))
	pic, _ := domain.NewProfilePicture("https://img.com/pic.jpg", now)
	loc, _ := domain.NewLocation("Berlin", "DE", ptr(52.52), ptr(13.405))

	up := domain.ReconstructAccountProfile(
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

	model := toAccountProfileModel(up)

	assert.Equal(t, id, model.accountID)
	assert.True(t, model.nickname.Valid)
	assert.Equal(t, "john_doe", model.nickname.String)
	assert.True(t, model.profilePictureURL.Valid)
	assert.Equal(t, "https://img.com/pic.jpg", model.profilePictureURL.String)
	assert.True(t, model.locationLatitude.Valid)
	assert.Equal(t, 52.52, model.locationLatitude.Float64)
}

func TestToAccountProfileModel_EmptyOptionalFields(t *testing.T) {
	id := uuid.New()
	now := time.Now().UTC()

	up := domain.ReconstructAccountProfile(
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

	model := toAccountProfileModel(up)

	assert.False(t, model.nickname.Valid)
	assert.False(t, model.fullName.Valid)
	assert.False(t, model.profilePictureURL.Valid)
	assert.False(t, model.dateOfBirth.Valid)
	assert.False(t, model.locationCity.Valid)
	assert.False(t, model.deletedAt.Valid)
}

func TestToDomainAccountProfile_FullModel(t *testing.T) {
	id := uuid.New()
	now := time.Now().UTC()

	model := accountProfile{
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

	interests := []accountProfileActivityInterest{
		{
			ActivityType:  string(domain.ActivityTypeRunning),
			ActivityLevel: "beginner",
		},
	}

	up, err := toDomainAccountProfile(model, interests)
	require.NoError(t, err)

	assert.Equal(t, id, up.ID())
	assert.Equal(t, "john_doe", up.Nickname().Value())
	assert.Equal(t, "Berlin", up.Location().City())
	assert.True(t, up.HasActivityInterest(domain.ActivityTypeRunning))
}

func TestToDomainAccountProfile_InvalidNickname(t *testing.T) {
	model := accountProfile{
		accountID: uuid.New(),
		nickname:  sql.NullString{String: "!!", Valid: true},
		createdAt: time.Now(),
		updatedAt: time.Now(),
	}

	_, err := toDomainAccountProfile(model, nil)

	require.Error(t, err)
}

func TestToActivityInterestModels(t *testing.T) {
	id := uuid.New()

	interest, _ := domain.NewActivityInterest(domain.ActivityTypeRunning, domain.ActivityLevelBeginner)

	models := toActivityInterestModels(id, []*domain.ActivityInterest{interest})

	require.Len(t, models, 1)
	assert.Equal(t, "running", models[0].ActivityType)
	assert.Equal(t, "beginner", models[0].ActivityLevel)
}

func TestToDomainActivityInterests_InvalidType(t *testing.T) {
	models := []accountProfileActivityInterest{
		{
			ActivityType:  "invalid",
			ActivityLevel: "beginner",
		},
	}

	_, err := toDomainActivityInterests(models)

	require.Error(t, err)
}

func ptr[T any](v T) *T {
	return &v
}
