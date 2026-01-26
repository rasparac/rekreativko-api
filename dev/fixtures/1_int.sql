CREATE EXTENSION IF NOT EXISTS "uuid-ossp" WITH SCHEMA public;

CREATE EXTENSION citext WITH SCHEMA public;

CREATE TABLE IF NOT EXISTS accounts(
    id uuid PRIMARY KEY DEFAULT uuid_generate_v4(),
    phone_number CITEXT NOT NULL,
    password text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT NOW(),
    updated_at timestamptz NOT NULL DEFAULT NOW(),
    deleted_at timestamptz DEFAULT NULL,
    expires_at timestamptz DEFAULT NULL
);

CREATE UNIQUE INDEX accounts_phonenumber_uq_idx ON accounts(phone_number)
WHERE (deleted_at IS NULL);

CREATE INDEX accounts_updated_at_idx ON accounts(updated_at);

CREATE INDEX accounts_created_at_idx ON accounts(created_at);

CREATE INDEX accounts_expires_at_idx ON accounts(expires_at);

CREATE TABLE IF NOT EXISTS account_profiles(
    account_id uuid NOT NULL,
    email CITEXT NOT NULL,
    nickname CITEXT NOT NULL,
    first_name text NOT NULL,
    last_name text NOT NULL,
    city text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT NOW(),
    updated_at timestamptz NOT NULL DEFAULT NOW(),
    CONSTRAINT fk_account_id FOREIGN KEY (account_id) REFERENCES accounts(id) ON UPDATE CASCADE ON DELETE CASCADE
);

CREATE UNIQUE INDEX account_profiles_account_uuid_uq_idx ON account_profiles(account_id);

CREATE UNIQUE INDEX accounts_profiles_email_uq_idx ON account_profiles(email);

CREATE UNIQUE INDEX accounts_profiles_nickname_uq_idx ON account_profiles(nickname);

CREATE INDEX accounts_profiles_created_at_idx ON account_profiles(created_at);

CREATE INDEX accounts_profiles_updated_at_idx ON account_profiles(updated_at);

CREATE TABLE IF NOT EXISTS activity_types(
    id uuid PRIMARY KEY DEFAULT uuid_generate_v4(),
    name text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX activity_types_name_uq_idx ON activity_types(name);

INSERT INTO activity_types(name)
VALUES
    ('basketball'),
('football'),
('tennis'),
('volleyball');

CREATE TABLE activity_locations(
    id uuid PRIMARY KEY DEFAULT uuid_generate_v4(),
    name text NOT NULL,
    activity_location_type text NOT NULL,
    address text NOT NULL,
    latitude numeric(20, 18) NOT NULL,
    longitude numeric(20, 17) NOT NULL,
    created_at timestamptz NOT NULL DEFAULT NOW(),
    updated_at timestamptz NOT NULL DEFAULT NOW()
);

CREATE INDEX activity_locations_name_idx ON activity_locations(name);

CREATE INDEX activity_locations_type_idx ON activity_locations(activity_location_type);

CREATE INDEX activity_locations_address_idx ON activity_locations(address);

CREATE INDEX activity_locations_lat_lng_idx ON activity_locations(latitude, longitude);

CREATE TABLE IF NOT EXISTS account_profile_activity_types(
    account_id uuid NOT NULL,
    activity_type_id uuid NOT NULL,
    created_at timestamptz NOT NULL DEFAULT NOW(),
    CONSTRAINT fk_account_id FOREIGN KEY (account_id) REFERENCES accounts(id) ON UPDATE CASCADE ON DELETE CASCADE,
    CONSTRAINT fk_activity_type_id FOREIGN KEY (activity_type_id) REFERENCES activity_types(id) ON UPDATE CASCADE ON DELETE RESTRICT
);

CREATE TABLE IF NOT EXISTS activity_groups(
    id uuid PRIMARY KEY DEFAULT uuid_generate_v4(),
    owner_id uuid NOT NULL,
    activity_type_id uuid NOT NULL,
    activity_location_id uuid NOT NULL,
    name text NOT NULL,
    is_public boolean NOT NULL DEFAULT FALSE,
    created_at timestamptz NOT NULL DEFAULT NOW(),
    updated_at timestamptz NOT NULL DEFAULT NOW(),
    deleted_at timestamptz DEFAULT NULL,
    CONSTRAINT fk_activity_location_id FOREIGN KEY (activity_location_id) REFERENCES activity_locations(id) ON UPDATE CASCADE ON DELETE RESTRICT,
    CONSTRAINT fk_owner_id FOREIGN KEY (owner_id) REFERENCES accounts(id) ON UPDATE CASCADE ON DELETE RESTRICT,
    CONSTRAINT fk_activity_type_id FOREIGN KEY (activity_type_id) REFERENCES activity_types(id) ON UPDATE CASCADE ON DELETE RESTRICT
);

CREATE INDEX activity_groups_created_at_idx ON activity_groups(created_at);

CREATE INDEX activity_groups_is_public_idx ON activity_groups(is_public);

CREATE INDEX activity_groups_updated_at_idx ON activity_groups(updated_at);

CREATE INDEX activity_groups_owner_id_idx ON activity_groups(owner_id);

CREATE INDEX activity_groups_activity_type_id_idx ON activity_groups(activity_type_id);

CREATE INDEX activity_groups_public_type_idx ON activity_groups(activity_type_id, is_public);

CREATE INDEX activity_groups_owner_public_idx ON activity_groups(owner_id, is_public);

CREATE INDEX activity_groups_created_desc_idx ON activity_groups(created_at DESC);

CREATE TABLE IF NOT EXISTS activity_group_members(
    activity_group_id uuid NOT NULL,
    account_id uuid NOT NULL,
    created_at timestamptz NOT NULL DEFAULT NOW(),
    CONSTRAINT fk_activity_group_id FOREIGN KEY (activity_group_id) REFERENCES activity_groups(id) ON UPDATE CASCADE ON DELETE CASCADE,
    CONSTRAINT fk_account_id FOREIGN KEY (account_id) REFERENCES accounts(id)
);

CREATE UNIQUE INDEX activity_group_members_group_account_uq_idx ON activity_group_members(activity_group_id, account_id);

CREATE INDEX activity_group_members_activity_group_id_idx ON activity_group_members(activity_group_id);

CREATE INDEX activity_group_members_account_id_idx ON activity_group_members(account_id);

CREATE INDEX activity_group_members_group_created_idx ON activity_group_members(activity_group_id, created_at DESC);

CREATE TABLE IF NOT EXISTS activity_group_invites(
    invite_code text NOT NULL,
    activity_group_id uuid NOT NULL,
    expires_at timestamptz NOT NULL,
    created_at timestamptz NOT NULL DEFAULT NOW(),
    CONSTRAINT fk_activity_group_id FOREIGN KEY (activity_group_id) REFERENCES activity_groups(id) ON UPDATE CASCADE ON DELETE CASCADE
);

CREATE UNIQUE INDEX activity_group_invites_invite_code_uq_idx ON activity_group_invites(invite_code);

CREATE INDEX activity_group_invites_group_id_idx ON activity_group_invites(activity_group_id);

CREATE INDEX activity_group_invites_expires_at_idx ON activity_group_invites(expires_at);

CREATE INDEX activity_group_invites_created_at_idx ON activity_group_invites(created_at);

