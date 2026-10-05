Core part of the applicatio is activity. In the context of the application, an activity group is a group of members who are enjoying the same activity.
In the the group there are three roles: creator, member and admin. The creator is the person who created the activity. The member is the person who is participating in the activity. The admin is the person who is managing the activity.
Each activity group can have multiple sessions. Each session can have multiple attendees.


***Rules***

Acitvity Group rules:
- Creater auto-confrimed member on creation
- Only creator can cancel group
- Any confirmed member can create session
- Recurrence rule optiona (group can be non-recurring)
- Cancelled group -> all future sessions cancelled: in the same transaction, every scheduled/started session gets
  `Session.CancelFromGroup` (attendees notified via `session.cancelled`) and any running draft / open voting round
  ends with `session_ended` (`SessionService.CancelGroupSessions`). The recurring-session cron generates nothing for
  a cancelled or deleted group.
- Deleted group -> its live sessions are cancelled exactly as above (reason `activity group deleted`), then every
  session of the group, ended ones included, is soft-deleted (`session.deleted_at`) in the same transaction
  (`SessionService.DeleteGroupSessions`). Deleted sessions are invisible: 404 on fetch, absent from every listing,
  discover and the expiry cron.

Session rules:
- Any confirmed member can create session
- Creator/Admin can mark members as auto-attend on creation
- Auto-attend processed in order, respects capacity
- Capacitiy full -> auto-attend overflow goes pending
- RSVP going + capacitiy available -> confirmed
- RSVP going + capacitiy not available -> pending (waitlist)
- Someone leaves -> first pending member is promoted to confirmed -> notification sent
- Session cancelled -> all going/pending notified via event
- Session completed + group has recurrence -> generate next session

Attendee rules:
- Must be confirmed group memeber to attend
- Can change RSVP (going > not going > maybe)
- Maybe holds no capacity
- Pending is ordered (FIFO waitlist)
- Promote first pending to confirmed -> notification sent
- Leave -> notification sent



====================================================================================================

AcitivitySession:
|----NewActivitiySession():
|    |----Creates sessoion
|    |---- Process auto-attendees inline
|    |____Returns Session, []Attendee, error
|         |-- Attendees already have correct status
|
|
|----Lifecycle:
|    |---- scheduled -> started -> completed
|    |---- canceled (from scheduled or started)
|    |----- CancelFromGroup() bypasses permission check
|
|
|----Permissions:
|    |---- Edit/Cancel/Start/Complete: session creator OR group admin/creator (canManageSession)
|
|
|----Capacity:
|    |---- nil = unlimited
|    |---- HasCapacitiyFor() public for service layer RSVPs


Atendee:

AteendeSource:
- tells us HOW the atendee got added
- auto_confirmed -> added by admin/creator, got spot
- auto_pending - added by admin/creator, capacity full
- rsvp -> member manually RSVPed

HoldsSPot() on stastus:
- going + promoted = holds a spot
- pending/not_going/maybe = no spot held
- used in UpdateRSVP events to signal service layer that a spot just opened up - trigger promotion

RSVP going but capacity full:
- updateRSVP(going) -> internally sets pending
- No error returned - this is valid behavior
- users get notified they are on waitlist via event

Pending - going transation blocked:
- user cannot manually set themselves to going from pending
- must wait for Promote() called by service layer
- ensures FIFO fairness on waitlist

Atendee flow example:

Session capacity: 3
Auto-attende list [Alice, Bob, Charlie, Dave]

NewSession processes:
- Alice -> going (slot 1/3)
- Bob -> going (slot 2/3)
- Charlie -> pending (slot 3/3)
- Dave -> pending (capacity full)

Eve RSVPss going:
- NewRSVPAtendee -> pending (capacity full)

Bob changes to not_going:
- UpdateRSVP(not_going) -> HeldSpot: true in event
- Service layer: find first pending by created
    - Dave (joined before Eve)
- Dave.Promote() -> status: promoted
- Dave gets notification "you got a spot!"

Eve is still pending

====================================================================================================

## Session Templates (Recurring Sessions)

Session templates allow groups to define recurring sessions that auto-generate at specified intervals.

**SessionTemplate:**
- Belongs to an ActivityGroup
- Defines recurrence rules (frequency, day of week, time)
- Has default capacity that applies to all generated sessions
- Tracks `generated_up_to` timestamp to know how far ahead sessions have been created
- Can be active or inactive
- When template deleted/inactive, no new sessions generated but existing ones remain

