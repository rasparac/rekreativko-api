package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/rasparac/rekreativko-api/location/internal/application"
	"github.com/rasparac/rekreativko-api/location/internal/infrastructure/nominatim"
	"github.com/rasparac/rekreativko-api/location/internal/infrastructure/persistence"
	locationHTTP "github.com/rasparac/rekreativko-api/location/internal/interfaces/http"
	"github.com/rasparac/rekreativko-api/location/internal/metrics"
	"github.com/rasparac/rekreativko-api/shared/config"
	"github.com/rasparac/rekreativko-api/shared/logger"
	"github.com/rasparac/rekreativko-api/shared/middleware"
	metricstracer "github.com/rasparac/rekreativko-api/shared/store/metrics_tracer"
	"github.com/rasparac/rekreativko-api/shared/store/postgres"
	"github.com/rasparac/rekreativko-api/shared/telemetry"
)

//	@title			Location Service API
//	@version		1.0

//	@securityDefinitions.apikey	GatewayKeyAuth
//	@in								header
//	@name							X-Gateway-Key
//	@description					Key added automatically by the gateway when proxying requests; required directly here only when bypassing the gateway.

//	@securityDefinitions.apikey	BearerAuth
//	@in								header
//	@name							Authorization

// @security		GatewayKeyAuth
func main() {
	cfg, err := config.Load()
	if err != nil {
		slog.Error("failed to load configuration", "error", err)
		os.Exit(1)
	}

	ctx := context.Background()
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	logger := logger.New(cfg.Logger.Level, cfg.Logger.Format)

	log := logger.WithName(cfg.Service.Name)

	log.Info(
		ctx,
		"starting rekreativko location service",
		"version", cfg.Service.Version,
		"environment", cfg.Service.Environment,
	)

	if err := run(ctx, cfg, log); err != nil {
		logger.Error(ctx, "failed to start service", "error", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, cfg *config.Config, log *logger.Logger) error {
	appMetrics := metrics.New()

	dbTracer := metricstracer.New(appMetrics)

	pg, err := postgres.New(ctx, log, cfg.Postgres, dbTracer)
	if err != nil {
		return err
	}
	defer pg.Close()

	txManager := postgres.NewTransactionManager(pg.Pool)

	geocodeCacheRepo := persistence.NewGeocodeCacheRepository(txManager, log)
	forwardGeocodeCacheRepo := persistence.NewForwardGeocodeCacheRepository(txManager, log)

	nominatimClient := nominatim.NewClient(cfg.Nominatim, log, appMetrics)

	geocodeService := application.NewGeocodeService(
		log,
		geocodeCacheRepo,
		forwardGeocodeCacheRepo,
		nominatimClient,
		appMetrics,
	)

	telemetryConfig := telemetry.Config{
		ServiceName:       cfg.Service.Name,
		ServiceVersion:    cfg.Service.Version,
		Environment:       cfg.Service.Environment,
		OTLPEndpoint:      cfg.Telemetry.OTLPEndpoint,
		Enabled:           cfg.Telemetry.Enabled,
		TraceIDRatioBased: cfg.Telemetry.OTELTracesSampleRate,
	}

	shutdownTracing, err := telemetry.InitTracing(ctx, telemetryConfig)
	if err != nil {
		log.Error(ctx, "failed to initialize telemetry", "error", err)
		os.Exit(1)
	}
	defer func() {
		if tracingErr := shutdownTracing(ctx); tracingErr != nil {
			log.Error(ctx, "failed to shutdown telemetry", "error", tracingErr)
		}
	}()

	locationHandler := locationHTTP.NewHandler(
		geocodeService,
		log,
	)

	middlewaresChain := middleware.NewChain(
		middleware.Recover(log),
		middleware.RequestID,
		middleware.ClientInfo,
		middleware.CheckGatewayKey(
			log,
			cfg.Service.GatewayKey,
		),
		middleware.ExtractUserContext,
		middleware.Tracing,
		middleware.SpanEnrichment,
	)

	mux := http.NewServeMux()

	if cfg.Telemetry.MetricsEnabled {
		log.Info(ctx, "metrics enabled")
		middlewaresChain = middlewaresChain.Append(middleware.Metrics(appMetrics))
		mux.Handle("GET /metrics", promhttp.Handler())
	}

	mux.Handle("GET /health", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	locationHandler.RegisterRoutes(mux, middlewaresChain)

	return startServer(cfg.Server.Address(), mux, log)
}

func startServer(
	addr string,
	handler http.Handler,
	log *logger.Logger,
) error {
	trimmed := strings.TrimPrefix(addr, "https://")
	trimmed = strings.TrimPrefix(trimmed, "http://")

	srv := http.Server{
		Addr:         trimmed,
		Handler:      handler,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	serverErrors := make(chan error, 1)

	go func() {
		log.Info(context.Background(), "starting location service", "addr", addr)
		err := srv.ListenAndServe()
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErrors <- err
		}
	}()

	shutdown := make(chan os.Signal, 1)
	signal.Notify(shutdown, os.Interrupt, syscall.SIGTERM)

	select {
	case err := <-serverErrors:
		return err
	case <-shutdown:
		log.Info(context.Background(), "shutting down http server")
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := srv.Shutdown(ctx); err != nil {
			return err
		}
	}

	return nil
}
