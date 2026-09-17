package http

import (
	"context"
	"net/http"
	"strconv"

	"github.com/rasparac/rekreativko-api/location/internal/application"
	"github.com/rasparac/rekreativko-api/location/internal/domain"
	"github.com/rasparac/rekreativko-api/shared/api"
	"github.com/rasparac/rekreativko-api/shared/domainerror"
	"github.com/rasparac/rekreativko-api/shared/logger"
	"github.com/rasparac/rekreativko-api/shared/middleware"
)

type (
	geocodeService interface {
		ReverseGeocode(ctx context.Context, params application.ReverseGeocodeParams) (*domain.GeocodeResult, error)
		ForwardGeocode(ctx context.Context, params application.ForwardGeocodeParams) (*domain.ForwardGeocodeResult, error)
	}

	Handler struct {
		geocodeService geocodeService
		logger         *logger.Logger
	}

	// ReverseGeocodeResponse is the mobile contract for reverse geocoding.
	ReverseGeocodeResponse struct {
		City        string `json:"city"`
		Country     string `json:"country"`
		CountryCode string `json:"country_code"`
	}

	// ForwardGeocodeResponse is the mobile contract for forward geocoding.
	ForwardGeocodeResponse struct {
		Latitude    float64 `json:"latitude"`
		Longitude   float64 `json:"longitude"`
		DisplayName string  `json:"display_name"`
		City        string  `json:"city"`
		Country     string  `json:"country"`
		CountryCode string  `json:"country_code"`
	}
)

// NewHandler creates a new location HTTP handler.
func NewHandler(
	geocodeService geocodeService,
	log *logger.Logger,
) *Handler {
	return &Handler{
		geocodeService: geocodeService,
		logger:         log.WithName("location.http.handler"),
	}
}

func (h *Handler) RegisterRoutes(
	mux *http.ServeMux,
	middlewares *middleware.Chain,
) {
	mux.Handle(
		"GET /api/v1/geocode/reverse",
		middlewares.ThenFunc(h.ReverseGeocode),
	)
	mux.Handle(
		"GET /api/v1/geocode/forward",
		middlewares.ThenFunc(h.ForwardGeocode),
	)
}

// ReverseGeocode handles GET /api/v1/geocode/reverse
//
//	@Summary		Reverse geocode coordinates
//	@Description	Resolves City/Country for a lat/long pair. Results are cached server-side by grid cell, so repeated lookups for nearby coordinates don't call out to Nominatim again.
//	@Tags			Geocoding
//	@Produce		json
//	@Security		GatewayKeyAuth && BearerAuth
//	@Param			lat	query		number									true	"Latitude, -90 to 90"
//	@Param			lon	query		number									true	"Longitude, -180 to 180"
//	@Success		200	{object}	api.Response[ReverseGeocodeResponse]	"Location resolved"
//	@Failure		400	{object}	api.Response[any]						"Invalid coordinates"
//	@Failure		401	{object}	api.Response[any]						"Unauthorized"
//	@Failure		404	{object}	api.Response[any]						"No location found for the given coordinates"
//	@Failure		502	{object}	api.Response[any]						"Geocoding provider unavailable"
//	@Failure		500	{object}	api.Response[any]						"Internal server error"
//	@Router			/api/v1/geocode/reverse [get]
func (h *Handler) ReverseGeocode(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	lat, err := strconv.ParseFloat(r.URL.Query().Get("lat"), 64)
	if err != nil {
		api.WriteBadRequestResponse(w, "invalid_lat", "Query parameter 'lat' must be a valid number")
		return
	}

	lon, err := strconv.ParseFloat(r.URL.Query().Get("lon"), 64)
	if err != nil {
		api.WriteBadRequestResponse(w, "invalid_lon", "Query parameter 'lon' must be a valid number")
		return
	}

	result, err := h.geocodeService.ReverseGeocode(ctx, application.ReverseGeocodeParams{
		Latitude:  lat,
		Longitude: lon,
	})
	if err != nil {
		h.handleServiceError(ctx, w, err)
		return
	}

	api.WriteOkResponse(w, ReverseGeocodeResponse{
		City:        result.City,
		Country:     result.Country,
		CountryCode: result.CountryCode,
	}, "")
}

// ForwardGeocode handles GET /api/v1/geocode/forward
//
//	@Summary		Forward geocode a City/Country (optionally Street)
//	@Description	Resolves coordinates for a City+Country query, or a free-text search when Street is also given. Results are cached server-side by the normalized query, so repeated lookups don't call out to Nominatim again.
//	@Tags			Geocoding
//	@Produce		json
//	@Security		GatewayKeyAuth && BearerAuth
//	@Param			city	query		string									true	"City name"
//	@Param			country	query		string									true	"Country name"
//	@Param			street	query		string									false	"Street (switches to free-text search)"
//	@Success		200	{object}	api.Response[ForwardGeocodeResponse]	"Location resolved"
//	@Failure		400	{object}	api.Response[any]						"Invalid query"
//	@Failure		401	{object}	api.Response[any]						"Unauthorized"
//	@Failure		404	{object}	api.Response[any]						"No location found for the given query"
//	@Failure		502	{object}	api.Response[any]						"Geocoding provider unavailable"
//	@Failure		500	{object}	api.Response[any]						"Internal server error"
//	@Router			/api/v1/geocode/forward [get]
func (h *Handler) ForwardGeocode(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	result, err := h.geocodeService.ForwardGeocode(ctx, application.ForwardGeocodeParams{
		City:    r.URL.Query().Get("city"),
		Country: r.URL.Query().Get("country"),
		Street:  r.URL.Query().Get("street"),
	})
	if err != nil {
		h.handleServiceError(ctx, w, err)
		return
	}

	api.WriteOkResponse(w, ForwardGeocodeResponse{
		Latitude:    result.Latitude,
		Longitude:   result.Longitude,
		DisplayName: result.DisplayName,
		City:        result.City,
		Country:     result.Country,
		CountryCode: result.CountryCode,
	}, "")
}

func (h *Handler) handleServiceError(ctx context.Context, w http.ResponseWriter, err error) {
	appErr := domainerror.GetAppError(err)

	api.WriteError(
		w,
		appErr.StatusCode,
		appErr.Code,
		appErr.Message,
		nil,
	)
}
