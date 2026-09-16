# Gateway Service

Single entry point for all external traffic: routing to backend services, JWT authentication, CORS, rate limiting, and per-request correlation (request ID, client IP/user-agent, tracing). Runs on port `8080`, the only service with a published host port.

Does **not** handle business logic, own any database, or talk to NATS — it's pure edge/routing.

## Running it

Part of `docker compose up` (container `gateway`). Reachable at `http://localhost:8080` (or the host's LAN IP for physical-device testing — see `shared/api/env.ts` in the mobile app).

## Routing

Requests are matched by path prefix, then proxied to the corresponding backend with the prefix stripped:

| Prefix | Backend | Env var |
|---|---|---|
| `/identity` | `identity` | `IDENTITY_SERVICE_URL` |
| `/account-profile` | `account-profile` | `ACCOUNT_PROFILE_SERVICE_URL` |
| `/activity` | `activity` | `ACTIVITY_SERVICE_URL` |
| `/notifications` | `notifications` | `NOTIFICATIONS_SERVICE_URL` |

`GET /health` and `GET /{$}` (root) are served by the gateway itself. `GET /metrics` is served by the gateway itself too, gated by `middleware.RequireMetricsToken` (a static bearer credential, not user JWT auth) rather than the routing rules below.

### Auth rules — two separate lists, must be kept in sync

There are **two independent, duplicated public-path allowlists** that both have to agree for a route to actually be reachable without a JWT:

1. `gateway/cmd/api/main.go`'s `publicPaths` — read by `middleware.NewAuthMiddleware(...).RequireAuth`, which is what actually decodes the JWT and injects `accountID` into request context.
2. `gateway/internal/router.go`'s `loadRoutes()` — a second, separately-maintained `AuthRule` list per route prefix, checked by `Router.isAuthSatisfied`, which just checks whether `accountID` ended up non-nil in context (i.e. whether step 1 already authenticated the request).

**Adding a public path in only one of these fails silently** — the request gets a `401` from whichever list still requires auth, with no indication that the other list already allowed it. If you add a new public endpoint, grep both files.

Current public paths (everything else under a registered prefix requires a valid `Bearer` JWT):
```
^/identity/api/v1/(login|register|verify-account|resend-verification-code|refresh-token)$
^/swagger/.*
^/health$
```

Per-prefix method restrictions: `account-profile` only allows `POST`, `GET`, `PUT` (no `DELETE`) — everything else (`identity`, `activity`, `notifications`) allows all methods.

### What an authenticated request gets

On success, `RequireAuth` parses the JWT and puts `accountID` in context; the router's `addUserHeaders` then forwards it to the backend as `X-User-Id` and `X-User-Roles` headers — backends trust these headers rather than re-validating the JWT themselves (see `shared/authcontext`).

### Service-to-service secret (`X-Gateway-Key`)

Independent of user JWT auth: the gateway injects `X-Gateway-Key` (`AddGatewayKey` middleware, from `GATEWAY_API_KEYS`, comma-separated to support rotation) on every proxied request. Every backend service validates it (`middleware.CheckGatewayKey`, constant-time comparison) before processing anything — this exists so backends are never reachable by bypassing the gateway, even on the internal docker network. `/metrics` uses the same shared secret via `RequireMetricsToken` (see `observability/prometheus.yml`'s scrape config).

## Middleware chain (order matters)

```
Recover → RequestID → ClientInfo → Logging → Tracing → SpanEnrichment
  → CORS → RateLimiter → RequireAuth → AddGatewayKey → [Metrics, if enabled] → Router
```

`ClientInfo` must run before anything that logs (it puts IP/user-agent into context); `RequireAuth` must run before the router (it's what populates `accountID`); `AddGatewayKey` runs last so it applies to the actual outbound proxied request.

## Configuration

| Env var | Notes |
|---|---|
| `IDENTITY_SERVICE_URL`, `ACCOUNT_PROFILE_SERVICE_URL`, `ACTIVITY_SERVICE_URL`, `NOTIFICATIONS_SERVICE_URL` | Backend targets |
| `GATEWAY_API_KEYS` | Comma-separated; any one matching is accepted (rotation support) |
| `GATEWAY_API_KEY` | The gateway's own key, used by `RequireMetricsToken` on `/metrics` |
| `JWT_SECRET` | Must match `identity`'s — the gateway validates tokens `identity` issues |
| `CORS_ALLOWED_ORIGINS` | Defaults to `*` in dev; `AllowCredentials` is hardcoded `false` (browser requires exact origins, not `*`, to use credentials — flip both together if this ever needs real cookie-based auth) |
| `REQUESTS_PER_MINUTE` | Per-client-IP rate limit (in-memory, resets on restart — see [Known issues](#known-issues)) |
| `METRICS_ENABLED` | Gates `/metrics` registration |
| `SERVICE_ENVIRONMENT=development` | Also enables `/swagger/*` (aggregates all 4 services' specs via `swagger-ui` container, separate from this one) |

No database, no NATS connection (removed this session — notification-sending was moved to the `notifications` service, which now owns that NATS subscription).

## Reverse proxy behavior

- `DisableKeepAlives: true` on every backend connection — deliberate: a pooled connection to a backend that restarted (common in local dev, or during a rolling deploy) fails with "connection reset by peer" rather than being retried, especially for non-idempotent methods. Trades a little latency for correctness.
- Forwards `X-Forwarded-For`/`X-Forwarded-Host`/`X-Forwarded-Proto` (`pr.SetXForwarded()`), which is how `ClientInfo` downstream recovers the real client IP through the proxy hop.
- Adds `X-ProxiedBy: rekreativko-gateway` and `X-Service-Name: <backend>` to every response.
- On a dead/unreachable backend: `503 Service unavailable`. On no route match: `404`. On an unregistered method for a prefix that restricts methods: `405`.

## Known issues

- **Dual auth-allowlist duplication** (see above) — a real design smell, not just a one-off bug. Worth consolidating into one source of truth so a future "make X public" change can't silently half-apply.
- **Rate limiter is in-memory, per-instance.** Fine for a single gateway replica (today's setup); would under-count if the gateway is ever scaled horizontally, since each replica tracks its own counters.
- **Root handler (`GET /`) returns malformed JSON**: `{"service": "rekreativko-api", "status": "running", "version": 1.0.0}` — `1.0.0` is an unquoted string in a JSON literal, which is invalid JSON (two decimal points). Cosmetic only (nothing parses this response), but confusing if you ever curl it expecting valid JSON.
