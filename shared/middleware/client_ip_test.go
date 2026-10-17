package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/rasparac/rekreativko-api/shared/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func requestFrom(remoteAddr string, headers map[string]string) *http.Request {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.RemoteAddr = remoteAddr
	for k, v := range headers {
		r.Header.Set(k, v)
	}
	return r
}

func TestClientIPResolver_IP(t *testing.T) {
	proxies := []string{"10.0.0.0/8", "192.168.1.5"}

	for name, tc := range map[string]struct {
		proxies []string
		remote  string
		headers map[string]string
		want    string
	}{
		"no proxy headers: the connection's address, port stripped": {
			remote: "203.0.113.7:5000", want: "203.0.113.7",
		},
		"ipv6 connection": {
			remote: "[2001:db8::1]:5000", want: "2001:db8::1",
		},
		"no trusted proxies: forwarded headers are ignored": {
			remote: "203.0.113.7:5000",
			headers: map[string]string{
				"X-Forwarded-For": "1.2.3.4", "X-Real-IP": "5.6.7.8",
				"CF-Connecting-IP": "9.9.9.9", "True-Client-IP": "8.8.8.8", "Forwarded": "for=7.7.7.7",
			},
			want: "203.0.113.7",
		},
		"untrusted peer cannot speak for a client": {
			proxies: proxies, remote: "203.0.113.7:5000",
			headers: map[string]string{"X-Forwarded-For": "1.2.3.4"},
			want:    "203.0.113.7",
		},
		"trusted proxy: the client it forwarded": {
			proxies: proxies, remote: "10.1.2.3:5000",
			headers: map[string]string{"X-Forwarded-For": "198.51.100.9"},
			want:    "198.51.100.9",
		},
		"trusted proxy (single address entry)": {
			proxies: proxies, remote: "192.168.1.5:5000",
			headers: map[string]string{"X-Forwarded-For": "198.51.100.9"},
			want:    "198.51.100.9",
		},
		"a client-made left part of the chain is not believed": {
			proxies: proxies, remote: "10.1.2.3:5000",
			headers: map[string]string{"X-Forwarded-For": "6.6.6.6, 198.51.100.9"},
			want:    "198.51.100.9",
		},
		"several trusted hops are skipped": {
			proxies: proxies, remote: "10.1.2.3:5000",
			headers: map[string]string{"X-Forwarded-For": "198.51.100.9, 10.4.4.4, 10.5.5.5"},
			want:    "198.51.100.9",
		},
		"chain of only trusted addresses: the leftmost": {
			proxies: proxies, remote: "10.1.2.3:5000",
			headers: map[string]string{"X-Forwarded-For": "10.9.9.9, 10.4.4.4"},
			want:    "10.9.9.9",
		},
		"a garbage entry is not used as a key": {
			proxies: proxies, remote: "10.1.2.3:5000",
			headers: map[string]string{"X-Forwarded-For": "not-an-ip"},
			want:    "10.1.2.3",
		},
		"X-Real-IP from a trusted proxy when there is no chain": {
			proxies: proxies, remote: "10.1.2.3:5000",
			headers: map[string]string{"X-Real-IP": "198.51.100.9"},
			want:    "198.51.100.9",
		},
		"Cloudflare and Forwarded headers are not believed even from a trusted proxy": {
			proxies: proxies, remote: "10.1.2.3:5000",
			headers: map[string]string{"CF-Connecting-IP": "9.9.9.9", "True-Client-IP": "8.8.8.8", "Forwarded": "for=7.7.7.7"},
			want:    "10.1.2.3",
		},
		"ipv4-mapped ipv6 is the same client": {
			proxies: proxies, remote: "10.1.2.3:5000",
			headers: map[string]string{"X-Forwarded-For": "::ffff:198.51.100.9"},
			want:    "198.51.100.9",
		},
		"remote address without a port": {
			remote: "203.0.113.7", want: "203.0.113.7",
		},
	} {
		t.Run(name, func(t *testing.T) {
			resolver, err := NewClientIPResolver(tc.proxies)
			require.NoError(t, err)

			assert.Equal(t, tc.want, resolver.IP(requestFrom(tc.remote, tc.headers)))
		})
	}
}

func TestClientIPResolver_TrustAllProxies(t *testing.T) {
	resolver := TrustAllProxies()

	assert.Equal(t, "198.51.100.9", resolver.IP(requestFrom("10.1.2.3:5000", map[string]string{"X-Forwarded-For": "198.51.100.9"})))
	assert.Equal(t, "10.1.2.3", resolver.IP(requestFrom("10.1.2.3:5000", nil)), "no header: the peer")
	assert.Equal(t, "10.1.2.3", resolver.IP(requestFrom("10.1.2.3:5000", map[string]string{"X-Forwarded-For": "nonsense"})))
}

func TestNewClientIPResolver_RejectsGarbage(t *testing.T) {
	_, err := NewClientIPResolver([]string{"10.0.0.0/8", "not a cidr"})
	assert.Error(t, err)

	_, err = NewClientIPResolver([]string{"", "  "})
	assert.NoError(t, err, "blank entries (an empty TRUSTED_PROXY_CIDRS) are fine")
}

// The limiter keys on the resolved address: without proxy headers two machines
// get a bucket each, and making up a header does not buy a fresh bucket.
func TestRateLimiter_KeysOnTheResolvedClientAddress(t *testing.T) {
	resolver, err := NewClientIPResolver(nil)
	require.NoError(t, err)

	rl, err := NewRateLimiter(RateLimit{Burst: 5, PerMinute: 5})
	require.NoError(t, err)
	defer rl.Stop()
	handler := ClientInfoFrom(resolver)(rl.ByClientIP(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {})))

	send := func(remote string, headers map[string]string) int {
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, requestFrom(remote, headers))
		return w.Code
	}

	for range 5 {
		require.Equal(t, http.StatusOK, send("203.0.113.1:1111", nil))
	}
	assert.Equal(t, http.StatusTooManyRequests, send("203.0.113.1:1111", nil), "the same machine is limited")
	assert.Equal(t, http.StatusOK, send("203.0.113.2:2222", nil), "another machine has its own bucket")

	for i := range 20 {
		code := send("203.0.113.1:1111", map[string]string{"X-Forwarded-For": "1.2.3." + string(rune('0'+i%10))})
		assert.Equal(t, http.StatusTooManyRequests, code, "a made-up X-Forwarded-For is no new bucket")
	}
}

func TestClientInfoFrom_SetsTheResolvedAddressOnTheContext(t *testing.T) {
	var got string
	handler := ClientInfoFrom(TrustAllProxies())(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		got = api.IpAddressFromContext(r.Context())
	}))

	handler.ServeHTTP(httptest.NewRecorder(), requestFrom("10.1.2.3:1", map[string]string{"X-Forwarded-For": "198.51.100.9"}))
	assert.Equal(t, "198.51.100.9", got)
}
