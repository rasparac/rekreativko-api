package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/rasparac/rekreativko-api/notifications/internal/metrics"
	"github.com/rasparac/rekreativko-api/shared/config"
	"github.com/rasparac/rekreativko-api/shared/events"
	"github.com/rasparac/rekreativko-api/shared/logger"
	"github.com/rasparac/rekreativko-api/shared/middleware"
	"github.com/rasparac/rekreativko-api/shared/notification"
	metricstracer "github.com/rasparac/rekreativko-api/shared/store/metrics_tracer"
	"github.com/rasparac/rekreativko-api/shared/store/postgres"
	"github.com/rasparac/rekreativko-api/shared/telemetry"

	notificationsEvents "github.com/rasparac/rekreativko-api/notifications/internal/interfaces/events"

	"github.com/rasparac/rekreativko-api/notifications/internal/application"
	"github.com/rasparac/rekreativko-api/notifications/internal/infrastructure/persistence"
	notificationsHTTP "github.com/rasparac/rekreativko-api/notifications/internal/interfaces/http"
)

//	@title			Notifications Service API
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
		"starting rekreativko notifications service",
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

	notificationRepo := persistence.NewNotificationRepository(txManager, log)

	notificationService := application.NewNotificationService(
		log,
		txManager,
		notificationRepo,
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

	notificationsHandler := notificationsHTTP.NewHandler(
		notificationService,
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

	notificationsHandler.RegisterRoutes(mux, middlewaresChain)

	messageBroker, err := events.NewNatsBroker(
		cfg.NatsConfig.URL,
		cfg.Service.Name,
		log,
	)
	if err != nil {
		return err
	}
	defer messageBroker.Close(ctx)

	// Delivery channels for identity's account lifecycle events (verification
	// emails/SMS, password-changed/account-locked alerts) - moved here from
	// gateway, which had no business owning this. In dev mode these just log
	// instead of actually sending. There is no real SMTP/SMS provider
	// implementation yet - failing fast here instead of letting a nil sender
	// panic later, mid-flight, the first time a real event arrives.
	var (
		emailSender notification.EmailSender
		smsSender   notification.SMSSender
	)
	if cfg.IsDevMode() {
		emailSender = notification.NewInMemoryEmailSender(log)
		smsSender = notification.NewInMemorySMSSender(log)
	} else if !cfg.Features.PhoneRegistrationEnabled {
		// Phone registration is off, so this path is unreachable - wire a
		// sender that fails loudly instead of leaving it nil, in case it's
		// ever invoked unexpectedly.
		smsSender = notification.NewDisabledSMSSender(log)
	}
	if emailSender == nil {
		return fmt.Errorf("no email sender configured - required outside dev mode, and no real SMTP sender is implemented yet")
	}
	if smsSender == nil {
		return fmt.Errorf("no SMS sender configured - required outside dev mode when phone registration is enabled, and no real SMS provider is implemented yet")
	}
	identityNotifier := notification.NewService(smsSender, emailSender, appMetrics)

	subscriber := notificationsEvents.NewSubscriber(
		messageBroker,
		notificationService,
		identityNotifier,
		log,
	)

	if err := subscriber.Subscribe(ctx); err != nil {
		return fmt.Errorf("subscribe to notification events: %w", err)
	}

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
		log.Info(context.Background(), "starting notifications service", "addr", addr)
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
