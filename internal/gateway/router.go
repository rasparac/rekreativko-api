package gateway

import (
	"log/slog"
	"net/http"
	"regexp"
	"slices"
	"strings"

	"github.com/google/uuid"
	"github.com/rasparac/rekreativko-api/internal/shared/authcontext"
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
	}

	AuthRule struct {
		PathPattern   string
		RequireAuth   bool
		RequiredRoles []string
		compiled      *regexp.Regexp
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
				PathPattern: "^/identity/api/v1/(login|register|verify-account|resend-verification-code)$",
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

	r.routes = append(r.routes, route)
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

	if !r.hasRequiredRoles(authRule, req) {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	r.addUserHeaders(req)

	if route.StripPrefix {
		req.URL.Path = strings.TrimPrefix(req.URL.Path, route.Prefix)
		if req.URL.Path == "" {
			req.URL.Path = "/"
		}
	}

	r.proxy.ProxyToService(route.Service, w, req)
}

func (r *Router) matchRoute(path string) *Route {
	for i := range r.routes {
		if strings.HasPrefix(path, r.routes[i].Prefix) {
			return &r.routes[i]
		}
	}

	return nil
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

func (r *Router) hasRequiredRoles(authRule *AuthRule, req *http.Request) bool {
	if authRule == nil || len(authRule.RequiredRoles) == 0 {
		return true
	}

	userRoles := authcontext.GetRoles(req.Context())
	if len(userRoles) == 0 {
		return false
	}

	roleSet := make(map[string]struct{}, len(userRoles))
	for _, role := range userRoles {
		roleSet[strings.TrimSpace(role)] = struct{}{}
	}

	for _, requiredRole := range authRule.RequiredRoles {
		if _, exists := roleSet[requiredRole]; !exists {
			return false
		}
	}

	return true
}

func (r *Router) addUserHeaders(req *http.Request) {
	ctx := req.Context()
	userID := authcontext.GetAccountID(ctx)
	if userID != uuid.Nil {
		req.Header.Set(authcontext.XUserIDHeader, userID.String())
	}

	authcontext.GetRoles(ctx)
	roles := strings.Join(authcontext.GetRoles(ctx), ",")
	req.Header.Set(authcontext.XUserRolesHeader, roles)
}
