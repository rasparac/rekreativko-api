package config

import (
	"fmt"
	"strings"
	"time"

	"github.com/kelseyhightower/envconfig"
)

type (
	Config struct {
		Service     ServiceConfig
		Logger      Logger
		RateLimiter RateLimiterConfig
		Server      ServerConfig
		Postgres    PostgresConfig
		Redis       RedisConfig
		JWT         JWTConfig
		CORS        CORSConfig
		Telemetry   TelemetryConfig
		Outbox      OutboxConfig
		NatsConfig  NatsConfig

		GatewayServiceConfig        GatewayServiceConfig
		IdentityServiceConfig       IdentityServiceConfig
		AccountProfileServiceConfig AccountProfileServiceConfig
		ActivityServiceConfig       ActivityServiceConfig
		NotificationsServiceConfig  NotificationsServiceConfig
		LocationServiceConfig       LocationServiceConfig

		Nominatim NominatimConfig

		Features FeaturesConfig
	}

	FeaturesConfig struct {
		// Phone/SMS registration and verification are built but not launched -
		// no SMS provider is wired up yet. Keep this false until one is.
		PhoneRegistrationEnabled bool `envconfig:"PHONE_REGISTRATION_ENABLED" default:"false"`
	}

	Logger struct {
		Level  string `envconfig:"LOG_LEVEL" default:"info"`
		Format string `envconfig:"LOGGER_FORMAT" default:"json"`
	}

	ServiceConfig struct {
		Name        string `envconfig:"SERVICE_NAME" default:"rekreativko"`
		Version     string `envconfig:"SERVICE_VERSION" default:"v1.0.0"`
		Environment string `envconfig:"SERVICE_ENVIRONMENT" default:"development"`
		GatewayKey  string `envconfig:"GATEWAY_API_KEY" required:"true"`
	}

	GatewayServiceConfig struct {
		GatewayKeys string `envconfig:"GATEWAY_API_KEYS" required:"true"`
	}

	ServerConfig struct {
		Host string `envconfig:"HOST"`
		Port string `envconfig:"PORT" default:"8080"`

		// TrustedProxyCIDRs lists the proxies in front of the gateway (CIDRs or
		// addresses, comma separated) whose X-Forwarded-For / X-Real-IP the
		// gateway believes when working out a client's address. Empty: forwarded
		// headers are ignored and the connection's own address is the client.
		TrustedProxyCIDRs []string `envconfig:"TRUSTED_PROXY_CIDRS"`
	}

	// RateLimiterConfig sets the gateway's token buckets: Burst requests at
	// once, PerMinute refilled each minute.
	RateLimiterConfig struct {
		// IP is the backstop in front of authentication, per client address.
		// Generous: many users can sit behind one address (mobile carriers).
		IPBurst     int `envconfig:"RATE_LIMIT_IP_BURST" default:"600"`
		IPPerMinute int `envconfig:"RATE_LIMIT_IP_PER_MINUTE" default:"600"`
		// Account is the limit for a logged-in caller, per account.
		AccountBurst     int `envconfig:"RATE_LIMIT_ACCOUNT_BURST" default:"120"`
		AccountPerMinute int `envconfig:"RATE_LIMIT_ACCOUNT_PER_MINUTE" default:"90"`
		// Anonymous is for public endpoints without a stricter class (refresh
		// token), per client address.
		AnonymousBurst     int `envconfig:"RATE_LIMIT_ANONYMOUS_BURST" default:"60"`
		AnonymousPerMinute int `envconfig:"RATE_LIMIT_ANONYMOUS_PER_MINUTE" default:"60"`
		// Login is for the login endpoint, per client address.
		LoginBurst     int `envconfig:"RATE_LIMIT_LOGIN_BURST" default:"10"`
		LoginPerMinute int `envconfig:"RATE_LIMIT_LOGIN_PER_MINUTE" default:"10"`
		// Sensitive is for endpoints that send an SMS or email or create an
		// account (register, verify, resend), per client address.
		SensitiveBurst     int `envconfig:"RATE_LIMIT_SENSITIVE_BURST" default:"5"`
		SensitivePerMinute int `envconfig:"RATE_LIMIT_SENSITIVE_PER_MINUTE" default:"5"`
	}

	PostgresConfig struct {
		Host            string        `envconfig:"POSTGRES_HOST" default:"localhost"`
		Port            string        `envconfig:"POSTGRES_PORT" default:"5432"`
		User            string        `envconfig:"POSTGRES_USER" default:"postgres"`
		Pass            string        `envconfig:"POSTGRES_PASSWORD" default:"postgres"`
		Database        string        `envconfig:"POSTGRES_DB" default:"rekreativko"`
		SSLMode         string        `envconfig:"POSTGRES_SSLMODE" default:"disable"`
		MaxOpenConn     int           `envconfig:"POSTGRES_MAX_OPEN_CONN" default:"25"`
		MaxIdleConn     int           `envconfig:"POSTGRES_MAX_IDLE_CONN" default:"25"`
		ConnMaxlifeTime time.Duration `envconfig:"POSTGRES_CONN_MAX_LIFETIME" default:"5m"`
	}

	RedisConfig struct {
		Host        string        `envconfig:"REDIS_HOST" default:"localhost"`
		Port        string        `envconfig:"REDIS_PORT" default:"6379"`
		Password    string        `envconfig:"REDIS_PASSWORD" default:""`
		DB          int           `envconfig:"REDIS_DB" default:"0"`
		MaxRetries  int           `envconfig:"REDIS_MAX_RETRIES" default:"3"`
		PoolSize    int           `envconfig:"REDIS_POOL_SIZE" default:"10"`
		PoolTimeout time.Duration `envconfig:"REDIS_POOL_TIMEOUT" default:"4s"`
	}

	JWTConfig struct {
		Secret               string        `envconfig:"JWT_SECRET" required:"true"`
		AccessTokenDuration  time.Duration `envconfig:"JWT_ACCESS_TOKEN_DURATION" default:"15m"`
		RefreshTokenDuration time.Duration `envconfig:"JWT_REFRESH_TOKEN_DURATION" default:"360h"`
	}

	CORSConfig struct {
		AllowedOrigins   string `envconfig:"CORS_ALLOWED_ORIGINS" default:"*"`
		AllowedHeaders   string `envconfig:"CORS_ALLOWED_HEADERS" default:"Accept, Content-Type, X-Request-ID, Authorization"`
		AllowedMethods   string `envconfig:"CORS_ALLOWED_METHODS" default:"GET, POST, PUT, PATCH, DELETE, OPTIONS"`
		ExposedHeaders   string `envconfig:"CORS_EXPOSED_HEADERS" default:"X-Request-ID"`
		AllowCredentials bool   `envconfig:"CORS_ALLOW_CREDENTIALS" default:"true"`
		MaxAge           int    `envconfig:"CORS_MAX_AGE" default:"3600"`
	}

	TelemetryConfig struct {
		OTLPEndpoint         string  `envconfig:"OTEL_EXPORTER_OTLP_ENDPOINT" default:"localhost:4318"`
		OTELTracesSampleRate float64 `envconfig:"OTEL_TRACES_SAMPLER_ARG" default:"0.5"`
		Enabled              bool    `envconfig:"TELEMETRY_ENABLED" default:"false"`
		MetricsEnabled       bool    `envconfig:"METRICS_ENABLED" default:"false"`
	}

	OutboxConfig struct {
		// PollIntervalS is how often the outbox is polled. With Listen on it is
		// only the fallback for notifications that were missed.
		PollIntervalS time.Duration `envconfig:"OUTBOX_POLL_INTERVAL" default:"5s"`
		// Listen publishes as soon as a transaction commits events, via
		// Postgres LISTEN/NOTIFY, instead of waiting for the next poll.
		Listen     bool   `envconfig:"OUTBOX_LISTEN" default:"true"`
		ReadLimit  int    `envconfig:"OUTBOX_READ_LIMIT" default:"100"`
		Schemas    string `envconfig:"OUTBOX_SCHEMAS" required:"true"`
		MaxRetries int    `envconfig:"OUTBOX_MAX_RETRIES" default:"5"`
	}

	NatsConfig struct {
		URL string `envconfig:"NATS_URL" required:"true" default:"nats://localhost:4222"`
	}

	// NominatimConfig configures the outbound client the location service uses
	// to call Nominatim's public API. MinRequestInterval enforces Nominatim's
	// usage policy of at most 1 request/second across this process.
	NominatimConfig struct {
		BaseURL            string        `envconfig:"NOMINATIM_BASE_URL" default:"https://nominatim.openstreetmap.org"`
		UserAgent          string        `envconfig:"NOMINATIM_USER_AGENT" default:"rekreativko-api/1.0 (+https://github.com/rasparac/rekreativko-api)"`
		RequestTimeout     time.Duration `envconfig:"NOMINATIM_REQUEST_TIMEOUT" default:"10s"`
		MinRequestInterval time.Duration `envconfig:"NOMINATIM_MIN_REQUEST_INTERVAL" default:"1s"`
	}
)

func Load() (*Config, error) {
	var c Config

	err := envconfig.Process("", &c)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch configuration: %w", err)
	}

	return &c, nil
}

func (pc *PostgresConfig) DSN() string {
	return fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=%s",
		pc.User,
		pc.Pass,
		pc.Host,
		pc.Port,
		pc.Database,
		pc.SSLMode,
	)
}

func (s *ServerConfig) Address() string {
	return fmt.Sprintf("%s:%s", s.Host, s.Port)
}

func (rc *RedisConfig) Address() string {
	return fmt.Sprintf("%s:%s", rc.Host, rc.Port)
}

func (c Config) IsDevMode() bool {
	return c.Service.Environment == "development"
}

func (c Config) IsProdMode() bool {
	return c.Service.Environment == "production"
}

func (oc *OutboxConfig) GetSchemas() []string {
	if oc.Schemas == "" {
		return []string{}
	}

	schemas := make([]string, 0)
	for _, schema := range strings.Split(oc.Schemas, ",") {
		schema = strings.TrimSpace(schema)
		if schema != "" {
			schemas = append(schemas, schema)
		}
	}
	return schemas
}
