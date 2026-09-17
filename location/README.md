# Location Service

Backend proxy/cache in front of [Nominatim](https://nominatim.org/) (OpenStreetMap's geocoding API), so mobile no longer calls a third party directly. Introduced to replace direct `mobile -> nominatim.openstreetmap.org` calls, which had no server-side rate limiting, caching, observability, or User-Agent compliance with Nominatim's usage policy.

```
mobile -> gateway -> location -> nominatim -> location -> mobile
```

## Running it

Part of `docker compose up` (container `location`, port `8085` internally, no host port published — reachable only via the gateway or docker network).

## API (mobile contract)

| Method | Path | Notes |
|---|---|---|
| GET | `/api/v1/geocode/reverse?lat={lat}&lon={lon}` | Resolve City/Country for a coordinate pair |

Response: `{ city, country, country_code }`. Errors: `400` invalid/missing `lat`/`lon`, `404` no location found for the coordinates, `502` Nominatim is unavailable.

Auth: gateway requires a bearer token for all of `/location/api/v1/*` (GET only); this service additionally validates `X-Gateway-Key`, same as every other backend service.

**Forward geocoding (City/Country -> lat/long) is not implemented** — out of scope for this pass, follow-up if/when mobile needs it.

## Caching strategy

Coordinates are rounded to a 3-decimal-place grid cell (`domain.Coordinates.Bucket`, ~110m at the equator) and used as the cache key in `location.geocode_cache`. On a cache hit, no Nominatim call is made. On a miss, the result is persisted for that bucket.

**Negative results are cached too**: if Nominatim definitively returns no address for a bucket, that's stored (`not_found = true`) so the same unresolvable bucket doesn't burn another rate-limited Nominatim call on every request. This is distinct from a transient Nominatim failure (network error, non-200 response) — those are never cached, since they're worth retrying on the next request rather than being treated as a permanent answer.

**No TTL / cache invalidation** — City/Country resolution for a given coordinate is treated as effectively immutable, so entries live indefinitely. A cached row is only ever overwritten by a fresher Nominatim answer for the same bucket (there's no background refresh — this only happens if the bucket is looked up again).

## Nominatim usage-policy compliance

- `nominatim.Client` sets a `User-Agent` identifying this app (`NOMINATIM_USER_AGENT`, see below) on every outbound call, per Nominatim's usage policy.
- A single process-wide rate limiter (`nominatim.rateLimiter`) gates every outbound call to at most 1 per `NOMINATIM_MIN_REQUEST_INTERVAL` (default `1s`) — a plain mutex + last-call-timestamp gate, not a token-bucket library, since this service only ever needs a hard minimum-interval floor.

## Configuration

| Env var | Notes |
|---|---|
| `NOMINATIM_BASE_URL` | Default `https://nominatim.openstreetmap.org` |
| `NOMINATIM_USER_AGENT` | Default is a placeholder — **set this to something with a real contact per Nominatim's usage policy before using it against the public instance in production** |
| `NOMINATIM_REQUEST_TIMEOUT` | Default `10s` |
| `NOMINATIM_MIN_REQUEST_INTERVAL` | Default `1s` (Nominatim's policy: no more than 1 req/sec) |
| `GATEWAY_API_KEY` | Required, validated via `X-Gateway-Key` |
| `PORT` | `8085` in compose |
| `POSTGRES_*`, `TELEMETRY_*`, `METRICS_ENABLED`, `LOG_LEVEL`/`LOGGER_FORMAT` | Shared config |

Not used by this service despite being in the shared config struct: `JWT_*`, `CORS_*`, `REDIS_*`, rate limiter, `OUTBOX_*`, `NATS_URL`, `GATEWAY_API_KEYS` (plural — gateway-only).

## Database schema

One table, one schema:
```sql
CREATE TABLE location.geocode_cache(
    id uuid PRIMARY KEY DEFAULT uuid_generate_v4(),
    lat_bucket double precision NOT NULL,
    lng_bucket double precision NOT NULL,
    city varchar(255) NOT NULL DEFAULT '',
    country varchar(255) NOT NULL DEFAULT '',
    country_code varchar(2) NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT NOW(),
    updated_at timestamptz NOT NULL DEFAULT NOW()
);
```
Unique index on `(lat_bucket, lng_bucket)` — the cache lookup key.

## Known follow-ups

- Forward geocoding (City/Country/street -> lat/long) is not implemented. Mobile's web target currently calls Nominatim directly for this (`nominatimForwardGeocode` in its location client) — that direct third-party call remains until a follow-up adds it here. Tracked as `rekreativko-api-7ul`.
- `NOMINATIM_USER_AGENT`'s default is a placeholder; a real contact-identifying value needs to be set via env before this hits Nominatim's public instance for real traffic.
- No admin/ops endpoint to evict or refresh a stale cache entry if Nominatim's data for a bucket changes (e.g. new development, boundary change) — would need a manual DB delete today.
