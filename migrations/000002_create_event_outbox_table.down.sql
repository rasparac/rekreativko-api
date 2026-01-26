DROP INDEX IF EXISTS event_outbox_event_id_idx;

DROP INDEX IF EXISTS event_outbox_created_at_idx;

DROP INDEX IF EXISTS event_outbox_published_at_idx;

DROP TABLE IF EXISTS event_outbox;

