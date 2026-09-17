package persistence

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/rasparac/rekreativko-api/location/internal/domain"
	"github.com/rasparac/rekreativko-api/shared/logger"
	"github.com/rasparac/rekreativko-api/shared/store/postgres"
)

type geocodeCacheModel struct {
	city        sql.NullString
	country     sql.NullString
	countryCode sql.NullString
	notFound    bool
}

// GeocodeCacheRepository persists resolved City/Country lookups keyed by a
// rounded lat/long grid bucket (see domain.Coordinates.Bucket), so repeated
// lookups for nearby coordinates avoid another Nominatim call. City/Country
// resolution is treated as effectively immutable - entries have no TTL and
// are only ever overwritten by a fresher Nominatim answer for the same bucket.
//
// A bucket Nominatim definitively couldn't resolve is cached too (via
// SaveNotFound) - FindByBucket returns domain.ErrLocationNotFound for it,
// distinct from domain.ErrCacheMiss (nothing cached yet, go ask Nominatim).
type GeocodeCacheRepository interface {
	FindByBucket(ctx context.Context, latBucket, lngBucket float64) (*domain.GeocodeResult, error)
	Save(ctx context.Context, latBucket, lngBucket float64, result *domain.GeocodeResult) error
	SaveNotFound(ctx context.Context, latBucket, lngBucket float64) error
}

type geocodeCacheManager struct {
	tx     *postgres.TransactionManager
	logger *logger.Logger
}

// NewGeocodeCacheRepository creates a new geocode cache repository.
func NewGeocodeCacheRepository(
	tx *postgres.TransactionManager,
	logger *logger.Logger,
) GeocodeCacheRepository {
	return &geocodeCacheManager{
		tx:     tx,
		logger: logger,
	}
}

func (m *geocodeCacheManager) FindByBucket(
	ctx context.Context,
	latBucket, lngBucket float64,
) (*domain.GeocodeResult, error) {
	query := `
		SELECT city, country, country_code, not_found
		FROM location.geocode_cache
		WHERE lat_bucket = $1 AND lng_bucket = $2
	`

	q := m.tx.Querier(ctx)

	var model geocodeCacheModel
	err := q.QueryRow(ctx, query, latBucket, lngBucket).Scan(
		&model.city,
		&model.country,
		&model.countryCode,
		&model.notFound,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrCacheMiss
		}
		return nil, fmt.Errorf("failed to query geocode cache: %w", err)
	}

	if model.notFound {
		return nil, domain.ErrLocationNotFound
	}

	return &domain.GeocodeResult{
		City:        model.city.String,
		Country:     model.country.String,
		CountryCode: model.countryCode.String,
	}, nil
}

func (m *geocodeCacheManager) Save(
	ctx context.Context,
	latBucket, lngBucket float64,
	result *domain.GeocodeResult,
) error {
	query := `
		INSERT INTO location.geocode_cache (lat_bucket, lng_bucket, city, country, country_code, not_found)
		VALUES ($1, $2, $3, $4, $5, false)
		ON CONFLICT (lat_bucket, lng_bucket)
		DO UPDATE SET
			city = EXCLUDED.city,
			country = EXCLUDED.country,
			country_code = EXCLUDED.country_code,
			not_found = false,
			updated_at = NOW()
	`

	q := m.tx.Querier(ctx)

	_, err := q.Exec(ctx, query, latBucket, lngBucket, result.City, result.Country, result.CountryCode)
	if err != nil {
		return fmt.Errorf("failed to save geocode cache: %w", err)
	}

	return nil
}

// SaveNotFound caches a bucket that Nominatim definitively couldn't resolve,
// so repeated lookups of it don't burn another rate-limited Nominatim call.
func (m *geocodeCacheManager) SaveNotFound(ctx context.Context, latBucket, lngBucket float64) error {
	query := `
		INSERT INTO location.geocode_cache (lat_bucket, lng_bucket, not_found)
		VALUES ($1, $2, true)
		ON CONFLICT (lat_bucket, lng_bucket)
		DO UPDATE SET
			not_found = true,
			updated_at = NOW()
	`

	q := m.tx.Querier(ctx)

	_, err := q.Exec(ctx, query, latBucket, lngBucket)
	if err != nil {
		return fmt.Errorf("failed to save not-found geocode cache: %w", err)
	}

	return nil
}
