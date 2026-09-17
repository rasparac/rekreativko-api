package application

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"testing"

	"github.com/rasparac/rekreativko-api/location/internal/domain"
	"github.com/rasparac/rekreativko-api/location/internal/metrics"
	"github.com/rasparac/rekreativko-api/shared/domainerror"
	testutil "github.com/rasparac/rekreativko-api/shared/testing"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testMetrics returns a single metrics.New() instance shared by every test
// in this package. metrics.New() registers its collectors against the
// global Prometheus registry via promauto, so calling it more than once
// within the same test binary panics with "duplicate metrics collector
// registration attempted".
var (
	sharedTestMetricsOnce sync.Once
	sharedTestMetrics     *metrics.Metrics
)

func testMetrics() *metrics.Metrics {
	sharedTestMetricsOnce.Do(func() {
		sharedTestMetrics = metrics.New()
	})
	return sharedTestMetrics
}

type fakeRepo struct {
	findResult  *domain.GeocodeResult
	findErr     error
	saveErr     error
	saveCalled  bool
	savedLat    float64
	savedLng    float64
	savedResult *domain.GeocodeResult

	saveNotFoundErr    error
	saveNotFoundCalled bool
	savedNotFoundLat   float64
	savedNotFoundLng   float64
}

func (f *fakeRepo) FindByBucket(ctx context.Context, latBucket, lngBucket float64) (*domain.GeocodeResult, error) {
	return f.findResult, f.findErr
}

func (f *fakeRepo) Save(ctx context.Context, latBucket, lngBucket float64, result *domain.GeocodeResult) error {
	f.saveCalled = true
	f.savedLat = latBucket
	f.savedLng = lngBucket
	f.savedResult = result
	return f.saveErr
}

func (f *fakeRepo) SaveNotFound(ctx context.Context, latBucket, lngBucket float64) error {
	f.saveNotFoundCalled = true
	f.savedNotFoundLat = latBucket
	f.savedNotFoundLng = lngBucket
	return f.saveNotFoundErr
}

type fakeForwardRepo struct {
	findResult  *domain.ForwardGeocodeResult
	findErr     error
	saveErr     error
	saveCalled  bool
	savedKey    string
	savedResult *domain.ForwardGeocodeResult

	saveNotFoundErr    error
	saveNotFoundCalled bool
	savedNotFoundKey   string
}

func (f *fakeForwardRepo) FindByQuery(ctx context.Context, queryKey string) (*domain.ForwardGeocodeResult, error) {
	return f.findResult, f.findErr
}

func (f *fakeForwardRepo) Save(ctx context.Context, queryKey string, result *domain.ForwardGeocodeResult) error {
	f.saveCalled = true
	f.savedKey = queryKey
	f.savedResult = result
	return f.saveErr
}

func (f *fakeForwardRepo) SaveNotFound(ctx context.Context, queryKey string) error {
	f.saveNotFoundCalled = true
	f.savedNotFoundKey = queryKey
	return f.saveNotFoundErr
}

type fakeNominatimClient struct {
	result *domain.GeocodeResult
	err    error
	calls  int

	forwardResult *domain.ForwardGeocodeResult
	forwardErr    error
	forwardCalls  int
}

func (f *fakeNominatimClient) ReverseGeocode(ctx context.Context, coords domain.Coordinates) (*domain.GeocodeResult, error) {
	f.calls++
	return f.result, f.err
}

func (f *fakeNominatimClient) ForwardGeocode(ctx context.Context, query domain.ForwardGeocodeQuery) (*domain.ForwardGeocodeResult, error) {
	f.forwardCalls++
	return f.forwardResult, f.forwardErr
}

func newTestService(repo *fakeRepo, nominatim *fakeNominatimClient) *GeocodeService {
	return NewGeocodeService(testutil.CreateLogger(), repo, &fakeForwardRepo{}, nominatim, testMetrics())
}

func newTestForwardService(forwardRepo *fakeForwardRepo, nominatim *fakeNominatimClient) *GeocodeService {
	return NewGeocodeService(testutil.CreateLogger(), &fakeRepo{}, forwardRepo, nominatim, testMetrics())
}

