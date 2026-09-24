-- Activity schema migration
-- all timestamps are utc (timestamptz)
-- separate schema per boudned context (DDD)
CREATE EXTENSION IF NOT EXISTS "uuid-ossp" WITH SCHEMA public;

CREATE EXTENSION IF NOT EXISTS citext;

CREATE SCHEMA IF NOT EXISTS activity;

-- activity table
CREATE TABLE IF NOT EXISTS activity.activity_group(
    id uuid PRIMARY KEY DEFAULT uuid_generate_v4(),
    creator_id uuid NOT NULL,
    title text NOT NULL,
    description text NOT NULL,
    activity_type varchar(200) NOT NULL,
    difficulty_level varchar(50) NOT NULL DEFAULT 'beginner',
    visibility varchar(50) NOT NULL DEFAULT 'public',
    status varchar(50) NOT NULL DEFAULT 'draft',
    -- location (city/country for discovery, no coordinates)
    location_city varchar(100) DEFAULT NULL,
    location_country varchar(100) DEFAULT NULL,
    timezone varchar(100) NOT NULL DEFAULT 'UTC',
    default_capacity int DEFAULT NULL, -- NULL means no limit
    created_at timestamptz NOT NULL DEFAULT NOW(),
    updated_at timestamptz NOT NULL DEFAULT NOW(),
    cancelled_at timestamptz DEFAULT NULL,
    deleted_at timestamptz DEFAULT NULL
);

CREATE INDEX idx_activity_group_created_at ON activity.activity_group(created_at);

CREATE INDEX idx_activity_group_updated_at ON activity.activity_group(updated_at);

CREATE INDEX idx_activity_group_deleted_at ON activity.activity_group(deleted_at)
WHERE (deleted_at IS NOT NULL);

-- creator lookup INDEX
CREATE INDEX idx_activity_group_creator_id ON activity.activity_group(creator_id);

-- discovery queries: find active public groups by location
CREATE INDEX idx_activity_group_location ON activity.activity_group(location_city, location_country)
WHERE (visibility = 'public') AND (status = 'active');

-- filter by activity type
CREATE INDEX idx_activity_group_activity_type ON activity.activity_group(activity_type)
WHERE (visibility = 'public') AND (status = 'active');

-- group member
CREATE TABLE IF NOT EXISTS activity.member(
    id uuid PRIMARY KEY DEFAULT uuid_generate_v4(),
    activity_group_id uuid NOT NULL REFERENCES activity.activity_group(id),
    account_id uuid NOT NULL,
    status varchar(100) NOT NULL DEFAULT 'pending',
    role varchar(100) NOT NULL DEFAULT 'member',
    is_priority boolean NOT NULL DEFAULT FALSE,
    joined_at timestamptz NOT NULL DEFAULT NOW(),
    decided_at timestamptz DEFAULT NULL,
    left_at timestamptz DEFAULT NULL,
    deleted_at timestamptz DEFAULT NULL,
    CONSTRAINT uq_active_membership UNIQUE NULLS NOT DISTINCT (activity_group_id, account_id, deleted_at)
);

-- premission checks: find member by group + user
CREATE INDEX idx_members_groupuser ON activity.member(activity_group_id, account_id)
WHERE (deleted_at IS NULL);

-- find all members of a group
CREATE INDEX idx_members_group ON activity.member(activity_group_id)
WHERE (deleted_at IS NULL);

-- find all groups a user belongs to
CREATE INDEX idx_members_user ON activity.member(account_id)
WHERE (deleted_at IS NULL);

-- find pending members
CREATE INDEX idx_members_pending ON activity.member(activity_group_id, joined_at)
WHERE (status = 'pending') AND (deleted_at IS NULL);

-- find priority members in a group (for early RSVP access)
CREATE INDEX idx_member_priority ON activity.member(activity_group_id, is_priority)
WHERE (is_priority = TRUE) AND (deleted_at IS NULL);

-- group invites (direct - specific user)
CREATE TABLE IF NOT EXISTS activity.group_invites(
    id uuid PRIMARY KEY DEFAULT uuid_generate_v4(),
    activity_group_id uuid NOT NULL REFERENCES activity.activity_group(id),
    invited_user_id uuid NOT NULL,
    invited_by_user_id uuid NOT NULL,
    status varchar(100) NOT NULL DEFAULT 'pending',
    expires_at timestamptz NOT NULL,
    created_at timestamptz NOT NULL DEFAULT NOW(),
    responded_at timestamptz DEFAULT NULL,
    -- one pending invite per user per group at time
    CONSTRAINT uq_pending_invites UNIQUE NULLS NOT DISTINCT (activity_group_id, invited_user_id, responded_at)
);

