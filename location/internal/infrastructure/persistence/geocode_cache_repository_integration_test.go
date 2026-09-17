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

func setupTestDB(t *testing.T) *testutil.TestDatabase {
	t.Helper()

	db := testutil.NewTestDatabase(t)
	db.RunMigrationsForService()

	return db
}

func TestGeocodeCacheRepository_FindByBucket_Miss(t *testing.T) {
	db := setupTestDB(t)
	repo := NewGeocodeCacheRepository(db.CreateTransactionManager(), testutil.CreateLogger())

	ctx := context.Background()

	result, err := repo.FindByBucket(ctx, 45.815, 15.982)
	assert.Nil(t, result)
	require.True(t, errors.Is(err, domain.ErrCacheMiss))
}

func TestGeocodeCacheRepository_SaveNotFoundAndFindByBucket(t *testing.T) {
	db := setupTestDB(t)
	repo := NewGeocodeCacheRepository(db.CreateTransactionManager(), testutil.CreateLogger())

	ctx := context.Background()

	require.NoError(t, repo.SaveNotFound(ctx, 45.815, 15.982))

	result, err := repo.FindByBucket(ctx, 45.815, 15.982)
	assert.Nil(t, result)
	require.True(t, errors.Is(err, domain.ErrLocationNotFound))
}

func TestGeocodeCacheRepository_Save_ClearsPriorNotFound(t *testing.T) {
	db := setupTestDB(t)
	repo := NewGeocodeCacheRepository(db.CreateTransactionManager(), testutil.CreateLogger())

	ctx := context.Background()

	require.NoError(t, repo.SaveNotFound(ctx, 45.815, 15.982))

	resolved := &domain.GeocodeResult{City: "Zagreb", Country: "Croatia", CountryCode: "hr"}
	require.NoError(t, repo.Save(ctx, 45.815, 15.982, resolved))

	got, err := repo.FindByBucket(ctx, 45.815, 15.982)
	require.NoError(t, err)
	assert.Equal(t, resolved, got)
}

func TestGeocodeCacheRepository_SaveAndFindByBucket(t *testing.T) {
	db := setupTestDB(t)
	repo := NewGeocodeCacheRepository(db.CreateTransactionManager(), testutil.CreateLogger())

	ctx := context.Background()

	want := &domain.GeocodeResult{
		City:        "Zagreb",
		Country:     "Croatia",
		CountryCode: "hr",
	}

	require.NoError(t, repo.Save(ctx, 45.815, 15.982, want))

	got, err := repo.FindByBucket(ctx, 45.815, 15.982)
	require.NoError(t, err)
	assert.Equal(t, want, got)
}

func TestGeocodeCacheRepository_Save_OverwritesExistingBucket(t *testing.T) {
	db := setupTestDB(t)
	repo := NewGeocodeCacheRepository(db.CreateTransactionManager(), testutil.CreateLogger())

	ctx := context.Background()

	require.NoError(t, repo.Save(ctx, 45.815, 15.982, &domain.GeocodeResult{
		City:        "Zagreb",
		Country:     "Croatia",
		CountryCode: "hr",
	}))

	updated := &domain.GeocodeResult{
		City:        "Zagreb (updated)",
		Country:     "Croatia",
		CountryCode: "hr",
	}
	require.NoError(t, repo.Save(ctx, 45.815, 15.982, updated))

	got, err := repo.FindByBucket(ctx, 45.815, 15.982)
	require.NoError(t, err)
	assert.Equal(t, updated, got)
}

func TestGeocodeCacheRepository_DistinctBucketsDoNotCollide(t *testing.T) {
	db := setupTestDB(t)
	repo := NewGeocodeCacheRepository(db.CreateTransactionManager(), testutil.CreateLogger())

	ctx := context.Background()

	zagreb := &domain.GeocodeResult{City: "Zagreb", Country: "Croatia", CountryCode: "hr"}
	split := &domain.GeocodeResult{City: "Split", Country: "Croatia", CountryCode: "hr"}

	require.NoError(t, repo.Save(ctx, 45.815, 15.982, zagreb))
	require.NoError(t, repo.Save(ctx, 43.508, 16.440, split))

	gotZagreb, err := repo.FindByBucket(ctx, 45.815, 15.982)
	require.NoError(t, err)
	assert.Equal(t, zagreb, gotZagreb)

	gotSplit, err := repo.FindByBucket(ctx, 43.508, 16.440)
	require.NoError(t, err)
	assert.Equal(t, split, gotSplit)
}
