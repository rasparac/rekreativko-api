# Activity Service

The core domain: activity groups, sessions, invites, attendance/RSVP, and recurring session templates. Everything else in this system (identity, profiles, notifications) exists to support this.

## Running it

Two entry points, both part of `docker compose up`:
- **`cmd/api`** (container `activity`, port `8084`) — long-running HTTP server.
- **`cmd/cron`** (container `activity-cron`) — one-shot batch job, no server, no `restart` policy. Triggered on demand: `docker compose run --rm activity-cron`.

Every route requires a bearer token at the gateway (`^/activity/api/v1/.*` → `RequireAuth: true`, no public carve-outs, no method restrictions) — including the `discover` endpoints.

## The cron job

Runs 3 steps in order, each independent (a later step still runs if the current one logs an error, per the code, but a hard failure in step N aborts the job):

1. **`GenerateSessionsFromTemplates`** — 14-day lookahead window (hardcoded). For every active template whose `generated_up_to` watermark is behind that window, walks `RecurrenceRule.NextOccurrenceAfter` forward creating one `Session` per occurrence, then advances the watermark.
2. **`ExpireStaleInvites`** — flips every pending `GroupInvite` past `expires_at` to `expired`, publishes `activity.invite.expired`.
3. **`ExpireCompletedSessions`** — flips every scheduled/started `Session` whose `end_time` has passed to `completed` (distinct from user-initiated `Complete()`), publishes `activity.session.expired`.

Nothing else in the system ever performs these three transitions — without this cron running periodically, stale invites and past-due sessions accumulate forever and keep polluting list/discover queries.

## API

Routes, grouped by resource (all under `/api/v1`):

**Activity groups**: `POST /activity-groups`, `GET /activity-groups/discover`, `GET /activity-groups`, `GET /activity-groups/{id}`, `PUT /activity-groups/{id}`, `POST /activity-groups/{id}/activate`, `DELETE /activity-groups/{id}`.

**Session templates**: `POST/GET /activity-groups/{groupId}/templates`, `GET/PUT/DELETE /templates/{id}`, `POST /templates/{id}/activate`, `POST /templates/{id}/deactivate`.

**Sessions**: `POST /sessions`, `GET /sessions`, `GET /sessions/discover`, `GET /sessions/{id}`, `PUT /sessions/{id}`, `POST /sessions/{id}/start`, `POST /sessions/{id}/complete`, `DELETE /sessions/{id}` (cancel), `PATCH /sessions/{id}/visibility`.

**Members**: `POST /activity-groups/{groupId}/members` (invite), `POST /activity-groups/{groupId}/join-requests`, `GET /activity-groups/{groupId}/members`, `GET .../members/{userId}`, `POST .../members/{userId}/approve`, `POST .../members/{userId}/reject`, `PATCH .../members/{userId}/role`, `DELETE .../members/{userId}`, `POST /activity-groups/{groupId}/leave`.

**Invites**: `POST /activity-groups/{groupId}/invites`, `GET /invites`, `POST /invites/{id}/accept`, `POST /invites/{id}/decline`.

**RSVP / attendees**: `POST/PUT/DELETE/GET /sessions/{sessionId}/rsvp`, `GET /sessions/{sessionId}/attendees`, `POST .../rsvp/{userId}/approve`, `POST .../rsvp/{userId}/reject`, `DELETE .../rsvp/{userId}` (remove a confirmed attendee — distinct from the no-`{userId}` self-cancel route).

Service methods that exist but have **no route** (incomplete/parked, not necessarily broken): `CancelActivityGroup`, invite-link create/revoke/use (the `InviteLink` domain aggregate is fully built), `PromoteMember`/`DemoteMember` (only the generic `PATCH .../role` is exposed).

## Domain model

