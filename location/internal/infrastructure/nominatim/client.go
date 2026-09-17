package nominatim

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/rasparac/rekreativko-api/location/internal/domain"
	"github.com/rasparac/rekreativko-api/location/internal/metrics"
	"github.com/rasparac/rekreativko-api/shared/config"
	"github.com/rasparac/rekreativko-api/shared/logger"
)

// Client calls Nominatim's public reverse-geocoding API.
type Client struct {
	httpClient *http.Client
	baseURL    string
	userAgent  string
	limiter    *rateLimiter
	logger     *logger.Logger
	metrics    *metrics.Metrics
}

// NewClient creates a new Nominatim client, rate-limited per cfg.MinRequestInterval.
func NewClient(cfg config.NominatimConfig, log *logger.Logger, m *metrics.Metrics) *Client {
	return &Client{
		httpClient: &http.Client{Timeout: cfg.RequestTimeout},
		baseURL:    cfg.BaseURL,
		userAgent:  cfg.UserAgent,
		limiter:    newRateLimiter(cfg.MinRequestInterval),
		logger:     log.WithName("location.nominatim_client"),
		metrics:    m,
	}
}

type reverseGeocodeResponse struct {
	Address struct {
		City         string `json:"city"`
		Town         string `json:"town"`
		Village      string `json:"village"`
		Municipality string `json:"municipality"`
		Country      string `json:"country"`
		CountryCode  string `json:"country_code"`
	} `json:"address"`
	Error string `json:"error"`
}

// ReverseGeocode resolves City/Country for a coordinate pair via Nominatim's
// /reverse endpoint, setting a compliant User-Agent and honoring the
// process-wide rate limit required by Nominatim's usage policy.
func (c *Client) ReverseGeocode(ctx context.Context, coords domain.Coordinates) (*domain.GeocodeResult, error) {
	if err := c.limiter.Wait(ctx); err != nil {
		return nil, err
	}

	query := url.Values{
		"format": {"jsonv2"},
		"lat":    {strconv.FormatFloat(coords.Latitude, 'f', -1, 64)},
		"lon":    {strconv.FormatFloat(coords.Longitude, 'f', -1, 64)},
		// zoom=10 (Nominatim's own suggested "city" level) can resolve to a
		// sub-city administrative boundary instead of the city itself (e.g.
		// central Belgrade -> "Stari Grad Urban Municipality") - zoom=12
		// consistently gives the real city name.
		"zoom":           {"12"},
		"addressdetails": {"1"},
		// Without this, the address comes back in the location's own
		// script/language (e.g. Cyrillic for Belgrade, Croatian for Zagreb)
		// instead of a consistently readable one.
		"accept-language": {"en"},
	}
	reqURL := fmt.Sprintf("%s/reverse?%s", c.baseURL, query.Encode())

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("build nominatim request: %w", err)
	}
	req.Header.Set("User-Agent", c.userAgent)

	start := time.Now()
	resp, err := c.httpClient.Do(req)
	c.metrics.NominatimCallDuration.Observe(time.Since(start).Seconds())

	if err != nil {
		c.metrics.NominatimCallFailures.WithLabelValues("request_error").Inc()
		return nil, fmt.Errorf("%w: %v", domain.ErrGeocodingUnavailable, err)
	}
	defer resp.Body.Close()

	c.metrics.NominatimCallTotal.Inc()

	if resp.StatusCode != http.StatusOK {
		c.metrics.NominatimCallFailures.WithLabelValues(strconv.Itoa(resp.StatusCode)).Inc()
		return nil, fmt.Errorf("%w: nominatim returned status %d", domain.ErrGeocodingUnavailable, resp.StatusCode)
	}

	var parsed reverseGeocodeResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		c.metrics.NominatimCallFailures.WithLabelValues("decode_error").Inc()
		return nil, fmt.Errorf("decode nominatim response: %w", err)
	}

	if parsed.Error != "" {
		return nil, domain.ErrLocationNotFound
	}

	city := firstNonEmpty(
		parsed.Address.City,
		parsed.Address.Town,
		parsed.Address.Village,
		parsed.Address.Municipality,
	)
	if city == "" && parsed.Address.Country == "" {
		return nil, domain.ErrLocationNotFound
	}

	return &domain.GeocodeResult{
		City:        city,
		Country:     parsed.Address.Country,
		CountryCode: parsed.Address.CountryCode,
	}, nil
}

