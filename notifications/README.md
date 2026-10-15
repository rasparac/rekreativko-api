# Notifications Service

Two responsibilities bundled into one service (name is a bit narrow for what it does — see [Known issues](#known-issues)):

1. **In-app notification feed** — list/mark-read for notifications generated from activity-domain events (member joins, invites, attendee approvals/removals).
2. **Identity account-lifecycle delivery** — sends the actual email/SMS for verification codes, account-locked alerts, and password-changed alerts. Moved here from `gateway`, since gateway had no business owning delivery-channel concerns.

## How it works today

The two jobs share a container and nothing else:

```
                          NATS
identity events ──────────────────────┐
 (code created, verified,             ▼
  locked, password changed)   ┌───────────────────────────────┐
                              │ notifications container       │
                              │                               │
                              │ ① Message delivery            │──► email / SMS
                              │    shared/notification/*      │    (dev mode: only logged,
                              │    no DB, no API              │     no real sender yet)
                              │                               │
activity events ─────────────►│ ② In-app feed                 │
 (invites, join requests,     │    notifications/internal/*   │──► notifications.notification
  draft, voting…)             │    1 table, 2 endpoints       │    table
                              └───────────────────────────────┘
                                          ▲
           mobile app ── GET /notifications, POST /notifications/{id}/read
```

| | ① Message delivery | ② In-app feed |
|---|---|---|
| Purpose | Reach the user **outside** the app | A list the app shows **inside** the app |
| Triggered by | identity events | activity events |
| Code | `shared/notification` (only imported here, despite the `shared/` path) | `notifications/internal` (DDD layers) |
| State | none | `notifications.notification` table |
| Text | written here, hardcoded English (`shared/notification/service.go`) | **not** written here: a row stores `type` + `data`, the app turns it into a sentence |
| API | none | `GET /notifications`, `POST /notifications/{id}/read` |

- **Isolation:** every topic has its own durable JetStream consumer, so a failing email send is retried with a delay on its own consumer (and dead-lettered after the max deliveries) and never blocks the feed.
- **No push yet:** a feed item reaches the app only when the app next fetches `GET /notifications`. Live updates on an open team screen come from activity's SSE stream, not from here.
- **User settings aren't consulted:** nothing reads account-profile's `notification.email.enabled` or `preference.language`; every message goes out, in English.

**Planned direction** (decided 2026-10-14, ticket `rekreativko-api-me3`): no split into separate services — one bounded context with a *feed* layer and a *delivery* layer (channels: email, SMS, later push), with `shared/notification` moved into `notifications/internal` and message text moved into per-language templates. Push (`rekreativko-api-79p`), localized emails (`rekreativko-api-nvr.4`) and the email setting (`rekreativko-api-22a`) build on it.

## Running it

Part of `docker compose up` (container `notifications`, port `8083` internally, no host port published — reachable only via the gateway or docker network).

**This service cannot start outside dev mode today** — see [Known issues](#known-issues). No real SMTP/SMS provider is implemented yet.

## API

| Method | Path | Notes |
|---|---|---|
| GET | `/api/v1/notifications` | The authenticated user's whole feed, one unpaginated list |
| POST | `/api/v1/notifications/{id}/read` | Mark one notification read (must be its own recipient) |

`GET` query param: `unread_only` (bool). Response: `{ items: [...], unread_count }`, newest first; each item is `{ id, type, data, read_at, created_at }`. Not paginated: the list holds at most the 100 newest (`MaxListedNotifications`; hitting it logs a warning), and `unread_count` always counts every unread notification, listed or not. `limit` and `page_token` are ignored.

### Retention

What keeps the feed short enough to list in one go. An in-process ticker (`NotificationService.RunRetention`, started in `main.go`) deletes, right at startup and then every `NOTIFICATIONS_RETENTION_INTERVAL`:

- **read** notifications created more than `NOTIFICATIONS_RETENTION_READ` (30 days) ago,
- **every** notification created more than `NOTIFICATIONS_RETENTION_ALL` (90 days) ago, read or not.

The `DELETE` is idempotent, so every instance runs it and no locking is needed. A failed run is logged and retried on the next tick. The service refuses to start if the windows are not positive or `ALL` is shorter than `READ`.

There is **no `POST /notifications`** — creation only happens internally, via the event subscriber below. Auth: gateway requires a bearer token for all of `/notifications/api/v1/*`; this service additionally validates `X-Gateway-Key`.

## Event subscriptions

Each subscription creates its own durable JetStream consumer (`<service>+<subject>`), independent of any other service subscribed to the same topic.

**Identity account-lifecycle** (delivered to `shared/notification.Service`, not the in-app feed):

| Topic | Action |
|---|---|
| `identity.verification_code.created` | send the actual code (email, or SMS for `phone`) |
| `identity.account.verified` | send welcome email — email accounts only; a phone account sends nothing |
| `identity.account.locked` | send lockout alert — **never delivered today**: identity's event has no contact fields or delivery type, so the handler drops it (`rekreativko-api-yn0`) |
| `identity.account.password.changed` | send alert — **dead today**, identity has no route that changes passwords yet |

Each handler picks the channel from the event's `code_type` (`email` / `phone`); an unknown or missing value is dropped without an error (see [Known issues](#known-issues)).

**Activity-domain** (create an in-app `Notification` row via `CreateNotification`):

| Topic | `NotificationType` |
|---|---|
| `activity.member.join_requested` | `join_request_created` (fan-out to group managers) |
| `activity.member.approved` | `join_request_approved` |
| `activity.member.rejected` | `join_request_rejected` |
| `activity.session.attendee.promoted` | `attendee_promoted` |
| `activity.session.attendee.join_requested` | `session_join_request_created` (fan-out to session managers) |
| `activity.session.attendee.join_approved` | `session_join_request_approved` |
| `activity.session.attendee.join_rejected` | `session_join_request_rejected` |
| `activity.session.attendee.removed` | `session_attendee_removed` |
| `activity.invite.sent` | `invite_sent` |
| `activity.invite.accepted` | `invite_accepted` |
| `activity.invite.declined` | `invite_declined` |
| `activity.invite.expired` | `invite_expired` |
| `activity.session_invite.sent` | `session_invite_sent` (to the invitee) |
| `activity.session_invite.accepted` | `session_invite_accepted` (to the inviter) |
| `activity.session_invite.declined` | `session_invite_declined` (to the inviter) |
| `activity.session_invite.expired` | `session_invite_expired` (to the inviter) |

**Team formation** (in-app notifications; `data` always carries `session_id` and a deep-link `screen` of `team_draft`, `team_voting` or `session_teams`; whoever triggered the event is skipped; recipients come from the event payload, activity is never queried):

| Topic | `NotificationType` | Recipients |
|---|---|---|
| `activity.session.draft.started` | `team_draft_captain_selected` | both captains |
| `activity.session.draft.turn_changed` | `team_draft_your_turn` | the captain whose turn it is |
| `activity.session.draft.paused` | `team_draft_paused` | the organizer who started the draft |
| `activity.session.draft.completed` | `team_draft_completed` | every drafted player |
| `activity.session.draft.cancelled` | `team_draft_cancelled` | people going (not when the session ended) |
| `activity.session.voting.opened` | `team_voting_opened` | people going, except the author |
| `activity.session.voting.closed` | `team_voting_closed` | people going |
| `activity.session.voting.cancelled` | `team_voting_cancelled` | people going, only for `not_enough_players` |
| `activity.session.voting.tied` | `team_voting_tied` | session managers except the one who pressed Close |
| `activity.session.attendee.team_changed` | `team_changed` | the moved attendee (manual moves only) |
| `activity.session.teams_reset` | `team_formation_reset` | everyone still going except who caused it (a leaver, or the organizer who deleted the teams). The draft/voting cancellations it causes (`roster_changed`, `teams_reset`) are not notified separately. |

These are stored in the feed and read with `GET /notifications`. They are **not** pushed: there is no push delivery yet, so the app only sees them when it next fetches the feed. Live updates while a team screen is open come from activity's SSE stream, a separate channel (see [activity README](../activity/README.md#team-formation-and-live-updates)).

## Domain model

```
Notification
  id, recipientAccountID, notificationType (open string, not a DB enum)
  data map[string]any       // free-form payload, not modeled per-type
  readAt *time.Time         // nil = unread
  createdAt time.Time
```
`MarkRead()` is idempotent — no-ops if already read.

## The dual sender setup

`notifications/cmd/api/main.go`:
```go
if cfg.IsDevMode() {
    emailSender = notification.NewInMemoryEmailSender(log)  // logs only
    smsSender = notification.NewInMemorySMSSender(log)       // logs only
} else if !cfg.Features.PhoneRegistrationEnabled {
    smsSender = notification.NewDisabledSMSSender(log)        // errors loudly if ever called
}
if emailSender == nil {
    return fmt.Errorf("no email sender configured - required outside dev mode, and no real SMTP sender is implemented yet")
}
if smsSender == nil {
    return fmt.Errorf("no SMS sender configured - required outside dev mode when phone registration is enabled, and no real SMS provider is implemented yet")
}
```
Deliberate fail-fast: a nil sender would otherwise panic mid-flight on the first real event, instead of failing loudly at startup. `DisabledSMSSender` exists so a *deployment* choosing not to support phone registration doesn't need a real SMS provider just to boot — it only errors if the (supposedly unreachable) SMS path is actually hit.

`SMTPEmailSender` exists in `shared/notification/` but is a non-functional stub (`SendEmail` just `return nil`s) — it's never constructed by `main.go`. No real SMS provider implementation exists at all.

## Configuration

| Env var | Notes |
|---|---|
| `PHONE_REGISTRATION_ENABLED` | Default `false`. The **only** place in the codebase where this flag changes runtime behavior at startup (elsewhere it just gates registration routes) — see [Known issues](#known-issues) for why it doesn't actually save you outside dev mode |
| `GATEWAY_API_KEY` | Required, validated via `X-Gateway-Key` |
| `NATS_URL` | Required |
| `PORT` | `8083` in compose |
| `NOTIFICATIONS_RETENTION_READ` | Default `720h` (30 days) - read notifications older than this are deleted |
| `NOTIFICATIONS_RETENTION_ALL` | Default `2160h` (90 days) - every notification older than this is deleted |
| `NOTIFICATIONS_RETENTION_INTERVAL` | Default `1h` - how often the retention sweep runs |
| `POSTGRES_*`, `TELEMETRY_*`, `METRICS_ENABLED`, `LOG_LEVEL`/`LOGGER_FORMAT` | Shared config |

Not used by this service despite being in the shared config struct: `JWT_*`, `CORS_*`, `REDIS_*`, rate limiter, `OUTBOX_*`, `GATEWAY_API_KEYS` (plural — gateway-only).

## Database schema

One table, one schema:
```sql
CREATE TABLE notifications.notification(
    id uuid PRIMARY KEY DEFAULT uuid_generate_v4(),
    recipient_account_id uuid NOT NULL,   -- opaque FK to identity, no cross-schema constraint
    type varchar(100) NOT NULL,
    data jsonb NOT NULL DEFAULT '{}',
    read_at timestamptz DEFAULT NULL,
    created_at timestamptz NOT NULL DEFAULT NOW()
);
```
Three indexes: `(recipient_account_id, created_at DESC, id DESC)` for the feed (newest first), a partial index on `recipient_account_id WHERE read_at IS NULL` for the unread-count query, and `(created_at)` for the retention sweep across all recipients.

## Known issues

- **This service cannot start outside dev mode, regardless of `PHONE_REGISTRATION_ENABLED`.** The sender-construction `if`/`else if` never assigns `emailSender` in the non-dev branch at all — only `smsSender` gets a fallback there. So in any non-dev environment, `emailSender` stays `nil` and startup fails on "no email sender configured," whether or not phone registration is on. This is intentional (no real SMTP sender exists yet) but means **a real `EmailSender` implementation is a hard prerequisite for any non-dev deployment**, not just an eventual nice-to-have.
- When both senders end up `nil` (non-dev + `PHONE_REGISTRATION_ENABLED=true`), only the email error is ever surfaced (checked first) — the SMS error is equally true but never shown, which could confuse debugging.
- **Verification codes and contact details end up in traces** (`rekreativko-api-7r1`, P1): the identity-event handlers set `event.code`, `event.email` and `event.phone` as span attributes, so Jaeger stores a working verification code next to the user's email/phone.
- **Account-locked alerts are never sent** (`rekreativko-api-yn0`): identity's `AccountLockedEvent` carries only `Reason` and `LockedUntil` — no `code_type`, email, phone or account id — so `HandleAccountLocked` always takes the unknown-delivery-type branch.
- **SMS alerts can't be sent** (`rekreativko-api-yn0`): the locked and password-changed handlers match the delivery type `"sms"`, but identity's values are `"email"` and `"phone"`.
- The 4 identity-event handlers (`HandleAccountVerified` et al., in `shared/notification/event_handlers.go`) silently no-op on an unrecognized `DeliveryType` instead of erroring — which is why the two bugs above went unnoticed, and a future channel (e.g. push) would fail the same way.
- `identity.account.password.changed` is subscribed, but nothing publishes it — identity has no route wired to `Account.ChangePassword()` yet (see `identity/README.md`).
- `notification.email.enabled` (account-profile setting) is ignored (`rekreativko-api-22a`).
- Cosmetic: `Notification`'s private field is misspelled `recipientAccontID` (missing a `u`) — the exported getter `RecipientAccountID()` is correct, so this doesn't affect callers.
- The service name doesn't really describe its identity-delivery responsibility anymore; renaming has been discussed (e.g. "feed" service) but deliberately deferred — see project history if picking this up.
