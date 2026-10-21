---
paths:
  - "gateway/**"
---

# Gateway

Details: `gateway/README.md`. The gateway has no database; it routes, authenticates and rate-limits.

- **Routes** are declared in `gateway/internal/router.go`: a path prefix per backend service, the HTTP `Methods`
  it accepts, `StripPrefix` (the prefix is removed before proxying) and auth rules (regex patterns).
- **Public endpoints** (login, register, ...) are auth rules with `RequireAuth: false` - the only place they are
  declared. `main.go` builds `AuthMiddleware`'s public-path list from `Router.PublicPaths()`.
- **Identity of the caller**: after the JWT check the gateway sends the account to the backend in `X-User-ID`
  (`authcontext.XUserIDHeader`). A client-sent `X-User-ID` is always dropped, so backends trust the header and
  never validate the JWT themselves (they do check `X-Gateway-Key`). There are no global roles: roles exist per
  activity group and the activity service checks them against its own membership data.
- **Streaming (SSE)** endpoints are listed in `Route.StreamPaths` and proxied by `StreamToService`, without the
  15s `WriteTimeout` and the per-service timeout - only connecting and waiting for response headers are bounded.
- **Rate limiting**: per-IP and per-account token buckets (`newRateLimiters`, `config.RateLimiterConfig`), kept in
  memory by each gateway instance (`shared/middleware/rate_limit.go`).
- A new backend route needs: the route in `router.go` (methods, auth rules, public paths), a test in
  `router_test.go`, and the service's base URL (`<SERVICE>_SERVICE_URL`, `shared/config/context_service.go`).
