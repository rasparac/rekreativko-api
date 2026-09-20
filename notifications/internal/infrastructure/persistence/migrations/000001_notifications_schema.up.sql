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
