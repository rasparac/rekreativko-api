package gateway

import (
	"log/slog"
	"net/http"
	"regexp"
	"slices"
	"strings"

	"github.com/google/uuid"
	"github.com/rasparac/rekreativko-api/shared/authcontext"
)

type (
	Router struct {
		routes []Route
		proxy  *ReverseProxy
	}

	Route struct {
		Prefix      string
		Service     string
		StripPrefix bool
		Methods     []string
		AuthRule    []AuthRule
		// StreamPaths are patterns of long-lived streaming endpoints
		// (server-sent events). They are proxied without the write and
		// service timeouts that bound ordinary requests. Declared here, not
		// detected from the request, so a client can't lift the timeouts on
		// an arbitrary route.
		StreamPaths []string

		streamCompiled []*regexp.Regexp
	}

	AuthRule struct {
		PathPattern string
		RequireAuth bool
		compiled    *regexp.Regexp
	}
)

func NewRouter(proxy *ReverseProxy) *Router {
	r := &Router{
		routes: make([]Route, 0),
		proxy:  proxy,
	}

	r.loadRoutes()

	return r
}

func (r *Router) loadRoutes() {
	r.addRoute(Route{
		Prefix:      "/identity",
		Service:     "identity",
		StripPrefix: true,
		AuthRule: []AuthRule{
			{
				PathPattern: "^/identity/api/v1/(login|register|verify-account|resend-verification-code|refresh-token)$",
				RequireAuth: false,
			},
			{
				PathPattern: "^/identity/api/v1/.*",
				RequireAuth: true,
			},
		},
	})

	r.addRoute(Route{
		Prefix:      "/account-profile",
		Service:     "account-profile",
		StripPrefix: true,
		Methods:     []string{"POST", "GET", "PUT"},
		AuthRule: []AuthRule{
			{
				PathPattern: "^/account-profile/api/v1/.*",
				RequireAuth: true,
			},
		},
	})

	r.addRoute(Route{
		Prefix:      "/activity",
		Service:     "activity",
		StripPrefix: true,
		AuthRule: []AuthRule{
			{
				PathPattern: "^/activity/api/v1/.*",
				RequireAuth: true,
			},
		},
		StreamPaths: []string{
			"^/activity/api/v1/sessions/[^/]+/events$", // team-formation live updates
		},
	})

	r.addRoute(Route{
		Prefix:      "/notifications",
		Service:     "notifications",
		StripPrefix: true,
		AuthRule: []AuthRule{
			{
				PathPattern: "^/notifications/api/v1/.*",
				RequireAuth: true,
			},
		},
	})

	r.addRoute(Route{
		Prefix:      "/location",
		Service:     "location",
		StripPrefix: true,
		Methods:     []string{"GET"},
		AuthRule: []AuthRule{
			{
				PathPattern: "^/location/api/v1/.*",
				RequireAuth: true,
			},
		},
	})
}

func (r *Router) addRoute(route Route) {
	for i := range route.AuthRule {
		comp, err := regexp.Compile(route.AuthRule[i].PathPattern)
		if err != nil {
			slog.Error(
				"invalid regex pattern",
				"pattern", route.AuthRule[i].PathPattern,
				"error", err,
			)
			continue
		}
		route.AuthRule[i].compiled = comp
	}

	for _, pattern := range route.StreamPaths {
		comp, err := regexp.Compile(pattern)
		if err != nil {
			// Fails safe: the path is proxied as an ordinary request.
			slog.Error(
				"invalid stream path pattern",
				"pattern", pattern,
				"error", err,
			)
			continue
		}
		route.streamCompiled = append(route.streamCompiled, comp)
	}

	r.routes = append(r.routes, route)
}

// PublicPaths returns the patterns of every auth rule that does not require
// auth. The routes are the single source of truth for public endpoints: the
// gateway's AuthMiddleware runs before the router and must be built from this
// list, otherwise it rejects (or lets through) a route the router disagrees on.
func (r *Router) PublicPaths() []string {
	var paths []string
	for _, route := range r.routes {
		for _, rule := range route.AuthRule {
			if !rule.RequireAuth {
				paths = append(paths, rule.PathPattern)
			}
		}
	}
	return paths
}

func (r *Router) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	route := r.matchRoute(req.URL.Path)
	if route == nil {
		http.NotFound(w, req)
		return
	}

	if !r.isMethodAllowed(route, req.Method) {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	authRule := r.matchAuthRule(route, req.URL.Path)
	if !r.isAuthSatisfied(authRule, req) {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	r.addUserHeaders(req)

	stream := route.isStream(req.URL.Path)

	if route.StripPrefix {
		req.URL.Path = strings.TrimPrefix(req.URL.Path, route.Prefix)
		if req.URL.Path == "" {
			req.URL.Path = "/"
		}
	}

	if stream {
		r.proxy.StreamToService(route.Service, w, req)
		return
	}

	r.proxy.ProxyToService(route.Service, w, req)
}

func (r *Router) matchRoute(path string) *Route {
	for i := range r.routes {
		prefix := r.routes[i].Prefix
		if path == prefix || strings.HasPrefix(path, prefix+"/") {
			return &r.routes[i]
		}
	}

	return nil
}

// isStream reports whether path (before the prefix is stripped) is one of the
// route's streaming endpoints.
func (route *Route) isStream(path string) bool {
	for _, re := range route.streamCompiled {
		if re.MatchString(path) {
			return true
		}
	}
	return false
}

func (r *Router) matchAuthRule(route *Route, path string) *AuthRule {
	for i := range route.AuthRule {
		if route.AuthRule[i].compiled != nil && route.AuthRule[i].compiled.MatchString(path) {
			return &route.AuthRule[i]
		}
	}

	return &AuthRule{
		RequireAuth: true,
		PathPattern: path,
	}
}

func (r *Router) isMethodAllowed(router *Route, method string) bool {
	if len(router.Methods) == 0 {
		return true
	}

	return slices.Contains(router.Methods, method)
}

func (r *Router) isAuthSatisfied(authRule *AuthRule, req *http.Request) bool {
	if authRule == nil {
		return true
	}

	if !authRule.RequireAuth {
		return true
	}

	userID := authcontext.GetAccountID(req.Context())
	return userID != uuid.Nil
}

// addUserHeaders tells the backend who the caller is. Only the gateway may
// set X-User-ID - backends trust it as the authenticated account - so a value
// the client sent is always dropped, and replaced only when the caller is
// authenticated (public routes forward no identity at all).
func (r *Router) addUserHeaders(req *http.Request) {
	req.Header.Del(authcontext.XUserIDHeader)

	if userID := authcontext.GetAccountID(req.Context()); userID != uuid.Nil {
		req.Header.Set(authcontext.XUserIDHeader, userID.String())
	}
}
