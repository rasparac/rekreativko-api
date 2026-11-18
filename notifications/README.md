# Notifications Service

One bounded context with two layers (name is a bit narrow for what it does — see [Known issues](#known-issues)):

1. **In-app feed** — list/mark-read for notifications generated from activity-domain events (member joins, invites, attendee approvals/removals).
2. **Message delivery** — the actual email/SMS for identity account-lifecycle events: verification codes, password reset codes, the welcome email and password-changed alerts. Moved here from `gateway`, since gateway had no business owning delivery-channel concerns.

## How it works

The two layers share a container and the code layout, nothing else:

```
                          NATS
identity events ──────────────────────┐
 (code created, verified,             ▼
  password changed)           ┌───────────────────────────────────┐
                              │ notifications container           │
                              │                                   │
                              │ ① Delivery layer                  │──► email (Resend) / SMS
                              │    interfaces/events/identity_*   │    (dev mode without a key:
                              │    application/DeliveryService    │     only logged; no SMS yet)
                              │    infrastructure/{delivery,      │
                              │      templates}                   │
activity events ─────────────►│ ② In-app feed                     │
 (invites, join requests,     │    application/NotificationService│──► notifications.notification
  draft, voting…)             │    infrastructure/persistence     │    table
                              └───────────────────────────────────┘
                                          ▲
           mobile app ── GET /notifications, POST /notifications/{id}/read
```

```
notifications/internal/
  domain/          Notification (feed item); Message (kind, channel, recipient, locale, data)
  application/     NotificationService (feed); DeliveryService: render -> send, with metrics
  infrastructure/  persistence (feed repo); delivery (email/SMS senders); templates/<locale>/ (embedded)
  interfaces/      http (feed); events: activity events -> feed, identity events -> delivery
```

| | ① Message delivery | ② In-app feed |
|---|---|---|
| Purpose | Reach the user **outside** the app | A list the app shows **inside** the app |
| Triggered by | identity events | activity events |
| Code | `DeliveryService`, `IdentityHandlers`, `infrastructure/delivery`, `infrastructure/templates` | `NotificationService`, `infrastructure/persistence` |
| State | none | `notifications.notification` table |
| Text | `infrastructure/templates/<locale>/<kind>.<channel>.tmpl` (English only for now) | **not** written here: a row stores `type` + `data`, the app turns it into a sentence |
| API | none | `GET /notifications`, `POST /notifications/{id}/read` |

- **Isolation:** every topic has its own durable JetStream consumer, so a failing email send is retried with a delay on its own consumer (and dead-lettered after the max deliveries) and never blocks the feed.
- **No push yet:** a feed item reaches the app only when the app next fetches `GET /notifications`. Live updates on an open team screen come from activity's SSE stream, not from here.
- **User settings aren't consulted:** every message sent today must go out whatever account-profile's `notification.email.enabled`/`sms.enabled` say (see [Which messages respect the user's settings](#which-messages-respect-the-users-settings)), and `preference.language` isn't read either - everything is in English.

**Direction** (decided 2026-10-14, ticket `rekreativko-api-me3`, done): no split into separate services. Push (`rekreativko-api-79p`), localized emails (`rekreativko-api-nvr.4`) and the user's channel settings (`rekreativko-api-79p.3`) build on the delivery layer. Splitting it out later, if volume ever needs separate scaling, means copying that layer out.

### Message templates

One file per message and channel: `internal/infrastructure/templates/<locale>/<kind>.<channel>.tmpl`, for example `en/verification.email.tmpl`. It defines `body` and, for email, `subject`, and reads its values from `.` (`{{.code}}`). They are embedded in the binary and parsed at startup, so a broken template stops the service from starting. A missing value fails the render (`missingkey=error`) instead of sending a blank. `Message.Locale` picks the directory; an empty or unknown locale uses `en`. Adding a language is adding a directory (`rekreativko-api-nvr.4` wires the locale through).

| Kind (metric `type`) | Email | SMS | User's channel setting |
|---|---|---|---|
| `verification` | yes (`code`) | yes (`code`) | always sent |
| `welcome_message` | yes | — | always sent |
| `password_changed` | yes | yes | always sent |
| `password_reset` | yes (`code`) | yes (`code`) | always sent |

### Which messages respect the user's settings

account-profile has `notification.email.enabled`, `notification.sms.enabled` and `notification.push.enabled`. Each `MessageKind` declares whether they apply to it (`domain.settingsPolicies`, read with `kind.SettingsPolicy()`; a unit test fails for a template whose kind isn't listed):

- **`AlwaysSent`** - security and account messages a user can't opt out of: verification codes, password changed, anything they asked for or must know about their account. Welcome is here too: it goes out once, the moment the address is verified, before the user could have turned email off.
- **`RespectsSettings`** - everything else: activity updates, reminders, announcements, push. Skipped when the user turned that channel off.

Every kind is `AlwaysSent` today, so the settings aren't read. The first `RespectsSettings` kind has to bring them here: account-profile publishes a settings-changed event, this service keeps its own copy per account (covered by its erasure) and `DeliveryService` checks it. That is `rekreativko-api-79p.3`, built together with the stored language (`rekreativko-api-nvr.5`), since both need the same per-account copy.

## Running it

Part of `docker compose up` (container `notifications`, port `8083` internally, no host port published — reachable only via the gateway or docker network).

Outside dev mode it needs `RESEND_API_KEY` and `EMAIL_FROM` to start (see [Senders](#senders)). No SMS provider is implemented yet.

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

**Identity account-lifecycle** (delivered by `IdentityHandlers` through `DeliveryService`, not the in-app feed):

| Topic | Action |
|---|---|
| `identity.verification_code.created` | send the actual code (email, or SMS for `phone`) |
| `identity.account.verified` | send welcome email (unless `WELCOME_EMAIL_ENABLED=false`) — email accounts only; a phone account sends nothing, and neither does an event with `unconfirmed: true` (identity's `ACCOUNT_VERIFICATION_REQUIRED=false`: the address was never proven) |
| `identity.account.password.changed` | send the password-changed alert (email, or SMS for `phone`) - after `change-password` or `reset-password` |
| `identity.password_reset.requested` | send the forgot-password code (`password_reset`, email or SMS by `code_type`; same payload and handling as a verification code) |

**Identity account deletion and erasure** (`AccountLifecycleHandlers`, see [Account deletion and erasure](#account-deletion-and-erasure)):

| Topic | Action |
|---|---|
| `identity.account.deleted` | record the account (aggregate id) in `deleted_account`: no email/SMS and no feed notification for it any more; its feed stays |
| `identity.account.erased` | delete its feed, record it in `erased_account` and raise `notifications.account.erased` |

Each delivery handler picks the channel from the event's `code_type` (`DeliveryTypeEmail` / `DeliveryTypePhone`). Anything else is acked but dropped loudly: an error log (`dropping event with unknown delivery type`), `NotificationSendFailures{channel="unknown", reason="unknown_delivery_type"}`, and an error on the span - retrying can't fix a bad payload. There is deliberately **no lockout email/SMS** (`identity.account.locked` is not subscribed); whether to alert users about a lockout is a separate decision.

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
| `activity.session.draft.completed` | `team_draft_completed` | every drafted player |
| `activity.session.draft.cancelled` | `team_draft_cancelled` | people going (not when the session ended) |
| `activity.session.voting.opened` | `team_voting_opened` | people going, except the author |
| `activity.session.voting.closed` | `team_voting_closed` | people going |
| `activity.session.voting.cancelled` | `team_voting_cancelled` | people going except who cancelled it, for `teams_replaced` (an organizer replaced the teams) and `no_proposals` (the last proposal was withdrawn) |
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

## Senders

Chosen in `notifications/cmd/api/main.go`:

| Email | When |
|---|---|
| Resend (`delivery.NewResendEmailSender`, given the SDK client's `Emails`) | `RESEND_API_KEY` is set, in any mode (dev too, to test real delivery); `EMAIL_FROM` is then required |
| `InMemoryEmailSender` (logs only) | dev mode without a key |
| none - startup fails | any other environment without a key |

SMS: `InMemorySMSSender` (logs only) in dev mode; outside it `DisabledSMSSender` (errors loudly if ever called) while `PHONE_REGISTRATION_ENABLED=false`, and startup fails with it on, since no SMS provider exists. Deliberate fail-fast: a nil sender would otherwise panic mid-flight on the first real event.

**Resend** (`infrastructure/delivery/resend_sender.go`) sends with the official Go SDK, `github.com/resend/resend-go/v4`, which returns typed errors (`*resend.ResendError` with the status and error name, `*resend.RateLimitError` for a 429). The rendered templates are sent as plain text; templates stay in this repo (not in Resend) so they are versioned, tested and localized here.
- **Idempotency:** each email carries `Idempotency-Key: notifications/<event_id>/<hashed recipient>`, so a redelivered event, or a retry after a lost response, sends it once (Resend keeps keys 24 hours).
- **Errors:** `429`, `409` (the same key still in flight), `5xx` and network errors are retried by redelivering the event; any other `4xx` (bad key, unverified sender domain, invalid address) is `events.Permanent` and goes straight to the DLQ. An error carries only the status and Resend's error `name` - never the recipient, the text, or Resend's message (the SDK's error text is that message, which may quote the address, so it is not wrapped). `Retry-After` is not honored; JetStream's redelivery backoff spaces the retries.
- **Logs:** `email sent` (debug) with Resend's email id, to look it up in the Resend dashboard.
- **Key:** a sending-only key restricted to the sending domain - never the full-access one.
- **Sender domain:** none verified yet, so `EMAIL_FROM` can only be `Rekreativko <onboarding@resend.dev>`, which Resend delivers only to the account owner's address (good for local testing). Production needs a domain verified in Resend (SPF, DKIM, DMARC) and `EMAIL_FROM=Rekreativko <no-reply@<that domain>>`.

## Configuration

| Env var | Notes |
|---|---|
| `RESEND_API_KEY` | Resend sending-only key. Empty: dev mode logs emails, any other environment refuses to start |
| `EMAIL_FROM` | Sender, required with `RESEND_API_KEY`: `Rekreativko <no-reply@<verified domain>>` (`Rekreativko <onboarding@resend.dev>` before a domain is verified) |
| `WELCOME_EMAIL_ENABLED` | Default `true`. `false` skips the welcome email (the event is acked, nothing is sent); verification, password reset and password-changed messages are unaffected. Useful to save Resend's monthly quota |
| `PHONE_REGISTRATION_ENABLED` | Default `false`. The **only** place in the codebase where this flag changes runtime behavior at startup (elsewhere it just gates registration routes): off, outside dev mode the SMS sender is `DisabledSMSSender`; on, startup fails (no SMS provider) |
| `GATEWAY_API_KEY` | Required, validated via `X-Gateway-Key` |
| `NATS_URL` | Required |
| `PORT` | `8083` in compose |
| `NOTIFICATIONS_RETENTION_READ` | Default `720h` (30 days) - read notifications older than this are deleted |
| `NOTIFICATIONS_RETENTION_ALL` | Default `2160h` (90 days) - every notification older than this is deleted |
| `NOTIFICATIONS_RETENTION_INTERVAL` | Default `1h` - how often the retention sweep runs |
| `POSTGRES_*`, `TELEMETRY_*`, `METRICS_ENABLED`, `LOG_LEVEL`/`LOGGER_FORMAT` | Shared config |

Not used by this service despite being in the shared config struct: `JWT_*`, `CORS_*`, `REDIS_*`, rate limiter, `OUTBOX_*`, `GATEWAY_API_KEYS` (plural — gateway-only). The outbox-publisher's `OUTBOX_SCHEMAS` must include `notifications`, or `notifications.account.erased` is never published and identity counts every erasure as incomplete.

## Database schema

The feed table:
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

Also in the schema: `deleted_account` and `erased_account` (`account_id` PK plus a timestamp, see below) and `event_outbox` (same shape and publisher wake-up trigger as the other services' outboxes).

## Account deletion and erasure

Identity's grace period is the only one: this service only follows its two events (`application.AccountLifecycleService`).

- **Deleted** (`identity.account.deleted`): the account goes into `deleted_account`. From then on `NotificationService.CreateNotification` creates nothing for it (every feed handler goes through it, so a fan-out just skips that recipient) and `IdentityHandlers` deliver no email or SMS to it - both check `deleted_account` and `erased_account`. Its existing feed stays: the account can no longer log in, and the erasure deletes it.
- **Erased** (`identity.account.erased`, at the end of the grace period): one transaction inserts the account into `erased_account` (`ON CONFLICT DO NOTHING` - if it was already there, this is a redelivery and nothing more happens), deletes every `notification` it received, and raises `notifications.account.erased` (`account_id` only) for identity's erasure tracking (`notifications` is in identity's `ACCOUNT_ERASURE_CONTEXTS`). A late activity event for it then creates nothing. Notifications of *other* accounts that name it in `data` (as the actor) stay: they hold only its id, which identifies nobody once identity has erased the account. `notifications_accounts_erased_total` counts erasures.

A bad payload is dead-lettered; any other failure is retried (both operations are idempotent).

## Known issues

- **No sending domain is verified in Resend yet**, so outside a local test no email reaches a real user: production needs the domain, its DNS records and `EMAIL_FROM` on it (see [Senders](#senders)).
- When both senders end up `nil` (non-dev without `RESEND_API_KEY` + `PHONE_REGISTRATION_ENABLED=true`), only the email error is ever surfaced (checked first) — the SMS error is equally true but never shown, which could confuse debugging.
- **Dev senders log the code, email and phone.** `InMemoryEmailSender`/`InMemorySMSSender` (dev mode only) log recipient and message body, which is the only way to read a verification code locally. Nothing else logs or traces a code, email or phone: the handlers' spans carry only `event.id`, `event.account_id` and `event.delivery_type` (`TestEventHandlers_SpansCarryNoCodeEmailOrPhone`), and identity's log lines no longer carry the code, email or phone (`rekreativko-api-7r1`).
- `notification.email.enabled`/`sms.enabled`/`push.enabled` (account-profile settings) aren't read: nothing sent today respects them (all `AlwaysSent`); the first message that does brings them here (`rekreativko-api-79p.3`).
- Cosmetic: `Notification`'s private field is misspelled `recipientAccontID` (missing a `u`) — the exported getter `RecipientAccountID()` is correct, so this doesn't affect callers.
- The service name doesn't really describe its identity-delivery responsibility anymore; renaming has been discussed (e.g. "feed" service) but deliberately deferred — see project history if picking this up.
