package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/rasparac/rekreativko-api/internal/shared/config"
	"github.com/rasparac/rekreativko-api/internal/shared/domainevent"
	"github.com/rasparac/rekreativko-api/internal/shared/events"
	"github.com/rasparac/rekreativko-api/internal/shared/logger"
	metricstracer "github.com/rasparac/rekreativko-api/internal/shared/store/metrics_tracer"
	"github.com/rasparac/rekreativko-api/internal/shared/store/postgres"
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

	appMetrics := events.New(cfg.Service.Name)

	dbTracer := metricstracer.New(appMetrics)

	pg, err := postgres.New(
		ctx,
		log,
		cfg.Postgres,
		dbTracer,
	)
	if err != nil {
		slog.Error("error initializing postgres connection", "error", err)
		os.Exit(1)
	}
	defer pg.Close()

	messageBroker, err := events.NewNatsBroker(
		cfg.NatsConfig.URL,
		"outbox",
		log,
	)
	if err != nil {
		slog.Error("error initializing nats connection", "error", err)
		os.Exit(1)
	}
	defer messageBroker.Close(ctx)

	txManager := postgres.NewTransactionManager(pg.Pool)

	domainEventMgr := domainevent.NewDomainEventManager(txManager)

	outboxPublisher := events.NewOutboxPublisher(
		domainEventMgr,
		messageBroker,
		log,
		cfg.Outbox.ReadLimit,
		cfg.Outbox.PollIntervalS,
		appMetrics,
	)

	go func() {
		err := outboxPublisher.Start(ctx)
		if err != nil {
			log.Error(ctx, "failed to start outbox publisher", "error", err)
		}
	}()

	// mux := http.NewServeMux()

	// mux.Handle("GET /health", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
	// 	w.WriteHeader(http.StatusOK)
	// }))

	singalCh := make(chan os.Signal, 1)
	signal.Notify(singalCh, os.Interrupt, syscall.SIGTERM)

	<-singalCh

	log.Info(ctx, "shutting down rekreativko outbox publisher")
}
