package health

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/rasparac/rekreativko-api/internal/shared/store/postgres"
	"github.com/rasparac/rekreativko-api/internal/shared/store/redis"
	"go.opentelemetry.io/otel"
)

type handler struct {
	pgHealth    *postgres.HealthCheck
	redisHealth *redis.HealthCheck
}

func NewHandler(
	pgHealth *postgres.HealthCheck,
	redisHealth *redis.HealthCheck,
) *handler {
	return &handler{
		pgHealth:    pgHealth,
		redisHealth: redisHealth,
	}
}

func (h *handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /services/health", h.Check)
}

// @Summary		Check health of services
// @Description	This endpoint will check the health of services such as postgres and redis.
// @ID				check-health
// @Tags			health
// @Accept			json
// @Produce		json
// @Success		200	{object}	map[string]any	"All services are healthy"
// @Failure		503	{object}	map[string]any	"One or more services are unhealthy"
// @Router			/services/health [get]
func (h *handler) Check(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	tracer := otel.Tracer("health-chek")

	healthResp := map[string]any{
		"status":    "healthy",
		"timestamp": time.Now().UTC().Format(time.RFC3339),
		"telemetry": map[string]any{
			"tracing_enabled": tracer != nil,
			"tracer":          "opentelemetry",
		},
	}

	pgResult, err := h.pgHealth.Check(ctx)
	if err != nil {
		healthResp["status"] = "unhealthy"
		healthResp["postgres"] = map[string]any{
			"status": "unhealthy",
			"error":  err.Error(),
		}
	} else {
		healthResp["postgres"] = pgResult
	}

	redisResult, err := h.redisHealth.Check(ctx)
	if err != nil {
		healthResp["status"] = "unhealthy"
		healthResp["redis"] = map[string]any{
			"status": "unhealthy",
			"error":  err.Error(),
		}
	} else {
		healthResp["redis"] = redisResult
	}

	statusCode := http.StatusOK
	if healthResp["status"] == "unhealthy" {
		statusCode = http.StatusServiceUnavailable
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(healthResp)
}
