BEGIN;
DROP TRIGGER IF EXISTS update_profiles_updated_at_column ON account_profile.profiles;
DROP TRIGGER IF EXISTS update_settings_updated_at_column ON account_profile.settings;
DROP TRIGGER IF EXISTS update_statistics_updated_at_column ON account_profile.profile_statistics;
DROP FUNCTION IF EXISTS account_profile.update_updated_at_column();
DROP TABLE IF EXISTS account_profile.account_settings_meta;
DROP TABLE IF EXISTS account_profile.profile_statistics;
DROP TABLE IF EXISTS account_profile.settings;
DROP TABLE IF EXISTS account_profile.activity_interests;
DROP TABLE IF EXISTS account_profile.profiles;
COMMIT;

