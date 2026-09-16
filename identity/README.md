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
| `PHONE_REGISTRATION_ENABLED` | `false` | Feature flag. While off, `POST /register` rejects any request with a non-empty `phone_number` (`400 phone_registration_disabled`). Only gates registration — login and resend-verification-code still accept a phone identifier if an account already has one. |
| `PORT` | `8080` (compose overrides to `8081`) | |
| `OUTBOX_SCHEMAS` | — (required, shared) | Must include `identity` for the outbox publisher to pick up this service's events |

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

→ `201` `{ "account_id": "<uuid>" }`. Account starts `pending`; a verification code is generated and an `identity.verification_code.created` event fires (consumed by `notifications` to send it out).

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

**Failed-login lockout**: 5 consecutive failures → locked 15 minutes; 10 → locked ~365 days (effectively permanent, needs manual intervention). A successful login resets the counter.

### Refresh token — `POST /refresh-token`

```json
{ "refresh_token": "<token>" }
```
Single-use rotation: the old refresh token is looked up by hash, validated (not revoked/expired), revoked, and a brand-new access+refresh pair is issued in the same transaction. No reuse-detection beyond that — an already-used token is just rejected as revoked.

### Get current account — `GET /me`

→ `200` `{ "account_id", "email", "phone_number", "status", "created_at", "updated_at" }`.

### Delete current account — `DELETE /me`

Soft-deletes the account and revokes **all** of its refresh tokens. Fires `identity.account.deleted`.

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
| `identity.account.verified` | verify-account success | `notifications` (sends welcome / unblocks feature access) |
| `identity.account.activated` | verify-account success (paired with `verified`) | — |
| `identity.account.suspended` | `Account.Suspend` | — |
| `identity.account.locked` | failed-login threshold hit | `notifications` (alerts the user) |
| `identity.account.unlocked` | `Account.Unlock` | — |
| `identity.account.login.succeeded` | successful login | — |
| `identity.account.login.failed` | failed login attempt | — |
| `identity.account.password.changed` | `Account.ChangePassword` | `notifications` — **currently unreachable**, no route calls this (see below) |
| `identity.account.deleted` | `DELETE /me` | — |
| `identity.refresh_token.created` | any token issuance | — |
| `identity.refresh_token.revoked` | logout / rotation / account deletion | — |
| `identity.verification_code.created` | register / resend | `notifications` (sends the actual code via email/SMS) |
| `identity.verification_code.used` | verify-account success | — |

## Known issues

- **`expires_in` in the login response is hardcoded to `123`**, not derived from `JWT_ACCESS_TOKEN_DURATION`. The refresh-token endpoint's response hardcodes a different wrong value (`365*24*60*60` seconds). Neither reflects the real token lifetime.
- **`ChangePasswordRequest` DTO and `Account.ChangePassword()` domain method exist but no HTTP route calls them.** Password changes aren't actually possible through the API today, and the `password.changed` event/notification path is dead code until this ships.
- **Verification codes are unique across all accounts**, not per-account (`verification_codes_account_code_uq_idx` is a global unique index on `code` alone). Astronomically unlikely to collide in practice (6-digit code, 15-minute expiry), but worth knowing if you're ever debugging a "code already exists" insert failure.