-- find pending invite for a user in a group
CREATE INDEX idx_group_invites_pending ON activity.group_invites(activity_group_id, invited_user_id)
WHERE (status = 'pending');

-- cron job: find expired invites
CREATE INDEX idx_group_invites_expired ON activity.group_invites(expires_at)
WHERE (status = 'pending');

-- group invite links
CREATE TABLE IF NOT EXISTS activity.group_invite_links(
    id uuid PRIMARY KEY DEFAULT uuid_generate_v4(),
    activity_group_id uuid NOT NULL REFERENCES activity.activity_group(id),
    created_by_user_id uuid NOT NULL,
    token text NOT NULL UNIQUE,
    status varchar(100) NOT NULL DEFAULT 'active',
    created_at timestamptz NOT NULL DEFAULT NOW(),
    expires_at timestamptz NOT NULL,
    revoked_at timestamptz DEFAULT NULL
);

-- token lookup (most common query - user clicks link)
CREATE INDEX idx_group_invite_links_token ON activity.group_invite_links(token)
WHERE (status = 'active');

-- find active links for a group
CREATE INDEX idx_group_invite_links_active ON activity.group_invite_links(activity_group_id)
WHERE (status = 'active');

-- cron job: find expired links
CREATE INDEX idx_group_invite_links_expired ON activity.group_invite_links(expires_at)
WHERE (status = 'active');

-- group invite link usage (audit tral per user)
CREATE TABLE IF NOT EXISTS activity.group_invite_link_usage(
    id uuid PRIMARY KEY DEFAULT uuid_generate_v4(),
    group_invite_link_id uuid NOT NULL REFERENCES activity.group_invite_links(id),
    account_id uuid NOT NULL,
    status varchar(100) NOT NULL DEFAULT 'used',
    created_at timestamptz NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_link_usage UNIQUE (group_invite_link_id, account_id)
);

-- find usage by link (check if already used)
CREATE INDEX idx_group_invite_link_usage_link ON activity.group_invite_link_usage(group_invite_link_id, status)
WHERE (status = 'used');

-- ============================================================
-- session_template table
-- One group can have multiple templates (e.g. "Monday yoga",
-- "Saturday bootcamp"). Holds all recurrence config.
-- ============================================================
CREATE TABLE IF NOT EXISTS activity.session_template(
    id uuid PRIMARY KEY DEFAULT uuid_generate_v4(),
    activity_group_id uuid NOT NULL REFERENCES activity.activity_group(id),
    created_by_id uuid NOT NULL,
    title varchar(200) NOT NULL,
    description text DEFAULT NULL,
    -- default capacity for sessions generated from this template
    -- NULL = no limit
    capacity int DEFAULT NULL CHECK (capacity IS NULL OR capacity > 0),
    -- default location, inherited by every session generated from this template
    location_city varchar(100) NOT NULL,
    location_country varchar(100) NOT NULL,
    location_lat DECIMAL(9, 6) NOT NULL,
    location_lng DECIMAL(9, 6) NOT NULL,
    -- optional venue/address line, e.g. "Ada Ciganlija bb, Court 3"
    location_street varchar(255) DEFAULT NULL,
    -- tracks how far ahead sessions have been generated
    -- cron job generates sessions from this point forward
    generated_up_to timestamptz DEFAULT NULL,
    status varchar(50) NOT NULL DEFAULT 'active',
    -- recurrence config (all nullable = template is not recurring)
    recurrence_frequency varchar(50) DEFAULT NULL, -- 'daily','weekly','monthly'
    recurrence_interval smallint DEFAULT NULL, -- every N frequencies
    recurrence_day_of_week smallint DEFAULT NULL, -- 0=Sun..6=Sat
    recurrence_day_of_month smallint DEFAULT NULL,
    recurrence_time_hour smallint DEFAULT NULL CHECK (recurrence_time_hour BETWEEN 0 AND 23),
    recurrence_time_minute smallint DEFAULT NULL CHECK (recurrence_time_minute BETWEEN 0 AND 59),
    recurrence_ends_at timestamptz DEFAULT NULL,
    created_at timestamptz NOT NULL DEFAULT NOW(),
    updated_at timestamptz NOT NULL DEFAULT NOW(),
    deleted_at timestamptz DEFAULT NULL
);

-- find all templates for a group (most common query)
CREATE INDEX idx_session_template_group ON activity.session_template(activity_group_id)
WHERE (deleted_at IS NULL);

