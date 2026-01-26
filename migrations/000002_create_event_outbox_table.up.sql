CREATE TABLE event_outbox(
    event_id uuid NOT NULL,
    event_type text NOT NULL,
    aggregate_id uuid NOT NULL,
    payload jsonb NOT NULL,
    created_at timestamptz NOT NULL DEFAULT NOW(),
    published_at timestamptz DEFAULT NULL
);

CREATE UNIQUE INDEX event_outbox_event_id_idx ON event_outbox(event_id);

CREATE INDEX event_outbox_created_at_idx ON event_outbox(created_at);

CREATE INDEX event_outbox_published_at_idx ON event_outbox(published_at);

