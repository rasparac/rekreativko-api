CREATE EXTENSION IF NOT EXISTS "uuid-ossp" WITH SCHEMA public;

CREATE EXTENSION IF NOT EXISTS citext WITH SCHEMA public;

CREATE SCHEMA IF NOT EXISTS "user_profile";

CREATE TABLE user_profile.profiles(
    account_id uuid PRIMARY KEY, -- from Identity context
    nickname varchar(50) DEFAULT NULL,
    full_name text DEFAULT NULL,
    date_of_birth date DEFAULT NULL,
    location_city varchar(100) DEFAULT NULL,
    location_country varchar(100) DEFAULT NULL,
    location_latitude DECIMAL(10, 8) DEFAULT NULL,
    location_longitude DECIMAL(11, 8) DEFAULT NULL,
    region varchar(100) DEFAULT NULL,
    bio text DEFAULT NULL,
    profile_picture_url text DEFAULT NULL,
    profile_picture_uploaded_at timestamp DEFAULT NULL,
    created_at timestamptz NOT NULL DEFAULT NOW(),
    updated_at timestamptz NOT NULL DEFAULT NOW(),
    deleted_at timestamp -- soft delete
);

CREATE INDEX idx_profiles_deleted_at ON user_profile.profiles(deleted_at);

CREATE INDEX idx_profiles_created_at ON user_profile.profiles(created_at);

CREATE TABLE user_profile.activity_interests(
    id uuid PRIMARY KEY DEFAULT uuid_generate_v4(),
    account_id uuid NOT NULL,
    activity_type varchar(100) NOT NULL,
    level varchar(50) NOT NULL DEFAULT 'beginner' CHECK (level IN ('beginner', 'intermediate', 'advanced')),
    created_at timestamptz NOT NULL DEFAULT NOW(),
    CONSTRAINT fk_activity_interests_profile FOREIGN KEY (account_id) REFERENCES user_profile.profiles(account_id)
);

CREATE INDEX idx_activity_interests_created_at ON user_profile.activity_interests(created_at);

CREATE UNIQUE INDEX idx_activity_interests_user_activity ON user_profile.activity_interests(account_id, activity_type);

CREATE TABLE user_profile.settings(
    account_id uuid PRIMARY KEY,
    key VARCHAR(100) NOT NULL,
    value varchar(100) NOT NULL,
    type VARCHAR(100) NOT NULL CHECK (type IN ('string', 'int', 'bool', 'json')),
    updated_at timestamptz NOT NULL DEFAULT NOW(),
    CONSTRAINT fk_settings_profile FOREIGN KEY (account_id) REFERENCES user_profile.profiles(account_id)
);

CREATE INDEX idx_settings_updated_at ON user_profile.settings(updated_at);

CREATE UNIQUE INDEX idx_settings_user_key ON user_profile.settings(account_id, key);

CREATE TABLE IF NOT EXISTS user_profile.profile_statistics(
    account_id uuid PRIMARY KEY,
    activities_joined int NOT NULL,
    last_active_at timestamptz,
    most_active_activity varchar(50),
    activities_by_type jsonb NOT NULL,
    monthly_breakdown jsonb NOT NULL,
    achievements jsonb NOT NULL,
    updated_at timestamptz NOT NULL DEFAULT NOW(),
    CONSTRAINT fk_statistics_profile FOREIGN KEY (account_id) REFERENCES user_profile.profiles(account_id)
);

CREATE INDEX idx_statistics_updated_at ON user_profile.profile_statistics(updated_at);

CREATE INDEX idx_statistics_user_id ON user_profile.profile_statistics(account_id);

CREATE INDEX idx_statistics_activities_by_type ON user_profile.profile_statistics USING GIN(activities_by_type);

CREATE INDEX idx_statistics_monthly_breakdown ON user_profile.profile_statistics USING GIN(monthly_breakdown);

---Function to update update_at timestamp
CREATE OR REPLACE FUNCTION user_profile.update_updated_at_column()
    RETURNS TRIGGER
    AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$
LANGUAGE 'plpgsql';

CREATE TRIGGER update_profiles_updated_at_column
    BEFORE UPDATE ON user_profile.profiles
    FOR EACH ROW
    EXECUTE PROCEDURE user_profile.update_updated_at_column();

CREATE TRIGGER update_settings_updated_at_column
    BEFORE UPDATE ON user_profile.settings
    FOR EACH ROW
    EXECUTE PROCEDURE user_profile.update_updated_at_column();

CREATE TRIGGER update_statistics_updated_at_column
    BEFORE UPDATE ON user_profile.profile_statistics
    FOR EACH ROW
    EXECUTE PROCEDURE user_profile.update_updated_at_column();

