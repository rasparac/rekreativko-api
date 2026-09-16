# Notifications Service

Two responsibilities bundled into one service (name is a bit narrow for what it does — see [Known issues](#known-issues)):

1. **In-app notification feed** — list/mark-read for notifications generated from activity-domain events (member joins, invites, attendee approvals/removals).
2. **Identity account-lifecycle delivery** — sends the actual email/SMS for verification codes, account-locked alerts, and password-changed alerts. Moved here from `gateway` this session, since gateway had no business owning delivery-channel concerns.

## Running it

Part of `docker compose up` (container `notifications`, port `8083` internally, no host port published — reachable only via the gateway or docker network).

**This service cannot start outside dev mode today** — see [Known issues](#known-issues). No real SMTP/SMS provider is implemented yet.

## API

| Method | Path | Notes |
|---|---|---|
| GET | `/api/v1/notifications` | Paginated feed for the authenticated user |
| POST | `/api/v1/notifications/{id}/read` | Mark one notification read (must be its own recipient) |

`GET` query params: `unread_only` (bool), `limit` (default 20), `page_token` (opaque cursor). Response: `{ items: [...], limit, next_page_token, unread_count }`; each item is `{ id, type, data, read_at, created_at }`.

There is **no `POST /notifications`** — creation only happens internally, via the event subscriber below. Auth: gateway requires a bearer token for all of `/notifications/api/v1/*`; this service additionally validates `X-Gateway-Key`.

## Event subscriptions

Each subscription creates its own durable JetStream consumer (`<service>+<subject>`), independent of any other service subscribed to the same topic.

**Identity account-lifecycle** (delivered to `shared/notification.Service`, not the in-app feed):

| Topic | Action |
|---|---|
| `identity.account.verified` | send confirmation (email/SMS depending on delivery type) |
| `identity.account.locked` | send lockout alert |
| `identity.account.password.changed` | send alert — **dead today**, identity has no route that changes passwords yet |
| `identity.verification_code.created` | send the actual code |

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
Two indexes: `(recipient_account_id, created_at DESC, id DESC)` for the paginated feed's keyset pagination, and a partial index on `recipient_account_id WHERE read_at IS NULL` for the unread-count query.

## Known issues

- **This service cannot start outside dev mode, regardless of `PHONE_REGISTRATION_ENABLED`.** The sender-construction `if`/`else if` never assigns `emailSender` in the non-dev branch at all — only `smsSender` gets a fallback there. So in any non-dev environment, `emailSender` stays `nil` and startup fails on "no email sender configured," whether or not phone registration is on. This is intentional (no real SMTP sender exists yet) but means **a real `EmailSender` implementation is a hard prerequisite for any non-dev deployment**, not just an eventual nice-to-have.
- When both senders end up `nil` (non-dev + `PHONE_REGISTRATION_ENABLED=true`), only the email error is ever surfaced (checked first) — the SMS error is equally true but never shown, which could confuse debugging.
- `identity.account.password.changed` is subscribed and handled correctly here, but nothing publishes it — identity has no route wired to `Account.ChangePassword()` yet (see `identity/README.md`).
- The 4 identity-event handlers (`HandleAccountVerified` et al., in `shared/notification/event_handlers.go`) silently no-op on an unrecognized `DeliveryType` instead of erroring — a future third delivery channel (e.g. push) would need this updated or it'll fail silently.
- Cosmetic: `Notification`'s private field is misspelled `recipientAccontID` (missing a `u`) — the exported getter `RecipientAccountID()` is correct, so this doesn't affect callers.
- The service name doesn't really describe its identity-delivery responsibility anymore; renaming has been discussed (e.g. "feed" service) but deliberately deferred — see project history if picking this up.
