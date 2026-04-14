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
	"github.com/rasparac/rekreativko-api/internal/identity/application"
	"github.com/rasparac/rekreativko-api/internal/identity/infrastructure/persistance"
	"github.com/rasparac/rekreativko-api/internal/identity/metrics"
	"github.com/rasparac/rekreativko-api/internal/shared/config"
	"github.com/rasparac/rekreativko-api/internal/shared/domainevent"
	"github.com/rasparac/rekreativko-api/internal/shared/logger"
	"github.com/rasparac/rekreativko-api/internal/shared/middleware"
	metricstracer "github.com/rasparac/rekreativko-api/internal/shared/store/metrics_tracer"
	"github.com/rasparac/rekreativko-api/internal/shared/store/postgres"
	"github.com/rasparac/rekreativko-api/internal/shared/telemetry"
	"github.com/rasparac/rekreativko-api/internal/shared/token"

	identityHttp "github.com/rasparac/rekreativko-api/internal/identity/interfaces/http"
)

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

	log := logger.WithName(
		"rekreativko",
	)

	log.Info(
		ctx,
		"starting rekreativko",
		"version", cfg.Service.Version,
		"environment", cfg.Service.Environment,
	)

	err = run(ctx, cfg, log)
	if err != nil {
		logger.Error(ctx, "failed to start rekreativko", "error", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, cfg *config.Config, log *logger.Logger) error {
	appMetrics := metrics.New(cfg.Service.Name)

	dbTracer := metricstracer.New(appMetrics)

	pg, err := postgres.New(
		ctx,
		log,
		cfg.Postgres,
		dbTracer,
	)
	if err != nil {
		return err
	}
	defer pg.Close()

	txManager := postgres.NewTransactionManager(pg.Pool)

	domainEventMgr := domainevent.NewDomainEventManager(txManager)

	tokenGen := token.NewGenerator(
		[]byte(cfg.JWT.Secret),
		cfg.JWT.AccessTokenDuration,
		cfg.JWT.RefreshTokenDuration,
	)

	identityAccountRepo := persistance.NewAccountManager(
		pg.Pool,
		txManager,
		log,
	)

	identityRefreshTokenRepo := persistance.NewRefreshTokenManager(
		pg.Pool,
		txManager,
		log,
	)

	verificationCodeRepo := persistance.NewVerificationCodeManager(
		pg.Pool,
		txManager,
		log,
	)

	verificationCodeGenerator := token.NewVerificationCodeGenerator(6)
	passwordHasher := token.NewPasswordHasher(10)

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
		if tracingErr := shutdownTracing(ctx); err != nil {
			log.Error(ctx, "failed to shutdown telemetry", "error", tracingErr)
		}
	}()

	identityService := application.NewService(
		identityAccountRepo,
		verificationCodeRepo,
		identityRefreshTokenRepo,
		txManager,
		domainEventMgr,
		tokenGen,
		log,
		verificationCodeGenerator,
		passwordHasher,
		appMetrics,
	)

	mux := http.NewServeMux()

	identityHandler := identityHttp.NewHandler(
		identityService,
		log,
	)

	middlewaresChain := middleware.NewChain(
		middleware.RequestID,
		middleware.CheckGatewayKey(
			log,
			cfg.Service.GatewayKey,
		),
		middleware.ExtractUserContext,
		middleware.Tracing,
		middleware.SpanEnrichment,
	)

	if cfg.Telemetry.MetricsEnabled {
		log.Info(ctx, "metrics enabled")
		middlewaresChain = middlewaresChain.Append(middleware.Metrics(appMetrics))
		mux.Handle("GET /metrics", promhttp.Handler())
	}

	mux.Handle("GET /health", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Info(r.Context(), "checking status", "method", r.Method, "path", r.URL.Path)
		w.WriteHeader(http.StatusOK)
	}))

	identityHandler.RegisterRoutes(mux, middlewaresChain)

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
		log.Info(context.Background(), "starting identity service", "addr", addr)
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
