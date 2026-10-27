# Outbox Publisher

Background worker implementing the transactional outbox pattern: publishes each bounded context's unpublished `event_outbox` rows to NATS. It is woken by Postgres `LISTEN/NOTIFY` as soon as a transaction commits events, and polls as a fallback. This is the only bridge between "a domain event was written inside a DB transaction" and "the rest of the system heard about it" — no other service talks to NATS to publish (only to subscribe).

Not an HTTP API — exposes only `/health` and `/metrics`.

## Why this exists

Every service writes domain events to its own schema's `event_outbox` table in the *same transaction* as the business change (transactional outbox pattern — avoids the classic "DB commit succeeded but the message never got published" dual-write problem). This worker is what actually gets those rows out onto NATS afterward, asynchronously.

## How it works

**Wake-up (`LISTEN/NOTIFY`).** Every service's `event_outbox` has an `AFTER INSERT ... FOR EACH STATEMENT` trigger that runs `pg_notify('outbox_events', <schema>)`. Postgres delivers a `NOTIFY` only when the sending transaction commits, so the publisher can never be woken for an event it cannot yet see. The publisher holds one dedicated connection (not from the pool, and straight to Postgres via `POSTGRES_DIRECT_HOST`/`POSTGRES_DIRECT_PORT` - `LISTEN` is session state, which PgBouncer's transaction pooling would hand to other clients) that `LISTEN`s on `outbox_events`, built on [`jackc/pgxlisten`](https://github.com/jackc/pgxlisten) (`shared/events/outbox_listener.go`):
- a notification names the schema, and that schema is published immediately; a burst of commits is coalesced into one pass per schema
- notifications are lossy by nature (nothing is queued while the connection is down), so after every (re)connect the listener asks for a **full pass over every schema**
- if the connection drops, pgxlisten reconnects every 2s; the poll below covers the gap
- if the publisher is behind, extra wake-ups are dropped rather than blocking the listener — the poll picks those events up

**Publish pass** (on startup, on each wake-up, and every `OUTBOX_POLL_INTERVAL` as the fallback), for each affected schema, in one transaction:
1. `SELECT event_id, event_type, payload FROM <schema>.event_outbox WHERE published_at IS NULL AND failed_at IS NULL ORDER BY created_at ASC LIMIT <OUTBOX_READ_LIMIT> FOR UPDATE SKIP LOCKED`
2. For each row: publish to NATS (subject = `event_type`), then mark `published_at = NOW()`.
3. On publish failure: the failure is recorded (`retry_count`, `last_error`) and the event is retried on later polls; after `OUTBOX_MAX_RETRIES` it is dead-lettered (`failed_at` set) so it can't block younger events. At-least-once delivery — consumers must tolerate redelivery.
4. **If publish succeeds but the `MarkEventAsPublished` update fails**, the transaction rolls back and the batch is redelivered next cycle (same at-least-once tradeoff, different failure point).
5. A wake-up keeps publishing while batches come back full, but stops at the first failure, so a failing event is retried at the poll's pace instead of burning its retries back to back.

Latency from commit to NATS is therefore milliseconds in the normal case; `OUTBOX_POLL_INTERVAL` only bounds the worst case when a `NOTIFY` is missed.

## Configuration

| Env var | Notes |
|---|---|
| `OUTBOX_SCHEMAS` | Required, comma-separated (e.g. `activity,identity,account_profile,notifications`) — every schema this instance polls |
| `OUTBOX_POLL_INTERVAL` | Default `5s` — with `OUTBOX_LISTEN` on, only the fallback for missed notifications |
| `OUTBOX_LISTEN` | Default `true` — publish on `NOTIFY` instead of waiting for the next poll; `false` = polling only |
| `POSTGRES_DIRECT_HOST`, `POSTGRES_DIRECT_PORT` | Postgres itself, for the `LISTEN` connection when `POSTGRES_HOST`/`POSTGRES_PORT` point at PgBouncer (`db`/`5432` in `docker-compose.yaml`); empty = `POSTGRES_HOST`/`POSTGRES_PORT` |
| `OUTBOX_MAX_RETRIES` | Default `5` — failed publishes before an event is dead-lettered |
| `OUTBOX_READ_LIMIT` | Default `100` — rows fetched per schema per poll |
| `OUTBOX_PUBLISHED_RETENTION` | Default `168h` (7 days, the NATS stream's `MaxAge`) - published rows older than this are deleted, see [Purging old rows](#purging-old-rows) |
| `OUTBOX_DEAD_LETTER_RETENTION` | Default `336h` (14 days, the NATS `DLQ` stream's `MaxAge`) - rows dead-lettered longer ago than this are deleted |
| `OUTBOX_PURGE_INTERVAL` | Default `1h` - how often the purge runs (and once at start-up); `0` turns it off, published and dead-lettered rows are then kept forever |
| `OUTBOX_PURGE_BATCH` | Default `1000` - rows deleted per statement, so one delete never holds many locks |
| `NATS_URL` | Broker connection |
| `PORT` | `8085` in docker-compose |

## Purging old rows

Published rows are not an event history (the NATS stream keeps the messages for the same 7 days, and the analytics sink in epic `1uo` is for history), so they would only make the tables grow. Dead-lettered rows are kept for inspection and replay, but payloads can carry personal data (identity events hold email, phone and verification codes), so they may not stay forever - not even after the account is erased. The same process deletes both: at start-up and then every `OUTBOX_PURGE_INTERVAL`, for each schema in `OUTBOX_SCHEMAS`:

- `domainevent.DeletePublishedEvents` removes rows with `published_at` older than `OUTBOX_PUBLISHED_RETENTION`;
- `domainevent.DeleteDeadLetteredEvents` removes rows with `failed_at` older than `OUTBOX_DEAD_LETTER_RETENTION`;

each oldest first, `OUTBOX_PURGE_BATCH` at a time, until a batch comes back short.

- **Kept**: pending rows (`published_at` and `failed_at` NULL, however old), and anything published or dead-lettered inside its window. A dead-lettered event not handled within the window is lost.
- Several replicas are safe: the deletes are idempotent and use `FOR UPDATE SKIP LOCKED`. They use the `published_at` index and the partial `failed_at` index.
- A failing schema or kind is logged and does not stop the others; the next run retries.
- Metric `events_outbox_purged_total{schema, kind="published"|"dead_lettered"}`; an Info line `purged published outbox rows` / `purged dead-lettered outbox rows` per schema and kind that deleted anything.

## Metrics (Prometheus, subsystem `events`)

- `events_published_total{event_type, schema}` — successful publishes
- `events_processed_total{event_type, schema, status="success"|"failed"}` — every attempt, both outcomes
- `event_publish_duration_seconds{event_type, schema}` — NATS publish latency

## Known issues

- A dead-lettered event (`failed_at` set) is not retried or alerted on; it stays in the table for manual inspection until `OUTBOX_DEAD_LETTER_RETENTION` passes. See the dead-letter hardening ticket for the publish-failure path.
- The `NOTIFY` trigger is part of each service's `000001` schema (the project is not production-ready, so no separate migration). A database created before it needs its migration re-applied; until then the publisher simply falls back to polling.
- Running several replicas is safe (`FOR UPDATE SKIP LOCKED`), but every replica `LISTEN`s and wakes on every commit, so extra replicas add wake-ups, not throughput on a single schema's ordering.