func TestGeocodeService_ReverseGeocode_InvalidCoordinates(t *testing.T) {
	svc := newTestService(&fakeRepo{}, &fakeNominatimClient{})

	_, err := svc.ReverseGeocode(context.Background(), ReverseGeocodeParams{Latitude: 999, Longitude: 0})

	require.Error(t, err)
	assert.Equal(t, http.StatusBadRequest, statusCodeOf(t, err))
}

func TestGeocodeService_ReverseGeocode_CacheHit(t *testing.T) {
	cached := &domain.GeocodeResult{City: "Zagreb", Country: "Croatia", CountryCode: "hr"}
	repo := &fakeRepo{findResult: cached}
	nominatim := &fakeNominatimClient{}

	svc := newTestService(repo, nominatim)

	got, err := svc.ReverseGeocode(context.Background(), ReverseGeocodeParams{Latitude: 45.815, Longitude: 15.982})

	require.NoError(t, err)
	assert.Equal(t, cached, got)
	assert.Equal(t, 0, nominatim.calls, "cache hit must not call nominatim")
	assert.False(t, repo.saveCalled, "cache hit must not re-save")
}

func TestGeocodeService_ReverseGeocode_CacheHit_NotFound(t *testing.T) {
	repo := &fakeRepo{findErr: domain.ErrLocationNotFound}
	nominatim := &fakeNominatimClient{}

	svc := newTestService(repo, nominatim)

	_, err := svc.ReverseGeocode(context.Background(), ReverseGeocodeParams{Latitude: 45.815, Longitude: 15.982})

	require.Error(t, err)
	assert.Equal(t, http.StatusNotFound, statusCodeOf(t, err))
	assert.Equal(t, 0, nominatim.calls, "a cached not-found answer must not call nominatim again")
	assert.False(t, repo.saveNotFoundCalled, "already-cached not-found must not be re-saved")
}

func TestGeocodeService_ReverseGeocode_CacheMiss_CallsNominatimAndCaches(t *testing.T) {
	repo := &fakeRepo{findErr: domain.ErrCacheMiss}
	resolved := &domain.GeocodeResult{City: "Zagreb", Country: "Croatia", CountryCode: "hr"}
	nominatim := &fakeNominatimClient{result: resolved}

	svc := newTestService(repo, nominatim)

	got, err := svc.ReverseGeocode(context.Background(), ReverseGeocodeParams{Latitude: 45.815, Longitude: 15.982})

	require.NoError(t, err)
	assert.Equal(t, resolved, got)
	assert.Equal(t, 1, nominatim.calls)
	assert.True(t, repo.saveCalled)
	assert.Equal(t, resolved, repo.savedResult)
	assert.False(t, repo.saveNotFoundCalled)
}

func TestGeocodeService_ReverseGeocode_CacheReadError(t *testing.T) {
	repo := &fakeRepo{findErr: errors.New("boom")}
	nominatim := &fakeNominatimClient{}

	svc := newTestService(repo, nominatim)

	_, err := svc.ReverseGeocode(context.Background(), ReverseGeocodeParams{Latitude: 45.815, Longitude: 15.982})

	require.Error(t, err)
	assert.Equal(t, http.StatusInternalServerError, statusCodeOf(t, err))
	assert.Equal(t, 0, nominatim.calls)
}

func TestGeocodeService_ReverseGeocode_NominatimNotFound_CachesNegativeResult(t *testing.T) {
	repo := &fakeRepo{findErr: domain.ErrCacheMiss}
	nominatim := &fakeNominatimClient{err: domain.ErrLocationNotFound}

	svc := newTestService(repo, nominatim)

	_, err := svc.ReverseGeocode(context.Background(), ReverseGeocodeParams{Latitude: 45.815, Longitude: 15.982})

	require.Error(t, err)
	assert.Equal(t, http.StatusNotFound, statusCodeOf(t, err))
	assert.False(t, repo.saveCalled)
	assert.True(t, repo.saveNotFoundCalled, "a definitive not-found answer must be cached")
	assert.Equal(t, 45.815, repo.savedNotFoundLat)
	assert.Equal(t, 15.982, repo.savedNotFoundLng)
}