**Recurrence Fields:**
- `recurrence_frequency`: "daily", "weekly", "monthly"
- `recurrence_interval`: every N frequencies (e.g., every 2 weeks)
- `recurrence_day_of_week`: 0=Sunday, 1=Monday, ... 6=Saturday
- `recurrence_day_of_month`: day of month for monthly recurrence
- `recurrence_time_hour`/`recurrence_time_minute`: time of day
- `recurrence_ends_at`: optional end date for recurrence
- All nullable = template exists but is not recurring (manual sessions only)

**Session Generation:**
- Cron job finds active templates where `generated_up_to < NOW() + lookahead_window`
- Generates sessions according to recurrence rules
- Updates `generated_up_to` timestamp
- Each generated session links back to template via `session_template_id`

**Example:**
```
Template: "Monday Basketball"
- frequency: weekly
- interval: 1
- day_of_week: 1 (Monday)
- time: 18:00
- capacity: 10
- generated_up_to: 2025-02-01

Cron runs on 2025-01-15, generates sessions for:
- 2025-01-20 18:00
- 2025-01-27 18:00
- 2025-02-03 18:00
- 2025-02-10 18:00
Updates generated_up_to to 2025-02-17
```

====================================================================================================

## Group Invite System

Two types of invites: **Direct Invites** (specific user) and **Invite Links** (shareable).

### Direct Invites (group_invites table)

- Creator/Admin invites specific user by user_id
- User receives notification
- Invite expires after N days
- User can accept or decline
- One pending invite per user per group at a time

**States:**
- pending: invite sent, awaiting response
- accepted: user accepted and joined group
- declined: user declined invite
- expired: invite expired before response

### Invite Links (group_invite_links table)

- Creator/Admin generates shareable link with unique token
- Link can be used by multiple users
- Has expiration date
- Can be revoked by creator/admin
- Tracks usage via `group_invite_link_usage` table

**Invite Link Flow:**
1. Admin creates invite link → generates unique token
2. Link shared externally (copy/paste, QR code, etc.)
3. User clicks link → validates token (active, not expired)
4. Check if user already used this link (via usage table)
5. User auto-joins group (or goes to pending based on group rules)
6. Record usage in `group_invite_link_usage` table

**States:**
- active: link is valid and can be used
- expired: link passed expiration date
- revoked: admin manually revoked link

**Usage Tracking:**
- Prevents same user from using link multiple times
- Provides audit trail of who joined via which link
- Helps detect abuse or track invite campaigns

====================================================================================================

## Session Invites (Standalone Sessions)

A standalone session (no `activity_group_id`) has no group, so group invites can't cover it, and if it is
private nobody but its creator could ever attend (`CreateRSVP` only admits confirmed group members or, for
public sessions, anyone). Session invites let the creator bring specific users into it. The invitee must
accept - nobody is added to a session without consenting.

**Rules:**
- Standalone sessions only (`ErrSessionNotStandalone` otherwise); group sessions are joined via the group
- Only the session creator can invite (standalone sessions have no group roles)
- Session must be `scheduled`
- Cannot invite yourself, someone already attending, or someone with a pending invite
- Expires after 7 days (same as group invites); cron marks stale invites `expired`
- Only the invited user can accept or decline
- Accept creates the attendee (source `invited`) under the session capacity advisory lock:
  going if there is capacity, otherwise pending (waitlist) - same as a "going" RSVP
- Invites bypass `requires_approval` (the creator already vetted the user by inviting them)
- Decline/expire creates no attendee

**States:** pending -> accepted | declined | expired (same `InviteStatus` as group invites)

**Endpoints:**
- `POST /api/v1/sessions/{sessionId}/invites` - creator invites `{user_id}`
- `GET /api/v1/session-invites` - caller's pending, unexpired invites
- `POST /api/v1/session-invites/{id}/accept` - returns the created attendee
- `POST /api/v1/session-invites/{id}/decline`

**Events:** `activity.session_invite.sent | accepted | declined | expired`. Accepting also emits the usual
`activity.session.attendee.rsvp_going` / `rsvp_auto_pending` event for the new attendee.

**Note:** a pending invitee can't `GET` a private session until they accept (visibility is
creator/attendees only); the invite response carries `session_id` only.

====================================================================================================

## Teams (team sports)

