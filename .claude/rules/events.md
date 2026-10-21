---
paths:
  - "**/interfaces/events/**"
  - "**/domain/*event*.go"
  - "shared/events/**"
  - "shared/domainevent/**"
  - "outbox-publisher/**"
---

# Domain events

- **Type** is `{context}.{aggregate}.{event}` (`activity.session.cancelled`, `identity.account.erased`). The payload
  embeds `domainevent.BaseEvent` (`event_id`, `event_type`, `occurred_at`, `aggregate_id`) plus the fields
  consumers need - as little personal data as possible; an id is usually enough.
- **Raising**: the aggregate adds the event; the application service writes it with `InsertEvents` in the same
  transaction as the state change (transactional outbox), then `ClearEvents()`. The outbox-publisher relays it
  to the NATS stream `EVENTS` on subject `events.<type>` (kept 7 days); the broker adds and strips the
  `events.` prefix, so code always uses the bare type. Dead-lettered messages go to the `DLQ` stream (14 days).
- **Consuming**: `broker.Subscribe(ctx, topic, handler)` creates a durable JetStream consumer named after the
  service and subject, shared by all instances (each event handled once per service). `SubscribeBroadcast` is
  for work every instance must do (activity's live hub). Subscriptions are listed in the service's
  `interfaces/events/subscriber.go`.
- **Handlers** decode into a local struct - never import another context's domain package; only the JSON shape
  is shared. First thing: `ctx = api.WithEventID(ctx, event.EventID.String())`.
- **At-least-once delivery**: every handler is idempotent (unique keys, `ON CONFLICT DO NOTHING`, a state table
  checked first). A redelivery must not create a second row or raise a second event.
- **Errors**: a payload that can never succeed (bad JSON, missing id, unknown referenced row) returns
  `events.Permanent(err)` and goes to the dead-letter stream. Any other error is returned as is and retried.
- Tracing one event through every hop: `.claude/rules/observability.md`.
