package application

import (
	"context"
	"errors"
	"net/http"

	"github.com/rasparac/rekreativko-api/location/internal/domain"
	"github.com/rasparac/rekreativko-api/location/internal/infrastructure/persistence"
	"github.com/rasparac/rekreativko-api/location/internal/metrics"
	"github.com/rasparac/rekreativko-api/shared/domainerror"
	"github.com/rasparac/rekreativko-api/shared/logger"
	"github.com/rasparac/rekreativko-api/shared/telemetry"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

type (
	nominatimClient interface {
		ReverseGeocode(ctx context.Context, coords domain.Coordinates) (*domain.GeocodeResult, error)
		ForwardGeocode(ctx context.Context, query domain.ForwardGeocodeQuery) (*domain.ForwardGeocodeResult, error)
	}

	// ReverseGeocodeParams are the raw, not-yet-validated inputs to ReverseGeocode.
	ReverseGeocodeParams struct {
		Latitude  float64
		Longitude float64
	}

	// ForwardGeocodeParams are the raw, not-yet-validated inputs to ForwardGeocode.
	ForwardGeocodeParams struct {
		City    string
		Country string
		Street  string
	}

	// GeocodeService resolves City/Country<->coordinates, serving from a
	// Postgres cache before falling back to a rate-limited Nominatim call.
	GeocodeService struct {
		logger      *logger.Logger
		repo        persistence.GeocodeCacheRepository
		forwardRepo persistence.ForwardGeocodeCacheRepository
		nominatim   nominatimClient
		tracer      trace.Tracer
		metrics     *metrics.Metrics
	}
)

// NewGeocodeService creates a new geocode service.
func NewGeocodeService(
	logger *logger.Logger,
	repo persistence.GeocodeCacheRepository,
	forwardRepo persistence.ForwardGeocodeCacheRepository,
	nominatim nominatimClient,
	metrics *metrics.Metrics,
) *GeocodeService {
	return &GeocodeService{
		logger:      logger.WithName("location.geocode_service"),
		repo:        repo,
		forwardRepo: forwardRepo,
		nominatim:   nominatim,
		tracer:      telemetry.Tracer(telemetry.TracerLocationService),
		metrics:     metrics,
	}
}

// ReverseGeocode resolves City/Country for a coordinate pair. A cache hit on
// the coordinates' grid bucket avoids calling Nominatim entirely; a miss
// calls out (rate-limited) and caches the result for next time.
func (s *GeocodeService) ReverseGeocode(
	ctx context.Context,
	params ReverseGeocodeParams,
) (*domain.GeocodeResult, error) {
	ctx, span := s.tracer.Start(ctx, "location.service.ReverseGeocode")
	defer span.End()

	log := s.logger.WithValues(
		"method", "ReverseGeocode",
		"latitude", params.Latitude,
		"longitude", params.Longitude,
	)

	coords, err := domain.NewCoordinates(params.Latitude, params.Longitude)
	if err != nil {
		span.SetStatus(codes.Error, err.Error())
		return nil, domainerror.BadRequest("invalid_coordinates", err.Error(), err)
	}

	span.SetAttributes(
		attribute.Float64("latitude", coords.Latitude),
		attribute.Float64("longitude", coords.Longitude),
	)

	latBucket, lngBucket := coords.Bucket()

	cached, err := s.repo.FindByBucket(ctx, latBucket, lngBucket)
	switch {
	case err == nil:
		s.metrics.CacheHits.Inc()
		span.SetStatus(codes.Ok, "cache hit")
		return cached, nil
	case errors.Is(err, domain.ErrLocationNotFound):
		// A prior lookup already asked Nominatim and got a definitive "no
		// location" for this bucket - no need to ask again.
		s.metrics.CacheHits.Inc()
		span.SetStatus(codes.Ok, "cache hit (not found)")
		return nil, mapToAppErr(err)
	case errors.Is(err, domain.ErrCacheMiss):
		s.metrics.CacheMisses.Inc()
	default:
		span.SetStatus(codes.Error, err.Error())
		log.Error(ctx, "failed to read geocode cache", "error", err)
		return nil, domainerror.InternalWithErr(err)
	}

	result, err := s.nominatim.ReverseGeocode(ctx, coords)
	if err != nil {
		span.SetStatus(codes.Error, err.Error())
		log.Error(ctx, "nominatim reverse geocode failed", "error", err)

		if errors.Is(err, domain.ErrLocationNotFound) {
			// Definitive answer - cache it so the next lookup of this bucket
			// doesn't burn another rate-limited Nominatim call. A transient
			// ErrGeocodingUnavailable is deliberately not cached here, since
			// it's worth retrying on the next request.
			if saveErr := s.repo.SaveNotFound(ctx, latBucket, lngBucket); saveErr != nil {
				log.Error(ctx, "failed to persist not-found geocode cache", "error", saveErr)
			}
		}

		return nil, mapToAppErr(err)
	}

	if err := s.repo.Save(ctx, latBucket, lngBucket, result); err != nil {
		// The caller already has a good answer - don't fail the request just
		// because we couldn't cache it for next time.
		log.Error(ctx, "failed to persist geocode cache", "error", err)
	}

	span.SetStatus(codes.Ok, "resolved via nominatim")
	log.Debug(ctx, "resolved via nominatim", "city", result.City, "country", result.Country)

	return result, nil
}

// ForwardGeocode resolves coordinates for a City/Country (optionally Street)
// query. A cache hit on the normalized query avoids calling Nominatim
// entirely; a miss calls out (rate-limited, using free-text search when a
// Street is given) and caches the result for next time.
func (s *GeocodeService) ForwardGeocode(
	ctx context.Context,
	params ForwardGeocodeParams,
) (*domain.ForwardGeocodeResult, error) {
	ctx, span := s.tracer.Start(ctx, "location.service.ForwardGeocode")
	defer span.End()

	log := s.logger.WithValues(
		"method", "ForwardGeocode",
		"city", params.City,
		"country", params.Country,
		"street", params.Street,
	)

	query, err := domain.NewForwardGeocodeQuery(params.City, params.Country, params.Street)
	if err != nil {
		span.SetStatus(codes.Error, err.Error())
		return nil, domainerror.BadRequest("invalid_query", err.Error(), err)
	}

	span.SetAttributes(
		attribute.String("city", query.City),
		attribute.String("country", query.Country),
	)

	cacheKey := query.CacheKey()

	cached, err := s.forwardRepo.FindByQuery(ctx, cacheKey)
	switch {
	case err == nil:
		s.metrics.ForwardCacheHits.Inc()
		span.SetStatus(codes.Ok, "cache hit")
		return cached, nil
	case errors.Is(err, domain.ErrLocationNotFound):
		// A prior lookup already asked Nominatim and got a definitive "no
		// location" for this query - no need to ask again.
		s.metrics.ForwardCacheHits.Inc()
		span.SetStatus(codes.Ok, "cache hit (not found)")
		return nil, mapForwardAppErr(err)
	case errors.Is(err, domain.ErrCacheMiss):
		s.metrics.ForwardCacheMisses.Inc()
	default:
		span.SetStatus(codes.Error, err.Error())
		log.Error(ctx, "failed to read forward geocode cache", "error", err)
		return nil, domainerror.InternalWithErr(err)
	}

	result, err := s.nominatim.ForwardGeocode(ctx, query)
	if err != nil {
		span.SetStatus(codes.Error, err.Error())
		log.Error(ctx, "nominatim forward geocode failed", "error", err)

		if errors.Is(err, domain.ErrLocationNotFound) {
			// Definitive answer - cache it so the next lookup of this query
			// doesn't burn another rate-limited Nominatim call. A transient
			// ErrGeocodingUnavailable is deliberately not cached here, since
			// it's worth retrying on the next request.
			if saveErr := s.forwardRepo.SaveNotFound(ctx, cacheKey); saveErr != nil {
				log.Error(ctx, "failed to persist not-found forward geocode cache", "error", saveErr)
			}
		}

		return nil, mapForwardAppErr(err)
	}

	if err := s.forwardRepo.Save(ctx, cacheKey, result); err != nil {
		// The caller already has a good answer - don't fail the request just
		// because we couldn't cache it for next time.
		log.Error(ctx, "failed to persist forward geocode cache", "error", err)
	}

	span.SetStatus(codes.Ok, "resolved via nominatim")
	log.Debug(ctx, "resolved via nominatim", "latitude", result.Latitude, "longitude", result.Longitude)

	return result, nil
}

func mapToAppErr(err error) *domainerror.AppError {
	switch {
	case errors.Is(err, domain.ErrLocationNotFound):
		return domainerror.NotFound("location_not_found", "No location found for the given coordinates", err)
	case errors.Is(err, domain.ErrGeocodingUnavailable):
		return domainerror.NewWithErr("Geocoding service is temporarily unavailable", "geocoding_unavailable", http.StatusBadGateway, err)
	default:
		return domainerror.InternalWithErr(err)
	}
}

func mapForwardAppErr(err error) *domainerror.AppError {
	switch {
	case errors.Is(err, domain.ErrLocationNotFound):
		return domainerror.NotFound("location_not_found", "No location found for the given query", err)
	case errors.Is(err, domain.ErrGeocodingUnavailable):
		return domainerror.NewWithErr("Geocoding service is temporarily unavailable", "geocoding_unavailable", http.StatusBadGateway, err)
	default:
		return domainerror.InternalWithErr(err)
	}
}