A session can be split into teams - only for team sports (`ActivityType.IsTeamSport()`: basketball,
football, volleyball; `ErrTeamsNotSupported` otherwise). Sessions are always created without teams.
Later work builds on this model: captain draft (6gg.2), proposals/voting (6gg.3), live updates over
SSE (6gg.4).

**Workflow:**
1. Session is created (no teams)
2. People join (RSVP) until it's full - or not, capacity may be unlimited
3. Creator/admin creates the teams (`Session.CreateTeams`), then fills them (direct assignment now;
   captain draft / proposals later)

**TeamConfig** (value object, `*TeamConfig` nil = no teams):
- `team_count`: 2..8, defaults to 2
- `min_players_per_team`: optional **minimum** ("at least this many per team to play"), never a maximum -
  a team is never full and everyone going plays (no substitutes; uneven teams are fine). Independent of
  session `capacity`, which still bounds the confirmed pool/waitlist
- `PlayersNeeded()` = team_count x min (or 1 per team without a minimum): "enough people". Flows that
  form teams from everyone going (captain draft, proposals) require it (`not_enough_players`); manual
  assignment and `POST /teams` never do
- `colors`: optional `#RRGGBB` per team (shirts/bibs) - none, or exactly one per team

**Creating teams** (`POST /sessions/{id}/teams`):
- Session creator or group admin/creator, any time while the session is scheduled or started (not tied
  to the session being full)
- Creates `Team A`, `Team B`, ... (`position` 0..n-1, color from config) and stores the config on the
  session (`session.team_count` / `min_players_per_team`)
- Calling it again **replaces** the teams: every assigned attendee is unassigned (`team_unassigned`,
  reason `teams_replaced`), old teams are deleted, new empty ones created. Runs under the session
  advisory lock so no assignment can race with the replacement
- Session templates do not carry teams - each generated session gets its teams the same way

**Team** (entity inside the Session aggregate). Loaded by `GetSessionByID` only; list/discover queries
carry the config but not the teams.

**Assignment** is stored on the attendee (`session_attendee.team_id`, NULL = unassigned):
- `Attendee.AssignToTeam` / `UnassignFromTeam`, by session creator or group admin/creator only
- Only confirmed attendees (going/promoted) can be on a team; one team per attendee (moving = reassign)
- No size check (the minimum is not a cap); runs under the session advisory lock so it can't race a team
  replacement or a draft. Rejected with `draft_already_active` while a captain draft runs
- Allowed while the session is scheduled or started; not once canceled/completed
- A confirmed attendee leaving (RSVP -> not_going/maybe, RSVP cancelled, or removed by a manager) no longer just
  frees their slot: it resets team formation for everyone (see "Team formation reset")
- Promotion from the waitlist does not assign a team

**Endpoints:**
- `POST /api/v1/sessions/{id}/teams` `{team_count?, min_players_per_team?, colors?}` - create or replace
  teams, returns the session with its (empty) teams
- `PUT /api/v1/sessions/{sessionId}/rsvp/{userId}/team` `{team_id}` - assign or move, returns attendee
- `DELETE /api/v1/sessions/{sessionId}/rsvp/{userId}/team` - unassign (no-op if not on a team)
- `GET /api/v1/sessions/{id}` includes `team_config` and `teams[]` with `member_user_ids`; list/discover
  include `team_config` only; attendee responses include `team_id`

**Events** (via outbox):
- `activity.session.teams_created` - teams (id/name/color/position), `min_players_per_team`, `replaced`
- `activity.session.attendee.team_assigned` (was unassigned), `team_changed` (carries `previous_team_id`)
- `activity.session.attendee.team_unassigned` with `reason`: `manual` | `left` | `removed` |
  `teams_replaced`

### Captain draft (6gg.2)

Two captains pick everyone going, in turn; the result becomes the session's two teams. Product rules are the
mobile team's decisions D4-D9 (see the ticket design).

- **Start** `POST /api/v1/sessions/{id}/draft` `{captain_ids: [A, B], pick_order?, min_players_per_team?, colors?}`
  - session creator or group admin/creator, team sports only, session scheduled or started, captains must be
  going, and enough people going (`PlayersNeeded` = 2 x min, or 2) else `not_enough_players`. At most one
  running (active or paused) draft per session. The first captain leads Team A and picks first.
- **Pick order** lives on the draft (a session can be re-drafted): `snake` (A,B,B,A - default) or `alternate`
  (A,B,A,B). `PickOrder.sideForTurn(turn)` is the whole rule; the server enforces turns.
