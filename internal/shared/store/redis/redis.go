package redis

import (
	"context"
	"time"

	"github.com/rasparac/rekreativko-api/internal/shared/config"
	"github.com/rasparac/rekreativko-api/internal/shared/logger"
	"github.com/redis/go-redis/v9"
)

type Client struct {
	*redis.Client
	logger *logger.Logger
}

func New(
	ctx context.Context,
	logger *logger.Logger,
	cfg config.RedisConfig,
) (*Client, error) {
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

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	if err := rdb.Ping(ctx).Err(); err != nil {
		return nil, err
	}

	logger.Info(ctx, "connected to redis")

	return &Client{
		Client: rdb,
		logger: logger.WithName("redis"),
	}, nil
}

func (c *Client) Close() error {
	return c.Client.Close()
}

func (c *Client) HealthCheck(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	return c.Ping(ctx).Err()
}

func (c *Client) Stats() map[string]any {
	stats := c.Client.PoolStats()
	return map[string]any{
		"hits":        stats.Hits,
		"misses":      stats.Misses,
		"timeouts":    stats.Timeouts,
		"total_conns": stats.TotalConns,
		"idle_conns":  stats.IdleConns,
		"stale_conns": stats.StaleConns,
	}
}