-- cron job: find active recurring templates that need new sessions generated
CREATE INDEX idx_session_template_recurring ON activity.session_template(generated_up_to)
WHERE
    recurrence_frequency IS NOT NULL AND status = 'active' AND deleted_at IS NULL;

-- ============================================================
-- session table (revised)
-- session_template_id = NULL  → manually created, never recurring
-- session_template_id = <id>  → auto-generated from template
-- ============================================================
CREATE TABLE IF NOT EXISTS activity.session(
    id uuid PRIMARY KEY DEFAULT uuid_generate_v4(),
    -- NULL = standalone session with no group (always public - see visibility below)
    activity_group_id uuid DEFAULT NULL REFERENCES activity.activity_group(id),
    session_template_id uuid DEFAULT NULL REFERENCES activity.session_template(id),
    created_by_id uuid NOT NULL,
    title varchar(200) NOT NULL,
    -- inherited from the group when activity_group_id is set, required from the
    -- user when standalone; always required from the template when generated
    activity_type varchar(50) NOT NULL,
    -- same inheritance rule as activity_type above
    difficulty_level varchar(50) NOT NULL DEFAULT 'beginner',
    location_lat DECIMAL(9, 6) NOT NULL,
    location_lng DECIMAL(9, 6) NOT NULL,
    location_city varchar(100) NOT NULL,
    location_country varchar(100) NOT NULL,
    -- optional venue/address line, e.g. "Ada Ciganlija bb, Court 3"
    location_street varchar(255) DEFAULT NULL,
    start_time timestamptz NOT NULL,
    -- end_time is optional - a standalone session may have no fixed end time
    end_time timestamptz,
    -- capacity == NULL means no limit
    capacity int NULL,
    status varchar(100) NOT NULL DEFAULT 'scheduled',
    -- visibility: 'private' (group members only) or 'public' (anyone can RSVP as an attendee, without joining the group)
    visibility varchar(20) NOT NULL DEFAULT 'private',
    -- requires_approval: if true, RSVPing "going" creates a pending join request
    -- the creator/admin must approve rather than joining immediately
    requires_approval boolean NOT NULL DEFAULT FALSE,
    is_recurring boolean NOT NULL DEFAULT FALSE,
    note text DEFAULT NULL,
    -- open_at: when regular members can start RSVPing (NULL = immediately open)
    -- priority members can RSVP anytime regardless of this timestamp
    open_at timestamptz DEFAULT NULL,
    created_at timestamptz NOT NULL DEFAULT NOW(),
    updated_at timestamptz NOT NULL DEFAULT NOW(),
    cancelled_at timestamptz DEFAULT NULL,
    started_at timestamptz DEFAULT NULL,
    completed_at timestamptz DEFAULT NULL
);

COMMENT ON COLUMN activity.session.session_template_id IS 'Links to the template that generated this session (NULL for manual sessions)';
COMMENT ON COLUMN activity.session.location_city IS 'City name for the session location';
COMMENT ON COLUMN activity.session.location_country IS 'Country name for the session location';

-- find sessions for a group
CREATE INDEX idx_sessions_group ON activity.session(activity_group_id, start_time)
WHERE (status != 'cancelled');

-- find upcoming scheduled sessions
CREATE INDEX idx_sessions_scheduled ON activity.session(start_time)
WHERE (status = 'scheduled');

-- cronjob: find recurring sessions to generate next
CREATE INDEX idx_sessions_recurring ON activity.session(activity_group_id, completed_at)
WHERE (is_recurring = TRUE) AND (status = 'completed');

-- location based queries: find active public sessions by location
CREATE INDEX idx_sessions_location ON activity.session(location_lat, location_lng)
WHERE (status = 'scheduled') AND (visibility = 'public');

-- find all sessions generated from a template
CREATE INDEX idx_sessions_template ON activity.session(session_template_id)
WHERE (session_template_id IS NOT NULL);

-- find sessions opening soon (for notifications to regular members)
CREATE INDEX idx_session_open_at ON activity.session(open_at)
WHERE (open_at IS NOT NULL) AND (status = 'scheduled');

-- session attendees
CREATE TABLE IF NOT EXISTS activity.session_attendee(
    id uuid PRIMARY KEY DEFAULT uuid_generate_v4(),
    session_id uuid NOT NULL REFERENCES activity.session(id),
    -- NULL for an attendee of a standalone session with no group
    activity_group_id uuid DEFAULT NULL REFERENCES activity.activity_group(id),
    account_id uuid NOT NULL,
    status varchar(100) NOT NULL DEFAULT 'pending',
    source varchar(100) NOT NULL DEFAULT 'auto_pending',
    created_at timestamptz NOT NULL DEFAULT NOW(),
    updated_at timestamptz NOT NULL DEFAULT NOW(),
    deleted_at timestamptz DEFAULT NULL,
    CONSTRAINT uq_active_attendees UNIQUE NULLS NOT DISTINCT (session_id, account_id, deleted_at)
);

