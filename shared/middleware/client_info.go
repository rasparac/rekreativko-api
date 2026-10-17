package middleware

import (
	"net"
	"net/http"
	"net/netip"
	"strings"

	"github.com/rasparac/rekreativko-api/shared/api"
)

// ClientInfo records the caller's user agent and IP address on the request
// context (logs, traces and the login-attempt record read them from there).
//
// For services that sit behind the gateway: their only caller is the gateway,
// which has already worked out the real client address and sends it as the
// X-Forwarded-For header, so that header is taken as it is. The gateway itself
// uses ClientInfoFrom with its trusted proxies instead.
func ClientInfo(next http.Handler) http.Handler {
	return ClientInfoFrom(TrustAllProxies())(next)
}

// ClientInfoFrom is ClientInfo with an explicit rule for which proxies may
// speak for the client.
func ClientInfoFrom(resolver *ClientIPResolver) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := api.WithUserAgent(r.Context(), r.UserAgent())
			ctx = api.WithIpAddress(ctx, resolver.IP(r))

			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// ClientIPResolver decides which address a request's client has.
//
// Forwarded headers (X-Forwarded-For, X-Real-IP) are written by whoever sends
// the request, so they are believed only when the connection itself comes from
// a trusted proxy; from anyone else they are ignored and the connection's own
// address is the client. Without this a client could pick its own address by
// sending a header (and with it its rate-limit bucket).
type ClientIPResolver struct {
	trusted  []netip.Prefix
	trustAll bool
}

// NewClientIPResolver trusts forwarded headers from the given proxies, each a
// CIDR ("10.0.0.0/8") or a single address. No proxies means no forwarded
// header is ever believed.
func NewClientIPResolver(proxies []string) (*ClientIPResolver, error) {
	resolver := &ClientIPResolver{}

	for _, raw := range proxies {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}

		if prefix, err := netip.ParsePrefix(raw); err == nil {
			resolver.trusted = append(resolver.trusted, prefix.Masked())
			continue
		}

		addr, err := netip.ParseAddr(raw)
		if err != nil {
			return nil, &net.ParseError{Type: "trusted proxy CIDR or address", Text: raw}
		}
		resolver.trusted = append(resolver.trusted, netip.PrefixFrom(addr.Unmap(), addr.Unmap().BitLen()))
	}

	return resolver, nil
}

// TrustAllProxies believes X-Forwarded-For from any connection. Only for
// services that cannot be reached except through the gateway.
func TrustAllProxies() *ClientIPResolver {
	return &ClientIPResolver{trustAll: true}
}

// IP returns the client's address. It is never empty for a real connection.
func (c *ClientIPResolver) IP(r *http.Request) string {
	peer := peerHost(r)

	peerAddr, peerOK := parseAddr(peer)
	if !c.trustAll && !(peerOK && c.isTrusted(peerAddr)) {
		return peer
	}

	if hops := forwardedFor(r); len(hops) > 0 {
		if c.trustAll {
			if addr, ok := parseAddr(hops[0]); ok {
				return addr.String()
			}
			return peer
		}

		// Walk from the proxy nearest to us outwards: the first address that is
		// not one of our own proxies is the client. Anything further left was
		// written by the client and could be made up.
		for i := len(hops) - 1; i >= 0; i-- {
			addr, ok := parseAddr(hops[i])
			if !ok {
				return peer
			}
			if !c.isTrusted(addr) || i == 0 {
				return addr.String()
			}
		}
	}

	if addr, ok := parseAddr(r.Header.Get("X-Real-IP")); ok {
		return addr.String()
	}

	return peer
}

func (c *ClientIPResolver) isTrusted(addr netip.Addr) bool {
	for _, prefix := range c.trusted {
		if prefix.Contains(addr) {
			return true
		}
	}
	return false
}

// peerHost is the address of the other end of the connection, without the port.
func peerHost(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func parseAddr(s string) (netip.Addr, bool) {
	addr, err := netip.ParseAddr(strings.TrimSpace(s))
	if err != nil {
		return netip.Addr{}, false
	}
	return addr.Unmap().WithZone(""), true
}

// forwardedFor is the X-Forwarded-For chain, client first, over all header lines.
func forwardedFor(r *http.Request) []string {
	var hops []string
	for _, line := range r.Header.Values("X-Forwarded-For") {
		for _, hop := range strings.Split(line, ",") {
			if hop = strings.TrimSpace(hop); hop != "" {
				hops = append(hops, hop)
			}
		}
	}
	return hops
}