func TestGeocodeService_ReverseGeocode_NominatimUnavailable_DoesNotCache(t *testing.T) {
	repo := &fakeRepo{findErr: domain.ErrCacheMiss}
	nominatim := &fakeNominatimClient{err: domain.ErrGeocodingUnavailable}

	svc := newTestService(repo, nominatim)

	_, err := svc.ReverseGeocode(context.Background(), ReverseGeocodeParams{Latitude: 45.815, Longitude: 15.982})

	require.Error(t, err)
	assert.Equal(t, http.StatusBadGateway, statusCodeOf(t, err))
	assert.False(t, repo.saveCalled)
	assert.False(t, repo.saveNotFoundCalled, "a transient failure must not be cached as a permanent not-found")
}

func TestGeocodeService_ReverseGeocode_SaveErrorDoesNotFailRequest(t *testing.T) {
	repo := &fakeRepo{findErr: domain.ErrCacheMiss, saveErr: errors.New("write failed")}
	resolved := &domain.GeocodeResult{City: "Zagreb", Country: "Croatia", CountryCode: "hr"}
	nominatim := &fakeNominatimClient{result: resolved}

	svc := newTestService(repo, nominatim)

	got, err := svc.ReverseGeocode(context.Background(), ReverseGeocodeParams{Latitude: 45.815, Longitude: 15.982})

	require.NoError(t, err)
	assert.Equal(t, resolved, got)
}

func statusCodeOf(t *testing.T, err error) int {
	t.Helper()
	return domainerror.GetAppError(err).StatusCode
}

func TestGeocodeService_ForwardGeocode_InvalidQuery(t *testing.T) {
	svc := newTestForwardService(&fakeForwardRepo{}, &fakeNominatimClient{})

	_, err := svc.ForwardGeocode(context.Background(), ForwardGeocodeParams{City: "", Country: "Croatia"})

	require.Error(t, err)
	assert.Equal(t, http.StatusBadRequest, statusCodeOf(t, err))
}

func TestGeocodeService_ForwardGeocode_CacheHit(t *testing.T) {
	cached := &domain.ForwardGeocodeResult{Latitude: 45.815, Longitude: 15.982, City: "Zagreb", Country: "Croatia"}
	repo := &fakeForwardRepo{findResult: cached}
	nominatim := &fakeNominatimClient{}

	svc := newTestForwardService(repo, nominatim)

	got, err := svc.ForwardGeocode(context.Background(), ForwardGeocodeParams{City: "Zagreb", Country: "Croatia"})

	require.NoError(t, err)
	assert.Equal(t, cached, got)
	assert.Equal(t, 0, nominatim.forwardCalls, "cache hit must not call nominatim")
	assert.False(t, repo.saveCalled, "cache hit must not re-save")
}

func TestGeocodeService_ForwardGeocode_CacheHit_NotFound(t *testing.T) {
	repo := &fakeForwardRepo{findErr: domain.ErrLocationNotFound}
	nominatim := &fakeNominatimClient{}

	svc := newTestForwardService(repo, nominatim)

	_, err := svc.ForwardGeocode(context.Background(), ForwardGeocodeParams{City: "Zagreb", Country: "Croatia"})

	require.Error(t, err)
	assert.Equal(t, http.StatusNotFound, statusCodeOf(t, err))
	assert.Equal(t, 0, nominatim.forwardCalls, "a cached not-found answer must not call nominatim again")
	assert.False(t, repo.saveNotFoundCalled, "already-cached not-found must not be re-saved")
}

