package config

import (
	"testing"
	"time"
)

func Test_config_Load(t *testing.T) {
	t.Run("should load config from environment variables", func(t *testing.T) {
		// Set environment variables for testing
		t.Setenv("SERVICE_NAME", "test-service")
		t.Setenv("SERVICE_VERSION", "v2.0.0")
		t.Setenv("SERVICE_ENVIRONMENT", "production")
		t.Setenv("HOST", "localhost")
		t.Setenv("PORT", "8080")
		t.Setenv("POSTGRES_HOST", "localhost")
		t.Setenv("POSTGRES_PORT", "5432")
		t.Setenv("POSTGRES_USER", "postgres")
		t.Setenv("POSTGRES_PASSWORD", "postgres")
		t.Setenv("POSTGRES_DB", "test-db")
		t.Setenv("POSTGRES_SSLMODE", "disable")
		t.Setenv("POSTGRES_MAX_OPEN_CONN", "25")
		t.Setenv("POSTGRES_MAX_IDLE_CONN", "25")
		t.Setenv("POSTGRES_CONN_MAX_LIFETIME", "5m")
		t.Setenv("REDIS_HOST", "localhost")
		t.Setenv("REDIS_PORT", "6379")
		t.Setenv("REDIS_PASSWORD", "")
		t.Setenv("REDIS_DB", "0")
		t.Setenv("REDIS_MAX_RETRIES", "3")
		t.Setenv("REDIS_POOL_SIZE", "10")
		t.Setenv("REDIS_POOL_TIMEOUT", "4s")
		t.Setenv("JWT_SECRET", "secret")
		t.Setenv("JWT_ACCESS_TOKEN_DURATION", "15m")
		t.Setenv("JWT_REFRESH_TOKEN_DURATION", "360h")

		cfg, err := Load()
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		if cfg.Service.Name != "test-service" {
			t.Errorf("expected service name 'test-service', got %s", cfg.Service.Name)
		}
		if cfg.Service.Version != "v2.0.0" {
			t.Errorf("expected service version 'v2.0.0', got %s", cfg.Service.Version)
		}
		if cfg.Service.Environment != "production" {
			t.Errorf("expected service environment 'production', got %s", cfg.Service.Environment)
		}
		if cfg.Server.Host != "localhost" {
			t.Errorf("expected server host 'localhost', got %s", cfg.Server.Host)
		}
		if cfg.Server.Port != "8080" {
			t.Errorf("expected server port '8080', got %s", cfg.Server.Port)
		}
		if cfg.Postgres.Host != "localhost" {
			t.Errorf("expected postgres host 'localhost', got %s", cfg.Postgres.Host)
		}
		if cfg.Postgres.Port != "5432" {
			t.Errorf("expected postgres port '5432', got %s", cfg.Postgres.Port)
		}
		if cfg.Postgres.User != "postgres" {
			t.Errorf("expected postgres user 'postgres', got %s", cfg.Postgres.User)
		}
		if cfg.Postgres.Pass != "postgres" {
			t.Errorf("expected postgres password 'postgres', got %s", cfg.Postgres.Pass)
		}
		if cfg.Postgres.Database != "test-db" {
			t.Errorf("expected postgres database 'test-db', got %s", cfg.Postgres.Database)
		}
		if cfg.Postgres.SSLMode != "disable" {
			t.Errorf("expected postgres ssl mode 'disable', got %s", cfg.Postgres.SSLMode)
		}
		if cfg.Postgres.MaxOpenConn != 25 {
			t.Errorf("expected postgres max open connections '25', got %d", cfg.Postgres.MaxOpenConn)
		}
		if cfg.Postgres.MaxIdleConn != 25 {
			t.Errorf("expected postgres max idle connections '25', got %d", cfg.Postgres.MaxIdleConn)
		}
		if cfg.Postgres.ConnMaxlifeTime != 5*time.Minute {
			t.Errorf("expected postgres connection max life time '5m', got %v", cfg.Postgres.ConnMaxlifeTime)
		}
	})
}