- **Pick** `POST /api/v1/sessions/{id}/draft/picks` `{user_id}` - only the captain whose turn it is, only an
  unpicked person going. No team is ever full; there is no turn skipping.
- **Picks live in the draft** (`session_team_draft_pick`) until it completes - the session's teams are untouched
  while it runs (active or paused), and `POST /teams` / direct assign / unassign are rejected
  (`draft_already_active`).
- **Completes automatically** when everyone going has been picked. Completion replaces the session's teams with
  Team A (position 0) and Team B via `Session.ApplyDraftTeams` - old assignments get `team_unassigned`
  (teams_replaced), drafted players `team_assigned`.
- **Attendance changes mid-draft:** a joiner is simply in the pool. Anyone who stops holding a spot (captain
  or not) **resets team formation**, cancelling the draft with `cancelled_reason: roster_changed` - see
  "Team formation reset" below. The pause / `PUT /draft/captains` machinery (`paused_reason: captain_left`,
  `draft.player_dropped`, `draft.captain_replaced`) is no longer reachable from attendance changes and is only
  kept until it is removed (see its ticket).
- **Cancel** `DELETE /api/v1/sessions/{id}/draft` - creator/admin, active or paused (`cancelled_reason:
  organizer`); teams stay exactly as before.
- **GET** `/api/v1/sessions/{id}/draft` - latest draft (any status). Response (mobile-aligned): `status`
  active|paused|completed|cancelled, `paused_reason`, `cancelled_reason`, `captains[{user_id|null,
  team_position}]`, `turn{pick_number, total_picks, team_position, captain_user_id|null}` (null once ended),
  `turn_order` (team position of every turn from pick 0 to the last - `TeamDraft.TurnOrder`; `total_picks` =
  turns taken + people still available, so it changes as people join/leave), `teams[{position, user_ids}]`,
  `picks`, `available_user_ids`, `version` (bumped on every change, persisted).
- **No turn timeout** - a stuck draft is cancelled by the organizer.
- **Concurrency:** every draft command and every attendance change takes the session advisory lock, so two picks
  for the same turn can't both win (`not_your_turn`).
- **Events** `activity.session.draft.*` (all carry `version`, team positions 0/1): `started` (captains, pick
  order), `turn_changed` (whose turn - "your turn" notifications), `player_picked`, `player_dropped`, `paused`
  (captain who left, organizer), `captain_replaced` (`resumed`), `completed` (both rosters), `cancelled` (reason,
  `participant_user_ids` = people going; empty for `session_ended`).
- **Errors:** `not_your_turn`, `player_not_available`, `not_draft_captain` (403), `draft_not_active`,
  `draft_paused`, `draft_not_paused`, `draft_already_active`, `invalid_captains` (422), `not_enough_players`,
  `draft_not_found` (404), `unauthorized`.
- A draft can't start while team voting is open (`voting_open`), and nobody can propose while a draft runs.
- **Session ends** (cancelled, completed or auto-completed by the cron) -> a running draft is cancelled
  (`cancelled_reason: session_ended`), under the session lock, so `GET` stops reporting a live turn
  (`teamFormation.sessionEnded`).

### Team proposals and voting (6gg.3)

People going propose divisions into teams and vote; the organizer closes the round and the winner replaces the
teams. Product rules are the mobile team's D10-D16 plus backend decisions on the ticket.

- **Round** (`TeamVotingRound`, one open per session): opens with its first proposal. Team count is fixed then -
  the session's teams when it has some (plus their `min_players_per_team`, and "keep current teams" becomes a
  vote option), otherwise the first proposal's count with no minimum.
- **Players needed** = team count x minimum (1 per team without teams). Opening needs it (`not_enough_players`);
  falling below it while open cancels the round (`cancelled_reason: not_enough_players`).
- **Propose** `POST /api/v1/sessions/{id}/proposals` `{teams: [{user_ids}]}` - anyone going. Valid division (D11):
  the round's team count, everyone going on exactly one team, nobody not going, every team >= minimum; else
  422 `invalid_division` with `details.reason` (wrong_team_count, player_not_going, duplicate_player,
  missing_players, team_below_minimum) and `details.user_ids`. Proposing never changes the current teams.
- **Vote** `PUT /api/v1/sessions/{id}/proposals/vote` `{proposal_id}` | `{keep_current: true}` - anyone going,
  one vote, changeable.
