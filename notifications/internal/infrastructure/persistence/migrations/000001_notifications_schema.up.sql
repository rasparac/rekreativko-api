CREATE EXTENSION IF NOT EXISTS "uuid-ossp" WITH SCHEMA public;

CREATE SCHEMA IF NOT EXISTS notifications;

CREATE TABLE notifications.notification(
    id uuid PRIMARY KEY DEFAULT uuid_generate_v4(),
    -- id of the domain event that produced this notification. NATS delivers
    -- at-least-once, so (event_id, recipient_account_id) is unique to make
    -- redelivery of an event a no-op instead of a duplicate notification.
    event_id uuid NOT NULL,
    recipient_account_id uuid NOT NULL,
    -- e.g. 'join_request_created', 'invite_accepted' - open-ended on purpose,
    -- see domain.NotificationType.
    type varchar(100) NOT NULL,
    -- type-specific fields (group_id, session_id, actor id, etc.) - kept
    -- schemaless so new notification types don't need a migration.
    data jsonb NOT NULL DEFAULT '{}',
    read_at timestamptz DEFAULT NULL,
    created_at timestamptz NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_notification_event_recipient UNIQUE (event_id, recipient_account_id)
);

-- feed query: a recipient's notifications, newest first, optionally unread-only
CREATE INDEX idx_notifications_recipient_created ON notifications.notification(recipient_account_id, created_at DESC, id DESC);

-- unread-count query
CREATE INDEX idx_notifications_recipient_unread ON notifications.notification(recipient_account_id)
WHERE (read_at IS NULL);

-- retention sweep: notifications older than the retention window, across all recipients
CREATE INDEX idx_notifications_created ON notifications.notification(created_at);

-- Accounts deleted in identity (identity.account.deleted). Nothing more is
-- delivered to them and no feed rows are created for them; their existing feed
-- stays until the erasure, at the end of identity's grace period.
CREATE TABLE notifications.deleted_account(
    account_id uuid PRIMARY KEY,
    deleted_at timestamptz NOT NULL DEFAULT NOW()
);

-- Accounts whose notifications were erased on identity.account.erased (GDPR
-- Art. 17). Makes the erasure idempotent, lets a late activity event be dropped
-- instead of re-creating a notification, and lists the erasures to replay
-- after a backup restore.
CREATE TABLE notifications.erased_account(
    account_id uuid PRIMARY KEY,
    erased_at timestamptz NOT NULL DEFAULT NOW()
);

-- notifications event outbox table
CREATE TABLE IF NOT EXISTS notifications.event_outbox(
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

CREATE INDEX idx_notifications_event_outbox_created_at ON notifications.event_outbox(created_at);

CREATE INDEX idx_notifications_event_outbox_published_at ON notifications.event_outbox(published_at);

-- Pending (not published, not dead-lettered) events, in publish order.
CREATE INDEX idx_notifications_event_outbox_pending ON notifications.event_outbox(created_at)
    WHERE published_at IS NULL AND failed_at IS NULL;

-- Dead-lettered events, for the purge of those past their retention window.
CREATE INDEX idx_notifications_event_outbox_failed_at ON notifications.event_outbox(failed_at)
    WHERE failed_at IS NOT NULL;

-- Wake the outbox publisher as soon as a transaction that wrote events
-- commits (NOTIFY is delivered on commit) instead of on its next poll. The
-- payload is the schema; the publisher's poll stays as the fallback.
CREATE OR REPLACE FUNCTION notifications.notify_event_outbox() RETURNS trigger AS $$
BEGIN
    PERFORM pg_notify('outbox_events', TG_TABLE_SCHEMA);
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_event_outbox_notify
    AFTER INSERT ON notifications.event_outbox
    FOR EACH STATEMENT EXECUTE FUNCTION notifications.notify_event_outbox();
