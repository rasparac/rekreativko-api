# Account Profile Service

User profiles (name, nickname, bio, location, activity interests) and per-account settings. Runs on port `8082`. Does **not** own authentication (identity) or activity/session data.

## Running it

Part of `docker compose up` (container `account-profile`). Every route requires a bearer token at the gateway; the gateway restricts this prefix to `GET`/`POST`/`PUT` only (no `DELETE`, no `PATCH`) — matching the fact that none of this service's current routes use those methods. Any authenticated user can call any endpoint here, including bulk profile search.

Request bodies (`PUT /my/profile`, `PUT /my/settings`) are read by the shared `api.Decoder`, built in `main` and injected like in `identity`/`activity`: a body over 1 MiB, malformed JSON, an unknown field or a field of the wrong type is `400 validation_error` with `details` keyed by the JSON field name. The only `validate:` tags are the ranges of `location_latitude` (-90..90) and `location_longitude` (-180..180), because the domain does not check coordinates. Everything else is validated by hand in the domain layer (value objects) and application layer, with their own error codes (`invalid_nickname`, bio length, the activity type and level, the profile picture URL...), and deliberately **not** repeated as tags: the domain trims nickname, name, city and country first, so a tag would judge the raw input and could refuse what the domain accepts.

## API

| Method | Path | Notes |
|---|---|---|
| GET | `/api/v1/profiles` | Look profiles up by `account_id` and/or `nickname` — cut by [privacy](#privacy) |
| GET | `/api/v1/profiles/{id}` | Get a single profile by account ID — cut by [privacy](#privacy) |
| GET | `/api/v1/my/profile` | Current user's profile |
| PUT | `/api/v1/my/profile` | Update current user's profile |
| GET | `/api/v1/my/settings` | Current user's settings |
| PUT | `/api/v1/my/settings` | Update current user's settings |

`GET /profiles` is a **lookup** only: `account_id` and/or `nickname` (repeatable, both together narrow it), at least one and at most 100 values together, else 400 (about 49 characters per id, and proxies refuse a request line past ~8 KB, so a client with a longer roster sends it in chunks). Every match comes back in one `{"items": [...]}` list, newest profile first, not paged. There is no search by date of birth or location (`dob_gt`, `dob_lt`, `country`, `city` were removed in ep8.7): it would find people by what their privacy settings hide. The mobile app uses `account_id` for rosters and single profiles, one `nickname` for invites. No `/interests` or `/statistics` endpoints exist — interests are only editable as a full-replace array field on `PUT /my/profile`; statistics have no HTTP surface at all (dead feature, see below).

### Profile fields

`full_name`, `nickname` (3–50 chars, `^[a-zA-Z0-9_-]+$`, **not actually unique** despite what you might expect — no DB constraint or service check), `bio` (max 500 chars), `date_of_birth` (must be ≥16 years ago - `domain.MinimumAge`, the GDPR age of digital consent in Croatia - and not in the future; answers `age_too_young` otherwise. It is optional and not asked at registration, so this is a check on the field, not an age gate on creating an account), `location_city`/`location_country` (required together), `location_latitude`/`location_longitude` (optional, only set as a pair), `profile_picture_url` (must be http/https, ≤2048 chars), `activity_interests` (array of `{name, level}` — full replace on every update, one entry per activity type, deduplicated against a shared 16-type/3-level catalog also used by the `activity` service).

### Privacy

`GET /profiles` and `GET /profiles/{id}` return another account's profile cut to what its owner allows (`domain.AccountProfile.SeenByOthers`, applied in `application.service.seenBy`); `GET /my/profile` and your own entry in a lookup come back whole.

| Field | Shown to other accounts |
|---|---|
| `id`, `nickname`, `profile_picture` | always - rosters and invites need them |
| `full_name`, `bio`, `activity_interest` | only with `privacy.profile.public` on (default off) |
| `location` city and country | only with `privacy.show_location` on (default off) |
| `location` coordinates, `date_of_birth` | never |

The owners' settings are read in one query per request (`AccountProfileSettingsManager.FindProfilePrivacy`); an account without a settings row gets the defaults, so it is never shown more than it chose.

### Settings

Key/value store (`GET`/`PUT /my/settings`), namespaced dotted keys (`notification.email.enabled`, `preference.language`, etc.), each with a type (`string`/`bool`/`int`/`float`). The registry (`domain/account_settings_registry.go`) defines every key with its type and default: it renders `GET /my/settings`, validates `PUT /my/settings`, and is what an account is seeded with when it verifies (`GetDefaultSettings`, 10 keys), so the three cannot disagree. A key only goes in the registry once something reads it.

## Domain events

**Consumed**:
- `identity.account.verified` → creates a blank `AccountProfile` row and a default `AccountProfileSettings` row (every registry key at its default) in one transaction. This is the only place a profile gets created; there's no manual "create profile" endpoint. Dropped for an account already erased.
- `identity.account.erased` → erases every row of the account, see [Account erasure](#account-erasure).

Both subscriptions are created at startup. If either fails the service **exits** (the pod restarts), like `notifications`, `activity` and `identity` do: serving without them would mean new accounts never get a profile and erased ones are never erased, with nothing in `/health` to show it.

**Published** (to `account_profile.event_outbox`): `account_profile.created`, `account_profile.account.erased` (see [Account erasure](#account-erasure)), `.location.changed`, `.profile_picture.changed`, `.activity_interest.added/removed/level.changed`. `AccountProfileNicknameChangedEvent` is defined but never raised — see below. Settings changes publish nothing: no service consumes them, and the planned consumers (`notification.email.enabled`, stored language) need events carrying the new value, which they add when they are built.

## Configuration & schema

`PORT=8082`, `SERVICE_NAME=account_profile`. No `JWT_SECRET` dependency — this service trusts the `X-User-Id` header the gateway sets after validating the JWT itself, gated only by the shared `X-Gateway-Key`.

Schema (`account_profile` Postgres schema): `profiles` (PK is `account_id`, i.e. identity's account ID — no separate profile ID), `activity_interests` (unique per account+type), `settings` (composite PK `account_id, key`), `profile_statistics` (dead, see below), `account_settings_meta` (optimistic-lock version counter), `erased_account` (accounts erased, see below), `event_outbox`.

## Account erasure

Identity erases a deleted account once its grace period has passed and raises `identity.account.erased` (`account_id` only). This service then, in one transaction (`application.AccountErasureService`):

1. inserts the account into `erased_account` (`ON CONFLICT DO NOTHING`) - if it was already there, this is a redelivery and nothing more happens;
2. deletes the account's rows child-first from `activity_interests`, `settings`, `profile_statistics`, `account_settings_meta`, then `profiles` (`persistence.accountDataTables` - no `ON DELETE CASCADE`, so what is erased stays visible there; **a new table holding account data must be added to that list**, and the integration test fails until it is);
3. raises `account_profile.account.erased` (`account_id` only), which identity records in `identity.account_erasure` - `account_profile` is in identity's `ACCOUNT_ERASURE_CONTEXTS`.

An account that never verified (no profile) is recorded and confirmed all the same. A late `identity.account.verified` for an erased account is acked without re-creating the profile. A bad payload is dead-lettered; any other failure is retried. `account_profile_accounts_erased_total` counts erasures. The profile picture is only a URL the client supplied (no file storage here), so there is no file to delete.

## Known issues

This service had the most drift from its original design doc of anything documented so far — the list is long, but most items are low-severity/dead-code.

**Broken, will error or fail silently on use:**
- **A version conflict on `PUT /my/settings` returns `500` instead of `409`** — it is a raw `fmt.Errorf` with no sentinel `errors.Is` mapping, so it falls through to the generic internal-error handler.

**Dead code / scaffolding never wired up** (safe to ignore unless you're the one finishing them):

- The entire **statistics** feature: `AccountStatistics` domain type + `profile_statistics` table exist, but nothing persists, reads, or updates them — no event handler populates them from activity-domain events despite that being the evident intent.
- `AccountProfileSettings.CreatedAt()` returns `updatedAt` by mistake — the real `created_at` is never observable through the domain API (though stored correctly in the DB).
- `ResetToDefault()` and `AccountProfileSettings.UpdateSetting()` — no endpoint calls them.
- `AccountProfileNicknameChangedEvent` — defined, but `SetNickname()` never actually emits it.
- `EventAccountProfilePictureChanged` and `EventProfilePictureChanged` are two separately-named constants for the identical event-type string — only one is used; having both is just confusing.
- Optimistic locking isn't actually exposed to clients: `PUT /my/settings` accepts no `version` field, so there's no way for a caller to detect "someone else changed this concurrently" the way the original design intended — the internal version check only guards against two in-flight requests racing each other server-side.
- `location_region` column exists in the `profiles` table but no Go code reads or writes it.
- Minor field-name asymmetry: the update request uses `activity_interests` (plural), the response uses `activity_interest` (singular), for the same data.
- A typo'd exported identifier if you're browsing the code: `Cooridantes` (→ `Coordinates`).