-- permission + rsvp check: find attendee by session + user
CREATE INDEX idx_attendees_session_user ON activity.session_attendee(session_id, account_id)
WHERE (deleted_at IS NULL);

-- waitlist promotion: find fist pending attendee (FIFO)
CREATE INDEX idx_attendees_pending_fifo ON activity.session_attendee(session_id, created_at)
WHERE (status = 'pending') AND (deleted_at IS NULL);

-- count confirmed attendees
CREATE INDEX idx_attendees_confirmed_count ON activity.session_attendee(session_id)
WHERE (status = 'confirmed') AND (deleted_at IS NULL);

-- find all sessions a user is attending
CREATE INDEX idx_attendees_user ON activity.session_attendee(account_id, status)
WHERE (deleted_at IS NULL);

-- session invites (direct - specific user, standalone sessions only)
CREATE TABLE IF NOT EXISTS activity.session_invites(
    id uuid PRIMARY KEY DEFAULT uuid_generate_v4(),
    session_id uuid NOT NULL REFERENCES activity.session(id),
    invited_user_id uuid NOT NULL,
    invited_by_user_id uuid NOT NULL,
    status varchar(100) NOT NULL DEFAULT 'pending',
    expires_at timestamptz NOT NULL,
    created_at timestamptz NOT NULL DEFAULT NOW(),
    responded_at timestamptz DEFAULT NULL
);

-- one pending invite per user per session at a time
CREATE UNIQUE INDEX uq_session_invites_pending ON activity.session_invites(session_id, invited_user_id)
WHERE (status = 'pending');

-- list a user's pending invites
CREATE INDEX idx_session_invites_user_pending ON activity.session_invites(invited_user_id, created_at DESC)
WHERE (status = 'pending');

-- cron job: find expired invites
CREATE INDEX idx_session_invites_expired ON activity.session_invites(expires_at)
WHERE (status = 'pending');

-- group statistics
CREATE TABLE IF NOT EXISTS activity.activity_group_statistics(
    activity_group_id uuid PRIMARY KEY NOT NULL REFERENCES activity.activity_group(id),
    total_members int NOT NULL DEFAULT 1,
    total_pending int NOT NULL DEFAULT 0,
    total_left int NOT NULL DEFAULT 0,
    total_rejected int NOT NULL DEFAULT 0,
    -- role breakdwon
    total_creators int NOT NULL DEFAULT 0,
    total_admins int NOT NULL DEFAULT 0,
    total_member_roles int NOT NULL DEFAULT 0,
    updated_at timestamptz NOT NULL DEFAULT NOW()
);

-- session statistics
CREATE TABLE IF NOT EXISTS activity.session_statistics(
    session_id uuid PRIMARY KEY NOT NULL REFERENCES activity.session(id),
    -- NULL for a standalone session with no group
    activity_group_id uuid DEFAULT NULL REFERENCES activity.activity_group(id),
    total_going int NOT NULL DEFAULT 0,
    total_pending int NOT NULL DEFAULT 0,
    total_not_going int NOT NULL DEFAULT 0,
    total_maybe int NOT NULL DEFAULT 0,
    total_promoted int NOT NULL DEFAULT 0,
    updated_at timestamptz NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_session_statistics_group ON activity.session_statistics(activity_group_id);

-- activity event outbox table
CREATE TABLE IF NOT EXISTS activity.event_outbox(
    event_id uuid PRIMARY KEY NOT NULL,
    event_type text NOT NULL,
    aggregate_id uuid NOT NULL,
    payload jsonb NOT NULL,
    created_at timestamptz NOT NULL DEFAULT NOW(),
    published_at timestamptz DEFAULT NULL,
    -- publish failure tracking; failed_at set = dead-lettered, no longer polled
    retry_count integer NOT NULL DEFAULT 0,
    last_error text DEFAULT NULL,
    failed_at timestamptz DEFAULT NULL
);

CREATE UNIQUE INDEX idx_event_outbox_event_id ON activity.event_outbox(event_id);

CREATE INDEX idx_event_outbox_created_at ON activity.event_outbox(created_at);

CREATE INDEX idx_event_outbox_published_at ON activity.event_outbox(published_at);

-- Pending (not published, not dead-lettered) events, in publish order.
CREATE INDEX idx_event_outbox_pending ON activity.event_outbox(created_at)
    WHERE published_at IS NULL AND failed_at IS NULL;

