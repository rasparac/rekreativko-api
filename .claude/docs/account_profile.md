# Account Profile Context Design

## Responsibility
Manages user profiles, personal information, preferences, activity interests, and statistics.

This context is separated from Identity context following single responsibility principle:
- Identity: authentication and authorization
- Account Profile: user data, preferences, and activity tracking

## Does NOT Handle
- User authentication (handled by Identity context)
- Activity groups and sessions (handled by Activity context)

## Aggregates

```go
type AccountProfile struct {
    AccountID              uuid.UUID  // from Identity context
    Nickname               string
    FullName               string
    DateOfBirth            *time.Time
    Location               Location
    Bio                    string
    ProfilePicture         *ProfilePicture

    CreatedAt   time.Time
    UpdatedAt   time.Time
    DeletedAt   *time.Time
}

type Location struct {
    City      string
    Country   string
    Region    string
    Latitude  *float64  // optional for discovery
    Longitude *float64  // optional for discovery
}

type ProfilePicture struct {
    URL         string
    UploadedAt  time.Time
}

type ActivityInterest struct {
    ID            uuid.UUID
    AccountID     uuid.UUID
    ActivityType  string        // e.g., "basketball", "yoga", "running"
    ActivityLevel ActivityLevel // beginner, intermediate, advanced

    CreatedAt time.Time
    UpdatedAt time.Time
}

type ActivityLevel string

const (
    ActivityLevelBeginner     ActivityLevel = "beginner"
    ActivityLevelIntermediate ActivityLevel = "intermediate"
    ActivityLevelAdvanced     ActivityLevel = "advanced"
)

type AccountSettings struct {
    AccountID uuid.UUID
    Settings  map[string]Setting  // key-value pairs
    Version   int                 // for optimistic locking

    CreatedAt time.Time
    UpdatedAt time.Time
}

type Setting struct {
    Key   string
    Value string
    Type  SettingType  // for type safety
}

type SettingType string

const (
    SettingTypeString  SettingType = "string"
    SettingTypeBoolean SettingType = "boolean"
    SettingTypeNumber  SettingType = "number"
)

type AccountStatistics struct {
    AccountID            uuid.UUID
    ActivitiesJoined     int
    LastActiveAt         *time.Time
    MostActiveActivity   string
    ActivitiesByType     map[string]int  // e.g., {"basketball": 10, "yoga": 5}
    MonthlyBreakdown     map[string]int  // e.g., {"2025-01": 3, "2025-02": 7}
    Achievements         []string

    UpdatedAt time.Time
}
```

## Business Rules

### Profile Rules
1. **Account Creation**: Profile is automatically created when account is verified (listens to `identity.account.verified` event)
2. **Nickname**: Optional, must be unique if provided
3. **Location**: City and country are required for discovery features, coordinates optional
4. **Profile Picture**: Optional, stored as URL reference
5. **Soft Delete**: Profiles are soft-deleted (deleted_at) to maintain referential integrity

### Activity Interests Rules
1. **Multiple Interests**: Users can have multiple activity interests
2. **Unique per Activity**: One interest per activity type per user
3. **Skill Level**: Each interest has associated skill level (beginner/intermediate/advanced)
4. **Used for Discovery**: Interests help match users with relevant activity groups

### Settings Rules
1. **Dynamic Settings**: Settings are stored as key-value pairs for flexibility
2. **Type Safety**: Each setting has a type (string, boolean, number) for validation
3. **Version Control**: Uses optimistic locking (version field) for concurrent updates
4. **Common Settings**:
   - `notifications_enabled` (boolean)
   - `email_notifications` (boolean)
   - `language` (string)
   - `theme` (string: "light", "dark", "system")
   - `privacy_profile_visibility` (string: "public", "friends", "private")

### Statistics Rules
1. **Auto-Updated**: Statistics update via domain events from Activity context
2. **JSON Storage**: Complex data (activities by type, monthly breakdown) stored as JSONB
3. **Performance**: Pre-calculated stats for fast reads
4. **Events that update statistics**:
   - User joins activity group
   - User attends session
   - User completes session
   - Monthly aggregation jobs

## Domain Events

### Profile Events
- `AccountProfileCreated` - Profile created after account verification
- `AccountProfileUpdated` - User updates profile information
- `AccountProfileDeleted` - Profile soft-deleted
- `ProfilePictureUpdated` - Profile picture changed
- `LocationUpdated` - User location changed

### Interest Events
- `ActivityInterestAdded` - User adds new activity interest
- `ActivityInterestRemoved` - User removes activity interest
- `ActivityInterestLevelChanged` - User changes skill level for activity

### Settings Events
- `AccountSettingsUpdated` - Settings changed
- `NotificationPreferencesChanged` - Notification settings changed

### Statistics Events (Internal)
- `StatisticsUpdated` - Statistics recalculated

## Event Subscriptions

Listens to events from other contexts:
- `identity.account.verified` ’ Create initial profile
- `activity.member.joined` ’ Update activities joined count
- `activity.session.attended` ’ Update statistics
- `activity.session.completed` ’ Update monthly breakdown

## Database Schema

All tables use the `account_profile` schema for namespace isolation (DDD bounded context).

