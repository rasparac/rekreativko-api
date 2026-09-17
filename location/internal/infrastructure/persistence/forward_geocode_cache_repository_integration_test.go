//go:build integration

package persistence

import (
	"context"
	"errors"
	"testing"

	"github.com/rasparac/rekreativko-api/location/internal/domain"
	testutil "github.com/rasparac/rekreativko-api/shared/testing"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestForwardGeocodeCacheRepository_FindByQuery_Miss(t *testing.T) {
	db := setupTestDB(t)
	repo := NewForwardGeocodeCacheRepository(db.CreateTransactionManager(), testutil.CreateLogger())

	ctx := context.Background()

	result, err := repo.FindByQuery(ctx, "|zagreb|croatia")
	assert.Nil(t, result)
	require.True(t, errors.Is(err, domain.ErrCacheMiss))
}

func TestForwardGeocodeCacheRepository_SaveNotFoundAndFindByQuery(t *testing.T) {
	db := setupTestDB(t)
	repo := NewForwardGeocodeCacheRepository(db.CreateTransactionManager(), testutil.CreateLogger())

	ctx := context.Background()

	require.NoError(t, repo.SaveNotFound(ctx, "|zagreb|croatia"))

	result, err := repo.FindByQuery(ctx, "|zagreb|croatia")
	assert.Nil(t, result)
	require.True(t, errors.Is(err, domain.ErrLocationNotFound))
}

func TestForwardGeocodeCacheRepository_Save_ClearsPriorNotFound(t *testing.T) {
	db := setupTestDB(t)
	repo := NewForwardGeocodeCacheRepository(db.CreateTransactionManager(), testutil.CreateLogger())

	ctx := context.Background()

	require.NoError(t, repo.SaveNotFound(ctx, "|zagreb|croatia"))

	resolved := &domain.ForwardGeocodeResult{
		Latitude:    45.815,
		Longitude:   15.982,
		DisplayName: "Zagreb, Croatia",
		City:        "Zagreb",
		Country:     "Croatia",
		CountryCode: "hr",
	}
	require.NoError(t, repo.Save(ctx, "|zagreb|croatia", resolved))

	got, err := repo.FindByQuery(ctx, "|zagreb|croatia")
	require.NoError(t, err)
	assert.Equal(t, resolved, got)
}

func TestForwardGeocodeCacheRepository_SaveAndFindByQuery(t *testing.T) {
	db := setupTestDB(t)
	repo := NewForwardGeocodeCacheRepository(db.CreateTransactionManager(), testutil.CreateLogger())

	ctx := context.Background()

	want := &domain.ForwardGeocodeResult{
		Latitude:    45.815,
		Longitude:   15.982,
		DisplayName: "Zagreb, Croatia",
		City:        "Zagreb",
		Country:     "Croatia",
		CountryCode: "hr",
	}

	require.NoError(t, repo.Save(ctx, "|zagreb|croatia", want))

	got, err := repo.FindByQuery(ctx, "|zagreb|croatia")
	require.NoError(t, err)
	assert.Equal(t, want, got)
}

func TestForwardGeocodeCacheRepository_Save_OverwritesExistingQuery(t *testing.T) {
	db := setupTestDB(t)
	repo := NewForwardGeocodeCacheRepository(db.CreateTransactionManager(), testutil.CreateLogger())

	ctx := context.Background()

	require.NoError(t, repo.Save(ctx, "|zagreb|croatia", &domain.ForwardGeocodeResult{
		Latitude:  45.815,
		Longitude: 15.982,
		City:      "Zagreb",
		Country:   "Croatia",
	}))

	updated := &domain.ForwardGeocodeResult{
		Latitude:  45.816,
		Longitude: 15.983,
		City:      "Zagreb (updated)",
		Country:   "Croatia",
	}
	require.NoError(t, repo.Save(ctx, "|zagreb|croatia", updated))

	got, err := repo.FindByQuery(ctx, "|zagreb|croatia")
	require.NoError(t, err)
	assert.Equal(t, updated, got)
}

func TestForwardGeocodeCacheRepository_DistinctQueriesDoNotCollide(t *testing.T) {
	db := setupTestDB(t)
	repo := NewForwardGeocodeCacheRepository(db.CreateTransactionManager(), testutil.CreateLogger())

	ctx := context.Background()

	zagreb := &domain.ForwardGeocodeResult{Latitude: 45.815, Longitude: 15.982, City: "Zagreb", Country: "Croatia"}
	split := &domain.ForwardGeocodeResult{Latitude: 43.508, Longitude: 16.440, City: "Split", Country: "Croatia"}

	require.NoError(t, repo.Save(ctx, "|zagreb|croatia", zagreb))
	require.NoError(t, repo.Save(ctx, "|split|croatia", split))

	gotZagreb, err := repo.FindByQuery(ctx, "|zagreb|croatia")
	require.NoError(t, err)
	assert.Equal(t, zagreb, gotZagreb)

	gotSplit, err := repo.FindByQuery(ctx, "|split|croatia")
	require.NoError(t, err)
	assert.Equal(t, split, gotSplit)
}
