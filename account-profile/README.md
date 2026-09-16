# Account Profile Service

User profiles (name, nickname, bio, location, activity interests) and per-account settings. Runs on port `8082`. Does **not** own authentication (identity) or activity/session data.

## Running it

Part of `docker compose up` (container `account-profile`). Every route requires a bearer token at the gateway; the gateway restricts this prefix to `GET`/`POST`/`PUT` only (no `DELETE`, no `PATCH`) — matching the fact that none of this service's current routes use those methods. No route has a `RequiredRoles` restriction: any authenticated user can call any endpoint here, including bulk profile search.

Unlike `identity`/`activity`, this service has **no `validate:` tags anywhere** — all request validation happens by hand in the domain layer (value objects) and application layer, not via a validator library.

## API

| Method | Path | Notes |
|---|---|---|
| GET | `/api/v1/profiles` | Search/list profiles by query filter — see [Known issues](#known-issues) re: privacy |
| GET | `/api/v1/profiles/{id}` | Get a single profile by account ID |
| GET | `/api/v1/my/profile` | Current user's profile |
| PUT | `/api/v1/my/profile` | Update current user's profile |
| GET | `/api/v1/my/settings` | Current user's settings |
| PUT | `/api/v1/my/settings` | Update current user's settings |

`GET /profiles` requires at least one query filter (`account_id`, `nickname`, `dob_gt`, `dob_lt`, `country`, `city`) or it 400s. No `/interests` or `/statistics` endpoints exist — interests are only editable as a full-replace array field on `PUT /my/profile`; statistics have no HTTP surface at all (dead feature, see below).

### Profile fields

`full_name`, `nickname` (3–50 chars, `^[a-zA-Z0-9_-]+$`, **not actually unique** despite what you might expect — no DB constraint or service check), `bio` (max 500 chars), `date_of_birth` (must be ≥13 years ago, not in the future), `location_city`/`location_country` (required together), `location_latitude`/`location_longitude` (optional, only set as a pair), `profile_picture_url` (must be http/https, ≤2048 chars), `activity_interests` (array of `{name, level}` — full replace on every update, one entry per activity type, deduplicated against a shared 16-type/3-level catalog also used by the `activity` service).

### Settings

Free-form key/value store (`GET`/`PUT /my/settings`), namespaced dotted keys (`notification.email.enabled`, `preference.language`, etc.), each with a type (`string`/`bool`/`int`/`float`). 11 keys are seeded with defaults when an account verifies. See [Known issues](#known-issues) — the seeded defaults and the registry that validates/renders settings currently disagree on several key names and one value's type.

## Domain events

**Consumed**: exactly one — `identity.account.verified` → creates a blank `AccountProfile` row and a default `AccountProfileSettings` row (11 seeded keys) in one transaction. This is the only place a profile gets created; there's no manual "create profile" endpoint.

**Published** (to `account_profile.event_outbox`): `account_profile.created/deleted/anonymized`, `.location.changed`, `.profile_picture.changed`, `.activity_interest.added/removed/level.changed`, `.account_settings_bulk_updated`. Several other defined event types are dead — see below.

## Configuration & schema

`PORT=8082`, `SERVICE_NAME=account_profile`. No `JWT_SECRET` dependency — this service trusts the `X-User-Id` header the gateway sets after validating the JWT itself, gated only by the shared `X-Gateway-Key`.

Schema (`account_profile` Postgres schema): `profiles` (PK is `account_id`, i.e. identity's account ID — no separate profile ID), `activity_interests` (unique per account+type), `settings` (composite PK `account_id, key`), `profile_statistics` (dead, see below), `account_settings_meta` (optimistic-lock version counter), `event_outbox`.

## Known issues

This service had the most drift from its original design doc of anything documented so far — the list is long, but most items are low-severity/dead-code.

**Broken, will error or fail silently on use:**
- **`DeleteAccountProfile`'s SQL is malformed**: targets `account_profile` (the schema, not a table — should be `account_profile.profiles`), is missing a comma between its two `SET` assignments, and filters on a non-existent `id` column (the real PK is `account_id`). Currently unreachable anyway — there's no HTTP route for it (gateway doesn't even allow `DELETE` on this prefix) — so soft-delete/anonymize exists in the domain layer but doesn't actually work end-to-end today.
- **Two settings keys are unreachable once written.** The account-creation defaults (`GetDefaultSettings()`) seed `privacy.location.enabled` and `privacy.activity.public`, but the settings *registry* (which both renders `GET /my/settings` and validates `PUT /my/settings`) only knows `privacy.show_location`/`privacy.show_activities`/`privacy.show_statistics`. Every new account ends up with 2 settings rows the API can never show and never let you update.
- **`activity.search_radius` changes type on first update**: seeded as a float (`"10.00"`) but the registry declares it an int — updating any setting coerces this one from float to int silently.
- **Version-conflict and invalid-`sort_by` errors return `500` instead of `409`/`400`** — both are raw `fmt.Errorf`s with no sentinel `errors.Is` mapping, so they fall through to the generic internal-error handler.
- **Privacy settings are never enforced.** `privacy.profile.public`, `privacy.show_location`, etc. are stored but no query/handler anywhere actually filters profile visibility based on them — `GET /profiles` returns full profile data to any authenticated user regardless of these flags.

**Dead code / scaffolding never wired up** (safe to ignore unless you're the one finishing them):

- The entire **statistics** feature: `AccountStatistics` domain type + `profile_statistics` table exist, but nothing persists, reads, or updates them — no event handler populates them from activity-domain events despite that being the evident intent.
- `AccountProfileSettings.CreatedAt()` returns `updatedAt` by mistake — the real `created_at` is never observable through the domain API (though stored correctly in the DB).
- `ResetToDefault()` and its event (`account_settings_reset`) — no endpoint calls it.
- `AccountProfileNicknameChangedEvent` — defined, but `SetNickname()` never actually emits it.
- `EventAccountProfilePictureChanged` and `EventProfilePictureChanged` are two separately-named constants for the identical event-type string — only one is used; having both is just confusing.
- Optimistic locking isn't actually exposed to clients: `PUT /my/settings` accepts no `version` field, so there's no way for a caller to detect "someone else changed this concurrently" the way the original design intended — the internal version check only guards against two in-flight requests racing each other server-side.
- `location_region` column exists in the `profiles` table but no Go code reads or writes it.
- Minor field-name asymmetry: the update request uses `activity_interests` (plural), the response uses `activity_interest` (singular), for the same data.
- A few typo'd exported identifiers if you're browsing the code: `Cooridantes` (→ `Coordinates`), `SeetingPrivacyShowLocation` (→ `SettingPrivacyShowLocation`).
- If `Subscribe()` fails at startup (the one `identity.account.verified` subscription), the service logs it and keeps running anyway — meaning it could serve traffic indefinitely without ever creating profiles for newly-verified accounts, with no retry and no health-check signal for this state.