- **Close** `POST /api/v1/sessions/{id}/proposals/close` `{winner_proposal_id? | keep_current?}` - organizer. Most
  votes wins; a tie (no votes at all ties every option) -> 409 `tie_requires_winner` with
  `details.tied_proposal_ids` / `details.keep_current_tied` until a tied option is passed (`invalid_winner`
  otherwise). A winning proposal replaces the teams via `Session.ApplyProposalTeams` (colors kept when there were
  teams); keep-current changes nothing.
- **D16:** proposals and votes are deleted when the round closes or is cancelled; the round row stays (status,
  result, reason) so `GET` can show how it ended.
- **GET** `/api/v1/sessions/{id}/proposals` - latest round (or `voting_status: none`): status, reason,
  `team_count`, `players_going`, `players_needed`, `my_vote`, `votes_cast`, `eligible_voters`,
  `keep_current_votes`, `items[{id, author_id, created_at, teams[{position, user_ids}], vote_count}]`, `result`,
  `version`.
- **Attendance:** anyone who stops holding a spot resets team formation, which cancels the round
  (`roster_changed`) and clears its history - see "Team formation reset" below. Joiners can vote; they are in no
  proposal and stay unassigned when the winner is applied.
- **Teams replaced** by `POST /teams` while open -> round cancelled (`teams_replaced`).
- **Session ends** (cancelled, completed or auto-completed) while open -> round cancelled (`session_ended`).
- **Concurrency:** every voting command takes the session advisory lock - saving a round rewrites its votes, so
  this is what keeps concurrent votes from being lost.
- **Events** `activity.session.voting.*`: `opened` (first proposal), `proposal_created`, `vote_cast` (`changed`),
  `player_removed`, `closed` (winner / kept_current), `cancelled` (reason); a winner also emits `teams_created`
  and `team_assigned`. `opened`, `closed` and `cancelled` carry `participant_user_ids` (people going; empty on
  `cancelled` for `teams_replaced` / `session_ended`).

### Teams source (6gg.12)

`session.teams_source` (`manual` | `draft` | `vote`, NULL without teams) records where the **current** teams came
from: `POST /teams` -> `manual`, a completed captain draft -> `draft`, a winning proposal -> `vote`; a reset clears
it. Picked teams are not put to a vote, so while it is `draft` `POST /proposals` fails with **409
`teams_drafted`** (`Session.requireProposalsAllowed`, checked when opening a round and when adding a proposal).
It is tracked rather than read off the latest draft on purpose: a re-draft that gets cancelled leaves the drafted
teams in place, so proposals stay refused. Proposals are allowed again after a reset, or once the organizer
replaces the teams by hand (`manual`) or a vote does (`vote`). Sessions whose teams predate the column have no
source and allow proposals. The snapshot exposes it as `teams_source` (null without teams), so clients need not
infer it from `GET /draft`.

### Team formation reset (6gg.11)

"As if there had never been a draft": the session goes back to its initial state.
(`teamFormation.resetTeamFormation`, `Session.ResetTeams`)

- **What it does** (one transaction, under the session advisory lock): deletes the teams and the team setup
  (`team_count`, `min_players_per_team` back to NULL, every `team_id` cleared); cancels a running draft
  (`cancelled_reason`) and an open voting round (`cancel_reason`); deletes **every** draft and voting round of the
  session (`DeleteSessionDrafts` / `DeleteSessionRounds`), so `GET /draft` is 404 again, the snapshot has no draft
  and proposals are allowed. Nothing happens - and no event is raised - when there is nothing to reset (no teams,
  no running draft, no open round).
- **Also clears `teams_source`** (see above), so proposals are allowed again.
- **Triggers:** (1) **automatically** whenever a confirmed (going/promoted) attendee stops holding a spot - RSVP
  cancelled, switched to maybe/not_going, or removed by a manager - whoever they are (any leaver, not just a team
  member), reason `roster_changed`, after any waitlist promotion. A finished or called-off session keeps its
  teams. A **joiner does not reset** anything (decided 2026-10-04, for now): they stay unassigned. (2) **By
  hand:** `DELETE /api/v1/sessions/{id}/teams`, session creator or group admin/creator (403 otherwise), 409 once
  the session is canceled/completed, idempotent, reason `organizer`; responds with the resulting snapshot.
- **Events:** `draft.cancelled` / `voting.cancelled` with reason `roster_changed` (automatic) or `teams_reset`
  (by hand) and no participants, then `activity.session.teams_reset` `{session_id, reason, reset_by,
  participant_user_ids}` (the people going afterwards). No per-attendee `team_unassigned` events - the reset
  event and the snapshot carry it. The SSE stream pushes `teams_reset` like any session event.
