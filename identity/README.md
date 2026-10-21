# Identity Service

Authentication and account management for Rekreativko: registration, login, email/phone verification, JWT issuance, and refresh-token rotation.

Does **not** handle: user profiles (nickname, city, avatar — see `account-profile`), user preferences, or authorization/role rules beyond issuing a JWT.

## Running it

Runs as part of the monorepo's `docker compose up` (container `identity`, port `8081` internally, reachable only through the gateway or the docker network — no host port is published). For local debugging outside Docker, see the root `Taskfile.yml` / `.vscode/launch.json` (`SERVICE_NAME=identity`).

All routes are served under `/api/v1/...` by the service itself. The gateway mounts it at `/identity` and strips the prefix, so externally every path below is `/identity/api/v1/...`.

## Configuration

Beyond the shared config (`shared/config/config.go`), this service reads:

| Env var | Default | Notes |
|---|---|---|
| `JWT_SECRET` | — (required) | Signing key for access + refresh tokens |
| `JWT_ACCESS_TOKEN_DURATION` | `15m` | |
| `JWT_REFRESH_TOKEN_DURATION` | `360h` (15 days) | |
| `GATEWAY_API_KEY` | — (required) | Validated by `middleware.CheckGatewayKey` |
| `ACCOUNT_VERIFICATION_REQUIRED` | `true` | Whether a new account must confirm its email/phone with a code before it can log in. `false` activates the account at `POST /register` (no code, no `verification_code.created` event); the client logs in right after. The verified event carries `unconfirmed: true`, so account-profile still creates the profile but notifications sends no welcome email to an address nobody proved. Only affects registrations made while it is off: those accounts stay unverified when it is turned back on. |
| `PHONE_REGISTRATION_ENABLED` | `false` | Feature flag. While off, `POST /register` rejects any request with a non-empty `phone_number` (`400 phone_registration_disabled`). Only gates registration — login and resend-verification-code still accept a phone identifier if an account already has one. |
| `PORT` | `8080` (compose overrides to `8081`) | |
| `OUTBOX_SCHEMAS` | — (required, shared) | Must include `identity` for the outbox publisher to pick up this service's events |
| `NATS_URL` | `nats://localhost:4222` (shared) | Required since the service consumes the erasure confirmations; it does not start without NATS |
| `ACCOUNT_ERASURE_GRACE` | `720h` (30 days) | How long a deleted account is kept before it is erased, see [Account erasure](#account-erasure). `0` erases it on the next run; negative refuses to start |
| `ACCOUNT_ERASURE_INTERVAL` | `1h` | How often the erase job runs (and once at start-up); `0` turns it off - deleted accounts are then never erased |
| `ACCOUNT_ERASURE_BATCH` | `100` | Accounts erased per transaction |
| `ACCOUNT_ERASURE_CONTEXTS` | `account_profile,activity,notifications` | Contexts that must confirm each erasure with `<context>.account.erased` |
| `ACCOUNT_ERASURE_INCOMPLETE_AFTER` | `168h` (7 days) | An erasure some context has not confirmed by then is counted in `identity_account_erasures_incomplete` |

Verification codes are 6 digits; bcrypt cost is 10. Both are hardcoded, not configurable.

## API

| Method | Path | Auth | Notes |
|---|---|---|---|
| POST | `/register` | public | |
| POST | `/login` | public | |
| POST | `/verify-account` | public | |
| POST | `/resend-verification-code` | public | |
| POST | `/refresh-token` | public | |
| GET | `/me` | bearer | |
| DELETE | `/me` | bearer | |
| POST | `/logout` | bearer | |

Public/auth split is enforced at the gateway (`gateway/internal/router.go`), not by this service itself.

### Register — `POST /register`

```json
{ "email": "user@example.com", "phone_number": "+381601234567", "password": "Str0ng!Pass" }
```
Email or phone is required (not both); phone requires `PHONE_REGISTRATION_ENABLED=true`. Password: min 8 chars at the DTO layer, then full complexity checked in the service layer (`identity_weak_password` on failure) — at least one uppercase, one lowercase, one digit, one of `` !@#$%^&*()-_=+[]{}|;:',.<>?/ ``.

→ `201` `{ "account_id": "<uuid>" }`. With `ACCOUNT_VERIFICATION_REQUIRED=false` the account is created `active` and nothing below applies. Otherwise the account starts `pending`; a verification code is generated and an `identity.verification_code.created` event fires (consumed by `notifications` to send it out).

### Verify account — `POST /verify-account`

```json
{ "code": "123456" }
```
Codes are unique **globally**, not per-account — the code alone identifies which account to activate. → `200`, account becomes `active`, fires `identity.account.verified` (+ `identity.account.activated`).

### Resend verification code — `POST /resend-verification-code`

```json
{ "type": "email", "identity": "user@example.com" }
```
`type` is `email` or `phone`.

### Login — `POST /login`

```json
{ "email": "user@example.com", "password": "Str0ng!Pass" }
```
(or `phone_number` instead of `email`). Blocked for unverified/suspended/deleted/locked accounts.

→ `200` `{ "access_token", "refresh_token", "expires_in", "token_type": "Bearer" }`.

**Failed-login lockout** (`failedLoginLockTiers` in `domain/account.go`): the 5th consecutive failed login locks the account for 15 minutes, the 10th for 1 hour, the 15th and every one after for 24 hours - the longest a lock gets; nothing locks an account for good. When a lock runs out the count is kept, so the next wrong password locks again for the tier of the new count; only a successful login clears it. The count stops at 15. Every wrong password is saved in its own committed transaction (`Service.recordFailedLogin`, account row locked `FOR UPDATE`, so parallel attempts all count and a request that races an existing lock does not escalate it); the login itself still fails with `identity_invalid_credentials`. A locked account gets `403 identity_account_locked` ("Account is locked until <time>") - even with the right password. There is no lockout email/SMS and no way to unlock early yet.

### Refresh token — `POST /refresh-token`

```json
{ "refresh_token": "<token>" }
```
Single-use rotation: the old refresh token is looked up by hash, validated (not revoked/expired), revoked, and a brand-new access+refresh pair is issued in the same transaction. No reuse-detection beyond that — an already-used token is just rejected as revoked.

### Get current account — `GET /me`

→ `200` `{ "account_id", "email", "phone_number", "status", "created_at", "updated_at" }`.

### Delete current account — `DELETE /me`

Soft-deletes the account (status `deleted`, can no longer log in), revokes **all** of its refresh tokens and schedules its erasure `ACCOUNT_ERASURE_GRACE` later (`erase_after`). Fires `identity.account.deleted`. The personal data is removed only when the grace period has passed, see [Account erasure](#account-erasure).

### Logout — `POST /logout`

```json
{ "refresh_token": "<token>" }
```
Revokes only the one token passed in — not "all sessions" (despite older Swagger wording). Use `DELETE /me` if you actually need to invalidate every session.

## Domain model

```
Account
  id, email *Email, phoneNumber *PhoneNumber, password *Password
  status AccountStatus  // pending | active | suspended | deleted
  failedLoginAttempts int, lockedUntil *time.Time
  createdAt, updatedAt, deletedAt *time.Time
  eraseAfter, erasedAt *time.Time   // set by Delete / Erase

RefreshToken
  id, accountID, tokenHash (sha256 hex, plaintext never persisted)
  expiresAt, createdAt, revokedAt *time.Time

VerificationCode
  id, accountID, code, codeType (email | phone)
  expiresAt, createdAt, usedAt *time.Time
```

No roles, name, avatar, or profile fields live here by design — that's `account-profile`'s job.

## Domain events

Published to `identity.event_outbox`, relayed to NATS by the outbox publisher. Subject pattern: `identity.<aggregate>.<event>`.

| Event | Fires on | Consumed by |
|---|---|---|
| `identity.account.registered` | registration | — |
| `identity.account.verified` | verify-account success, or registration when verification is switched off (`unconfirmed: true`) | `notifications` (sends welcome / unblocks feature access) |
| `identity.account.activated` | verify-account success (paired with `verified`) | — |
| `identity.account.suspended` | `Account.Suspend` | — |
| `identity.account.locked` | failed-login threshold hit | — (no consumer; no lockout email/SMS) |
| `identity.account.unlocked` | `Account.Unlock` | — |
| `identity.account.login.succeeded` | successful login | — |
| `identity.account.login.failed` | failed login attempt | — |
| `identity.account.password.changed` | `Account.ChangePassword` | `notifications` — **currently unreachable**, no route calls this (see below) |
| `identity.account.deleted` | `DELETE /me` | `notifications` (stops email/SMS and feed notifications), `activity` (cancels upcoming RSVPs, declines invites, leaves groups) |
| `identity.account.erased` | erase job, `ACCOUNT_ERASURE_GRACE` after the deletion. Payload: `account_id` only | `account_profile`, `notifications` and `activity` - every context in `ACCOUNT_ERASURE_CONTEXTS` (each erases its data and confirms with `<context>.account.erased`) |
| `identity.refresh_token.created` | any token issuance | — |
| `identity.refresh_token.revoked` | logout / rotation / account deletion | — |
| `identity.verification_code.created` | register / resend | `notifications` (sends the actual code via email/SMS) |
| `identity.verification_code.used` | verify-account success | — |

## Account erasure

GDPR Art. 17: a deleted account's personal data is erased, in every context, within a month. The grace period lives only here; every other context erases immediately on the event.

1. `DELETE /me` sets `deleted_at` and `erase_after = now + ACCOUNT_ERASURE_GRACE`. The data stays until then.
2. The erase job (`application.ErasureService`, every `ACCOUNT_ERASURE_INTERVAL`) takes accounts with `erase_after` passed and `erased_at` NULL, `ACCOUNT_ERASURE_BATCH` at a time with `FOR UPDATE SKIP LOCKED` (several instances are safe), and in one transaction per batch: NULLs `email` and `phone_number`, replaces `password` with `domain.ErasedPasswordHash` (`!erased` - not a bcrypt hash, nothing matches it), sets `erased_at`, deletes the account's `verification_codes` and `refresh_tokens`, and raises `identity.account.erased`. The account row and its id stay, so other contexts can show a "deleted user"; the email and phone are free for a new account.
3. Each context erases its own data on `identity.account.erased` and confirms with `<context>.account.erased` (payload `{"event_id", "account_id"}`). Identity records each confirmation in `identity.account_erasure(account_id, context, completed_at)`; a repeated one is a no-op, one for an unknown account is dead-lettered.
4. Each run also sets the gauge `identity_account_erasures_incomplete`: accounts erased more than `ACCOUNT_ERASURE_INCOMPLETE_AFTER` ago that some context in `ACCOUNT_ERASURE_CONTEXTS` has not confirmed. The `AccountErasureIncomplete` alert fires when it stays above 0 for an hour. Find the missing contexts with:
   ```sql
   SELECT a.id, a.erased_at, array_agg(e.context) AS confirmed
   FROM identity.accounts a LEFT JOIN identity.account_erasure e ON e.account_id = a.id
   WHERE a.erased_at IS NOT NULL GROUP BY a.id;
   ```

Metrics: `identity_accounts_erased_total`, `identity_account_erasures_incomplete`. Log lines: `erased deleted accounts` (Info), `account erasures not confirmed by every context` (Warn).

## Known issues

- **`expires_in` in the login response is hardcoded to `123`**, not derived from `JWT_ACCESS_TOKEN_DURATION`. The refresh-token endpoint's response hardcodes a different wrong value (`365*24*60*60` seconds). Neither reflects the real token lifetime.
- **`ChangePasswordRequest` DTO and `Account.ChangePassword()` domain method exist but no HTTP route calls them.** Password changes aren't actually possible through the API today, and the `password.changed` event/notification path is dead code until this ships.
- **Verification codes are unique across all accounts**, not per-account (`verification_codes_account_code_uq_idx` is a global unique index on `code` alone). Astronomically unlikely to collide in practice (6-digit code, 15-minute expiry), but worth knowing if you're ever debugging a "code already exists" insert failure.