- **ActivityGroup** — `draft → active → cancelled`, separately soft-deletable. `Visibility`: public/private. Only the creator can update/activate/cancel/delete/change visibility (no admin exception, unlike sessions).
- **Member** — join table linking a user to a group. `MemberRole`: creator/admin/member (`CanManageMembers()` true for creator+admin). `MemberStatus`: pending/confirmed/rejected/left/removed. Carries an `isPriority` flag (early RSVP access — bypasses a session's `open_at` gate).
- **Session** — one occurrence, either group-linked or fully standalone (`activityGroupID == nil`, always forced public). `scheduled → started → completed`, or `canceled` from either. `capacity` (nil = unlimited), `requiresApproval` (RSVP "going" creates a pending request instead of auto-joining), `openAt` (regular members wait, priority members bypass).
- **Attendee** — a user's RSVP on a session. `AttendeeStatus`: going/pending/not_going/maybe/promoted (going+promoted "hold a spot"). `AttendeeSource`: auto_confirmed/auto_pending (creator-added)/rsvp_manual/requested (new — pending approval). `Approve`/`Reject` resolve a pending request; `Remove` lets a manager kick an already-confirmed attendee (can't target the session creator).
- **GroupInvite** — direct user-to-user invite, 7-day expiry. pending/accepted/declined/expired.
- **InviteLink** — shareable tokenized link (32 random bytes, base64url), 7-day expiry, active/expired/revoked, with per-user usage tracking so a link works once per user. Fully built, not yet exposed over HTTP.
- **SessionTemplate** — recurring-session blueprint: optional `RecurrenceRule`, default capacity/location, `generatedUpTo` cron watermark.
- **RecurrenceRule** — daily/weekly/monthly + interval, optional day-of-week/day-of-month, time-of-day, optional end date.

## Authorization

`MemberRole.CanManageMembers()` (creator/admin only) is the base check for most group operations. Sessions use a superset, `Session.canManageSession(userID, role)`:
```go
func (s *Session) canManageSession(userID uuid.UUID, userRole MemberRole) bool {
	if userRole.CanManageMembers() { return true }
	return s.createdByID == userID
}
```
This exists specifically for **standalone sessions**, where the creator has no group membership/role at all (`parseMemberRole("")` returns `("", nil)` rather than erroring) — a bare role check would incorrectly lock the creator out of managing their own session. Used by `Session.Update/SetVisibility/Cancel/Start` and by `Attendee.Approve/Reject/Remove`. **Not** used by `Session.Complete`, which still does a bare `CanManageMembers()` check — worth confirming whether a standalone session's creator can actually complete their own session (see [Known issues](#known-issues)).

## Domain events

Grouped by aggregate (see `activity/internal/domain/events.go` for full payload shapes):

- **ActivityGroup**: created, updated, cancelled, deleted, started, completed, recurrence_updated, visibility_changed
- **Member**: join_requested, joined, left, approved, rejected, removed, role_changed, priority_set, priority_removed
- **Invite**: sent, accepted, declined, expired
- **InviteLink**: created, used, revoked, expired
- **Session**: created, updated, started, cancelled, completed, expired, visibility_changed (`deleted` constant exists but nothing emits it)
- **Attendee**: auto_confirmed, rsvp_going, rsvp_not_going, rsvp_maybe, rsvp_auto_pending, promoted, join_requested, join_approved, join_rejected, removed
- **SessionTemplate**: created, activated, deactivated, deleted, updated

## Configuration & schema

`PORT=8084` (api), no port for cron (one-shot). `SERVICE_NAME=activity` / `activity_cron`. Plus shared config (Postgres, NATS, gateway key, telemetry).

Schema (`activity` Postgres schema, one migration file): `activity_group`, `member`, `group_invites`, `group_invite_links`, `group_invite_link_usage`, `session_template`, `session`, `session_attendee`, `activity_group_statistics`, `session_statistics` (see [Known issues](#known-issues) — the two `*_statistics` tables are dead), `event_outbox`.

## Known issues

- **Dead statistics subsystem.** `activity_group_statistics`/`session_statistics` tables and their domain types exist with rich update methods, but no application/repository code reads or writes them anywhere — looks like scaffolding that was never wired up.
- **`idx_attendees_confirmed_count` indexes the wrong status.** Defined as `WHERE status = 'confirmed'`, but `AttendeeStatus` has no `confirmed` value — the real "holds a spot" statuses are `going`/`promoted`, and the actual `CountConfirmedAttendees` query correctly checks those. The index is dead/never used.
- **`RecurrenceRule.nextMonthly` likely miscalculates for `interval > 1`** — uses `int(after.Month())*r.interval` (multiplication) rather than adding `interval` months, which doesn't match "every N months" semantics and diverges from how `nextDaily`/`nextWeekly` correctly use `AddDate`. Needs a unit test with `interval=2` or higher to confirm the actual generated dates.
- **`RecurrenceRule.HasEnded(t time.Time)` ignores its parameter**, always compares against `time.Now().UTC()` instead. Harmless today (both call sites pass `time.Now().UTC()` anyway) but a landmine if anyone calls it expecting the parameter to matter.
- Service methods with no route yet (see API section) — may be intentionally incomplete rather than bugs; confirm before building against them.