- **Notifications:** one `team_formation_reset` to everyone still going except `reset_by`; the draft/voting
  cancel handlers skip `roster_changed` and `teams_reset` so nobody is told twice.

### Live updates (6gg.4)

`GET /api/v1/sessions/{id}/events` streams a session's team formation as server-sent events
(`interfaces/http/team_formation_stream.go`, `interfaces/live/hub.go`).

- **Delivery:** every activity instance subscribes to `activity.session.>` with `SubscribeBroadcast` (plain NATS,
  at-most-once, every instance gets every event) - a phone's stream lives on one instance. On each event the hub
  rebuilds the session's snapshot once and pushes it to that session's clients. Latency = outbox
  publish time: the publisher is woken by Postgres `LISTEN/NOTIFY` (see below), the poll is only the fallback.
- **Wire format:** first event `snapshot`, then one event per change named without the `activity.session.` prefix
  (e.g. `draft.player_picked`); `data` = `{change, snapshot}` (raw domain event + full state, same document as
  `GET /team-formation`, `my_vote` personalised); `id` = snapshot version. `: ping` every 20s. Before closing on
  its own the server sends `disconnected` with `reason` (`removed`, `not_found`, `slow_consumer`, `server_shutdown`, `token_expired`). No
  replay - a reconnect starts from a fresh snapshot. On shutdown `main.go` registers `Hub.Shutdown` via
  `srv.RegisterOnShutdown`: every client gets `server_shutdown` (and later connects are closed at once), so
  `http.Server.Shutdown` doesn't wait out its grace period and clients reconnect to another instance.
- **Who sees what:** the snapshot carries `going_user_ids` (going/promoted attendees, join order; waitlisted,
  pending, maybe and declined people are never in it), so a client renders teams plus the unassigned pool
  (going minus everyone on a team) from one document. `change` is the raw domain event, filtered per viewer
  (`interfaces/live/visibility.go`): `attendee.join_requested`, `join_rejected`, `auto_pending` and
  `rsvp_auto_pending` go only to the session's managers (creator + group admins/creator) and to the person the
  event is about; the `manager_user_ids` field is stripped for everyone who is not a manager (it is on
  `join_requested` and `voting.tied`). If the managers cannot be resolved, nobody counts as one. Joining,
  switching to Maybe and leaving already push an `attendee.rsvp_*` event to everyone.
- **Token expiry:** the gateway checks the JWT once, on connect. It forwards the token's `exp` as
  `X-Token-Expires-At` (unix seconds; a client-sent value is always dropped, like `X-User-ID`) and the handler ends
  the stream with `disconnected` / `token_expired` when it passes. The client refreshes its token and reconnects;
  reconnecting with the expired one is a 401.
- **Metrics:** `middleware.Metrics` still counts a `text/event-stream` response but leaves it out of the duration
  and response-size histograms (a stream lasts minutes and would skew p95/p99).
- **Multiple instances** are exercised by `TestStream_ChangesReachClientsOnEveryInstance`
  (`activity/internal/infrastructure/persistence/stream_multi_instance_integration_test.go`): two hubs, each with
  its own NATS connection, a stream client on each; a change reaches both, snapshots only move forward, one hub
  shutting down (`server_shutdown`) leaves the other's client alone, and a reconnect resyncs from a fresh
  snapshot. Real processes, the gateway and failure/latency checks are manual (ticket gl5).
- **Guarantees:** a client never gets an older snapshot than its last (version check); a client more than 16
  updates behind is disconnected (`slow_consumer`) instead of blocking others; a viewer removed from the session
  (or no longer able to see it) is disconnected.
- **Timeouts:** the handler lifts the server's WriteTimeout for its response (`http.ResponseController`, which
  reaches the real writer through the middlewares' `responseWriter.Unwrap`). In the gateway the route is declared
  in `Route.StreamPaths` and proxied by `ReverseProxy.StreamToService`: no WriteTimeout, no per-service context
  timeout, but connecting to the backend (dial timeout) and receiving its headers
  (`Transport.ResponseHeaderTimeout`) are each bounded by the service timeout.
  The client hanging up cancels the backend request.

### Outbox wake-up (q2l)

