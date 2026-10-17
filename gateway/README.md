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

On success, `RequireAuth` parses the JWT and puts `accountID` in context; the router's `addUserHeaders` then forwards it to the backend as the `X-User-ID` header (a client-sent value is always dropped) — backends trust this header rather than re-validating the JWT themselves (see `shared/authcontext`).

### Service-to-service secret (`X-Gateway-Key`)

Independent of user JWT auth: the gateway injects `X-Gateway-Key` (`AddGatewayKey` middleware, from `GATEWAY_API_KEYS`, comma-separated to support rotation) on every proxied request. Every backend service validates it (`middleware.CheckGatewayKey`, constant-time comparison) before processing anything — this exists so backends are never reachable by bypassing the gateway, even on the internal docker network. `/metrics` uses the same shared secret via `RequireMetricsToken` (see `observability/prometheus.yml`'s scrape config).

## Middleware chain (order matters)

```
Recover → RequestID → ClientInfo → Logging → Tracing → SpanEnrichment
  → CORS → RequireAppKey → RateLimiter (per IP) → RequireAuth → LimitCaller (per account) → AddGatewayKey
  → [Metrics, if enabled] → Router
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
| `RATE_LIMIT_<CLASS>_BURST`, `RATE_LIMIT_<CLASS>_PER_MINUTE` | Token buckets, classes `IP`, `ACCOUNT`, `ANONYMOUS`, `LOGIN`, `SENSITIVE`. See [Rate limiting](#rate-limiting) for the defaults. Replaces `REQUESTS_PER_MINUTE` |
| `APP_KEYS`, `APP_KEY_ENFORCE` | Mobile app key, see [App key](#app-key). Empty `APP_KEYS` (the default) turns the check off; `APP_KEY_ENFORCE=false` (the default) only counts requests without a valid key |
| `TRUSTED_PROXY_CIDRS` | Comma-separated CIDRs/addresses of the proxies in front of the gateway (ingress, load balancer). See [Client IP](#client-ip). Empty (the default, right for docker compose) means no forwarded header is believed |
| `METRICS_ENABLED` | Gates `/metrics` registration |
| `SERVICE_ENVIRONMENT=development` | Also enables `/swagger/*` (aggregates all 4 services' specs via `swagger-ui` container, separate from this one) |

No database, no NATS connection (removed this session — notification-sending was moved to the `notifications` service, which now owns that NATS subscription).

## Client IP

One address per request, worked out once by `middleware.ClientInfoFrom(resolver)` and used for the rate limiter, the access log, traces and the IP stored with login attempts (`middleware.ClientIPResolver`):

- The client is the connection's own address (`RemoteAddr`, port stripped) - never empty.
- `X-Forwarded-For` / `X-Real-IP` are written by whoever sends the request, so they are believed **only when the connection comes from a proxy in `TRUSTED_PROXY_CIDRS`**. Then the client is the first address in `X-Forwarded-For`, reading from the right, that is not itself a trusted proxy (what is further left may be made up by the client); `X-Real-IP` if there is no chain.
- `CF-Connecting-IP`, `True-Client-IP` and `Forwarded` are not believed at all.
- An address that does not parse is never used as a key: the connection's address is.

**Deploying behind an ingress / load balancer, set `TRUSTED_PROXY_CIDRS` to its address range** (the ingress controller's pod network in Kubernetes). Without it every client arrives as the proxy's own address and they all share one rate-limit bucket. Docker compose has no proxy, so leave it empty there - but note every browser on one machine is then one client, so the `IP` backstop and the anonymous limits are shared by all of them (logged-in users are limited per account, see [Rate limiting](#rate-limiting)).

## App key

The mobile app is built with a key and sends it in `X-App-Key` on every request (`middleware.RequireAppKey`, after CORS and before the rate limiter, so a browser's preflight - which can't carry the header - is answered first). `/health`, `/metrics` and `/swagger/` are outside the chain and need no key.

**It is not a secret.** Anyone can read it out of the app or from one intercepted request, so it is not authentication. It keeps out casual traffic (curl, scanners, bots), lets a leaked key be rotated, and lets old app versions be turned away. It does not replace per-account limits or login lockout.

- `APP_KEYS`: comma-separated; any one is accepted, so a new key can be released before the old one is removed. Empty: the check is off.
- `APP_KEY_ENFORCE`: `false` (default) only counts requests with a missing or invalid key (`gateway_app_key_rejections_total{reason="missing|invalid", enforced}`) and lets them through, so you can see who would be refused before turning it on. `true` refuses them with **403** `app_key_invalid` - deliberately not 401, which the app answers by refreshing its token and logging out.
- The header is removed before the request is proxied; the backends never see it.
- Rotation: add the new key to `APP_KEYS`, release the app with it, and remove the old one once its share of traffic is gone (`enforced="false"` counts show this while not enforcing).
- Rollout: set `APP_KEYS`, release the app that sends it, watch `gateway_app_key_rejections_total` fall to the clients that should be turned away (scripts, the web app, old builds), then set `APP_KEY_ENFORCE=true`.
- Keep the key out of the repo: inject it at build time in the app and as a secret for the gateway.

## Rate limiting

Token buckets (`middleware.RateLimit`): a caller can send `Burst` requests at once and gets `PerMinute` of them back each minute, so a burst is smoothed instead of locking the caller out until a fixed window ends. A refused request gets `429 too_many_requests` with `Retry-After` (whole seconds until the next token, at least 1).

Two places in the chain, three kinds of key:

| Limiter | Where | Key | Default burst / per minute |
|---|---|---|---|
| `IP` | before `RequireAuth` | client address | 600 / 600 |
| `ACCOUNT` | after `RequireAuth`, logged-in callers | account id | 120 / 90 |
| `ANONYMOUS` | after `RequireAuth`, public paths without a stricter class (`refresh-token`) | client address | 60 / 60 |
| `LOGIN` | `login` | client address | 10 / 10 |
| `SENSITIVE` | `register`, `verify-account`, `resend-verification-code` (they create accounts or send an SMS/email) | client address | 5 / 5 |

- `IP` is a backstop in front of authentication: requests that auth rejects (missing or bad token) still cost something, so they are counted. Keep it generous, as many users can share one address (mobile carriers, offices). A logged-in request is counted against both its address and its account.
- Logged-in callers are limited per account, so users behind one address do not share a bucket. A request's class for anonymous callers is declared on the router's auth rule (`AuthRule.RateClass`), next to whether it is public.
- Not counted: CORS preflight (answered before the limiter), `/health`, `/metrics`, `/swagger/`. An SSE stream (`/events`) costs one token when it connects, not one per event, so a reconnect loop is still limited.
- Sizing (measured): a Home load is ~6 requests, a session page ~8, idle polling ~4/min, fallback polling 12-36/min per open screen.
- A limit of 0 is refused at start-up.

## Reverse proxy behavior

- `DisableKeepAlives: true` on every backend connection — deliberate: a pooled connection to a backend that restarted (common in local dev, or during a rolling deploy) fails with "connection reset by peer" rather than being retried, especially for non-idempotent methods. Trades a little latency for correctness.
- Forwards `X-Forwarded-Host`/`X-Forwarded-Proto` (`pr.SetXForwarded()`) and sets `X-Forwarded-For` to the client address the gateway resolved (see [Client IP](#client-ip)), not the address of its own peer and not whatever the client wrote. Backends take that header as it is (`middleware.ClientInfo`, `TrustAllProxies`): they only accept requests carrying the gateway key, so only the gateway can have written it.
- Adds `X-ProxiedBy: rekreativko-gateway` and `X-Service-Name: <backend>` to every response.
- On a dead/unreachable backend: `503 Service unavailable`. On no route match: `404`. On an unregistered method for a prefix that restricts methods: `405`.

## Known issues

- **Dual auth-allowlist duplication** (see above) — a real design smell, not just a one-off bug. Worth consolidating into one source of truth so a future "make X public" change can't silently half-apply.
- **Rate limiter is in-memory, per-instance.** Each replica tracks its own buckets, so with N replicas (k8s runs 2) a caller can get up to N times the limit, and a restart resets them. Fine for these limits; if exact limits matter, back the buckets with Redis (already running).
- **Root handler (`GET /`) returns malformed JSON**: `{"service": "rekreativko-api", "status": "running", "version": 1.0.0}` — `1.0.0` is an unquoted string in a JSON literal, which is invalid JSON (two decimal points). Cosmetic only (nothing parses this response), but confusing if you ever curl it expecting valid JSON.
