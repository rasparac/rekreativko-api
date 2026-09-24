package main

import (
	"context"
	"log/slog"
	"os"
	"time"

	"github.com/rasparac/rekreativko-api/activity/internal/application"
	"github.com/rasparac/rekreativko-api/activity/internal/infrastructure/persistence"
	"github.com/rasparac/rekreativko-api/activity/internal/metrics"
	"github.com/rasparac/rekreativko-api/shared/config"
	"github.com/rasparac/rekreativko-api/shared/domainevent"
	"github.com/rasparac/rekreativko-api/shared/logger"
	metricstracer "github.com/rasparac/rekreativko-api/shared/store/metrics_tracer"
	"github.com/rasparac/rekreativko-api/shared/store/postgres"
	"github.com/rasparac/rekreativko-api/shared/telemetry"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		slog.Error("failed to load configuration", "error", err)
		os.Exit(1)
	}

	ctx := context.Background()

	logger := logger.New(cfg.Logger.Level, cfg.Logger.Format)
	log := logger.WithName(cfg.Service.Name)

	log.Info(ctx, "starting activity cron job", "version", cfg.Service.Version)

	if err := run(ctx, cfg, log); err != nil {
		log.Error(ctx, "cron job failed", "error", err)
		os.Exit(1)
	}

	log.Info(ctx, "cron job completed successfully")
}

func run(ctx context.Context, cfg *config.Config, log *logger.Logger) error {
	// Initialize metrics
	appMetrics := metrics.New()
	dbTracer := metricstracer.New(appMetrics)

	// Initialize database
	pg, err := postgres.New(ctx, log, cfg.Postgres, dbTracer)
	if err != nil {
		return err
	}
	defer pg.Close()

	txManager := postgres.NewTransactionManager(pg.Pool)
	domainEventMgr := domainevent.NewDomainEventManager(txManager)

	// Initialize telemetry
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
		return err
	}
	defer func() {
		if tracingErr := shutdownTracing(ctx); tracingErr != nil {
			log.Error(ctx, "failed to shutdown telemetry", "error", tracingErr)
		}
	}()

	// Initialize repositories
	sessionTemplateRepo := persistence.NewSessionTemplateManager(txManager, log)
	sessionRepo := persistence.NewSessionManager(txManager, log)
	activityGroupRepo := persistence.NewActivityGroupRepository(txManager, log)
	memberRepo := persistence.NewMemberRepository(txManager, log)
	groupInviteRepo := persistence.NewGroupInviteRepository(txManager, log)
	attendeeRepo := persistence.NewAttendeeRepository(txManager, log)
	sessionInviteRepo := persistence.NewSessionInviteRepository(txManager, log)

	// Initialize session generator service
	// Lookahead window: generate sessions for the next 14 days
	lookaheadWindow := 14 * 24 * time.Hour
	sessionGenerator := application.NewSessionGeneratorService(
		log,
		txManager,
		sessionTemplateRepo,
		sessionRepo,
		activityGroupRepo,
		domainEventMgr,
		appMetrics,
		lookaheadWindow,
	)

	// Generate sessions from templates
	log.Info(ctx, "generating sessions from recurring templates")
	if err := sessionGenerator.GenerateSessionsFromTemplates(ctx); err != nil {
		return err
	}

	// Expire group invites that have passed their expiry window
	inviteService := application.NewInviteService(
		log,
		txManager,
		groupInviteRepo,
		memberRepo,
		domainEventMgr,
		appMetrics,
	)

	log.Info(ctx, "expiring stale group invites")
	if _, err := inviteService.ExpireStaleInvites(ctx); err != nil {
		return err
	}

	// Expire session invites that have passed their expiry window
	sessionInviteService := application.NewSessionInviteService(
		log,
		txManager,
		sessionInviteRepo,
		sessionRepo,
		attendeeRepo,
		domainEventMgr,
		appMetrics,
	)

	log.Info(ctx, "expiring stale session invites")
	if _, err := sessionInviteService.ExpireStaleInvites(ctx); err != nil {
		return err
	}

	// Auto-complete sessions that have passed their end time
	sessionService := application.NewSessionService(
		log,
		txManager,
		sessionRepo,
		memberRepo,
		activityGroupRepo,
		attendeeRepo,
		domainEventMgr,
		appMetrics,
	)

	log.Info(ctx, "expiring sessions past their end time")
	if _, err := sessionService.ExpireCompletedSessions(ctx); err != nil {
		return err
	}

	return nil
}
