package gateway

import (
	"bufio"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/rasparac/rekreativko-api/shared/authcontext"
	"github.com/rasparac/rekreativko-api/shared/logger"
	"github.com/rasparac/rekreativko-api/shared/middleware"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Short stand-ins for the gateway's 15s WriteTimeout and 30s service timeout.
const (
	testWriteTimeout   = 200 * time.Millisecond
	testServiceTimeout = 150 * time.Millisecond
	tickEvery          = 40 * time.Millisecond
)

// testAccountID is the account every test gateway request is authenticated as.
var testAccountID = uuid.MustParse("6f1d2c3b-4a59-4e8d-9c7b-1a2b3c4d5e6f")

// tickingBackend streams an SSE tick every tickEvery until the request is
// cancelled, then reports the cancellation on cancelled.
func tickingBackend(cancelled chan<- struct{}) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		http.NewResponseController(w).Flush()

		ticker := time.NewTicker(tickEvery)
		defer ticker.Stop()

		for i := 0; ; i++ {
			select {
			case <-r.Context().Done():
				if cancelled != nil {
					close(cancelled)
				}
				return
			case <-ticker.C:
				if _, err := fmt.Fprintf(w, "event: tick\ndata: %d\n\n", i); err != nil {
					return
				}
				if err := http.NewResponseController(w).Flush(); err != nil {
					return
				}
			}
		}
	}
}

// newTestGateway runs backend as the activity service behind the real router,
// proxy and logging middleware, on a server with a short WriteTimeout. Every
// request is authenticated.
func newTestGateway(t *testing.T, backend http.Handler) *httptest.Server {
	t.Helper()

	upstream := httptest.NewServer(backend)
	t.Cleanup(upstream.Close)

	return newTestGatewayTo(t, upstream.URL)
}

// newTestGatewayTo is newTestGateway with the activity service at upstreamURL.
func newTestGatewayTo(t *testing.T, upstreamURL string) *httptest.Server {
	t.Helper()

	log := logger.New("error", "json")
	proxy, err := NewReverseProxy(map[string]ServiceConfig{
		"activity": {URL: upstreamURL, Timeout: testServiceTimeout},
	}, log)
	require.NoError(t, err)

	router := NewRouter(proxy)
	authenticated := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		router.ServeHTTP(w, r.WithContext(authcontext.WithAccountID(r.Context(), testAccountID)))
	})

	gw := httptest.NewUnstartedServer(middleware.Logging(log)(authenticated))
	gw.Config.WriteTimeout = testWriteTimeout
	gw.Start()
	t.Cleanup(gw.Close)

	return gw
}

// readTicks reads SSE data lines until the stream ends or for at most d, and
// returns how long the stream stayed alive and how many ticks arrived.
func readTicks(t *testing.T, resp *http.Response, d time.Duration) (alive time.Duration, ticks int) {
	t.Helper()

	start := time.Now()
	lines := make(chan string)
	done := make(chan struct{})
	defer close(done)
	go func() {
		defer close(lines)
		scanner := bufio.NewScanner(resp.Body)
		for scanner.Scan() {
			select {
			case lines <- scanner.Text():
			case <-done:
				return
			}
		}
		// The stream ending or being cut is what the callers measure, so a
		// read error is an expected outcome, not a failure.
		_ = scanner.Err()
	}()

	deadline := time.After(d)
	for {
		select {
		case line, ok := <-lines:
			if !ok {
				return time.Since(start), ticks
			}
			if strings.HasPrefix(line, "data:") {
				ticks++
			}
		case <-deadline:
			return time.Since(start), ticks
		}
	}
}

func get(t *testing.T, url string) *http.Response {
	t.Helper()
	resp, err := http.Get(url)
	require.NoError(t, err)
	t.Cleanup(func() { _ = resp.Body.Close() })
	return resp
}

func TestStreamRoute_OutlivesWriteAndServiceTimeouts(t *testing.T) {
	gw := newTestGateway(t, tickingBackend(nil))

	resp := get(t, gw.URL+"/activity/api/v1/sessions/"+uuid.NewString()+"/events")
	require.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "text/event-stream", resp.Header.Get("Content-Type"))

	window := 4 * (testWriteTimeout + testServiceTimeout)
	alive, ticks := readTicks(t, resp, window)

	assert.GreaterOrEqual(t, alive, window, "the stream must stay open past both timeouts")
	assert.Greater(t, ticks, int(window/tickEvery)/2, "ticks must keep arriving, not be buffered")
}

