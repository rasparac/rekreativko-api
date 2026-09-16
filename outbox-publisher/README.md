# Outbox Publisher

Background worker implementing the transactional outbox pattern: polls each bounded context's `event_outbox` table for unpublished domain events and publishes them to NATS. This is the only bridge between "a domain event was written inside a DB transaction" and "the rest of the system heard about it" — no other service talks to NATS to publish (only to subscribe).

Not an HTTP API — exposes only `/health` and `/metrics`.

## Why this exists

Every service writes domain events to its own schema's `event_outbox` table in the *same transaction* as the business change (transactional outbox pattern — avoids the classic "DB commit succeeded but the message never got published" dual-write problem). This worker is what actually gets those rows out onto NATS afterward, asynchronously.

## How it works

On startup and then every `OUTBOX_POLL_INTERVAL`, for each configured schema:
1. `SELECT event_id, event_type, payload FROM <schema>.event_outbox WHERE published_at IS NULL ORDER BY created_at ASC LIMIT <OUTBOX_READ_LIMIT>`
2. For each row: publish to NATS (subject = `event_type`), then mark `published_at = NOW()`.
3. On publish failure: leave the row unpublished, retried next poll (at-least-once delivery — consumers must tolerate redelivery).
4. **If publish succeeds but the `MarkEventAsPublished` update fails**, the event is redelivered next cycle too (same at-least-once tradeoff, just a different failure point).

## Configuration

| Env var | Notes |
|---|---|
| `OUTBOX_SCHEMAS` | Required, comma-separated (e.g. `activity,identity,account_profile,notifications`) — every schema this instance polls |
| `OUTBOX_POLL_INTERVAL` | Default `5s` |
| `OUTBOX_READ_LIMIT` | Default `100` — rows fetched per schema per poll |
| `NATS_URL` | Broker connection |
| `PORT` | `8085` in docker-compose |

## Metrics (Prometheus, subsystem `events`)

- `events_published_total{event_type, schema}` — successful publishes
- `events_processed_total{event_type, schema, status="success"|"failed"}` — every attempt, both outcomes
- `event_publish_duration_seconds{event_type, schema}` — NATS publish latency

## Known issues

- **Not safe to run more than one replica.** The `SELECT ... WHERE published_at IS NULL` query has no row locking (`FOR UPDATE SKIP LOCKED` or equivalent) — two concurrent instances polling the same schema would both fetch and publish the same unpublished rows, causing duplicate NATS messages beyond the normal at-least-once redelivery case. `docker-compose.yaml` only ever runs one instance today; if this is ever scaled out, add row locking first.
- No dead-letter handling: a payload that fails to publish forever (e.g. permanently malformed) would retry indefinitely every poll cycle rather than being set aside.
