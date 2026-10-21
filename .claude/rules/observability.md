# Observability

Local UIs: Grafana http://localhost:3000 (Loki logs, dashboards), Jaeger http://localhost:16686,
Prometheus http://localhost:9090. Every service serves `/metrics` when `METRICS_ENABLED`.

## Personal data in logs and traces

Identify a person by `account_id` only. Never put an email, phone number, name or nickname, date of birth,
verification code or token, exact coordinates, street address or free text a user typed (titles, descriptions,
notes, bios) in a log line, a span attribute or an error message that gets logged - city and country alone are
fine. Loki and Jaeger cannot erase one person's data, so whatever slips through only goes when its retention runs
out: Loki keeps logs 14 days (`retention_period`, `observability/loki/loki-config.yaml`), Jaeger traces 7 days
(`BADGER_SPAN_STORE_TTL`, `docker-compose.yaml`). Both must stay shorter than identity's `ACCOUNT_ERASURE_GRACE`
(30 days), so nothing outlives an account's erasure (GDPR Art. 17).

## Query strings

The access log (`query_params`) and the span's `http.target` show a parameter's value only if its name is in
`loggedQueryKeys` (`shared/middleware/query_redact.go`); every other parameter keeps its name and shows
`[redacted]`. A new endpoint's parameters are therefore redacted by default (tokens, codes, coordinates, street,
date of birth, free-text search...) - add a key there only if its value is safe to store in Loki/Jaeger.

## Tracing a domain event

With `LOG_LEVEL=debug`, every hop logs a line carrying the event's `event_id`, so one query shows its whole path -
in Grafana's Loki: `{service=~".+"} | json | event_id="<id>"` (promtail only sees containers; for a service run
from the IDE, grep its console). In order:

| Line (`msg`) | Where | What it adds |
|---|---|---|
| `event stored in outbox` | producing service (`shared/domainevent`) | `request_id` of the request that raised it; may still roll back |
| `outbox event published` / `published message` | outbox-publisher | NATS stream sequence |
| `event received` → `message acked` | each JetStream consumer (`shared/events`) | `consumer`, `num_delivered` (>1 = redelivery) |
| `notification created` / `duplicate event…` | notifications | `type`, `recipient_account_id` |
| `broadcast event received` | every activity instance | |
| `session event delivered` / `…no viewers on this instance` | activity live hub | `viewers`, `queued`, `skipped` (client already had that version), `withheld` (manager-only), `slow_consumer` |
| `team-formation stream update sent` | activity SSE stream | one per viewer: `account_id`, `version` |

Set the id with `api.WithEventID(ctx, …)` rather than passing `"event_id"` as a key, so lines logged further down
the call chain carry it too.

## Alerts

Prometheus alert rules live in `observability/alerts/rekreativko.yml`; add one there with any metric that
signals something a person must act on.