func TestGeocodeService_ForwardGeocode_CacheMiss_CallsNominatimAndCaches(t *testing.T) {
	repo := &fakeForwardRepo{findErr: domain.ErrCacheMiss}
	resolved := &domain.ForwardGeocodeResult{Latitude: 45.815, Longitude: 15.982, City: "Zagreb", Country: "Croatia"}
	nominatim := &fakeNominatimClient{forwardResult: resolved}

	svc := newTestForwardService(repo, nominatim)

	got, err := svc.ForwardGeocode(context.Background(), ForwardGeocodeParams{City: "Zagreb", Country: "Croatia"})

	require.NoError(t, err)
	assert.Equal(t, resolved, got)
	assert.Equal(t, 1, nominatim.forwardCalls)
	assert.True(t, repo.saveCalled)
	assert.Equal(t, resolved, repo.savedResult)
	assert.False(t, repo.saveNotFoundCalled)
}

func TestGeocodeService_ForwardGeocode_CacheReadError(t *testing.T) {
	repo := &fakeForwardRepo{findErr: errors.New("boom")}
	nominatim := &fakeNominatimClient{}

	svc := newTestForwardService(repo, nominatim)

	_, err := svc.ForwardGeocode(context.Background(), ForwardGeocodeParams{City: "Zagreb", Country: "Croatia"})

	require.Error(t, err)
	assert.Equal(t, http.StatusInternalServerError, statusCodeOf(t, err))
	assert.Equal(t, 0, nominatim.forwardCalls)
}

func TestGeocodeService_ForwardGeocode_NominatimNotFound_CachesNegativeResult(t *testing.T) {
	repo := &fakeForwardRepo{findErr: domain.ErrCacheMiss}
	nominatim := &fakeNominatimClient{forwardErr: domain.ErrLocationNotFound}

	svc := newTestForwardService(repo, nominatim)

	_, err := svc.ForwardGeocode(context.Background(), ForwardGeocodeParams{City: "Zagreb", Country: "Croatia"})

	require.Error(t, err)
	assert.Equal(t, http.StatusNotFound, statusCodeOf(t, err))
	assert.False(t, repo.saveCalled)
	assert.True(t, repo.saveNotFoundCalled, "a definitive not-found answer must be cached")
}

func TestGeocodeService_ForwardGeocode_NominatimUnavailable_DoesNotCache(t *testing.T) {
	repo := &fakeForwardRepo{findErr: domain.ErrCacheMiss}
	nominatim := &fakeNominatimClient{forwardErr: domain.ErrGeocodingUnavailable}

	svc := newTestForwardService(repo, nominatim)

	_, err := svc.ForwardGeocode(context.Background(), ForwardGeocodeParams{City: "Zagreb", Country: "Croatia"})

	require.Error(t, err)
	assert.Equal(t, http.StatusBadGateway, statusCodeOf(t, err))
	assert.False(t, repo.saveCalled)
	assert.False(t, repo.saveNotFoundCalled, "a transient failure must not be cached as a permanent not-found")
}

func TestGeocodeService_ForwardGeocode_SaveErrorDoesNotFailRequest(t *testing.T) {
	repo := &fakeForwardRepo{findErr: domain.ErrCacheMiss, saveErr: errors.New("write failed")}
	resolved := &domain.ForwardGeocodeResult{Latitude: 45.815, Longitude: 15.982, City: "Zagreb", Country: "Croatia"}
	nominatim := &fakeNominatimClient{forwardResult: resolved}

	svc := newTestForwardService(repo, nominatim)

	got, err := svc.ForwardGeocode(context.Background(), ForwardGeocodeParams{City: "Zagreb", Country: "Croatia"})

	require.NoError(t, err)
	assert.Equal(t, resolved, got)
}

func TestGeocodeService_ForwardGeocode_FreeTextWhenStreetGiven(t *testing.T) {
	repo := &fakeForwardRepo{findErr: domain.ErrCacheMiss}
	resolved := &domain.ForwardGeocodeResult{Latitude: 45.815, Longitude: 15.982, City: "Zagreb", Country: "Croatia"}
	nominatim := &fakeNominatimClient{forwardResult: resolved}

	svc := newTestForwardService(repo, nominatim)

	_, err := svc.ForwardGeocode(context.Background(), ForwardGeocodeParams{City: "Zagreb", Country: "Croatia", Street: "Ilica 1"})

	require.NoError(t, err)
	assert.Equal(t, 1, nominatim.forwardCalls)
}
