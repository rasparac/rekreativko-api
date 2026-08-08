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
)
