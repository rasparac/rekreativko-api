CREATE EXTENSION IF NOT EXISTS "uuid-ossp" WITH SCHEMA public;

CREATE EXTENSION IF NOT EXISTS citext WITH SCHEMA public;

CREATE SCHEMA IF NOT EXISTS "identity";

CREATE TABLE IF NOT EXISTS identity.accounts(
    id uuid PRIMARY KEY DEFAULT uuid_generate_v4(),
    email CITEXT DEFAULT NULL,
    phone_number CITEXT DEFAULT NULL,
    password text NOT NULL,
    status varchar(20) NOT NULL DEFAULT 'pending',
    failed_login_attempts int NOT NULL DEFAULT 0,
    locked_until timestamptz DEFAULT NULL,
    created_at timestamptz NOT NULL DEFAULT NOW(),
    updated_at timestamptz NOT NULL DEFAULT NOW(),
    deleted_at timestamptz DEFAULT NULL
);

CREATE UNIQUE INDEX accounts_phonenumber_uq_idx ON identity.accounts(phone_number)
WHERE (deleted_at IS NULL);

CREATE UNIQUE INDEX accounts_email_uq_idx ON identity.accounts(email)
WHERE (deleted_at IS NULL);

CREATE INDEX accounts_status_idx ON identity.accounts(status);

CREATE INDEX accounts_updated_at_idx ON identity.accounts(updated_at);

CREATE INDEX accounts_created_at_idx ON identity.accounts(created_at);

CREATE TABLE IF NOT EXISTS identity.refresh_tokens(
    id uuid PRIMARY KEY DEFAULT uuid_generate_v4(),
    account_id uuid NOT NULL,
    -- SHA-256 hex digest of the raw token; the raw value is only ever
    -- returned to the client once and is never persisted.
    token_hash text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT NOW(),
    expires_at timestamptz NOT NULL,
    revoked_at timestamptz DEFAULT NULL
);

ALTER TABLE identity.refresh_tokens
    ADD CONSTRAINT fk_account_id FOREIGN KEY (account_id) REFERENCES identity.accounts(id) ON DELETE CASCADE;

CREATE INDEX refresh_tokens_account_id_idx ON identity.refresh_tokens(account_id);

CREATE UNIQUE INDEX refresh_tokens_token_hash_uq_idx ON identity.refresh_tokens(token_hash);

CREATE INDEX refresh_tokens_created_at_idx ON identity.refresh_tokens(created_at);

CREATE INDEX refresh_tokens_revoked_at_idx ON identity.refresh_tokens(revoked_at)
WHERE (revoked_at IS NOT NULL);

CREATE INDEX refresh_tokens_expires_at_idx ON identity.refresh_tokens(expires_at);

CREATE TABLE IF NOT EXISTS identity.verification_codes(
    id uuid PRIMARY KEY DEFAULT uuid_generate_v4(),
    account_id uuid NOT NULL,
    code varchar(6) NOT NULL,
    type varchar(20) NOT NULL,
    expires_at timestamptz NOT NULL,
    created_at timestamptz NOT NULL DEFAULT NOW(),
    used_at timestamptz DEFAULT NULL
);

ALTER TABLE identity.verification_codes
    ADD CONSTRAINT fk_account_id FOREIGN KEY (account_id) REFERENCES identity.accounts(id) ON DELETE CASCADE;

CREATE INDEX verification_codes_account_id_idx ON identity.verification_codes(account_id);

CREATE UNIQUE INDEX verification_codes_account_code_uq_idx ON identity.verification_codes(code);

CREATE INDEX verification_codes_expires_at_idx ON identity.verification_codes(expires_at);

---Function to update update_at timestamp
CREATE OR REPLACE FUNCTION identity.update_updated_at_column()
    RETURNS TRIGGER
    AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$
LANGUAGE 'plpgsql';

---Trigger to update update_at column
CREATE TRIGGER update_updated_at_column
    BEFORE UPDATE ON identity.accounts
    FOR EACH ROW
    EXECUTE PROCEDURE identity.update_updated_at_column();

-- identity event outbox table
CREATE TABLE IF NOT EXISTS identity.event_outbox(
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

CREATE UNIQUE INDEX idx_identity_event_outbox_event_id ON identity.event_outbox(event_id);

CREATE INDEX idx_identity_event_outbox_created_at ON identity.event_outbox(created_at);

CREATE INDEX idx_identity_event_outbox_published_at ON identity.event_outbox(published_at);

-- Pending (not published, not dead-lettered) events, in publish order.
CREATE INDEX idx_identity_event_outbox_pending ON identity.event_outbox(created_at)
    WHERE published_at IS NULL AND failed_at IS NULL;
