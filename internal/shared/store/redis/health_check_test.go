package redis

import (
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/rasparac/rekreativko-api/internal/shared/config"
	"github.com/rasparac/rekreativko-api/internal/shared/logger"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func setupRedis(t *testing.T) (*Client, *miniredis.Miniredis, func()) {
	mr, err := miniredis.Run()

	require.NoError(t, err)

	cfg := config.RedisConfig{
		Host:        mr.Host(),
		Port:        mr.Port(),
		Password:    "",
		DB:          0,
		MaxRetries:  3,
		PoolSize:    10,
		PoolTimeout: 4 * time.Second,
	}

	rdb := redis.NewClient(&redis.Options{
		Addr:         cfg.Address(),
		Password:     cfg.Password,
		DB:           cfg.DB,
		MaxRetries:   cfg.MaxRetries,
		PoolSize:     cfg.PoolSize,
		PoolTimeout:  cfg.PoolTimeout,
		ReadTimeout:  3 * time.Second,
		WriteTimeout: 3 * time.Second,
		DialTimeout:  5 * time.Second,
	})

	client := &Client{
		Client: rdb,
		logger: logger.NewDevelopment(),
	}

	cleanup := func() {
		rdb.Close()
		mr.Close()
	}

	return client, mr, cleanup
}

func TestHealthCheck_Check(t *testing.T) {
	client, mr, cleanup := setupRedis(t)
	defer cleanup()

	healthCheck := NewHealthCheck(client)

	result, err := healthCheck.Check(t.Context())
	require.NoError(t, err)
	require.Equal(t, "healthy", result["status"])
	require.Contains(t, result, "ping_duration_ms")
	require.Contains(t, result, "stats")

	// Simulate Redis being down
	mr.Close()

	result, err = healthCheck.Check(t.Context())
	require.Error(t, err)
	require.Equal(t, "unhealthy", result["status"])
	require.Contains(t, result, "error")
}
