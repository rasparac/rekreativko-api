Core part of the applicatio is activity. In the context of the application, an activity group is a group of members who are enjoying the same activity.
In the the group there are three roles: creator, member and admin. The creator is the person who created the activity. The member is the person who is participating in the activity. The admin is the person who is managing the activity.
Each activity group can have multiple sessions. Each session can have multiple attendees.


***Rules***

Acitvity Group rules:
- Creater auto-confrimed member on creation
- Only creator can cancel group
- Any confirmed member can create session
- Recurrence rule optiona (group can be non-recurring)
- Cancelled group -> all future sessions cancelled

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
|    |---- Edit/Cancel: session creator OR group admin/creator
|    |---- Start/Complete: group admin/creator only
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
- Predefined enum: hiking, cycling, running, swimming, yoga, gym, other
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
   - is_recurring flag
   - Indexed for upcoming sessions and location queries

8. **session_attendee** - Session participants
   - Links account_id to session_id
   - status: going, pending, not_going, maybe, promoted
   - source: auto_confirmed, auto_pending, rsvp_manual
   - Indexed for waitlist FIFO (session_id, created_at)
   - Unique: one active attendance per user per session

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

For complete schema, see: `migrations/000004_activity_schema.up.sql`
