package middleware

import (
	"net/http"
	"regexp"
	"strings"

	"github.com/rasparac/rekreativko-api/internal/shared/api"
)

func ClientInfo(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		userAgent := r.UserAgent()
		ctx := api.WithUserAgent(r.Context(), userAgent)

		remoteAddr := GetIP(r)

		ctx = api.WithIpAddress(ctx, remoteAddr)

		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// RxFor regex is used to strip out remote address from request struct
// The first element will always be the 'for=' capture, which we ignore.
// In the case of multiple IP addresses (for=8.8.8.8, 8.8.4.4,172.16.1.20 is valid)
// we only extract the first, which should be the client IP.
var RxFor = regexp.MustCompile(`(?i)(?:for=)([^(;|,| )]+)`)

// this method is from robokiller enterprise.
// it will check cloudflare headers to get users IP and if it's empty
// it will check other IP headers
func GetIP(r *http.Request) string {
	var addr string

	if fwd := r.Header.Get("CF-Connecting-IP"); fwd != "" {
		addr = fwd
	} else if fwd := r.Header.Get("True-Client-IP"); fwd != "" {
		addr = fwd
	} else if fwd := r.Header.Get("X-Forwarded-For"); fwd != "" {
		// Only grab the first (client) address. Note that '192.168.0.1,
		// 10.1.1.1' is a valid key for X-Forwarded-For where addresses after
		// the first may represent forwarding proxies earlier in the chain.
		s := strings.Index(fwd, ", ")
		if s == -1 {
			s = len(fwd)
		}
		addr = fwd[:s]
	} else if fwd := r.Header.Get("X-Real-IP"); fwd != "" {
		// X-Real-IP should only contain one IP address (the client making the
		// request).
		addr = fwd
	} else if fwd := r.Header.Get("Forwarded"); fwd != "" {
		// match should contain at least two elements if the protocol was
		// specified in the Forwarded header. The first element will always be
		// the 'for=' capture, which we ignore. In the case of multiple IP
		// addresses (for=8.8.8.8, 8.8.4.4,172.16.1.20 is valid) we only
		// extract the first, which should be the client IP.

		if match := RxFor.FindStringSubmatch(fwd); len(match) > 1 {
			// IPv6 addresses in Forwarded headers are quoted-strings. We strip
			// these quotes.
			addr = strings.Trim(match[1], `"`)
		}
	}

	return addr
}