```sql
CREATE EXTENSION IF NOT EXISTS "uuid-ossp" WITH SCHEMA public;
CREATE EXTENSION IF NOT EXISTS citext;

CREATE SCHEMA IF NOT EXISTS "account_profile";

-- Profiles table
CREATE TABLE account_profile.profiles(
    account_id uuid PRIMARY KEY,  -- References identity.accounts
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

-- Activity interests table
CREATE TABLE account_profile.activity_interests(
    id uuid PRIMARY KEY DEFAULT uuid_generate_v4(),
    account_id uuid NOT NULL,
    activity_type varchar(100) NOT NULL,
    activity_level varchar(50) NOT NULL DEFAULT 'beginner',
    created_at timestamptz NOT NULL DEFAULT NOW(),
    updated_at timestamptz NOT NULL DEFAULT NOW(),
    CONSTRAINT fk_activity_interests_profile
        FOREIGN KEY (account_id) REFERENCES account_profile.profiles(account_id)
);

CREATE INDEX idx_activity_interests_created_at ON account_profile.activity_interests(created_at);
CREATE UNIQUE INDEX idx_activity_interests_account_activity
    ON account_profile.activity_interests(account_id, activity_type);

-- Settings table (key-value store)
CREATE TABLE account_profile.settings(
    account_id uuid NOT NULL,
    key varchar(100) NOT NULL,
    value varchar(100) NOT NULL,
    type varchar(100) NOT NULL,
    updated_at timestamptz NOT NULL DEFAULT NOW(),
    CONSTRAINT settings_pkey PRIMARY KEY (account_id, key),
    CONSTRAINT fk_settings_profile
        FOREIGN KEY (account_id) REFERENCES account_profile.profiles(account_id)
);

CREATE INDEX idx_settings_updated_at ON account_profile.settings(updated_at);

-- Statistics table
CREATE TABLE IF NOT EXISTS account_profile.profile_statistics(
    account_id uuid PRIMARY KEY,
    activities_joined int NOT NULL,
    last_active_at timestamptz,
    most_active_activity varchar(50),
    activities_by_type jsonb NOT NULL,
    monthly_breakdown jsonb NOT NULL,
    achievements jsonb NOT NULL,
    updated_at timestamptz NOT NULL DEFAULT NOW(),
    CONSTRAINT fk_statistics_profile
        FOREIGN KEY (account_id) REFERENCES account_profile.profiles(account_id)
);

CREATE INDEX idx_statistics_updated_at ON account_profile.profile_statistics(updated_at);
CREATE INDEX idx_statistics_activities_by_type
    ON account_profile.profile_statistics USING GIN(activities_by_type);
CREATE INDEX idx_statistics_monthly_breakdown
    ON account_profile.profile_statistics USING GIN(monthly_breakdown);

-- Settings metadata (for optimistic locking)
CREATE TABLE account_profile.account_settings_meta(
    account_id uuid PRIMARY KEY,
    version int NOT NULL DEFAULT 1,
    created_at timestamptz NOT NULL DEFAULT NOW(),
    updated_at timestamptz NOT NULL DEFAULT NOW(),
    CONSTRAINT fk_settings_meta_profile
        FOREIGN KEY (account_id) REFERENCES account_profile.profiles(account_id)
);

-- Triggers for auto-updating timestamps
CREATE OR REPLACE FUNCTION account_profile.update_updated_at_column()
    RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE 'plpgsql';

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
```

## API Contract

All endpoints use `/account-profile/api/v1` prefix and require authentication.

### Get Profile
```http
GET /api/v1/profile

Response: 200 OK
{
    "account_id": "uuid",
    "nickname": "johndoe",
    "full_name": "John Doe",
    "date_of_birth": "1990-01-15",
    "location": {
        "city": "New York",
        "country": "USA",
        "region": "NY"
    },
    "bio": "Love outdoor activities",
    "profile_picture_url": "https://...",
    "created_at": "2025-01-01T00:00:00Z",
    "updated_at": "2025-01-15T00:00:00Z"
}
```

### Update Profile
```http
PUT /api/v1/profile

Request:
{
    "nickname": "johndoe",
    "full_name": "John Doe",
    "location": {
        "city": "New York",
        "country": "USA"
    },
    "bio": "Love outdoor activities"
}

Response: 200 OK
```

### Get Activity Interests
```http
GET /api/v1/profile/interests

Response: 200 OK
{
    "interests": [
        {
            "id": "uuid",
            "activity_type": "basketball",
            "activity_level": "intermediate"
        }
    ]
}
```

### Add Activity Interest
```http
POST /api/v1/profile/interests

Request:
{
    "activity_type": "basketball",
    "activity_level": "intermediate"
}

Response: 201 Created
```

### Get Settings
```http
GET /api/v1/profile/settings

Response: 200 OK
{
    "settings": {
        "notifications_enabled": true,
        "email_notifications": false,
        "language": "en",
        "theme": "dark"
    },
    "version": 5
}
```

### Update Settings
```http
PUT /api/v1/profile/settings

Request:
{
    "settings": {
        "notifications_enabled": false,
        "theme": "light"
    },
    "version": 5  // for optimistic locking
}

Response: 200 OK
409 Conflict  // if version mismatch
```

### Get Statistics
```http
GET /api/v1/profile/statistics

Response: 200 OK
{
    "activities_joined": 15,
    "last_active_at": "2025-01-15T00:00:00Z",
    "most_active_activity": "basketball",
    "activities_by_type": {
        "basketball": 10,
        "yoga": 5
    },
    "monthly_breakdown": {
        "2025-01": 8,
        "2025-02": 7
    },
    "achievements": ["early_adopter", "social_butterfly"]
}
```

## Integration with Other Contexts

### Identity Context
- Profile created automatically when `identity.account.verified` event received
- Profile links to account via `account_id`
- Profile deleted when account deleted (via event subscription)

### Activity Context
- Activity interests used for discovery and matching
- Statistics updated when users join/attend activities
- Location used for finding nearby activities

### Discovery Context (future)
- Profile data used for user matching
- Activity interests used for recommendations
- Location used for proximity search
