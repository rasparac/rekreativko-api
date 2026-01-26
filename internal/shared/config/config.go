package config

import (
	"fmt"
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
	}

	Logger struct {
		Level  string `envconfig:"LOGGER_LEVEL" default:"info"`
		Format string `envconfig:"LOGGER_FORMAT" default:"json"`
	}

	ServiceConfig struct {
		Name        string `envconfig:"SERVICE_NAME" default:"rekreativko-api"`
		Version     string `envconfig:"SERVICE_VERSION" default:"v1.0.0"`
		Environment string `envconfig:"SERVICE_ENVIRONMENT" default:"development"`
	}

	ServerConfig struct {
		Host string `envconfig:"HOST"`
		Port string `envconfig:"PORT" default:"8080"`
	}

	RateLimiterConfig struct {
		RequestsPerMinute int `envconfig:"REQUESTS_PER_MINUTE" default:"60"`
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
		PollIntervalS time.Duration `envconfig:"OUTBOX_POLL_INTERVAL" default:"5s"`
		ReadLimit     int           `envconfig:"OUTBOX_READ_LIMIT" default:"100"`
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