Every service's `event_outbox` has an `AFTER INSERT ... FOR EACH STATEMENT` trigger that runs
`pg_notify('outbox_events', <schema>)`; Postgres delivers it on commit, so the publisher never sees an
uncommitted event. `outbox-publisher` LISTENs on a connection of its own (`events.ListenOutbox`, built on
[`jackc/pgxlisten`](https://github.com/jackc/pgxlisten), which reconnects every 2s after a failure) and publishes the named schema at once, draining full batches but stopping at the first failed event
(retries keep the poll's pace). After every (re)connect it does a full pass, since notifications sent while it
was down are lost. `OUTBOX_POLL_INTERVAL` (5s) stays as the fallback; `OUTBOX_LISTEN=false` turns the listener off.

### Team formation notifications (6gg.5)

The notifications service turns team formation events into in-app notifications
(`notifications/internal/interfaces/events/team_formation_handlers.go`). Recipients come from the event payload
(activity is never queried); whoever triggered the event is skipped. Every `data` carries `session_id` and a
deep-link `screen` (`team_draft`, `team_voting`, `session_teams`).

| Event | Notification type | Recipients |
|---|---|---|
| `draft.started` | `team_draft_captain_selected` | both captains (`team_position`) |
| `draft.turn_changed` | `team_draft_your_turn` | the captain whose turn it is |
| `draft.paused` | `team_draft_paused` | the organizer who started the draft |
| `draft.completed` | `team_draft_completed` | every drafted player (`team_position`) |
| `draft.cancelled` | `team_draft_cancelled` | people going (not for `session_ended`) |
| `voting.opened` | `team_voting_opened` | people going (`proposal_id`) |
| `voting.closed` | `team_voting_closed` | people going (`winner_proposal_id` / `kept_current`) |
| `voting.cancelled` | `team_voting_cancelled` | people going, only for `not_enough_players` |
| `teams_reset` | `team_formation_reset` | everyone going afterwards, except who caused it (`reason`: `roster_changed` / `organizer`) |
| `voting.tied` | `team_voting_tied` | session managers (group admins/creator + session creator) except the one who pressed Close |
| `attendee.team_changed` | `team_changed` | the moved attendee (manual moves only) |

A tie makes Close roll back, so `voting.tied` is written in a second transaction after the 409 is decided
(best effort). `session_team_voting_round.last_tie_key` (`TieError.Key()`: the sorted tied options) dedupes it:
repeating Close on the same tie notifies once, a changed tied set notifies again. Managers are resolved in
activity (`manager_user_ids`) so notifications never reads membership data.


====================================================================================================

## Domain Value Objects

The implementation uses rich value objects for type safety and validation:

**Title:**
- Cannot be empty
- Max 100 characters
- Encapsulates activity group title

**Description:**
- Cannot be empty
- Max 500 characters
- Encapsulates activity group description

**SessionLocation:**
- city: required
- country: required
- latitude: -90 to 90
- longitude: -180 to 180
- Ensures valid geographic coordinates

**SessionSchedule:**
- startTime: must be in future (UTC)
- endTime: optional, must be after startTime
- All times converted to UTC

**ActivityType:**
- Predefined enum from `shared/activitycatalog` (running, walking, jogging, basketball, football, tennis,
  volleyball, gym, dancing, skiing, climbing, cycling, swimming, hiking, yoga, weightlifting, other)
- `IsTeamSport()`: basketball, football, volleyball
- Type-safe, prevents invalid activity types

**DifficultyLevel:**
- beginner, intermediate, advanced
- Used for both groups and user skill levels

**Visibility:**
- public: discoverable by all users
- private: invitation only

====================================================================================================

## Statistics

- ActivityStatistics:
    - trakcs group membership stats
    - members by role
    - handles all member lifecycle events:
        - joim request, approved, rejected
        - left, removed
        - role changed
        - invite accepted
        - invite link used
    - created with 1 confirmed (creator) member

- ActivitySessionStatistics:
    - tracks session stats
    - initalized with auto-attendee count
        - NewSessionStatistics(sessionID, activityID, autoConfirmed, autoPending)
    - handles all attendee lifecycle events:
        - rsvp going/not_going/maybe/pending
        - promoted from waitlist
        - auto confirmed/pending
    - heldspot bool on not_going/maybe
        - tells stats if a confirmed spot was released

## How service layer uses these

Session creation:
- NewActivitySession() -> returns (session, attendees, error)
- count auto-confirmed and auto-pending from atendee list
- NewSessionStatistics(sessionID, activityID, autoConfirmed, autoPending)

RSVP going:
- Load SessionStatistics
- currentConfirmed = sessionStats.TotalGoing()
- NewRSVPAttendee()
- Update stats:
    - going -> stats.OnAttendeeRSVPGoing()
    - pending -> stats.OnAttendeeRSVPPending()
- save both in transaction

Spot released (not_going/maybe with heldspot):
- stats.ONAttendeeRSVPNotGoing(heldspot):
    - heldspot: true -> find first pending attendee
    - attendee.Promote()
    - stats.OnAttendeePromoted()
- Save all in transaction


====================================================================================================
## Database Schema

All activity tables use the `activity` schema for namespace isolation (DDD bounded context).

**Key Tables:**

1. **activity_group** - Main aggregate root
   - title, description, activity_type, difficulty_level
   - visibility (public/private), status (draft/active/cancelled)
   - location (city/country for discovery)
   - timezone, default_capacity
   - Indexed for discovery: location + visibility + status

2. **member** - Group membership
   - Links account_id to activity_group_id
   - status: pending, confirmed, rejected, left
   - role: member, admin, creator
   - Unique constraint: one active membership per user per group
   - Indexed for permission checks and member lists

3. **group_invites** - Direct user invitations
   - Invites specific user (invited_user_id)
   - Expires after set time
   - status: pending, accepted, declined
   - Unique: one pending invite per user per group

4. **group_invite_links** - Shareable invite links
   - Unique token for sharing
   - Can be used by multiple users
   - status: active, expired, revoked
   - Indexed on token for fast lookup

5. **group_invite_link_usage** - Audit trail
   - Tracks which users used which links
   - Prevents duplicate usage
   - Unique: (link_id, account_id)

6. **session_template** - Recurring session definitions
   - Defines recurrence rules (frequency, interval, day/time)
   - Has default capacity for generated sessions
   - Tracks generated_up_to for cron jobs
   - status: active, inactive

7. **session** - Individual session instances
   - Can be manual or generated from template
   - location (coordinates for mapping)
   - start_time, end_time, capacity
   - status: scheduled, started, cancelled, completed
   - deleted_at: soft delete, set when the owning group is deleted (excluded from every read)
   - is_recurring flag
   - Indexed for upcoming sessions and location queries
   - team_count, min_players_per_team (NULL team_count = no teams yet; set by create-teams)

   - **session_team** - Teams of a session (Team A, Team B, ...)
     - name, optional color, position (unique per session)

   - **session_team_draft** / **session_team_draft_pick** - Captain drafts and their picks
     - pick_order, status (active/paused/completed/cancelled), paused_reason, cancel_reason, captains (NULL =
       vacant while paused), turn, version, min_players_per_team, team_colors
     - Partial unique index: one active or paused draft per session

   - **session_team_voting_round** / **session_team_proposal** / **session_team_proposal_member** /
     **session_team_vote** - Team voting rounds; proposals, members and votes exist only while the round is open
     - Partial unique index: one open round per session

8. **session_attendee** - Session participants
   - Links account_id to session_id
   - status: going, pending, not_going, maybe, promoted
   - source: auto_confirmed, auto_pending, rsvp_manual, requested, invited
   - Indexed for waitlist FIFO (session_id, created_at)
   - Unique: one active attendance per user per session
   - team_id: nullable, composite FK (team_id, session_id) -> session_team so an attendee can only be on
     a team of their own session

   - **session_invites** - Direct invitations to standalone sessions
     - Same shape as group_invites, keyed on session_id instead of activity_group_id
     - Partial unique index: one pending invite per user per session

9. **activity_group_statistics** - Group stats
   - total_members, total_pending, total_left, total_rejected
   - Role breakdown: creators, admins, members

10. **session_statistics** - Session stats
    - total_going, total_pending, total_not_going, total_maybe, total_promoted
    - Updated via domain events

11. **event_outbox** - Event publishing
    - Stores domain events before publishing to NATS
    - Ensures at-least-once delivery
    - Processed by outbox publisher service

**Important Indexes:**
- Discovery queries: location + visibility + status on activity_group
- Permission checks: (activity_group_id, account_id) on member
- Waitlist promotion: (session_id, created_at, status='pending') on session_attendee
- Token lookup: token on group_invite_links
- Recurring jobs: recurrence_frequency, generated_up_to on session_template

For complete schema, see: `activity/internal/infrastructure/persistence/migrations/000001_activity_schema.up.sql`
