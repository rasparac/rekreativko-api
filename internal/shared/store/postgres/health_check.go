package postgres

import (
	"context"
	"time"
)

type HealthCheck struct {
	client *Client
}

func NewHealthCheck(client *Client) *HealthCheck {
	return &HealthCheck{client: client}
}

func (hc *HealthCheck) Check(ctx context.Context) (map[string]any, error) {
	result := make(map[string]any)

	start := time.Now()

	err := hc.client.Health(ctx)
	if err != nil {
		result["status"] = "unhealthy"
		result["error"] = err.Error()
		return result, err
	}

	duration := time.Since(start).Milliseconds()

	result["ping_duration_ms"] = duration
	result["stats"] = hc.client.Stats()
	result["status"] = "healthy"

	return result, nil
}
