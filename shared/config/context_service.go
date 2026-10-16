package config

import "time"

type (
	IdentityServiceConfig struct {
		URL     string        `envconfig:"IDENTITY_SERVICE_URL" default:"http://localhost:8081"`
		Timeout time.Duration `envconfig:"IDENTITY_SERVICE_TIMEOUT" default:"30s"`
		Version string        `envconfig:"IDENTITY_SERVICE_VERSION" default:"1.0.0"`
	}

	AccountProfileServiceConfig struct {
		URL     string        `envconfig:"ACCOUNT_PROFILE_SERVICE_URL" default:"http://localhost:8082"`
		Timeout time.Duration `envconfig:"ACCOUNT_PROFILE_SERVICE_TIMEOUT" default:"30s"`
		Version string        `envconfig:"ACCOUNT_PROFILE_SERVICE_VERSION" default:"1.0.0"`
	}

	ActivityServiceConfig struct {
		URL     string        `envconfig:"ACTIVITY_SERVICE_URL" default:"http://localhost:8084"`
		Timeout time.Duration `envconfig:"ACTIVITY_SERVICE_TIMEOUT" default:"30s"`
		Version string        `envconfig:"ACTIVITY_SERVICE_VERSION" default:"1.0.0"`

		// SessionExpiryInterval is how often the activity service completes
		// sessions whose end has passed (end time, or start + 3h without one).
		SessionExpiryInterval time.Duration `envconfig:"ACTIVITY_SESSION_EXPIRY_INTERVAL" default:"5m"`
	}

	NotificationsServiceConfig struct {
		URL     string        `envconfig:"NOTIFICATIONS_SERVICE_URL" default:"http://localhost:8083"`
		Timeout time.Duration `envconfig:"NOTIFICATIONS_SERVICE_TIMEOUT" default:"30s"`
		Version string        `envconfig:"NOTIFICATIONS_SERVICE_VERSION" default:"1.0.0"`

		// Retention keeps the feed small enough to list without pagination:
		// read notifications are deleted RetentionRead after they were created,
		// every notification after RetentionAll. The sweep runs every RetentionInterval.
		RetentionRead     time.Duration `envconfig:"NOTIFICATIONS_RETENTION_READ" default:"720h"`
		RetentionAll      time.Duration `envconfig:"NOTIFICATIONS_RETENTION_ALL" default:"2160h"`
		RetentionInterval time.Duration `envconfig:"NOTIFICATIONS_RETENTION_INTERVAL" default:"1h"`
	}

	LocationServiceConfig struct {
		URL     string        `envconfig:"LOCATION_SERVICE_URL" default:"http://localhost:8085"`
		Timeout time.Duration `envconfig:"LOCATION_SERVICE_TIMEOUT" default:"30s"`
		Version string        `envconfig:"LOCATION_SERVICE_VERSION" default:"1.0.0"`
	}
)