type forwardGeocodeResponse struct {
	Lat         string `json:"lat"`
	Lon         string `json:"lon"`
	DisplayName string `json:"display_name"`
	Address     struct {
		City         string `json:"city"`
		Town         string `json:"town"`
		Village      string `json:"village"`
		Municipality string `json:"municipality"`
		Country      string `json:"country"`
		CountryCode  string `json:"country_code"`
	} `json:"address"`
}

// ForwardGeocode resolves coordinates for a City/Country (optionally Street)
// query via Nominatim's /search endpoint, setting a compliant User-Agent and
// honoring the process-wide rate limit shared with ReverseGeocode. A
// structured city+country search is used unless query.IsFreeText() (Street
// given), in which case a single free-text string resolves street-level
// addresses far more reliably than Nominatim's structured street parameter.
func (c *Client) ForwardGeocode(ctx context.Context, query domain.ForwardGeocodeQuery) (*domain.ForwardGeocodeResult, error) {
	if err := c.limiter.Wait(ctx); err != nil {
		return nil, err
	}

	values := url.Values{
		"format":         {"jsonv2"},
		"limit":          {"1"},
		"addressdetails": {"1"},
		// See the comment on ReverseGeocode's identical parameter.
		"accept-language": {"en"},
	}

	if query.IsFreeText() {
		values.Set("q", query.FreeTextSearch())
	} else {
		values.Set("city", query.City)
		values.Set("country", query.Country)
	}

	reqURL := fmt.Sprintf("%s/search?%s", c.baseURL, values.Encode())

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("build nominatim request: %w", err)
	}
	req.Header.Set("User-Agent", c.userAgent)

	start := time.Now()
	resp, err := c.httpClient.Do(req)
	c.metrics.NominatimCallDuration.Observe(time.Since(start).Seconds())

	if err != nil {
		c.metrics.NominatimCallFailures.WithLabelValues("request_error").Inc()
		return nil, fmt.Errorf("%w: %v", domain.ErrGeocodingUnavailable, err)
	}
	defer resp.Body.Close()

	c.metrics.NominatimCallTotal.Inc()

	if resp.StatusCode != http.StatusOK {
		c.metrics.NominatimCallFailures.WithLabelValues(strconv.Itoa(resp.StatusCode)).Inc()
		return nil, fmt.Errorf("%w: nominatim returned status %d", domain.ErrGeocodingUnavailable, resp.StatusCode)
	}

	var parsed []forwardGeocodeResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		c.metrics.NominatimCallFailures.WithLabelValues("decode_error").Inc()
		return nil, fmt.Errorf("decode nominatim response: %w", err)
	}

	if len(parsed) == 0 {
		return nil, domain.ErrLocationNotFound
	}

	match := parsed[0]

	lat, err := strconv.ParseFloat(match.Lat, 64)
	if err != nil {
		return nil, fmt.Errorf("parse nominatim latitude %q: %w", match.Lat, err)
	}
	lon, err := strconv.ParseFloat(match.Lon, 64)
	if err != nil {
		return nil, fmt.Errorf("parse nominatim longitude %q: %w", match.Lon, err)
	}

	return &domain.ForwardGeocodeResult{
		Latitude:    lat,
		Longitude:   lon,
		DisplayName: match.DisplayName,
		City: firstNonEmpty(
			match.Address.City,
			match.Address.Town,
			match.Address.Village,
			match.Address.Municipality,
		),
		Country:     match.Address.Country,
		CountryCode: match.Address.CountryCode,
	}, nil
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
