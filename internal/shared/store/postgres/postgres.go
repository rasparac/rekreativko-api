package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rasparac/rekreativko-api/internal/shared/config"
	"github.com/rasparac/rekreativko-api/internal/shared/logger"
	metricstracer "github.com/rasparac/rekreativko-api/internal/shared/store/metrics_tracer"
)

type Client struct {
	*pgxpool.Pool
	logger *logger.Logger
}

func New(
	ctx context.Context,
	logger *logger.Logger,
	cfg config.PostgresConfig,
	m *metricstracer.MetricsTracer,
) (*Client, error) {
	config, err := pgxpool.ParseConfig(cfg.DSN())
	if err != nil {
		return nil, err
	}

	if m != nil {
		config.ConnConfig.Tracer = m
	}

	config.MaxConns = int32(cfg.MaxOpenConn)
	config.MinConns = int32(cfg.MaxIdleConn)
	config.MaxConnLifetime = cfg.ConnMaxlifeTime

	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	err = pool.Ping(ctx)
	if err != nil {
		return nil, fmt.Errorf("ping database: %w", err)
	}

	logger.Info(ctx, "connected to database")

	return &Client{
		Pool:   pool,
		logger: logger,
	}, nil
}

func (c *Client) Close() {
	c.logger.Info(context.Background(), "closing database connection pool")

	c.Pool.Close()
}

func (c *Client) Health(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	return c.Ping(ctx)
}

func (c *Client) Stats() map[string]any {
	return map[string]any{
		"max_connections":      c.Stat().MaxConns(),
		"acquired_connections": c.Stat().AcquiredConns(),
		"idle_connections":     c.Stat().IdleConns(),
		"total_connections":    c.Stat().TotalConns(),
	}
}

func GetPgxError(err error) *pgconn.PgError {
	var pgErr *pgconn.PgError
	if ok := errors.As(err, &pgErr); ok {
		return pgErr
	}
	return nil
}
