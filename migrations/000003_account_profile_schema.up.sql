CREATE EXTENSION IF NOT EXISTS "uuid-ossp" WITH SCHEMA public;

CREATE EXTENSION IF NOT EXISTS citext;

CREATE SCHEMA IF NOT EXISTS "account_profile";

CREATE TABLE account_profile.profiles(
    account_id uuid PRIMARY KEY, -- from Identity context
    nickname CITEXT DEFAULT NULL,
    full_name text DEFAULT NULL,
    date_of_birth date DEFAULT NULL,
    location_city varchar(100) DEFAULT NULL,
    location_country varchar(100) DEFAULT NULL,
    location_latitude DECIMAL(10, 8) DEFAULT NULL,
    location_longitude DECIMAL(11, 8) DEFAULT NULL,
    location_region varchar(100) DEFAULT NULL,
    bio text DEFAULT NULL,
    profile_picture_url text DEFAULT NULL,
    profile_picture_uploaded_at timestamptz DEFAULT NULL,
    created_at timestamptz NOT NULL DEFAULT NOW(),
    updated_at timestamptz NOT NULL DEFAULT NOW(),
    deleted_at timestamptz DEFAULT NULL
);

CREATE INDEX idx_profiles_deleted_at ON account_profile.profiles(deleted_at);

CREATE INDEX idx_profiles_created_at ON account_profile.profiles(created_at);

CREATE TABLE account_profile.activity_interests(
    id uuid PRIMARY KEY DEFAULT uuid_generate_v4(),
    account_id uuid NOT NULL,
    activity_type varchar(100) NOT NULL,
    activity_level varchar(50) NOT NULL DEFAULT 'beginner',
    created_at timestamptz NOT NULL DEFAULT NOW(),
    updated_at timestamptz NOT NULL DEFAULT NOW(),
    CONSTRAINT fk_activity_interests_profile FOREIGN KEY (account_id) REFERENCES account_profile.profiles(account_id)
);

CREATE INDEX idx_activity_interests_created_at ON account_profile.activity_interests(created_at);

CREATE UNIQUE INDEX idx_activity_interests_account_activity ON account_profile.activity_interests(account_id, activity_type);

CREATE TABLE account_profile.settings(
    account_id uuid NOT NULL,
    key varchar(100) NOT NULL,
    value varchar(100) NOT NULL,
    type varchar(100) NOT NULL,
    updated_at timestamptz NOT NULL DEFAULT NOW(),
    CONSTRAINT settings_pkey PRIMARY KEY (account_id, key),
    CONSTRAINT fk_settings_profile FOREIGN KEY (account_id) REFERENCES account_profile.profiles(account_id)
);

CREATE INDEX idx_settings_updated_at ON account_profile.settings(updated_at);

CREATE TABLE IF NOT EXISTS account_profile.profile_statistics(
    account_id uuid PRIMARY KEY,
    activities_joined int NOT NULL,
    last_active_at timestamptz,
    most_active_activity varchar(50),
    activities_by_type jsonb NOT NULL,
    monthly_breakdown jsonb NOT NULL,
    achievements jsonb NOT NULL,
    updated_at timestamptz NOT NULL DEFAULT NOW(),
    CONSTRAINT fk_statistics_profile FOREIGN KEY (account_id) REFERENCES account_profile.profiles(account_id)
);

CREATE INDEX idx_statistics_updated_at ON account_profile.profile_statistics(updated_at);

CREATE INDEX idx_statistics_activities_by_type ON account_profile.profile_statistics USING GIN(activities_by_type);

CREATE INDEX idx_statistics_monthly_breakdown ON account_profile.profile_statistics USING GIN(monthly_breakdown);

CREATE TABLE account_profile.account_settings_meta(
    account_id uuid PRIMARY KEY,
    version int NOT NULL DEFAULT 1,
    created_at timestamptz NOT NULL DEFAULT NOW(),
    updated_at timestamptz NOT NULL DEFAULT NOW(),
    CONSTRAINT fk_settings_meta_profile FOREIGN KEY (account_id) REFERENCES account_profile.profiles(account_id)
);

---Function to update update_at timestamp
CREATE OR REPLACE FUNCTION account_profile.update_updated_at_column()
    RETURNS TRIGGER
    AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$
LANGUAGE 'plpgsql';

CREATE TRIGGER update_profiles_updated_at_column
    BEFORE UPDATE ON account_profile.profiles
    FOR EACH ROW
    EXECUTE PROCEDURE account_profile.update_updated_at_column();

CREATE TRIGGER update_settings_updated_at_column
    BEFORE UPDATE ON account_profile.settings
    FOR EACH ROW
    EXECUTE PROCEDURE account_profile.update_updated_at_column();

CREATE TRIGGER update_statistics_updated_at_column
    BEFORE UPDATE ON account_profile.profile_statistics
    FOR EACH ROW
    EXECUTE PROCEDURE account_profile.update_updated_at_column();

