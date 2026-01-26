package redis

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

	err := hc.client.HealthCheck(ctx)
	if err != nil {
		result["status"] = "unhealthy"
		result["error"] = err.Error()
		return result, err
	}

	duration := time.Since(start).Milliseconds()

	result["ping_duration_ms"] = duration
	result["stats"] = hc.client.Stats()
	result["status"] = "healthy"

	infoCmd := hc.client.Info(ctx, "server", "memory", "stats")
	if infoCmd.Err() == nil {
		result["server_info"] = parseRedisInfo(infoCmd.Val())
		return result, err
	}

	return result, nil
}

func parseRedisInfo(info string) map[string]string {
	result := make(map[string]string)
	lines := splitLines(info)
	for _, line := range lines {
		if len(line) == 0 || line[0] == '#' {
			continue
		}

		parts := splitKeyValue(line, ':')
		if len(parts) == 2 {
			result[parts[0]] = parts[1]
		}
	}
	return result
}

func splitLines(s string) []string {
	var (
		lines []string
		start = 0
	)

	for i := 0; i < len(s); i++ {
		if s[i] == '\n' || s[i] == '\r' {
			if i > start {
				lines = append(lines, s[start:i])
			}
			start = i + 1

			// handle \r\n
			if i+1 < len(s) && s[i] == '\r' && s[i+1] == '\n' {
				i++
				start = i + 1
			}
		}
	}

	if start < len(s) {
		lines = append(lines, s[start:])
	}

	return lines
}

func splitKeyValue(s string, sep byte) []string {
	for i, _ := range s {
		if s[i] == sep {
			return []string{s[:i], s[i+1:]}
		}
	}
	return []string{s}
}
