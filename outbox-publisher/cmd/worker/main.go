package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/rasparac/rekreativko-api/shared/config"
	"github.com/rasparac/rekreativko-api/shared/domainevent"
	"github.com/rasparac/rekreativko-api/shared/events"
	"github.com/rasparac/rekreativko-api/shared/logger"
	metricstracer "github.com/rasparac/rekreativko-api/shared/store/metrics_tracer"
	"github.com/rasparac/rekreativko-api/shared/store/postgres"
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
		"starting rekreativko outbox publisher",
		"version", cfg.Service.Version,
		"environment", cfg.Service.Environment,
	)

	appMetrics := events.New()

	dbTracer := metricstracer.New(appMetrics)

	pg, err := postgres.New(
		ctx,
		log,
		cfg.Postgres,
		dbTracer,
	)
	if err != nil {
		log.Error(ctx, "error initializing postgres connection", "error", err)
		os.Exit(1)
	}
	defer pg.Close()

	messageBroker, err := events.NewNatsBroker(
		cfg.NatsConfig.URL,
		"outbox",
		log,
	)
	if err != nil {
		log.Error(ctx, "error initializing nats connection", "error", err)
		os.Exit(1)
	}
	defer messageBroker.Close(ctx)

	txManager := postgres.NewTransactionManager(pg.Pool)

	domainEventMgr := domainevent.NewDomainEventManager(txManager)

	schemas := cfg.Outbox.GetSchemas()
	if len(schemas) == 0 {
		log.Error(ctx, "no schemas configured for outbox publisher")
		os.Exit(1)
	}

	log.Info(ctx, "configured schemas for outbox publisher", "schemas", schemas)

	outboxPublisher := events.NewOutboxPublisher(
		domainEventMgr,
		messageBroker,
		log,
		cfg.Outbox.ReadLimit,
		cfg.Outbox.PollIntervalS,
		appMetrics,
		schemas,
	)

	go func() {
		err := outboxPublisher.Start(ctx)
		if err != nil {
			log.Error(ctx, "failed to start outbox publisher", "error", err)
		}
	}()

	mux := http.NewServeMux()

	mux.Handle("GET /metrics", promhttp.Handler())

	mux.Handle("GET /health", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	if err := startServer(cfg.Server.Address(), mux, log); err != nil {
		log.Error(ctx, "server error", "error", err)
		os.Exit(1)
	}

	log.Info(ctx, "shutting down rekreativko outbox publisher")
}

func startServer(
	addr string,
	handler http.Handler,
	log *logger.Logger,
) error {
	srv := http.Server{
		Addr:         addr,
		Handler:      handler,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	serverErrors := make(chan error, 1)

	go func() {
		log.Info(context.Background(), "starting outbox publisher metrics server", "addr", addr)
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
