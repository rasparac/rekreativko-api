package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/rasparac/rekreativko-api/internal/gateway"
	"github.com/rasparac/rekreativko-api/internal/notification"
	"github.com/rasparac/rekreativko-api/internal/shared/config"
	"github.com/rasparac/rekreativko-api/internal/shared/events"
	"github.com/rasparac/rekreativko-api/internal/shared/logger"
	"github.com/rasparac/rekreativko-api/internal/shared/middleware"
	"github.com/rasparac/rekreativko-api/internal/shared/telemetry"
	"github.com/rasparac/rekreativko-api/internal/shared/token"
	httpSwagger "github.com/swaggo/http-swagger/v2"
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
		cfg.Service.Name,
	)

	log.Info(
		ctx,
		"starting rekreativko gateway",
		"version", cfg.Service.Version,
		"environment", cfg.Service.Environment,
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
		if tracingErr := shutdownTracing(ctx); err != nil {
			log.Error(ctx, "failed to shutdown telemetry", "error", tracingErr)
		}
	}()

	serviceConfig := map[string]gateway.ServiceConfig{
		"identity": {
			URL:     cfg.IdentityServiceConfig.URL,
			Timeout: cfg.IdentityServiceConfig.Timeout,
		},
		"account-profile": {
			URL:     cfg.AccountProfileServiceConfig.URL,
			Timeout: cfg.AccountProfileServiceConfig.Timeout,
		},
	}

	proxy, err := gateway.NewReverseProxy(serviceConfig, log)
	if err != nil {
		log.Error(ctx, "failed to create reverse proxy", "error", err)
		os.Exit(1)
	}

	tokenGen := token.NewGenerator(
		[]byte(cfg.JWT.Secret),
		cfg.JWT.AccessTokenDuration,
		cfg.JWT.RefreshTokenDuration,
	)

	router := gateway.NewRouter(proxy)

	mux := http.NewServeMux()

	mux.Handle("GET /health", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Info(r.Context(), "checking status", "method", r.Method, "path", r.URL.Path)
		w.WriteHeader(http.StatusOK)
	}))

	mux.HandleFunc("/{$}", rootHandler)

	if cfg.IsDevMode() {
		mux.Handle("GET /swagger/", httpSwagger.WrapHandler)
		log.Info(ctx, "swagger UI enabled", "url", fmt.Sprintf("%s/swagger/index.html", cfg.Server.Address()))
	}

	publicPaths := []string{
		`^/identity/api/v1/(login|register|verify-account|resend-verification-code)$`,
		"^/swagger/.*",
		"^/metrics/.*",
		"^/health$",
	}

	middlewaresChain := middleware.NewChain(
		middleware.Recover(log),
		middleware.RequestID,
		middleware.ClientInfo,
		middleware.Logging(log),
		middleware.Tracing,
		middleware.SpanEnrichment,
		middleware.CORS(
			middleware.CORSConfig{
				AllowedOrigins:   []string{"*"},
				AllowedHeaders:   []string{"*"},
				AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
				ExposedHeaders:   []string{"X-Proxied-By", "X-Service-Name"},
				MaxAge:           3600,
				AllowCredentials: true,
			},
		),
		middleware.NewRateLimiter(cfg.RateLimiter.RequestsPerMinute).RateLimiter,
		middleware.NewAuthMiddleware(
			tokenGen,
			log,
			publicPaths,
		).RequireAuth,
	)

	appMetrics := gateway.New(cfg.Service.Name)

	if cfg.Telemetry.MetricsEnabled {
		log.Info(ctx, "metrics enabled")
		middlewaresChain = middlewaresChain.Append(middleware.Metrics(appMetrics))
		mux.Handle("GET /metrics", promhttp.Handler())
	}

	mux.Handle("/", middlewaresChain.Then(router))

	messageBroker, err := events.NewNatsBroker(
		cfg.NatsConfig.URL,
		cfg.Service.Name,
		log,
	)
	if err != nil {
		log.Error(ctx, "failed to create message broker", "error", err)
		os.Exit(1)
	}
	defer messageBroker.Close(ctx)

	prepareNotifications(messageBroker, appMetrics, cfg, log)

	err = startServer(cfg, mux, log)
	if err != nil {
		logger.Error(ctx, "failed to start rekreativko gateway", "error", err)
		os.Exit(1)
	}

}

func prepareNotifications(
	broker events.MessageBroker,
	appMetrics notification.Metrics,
	cfg *config.Config,
	log *logger.Logger,
) {
	var (
		emailSender notification.EmailSender
		smsSender   notification.SMSSender
	)

	if cfg.IsDevMode() {
		emailSender = notification.NewInMemoryEmailSender(log)
		smsSender = notification.NewInMemorySMSSender(log)
	}

	notificationService := notification.NewService(
		smsSender,
		emailSender,
		appMetrics,
	)

	ctx := context.Background()

	_ = broker.Subscribe(ctx, "identity.account.verified", notificationService.HandleAccountVerified)
	_ = broker.Subscribe(ctx, "identity.account.locked", notificationService.HandleAccountLocked)
	_ = broker.Subscribe(ctx, "identity.account.password.changed", notificationService.HandlePasswordChanged)
	_ = broker.Subscribe(ctx, "identity.verification_code.created", notificationService.HandleVerificationCodeGenerated)
}

func startServer(cfg *config.Config, handler http.Handler, log *logger.Logger) error {
	srv := http.Server{
		Addr:         cfg.Server.Address(),
		Handler:      handler,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	serverErrors := make(chan error, 1)

	go func() {
		log.Info(context.Background(), "http server starting", "addr", cfg.Server.Address())
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

func rootHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, `{"service": "rekreativko-api", "status": "running", "version": 1.0.0}`)
}