func TestOrdinaryRoute_KeepsItsTimeouts(t *testing.T) {
	// Same streaming backend, but on a path that is not a declared stream.
	gw := newTestGateway(t, tickingBackend(nil))

	resp := get(t, gw.URL+"/activity/api/v1/sessions/"+uuid.NewString()+"/other")
	require.Equal(t, http.StatusOK, resp.StatusCode)

	alive, _ := readTicks(t, resp, 4*(testWriteTimeout+testServiceTimeout))

	assert.Less(t, alive, 2*testWriteTimeout, "an ordinary response is still cut by the timeouts")
}

// backendRequest is what a test backend saw of a proxied request.
type backendRequest struct {
	method string
	path   string
	header http.Header
}

func TestStreamRoute_BackendWithoutHeadersFailsAfterServiceTimeout(t *testing.T) {
	received := make(chan backendRequest, 1)
	backendCancelled := make(chan struct{})
	release := make(chan struct{})
	t.Cleanup(func() { close(release) })

	gw := newTestGateway(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case received <- backendRequest{method: r.Method, path: r.URL.Path, header: r.Header.Clone()}:
		default: // only the first request is recorded
		}

		select { // never answers
		case <-release:
		case <-r.Context().Done():
			close(backendCancelled)
		}
	}))

	sessionID := uuid.NewString()
	start := time.Now()
	resp := get(t, gw.URL+"/activity/api/v1/sessions/"+sessionID+"/events")
	elapsed := time.Since(start)

	// The gateway waited the whole header timeout - no less, not much more -
	// then answered itself.
	assert.Equal(t, http.StatusServiceUnavailable, resp.StatusCode)
	assert.GreaterOrEqual(t, elapsed, testServiceTimeout, "must wait the full header timeout, not fail early")
	assert.Less(t, elapsed, 4*testServiceTimeout)

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.Equal(t, "Service unavailable\n", string(body), "the proxy's ErrorHandler answered")
	assert.Empty(t, resp.Header.Get("X-Service-Name"), "no backend response was proxied")
	assert.Empty(t, resp.Header.Get("X-ProxiedBy"), "no backend response was proxied")

	// The request did reach the backend - so it failed waiting for headers,
	// not routing or dialing - as an ordinary proxied request.
	var req backendRequest
	select {
	case req = <-received:
	default:
		t.Fatal("the request never reached the backend")
	}
	assert.Equal(t, http.MethodGet, req.method)
	assert.Equal(t, "/api/v1/sessions/"+sessionID+"/events", req.path, "the /activity prefix is stripped")
	assert.Equal(t, testAccountID.String(), req.header.Get(authcontext.XUserIDHeader), "the caller's identity is forwarded")
	assert.Equal(t, strings.TrimPrefix(gw.URL, "http://"), req.header.Get("X-Forwarded-Host"))
	assert.Equal(t, "http", req.header.Get("X-Forwarded-Proto"))
	assert.NotEmpty(t, req.header.Get("X-Forwarded-For"))

	// Giving up released the backend: its request was cancelled, not left
	// hanging.
	select {
	case <-backendCancelled:
	case <-time.After(2 * time.Second):
		t.Fatal("the backend request was not cancelled after the gateway gave up")
	}
}

func TestStreamRoute_UnreachableBackendFailsAfterServiceTimeout(t *testing.T) {
	// 192.0.2.0/24 (TEST-NET-1) is reserved and never routed: dialing it
	// usually hangs until a timeout, like a backend host that is down. Where
	// the network rejects it right away instead, the request fails fast and
	// the test still holds.
	gw := newTestGatewayTo(t, "http://192.0.2.1:80")

	// A client timeout far below the OS connect timeout (~2 minutes), so a
	// missing dial timeout fails the test instead of hanging it.
	client := &http.Client{Timeout: 3 * time.Second}

	start := time.Now()
	resp, err := client.Get(gw.URL + "/activity/api/v1/sessions/" + uuid.NewString() + "/events")
	require.NoError(t, err, "the gateway must answer before the client gives up")
	t.Cleanup(func() { _ = resp.Body.Close() })

	assert.Equal(t, http.StatusServiceUnavailable, resp.StatusCode)
	assert.Less(t, time.Since(start), 4*testServiceTimeout)
}

func TestStreamRoute_ClientHangupCancelsBackend(t *testing.T) {
	cancelled := make(chan struct{})
	gw := newTestGateway(t, tickingBackend(cancelled))

	resp := get(t, gw.URL+"/activity/api/v1/sessions/"+uuid.NewString()+"/events")
	require.Equal(t, http.StatusOK, resp.StatusCode)
	readTicks(t, resp, 2*tickEvery)

	require.NoError(t, resp.Body.Close())

	select {
	case <-cancelled:
	case <-time.After(2 * time.Second):
		t.Fatal("the backend request was not cancelled after the client hung up")
	}
}
