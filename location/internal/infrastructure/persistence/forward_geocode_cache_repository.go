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

type forwardGeocodeCacheModel struct {
	latitude    float64
	longitude   float64
	displayName sql.NullString
	city        sql.NullString
	country     sql.NullString
	countryCode sql.NullString
	notFound    bool
}

// ForwardGeocodeCacheRepository persists resolved coordinates for forward
// geocoding queries keyed by a normalized query string (see
// domain.ForwardGeocodeQuery.CacheKey), so repeated lookups for the same
// City/Country (or street) avoid another Nominatim call. Like the reverse
// cache, resolution is treated as effectively immutable - entries have no
// TTL and are only overwritten by a fresher Nominatim answer.
//
// A query Nominatim definitively couldn't resolve is cached too (via
// SaveNotFound) - FindByQuery returns domain.ErrLocationNotFound for it,
// distinct from domain.ErrCacheMiss (nothing cached yet, go ask Nominatim).
type ForwardGeocodeCacheRepository interface {
	FindByQuery(ctx context.Context, queryKey string) (*domain.ForwardGeocodeResult, error)
	Save(ctx context.Context, queryKey string, result *domain.ForwardGeocodeResult) error
	SaveNotFound(ctx context.Context, queryKey string) error
}

type forwardGeocodeCacheManager struct {
	tx     *postgres.TransactionManager
	logger *logger.Logger
}

// NewForwardGeocodeCacheRepository creates a new forward geocode cache repository.
func NewForwardGeocodeCacheRepository(
	tx *postgres.TransactionManager,
	logger *logger.Logger,
) ForwardGeocodeCacheRepository {
	return &forwardGeocodeCacheManager{
		tx:     tx,
		logger: logger,
	}
}

func (m *forwardGeocodeCacheManager) FindByQuery(
	ctx context.Context,
	queryKey string,
) (*domain.ForwardGeocodeResult, error) {
	query := `
		SELECT latitude, longitude, display_name, city, country, country_code, not_found
		FROM location.forward_geocode_cache
		WHERE query_key = $1
	`

	q := m.tx.Querier(ctx)

	var model forwardGeocodeCacheModel
	err := q.QueryRow(ctx, query, queryKey).Scan(
		&model.latitude,
		&model.longitude,
		&model.displayName,
		&model.city,
		&model.country,
		&model.countryCode,
		&model.notFound,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrCacheMiss
		}
		return nil, fmt.Errorf("failed to query forward geocode cache: %w", err)
	}

	if model.notFound {
		return nil, domain.ErrLocationNotFound
	}

	return &domain.ForwardGeocodeResult{
		Latitude:    model.latitude,
		Longitude:   model.longitude,
		DisplayName: model.displayName.String,
		City:        model.city.String,
		Country:     model.country.String,
		CountryCode: model.countryCode.String,
	}, nil
}

func (m *forwardGeocodeCacheManager) Save(
	ctx context.Context,
	queryKey string,
	result *domain.ForwardGeocodeResult,
) error {
	query := `
		INSERT INTO location.forward_geocode_cache
			(query_key, latitude, longitude, display_name, city, country, country_code, not_found)
		VALUES ($1, $2, $3, $4, $5, $6, $7, false)
		ON CONFLICT (query_key)
		DO UPDATE SET
			latitude = EXCLUDED.latitude,
			longitude = EXCLUDED.longitude,
			display_name = EXCLUDED.display_name,
			city = EXCLUDED.city,
			country = EXCLUDED.country,
			country_code = EXCLUDED.country_code,
			not_found = false,
			updated_at = NOW()
	`

	q := m.tx.Querier(ctx)

	_, err := q.Exec(
		ctx, query, queryKey,
		result.Latitude, result.Longitude, result.DisplayName,
		result.City, result.Country, result.CountryCode,
	)
	if err != nil {
		return fmt.Errorf("failed to save forward geocode cache: %w", err)
	}

	return nil
}

// SaveNotFound caches a query that Nominatim definitively couldn't resolve,
// so repeated lookups of it don't burn another rate-limited Nominatim call.
func (m *forwardGeocodeCacheManager) SaveNotFound(ctx context.Context, queryKey string) error {
	query := `
		INSERT INTO location.forward_geocode_cache (query_key, not_found)
		VALUES ($1, true)
		ON CONFLICT (query_key)
		DO UPDATE SET
			not_found = true,
			updated_at = NOW()
	`

	q := m.tx.Querier(ctx)

	_, err := q.Exec(ctx, query, queryKey)
	if err != nil {
		return fmt.Errorf("failed to save not-found forward geocode cache: %w", err)
	}

	return nil
}
