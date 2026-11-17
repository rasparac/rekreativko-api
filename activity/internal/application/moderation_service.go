package application

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/rasparac/rekreativko-api/activity/internal/domain"
	"github.com/rasparac/rekreativko-api/activity/internal/metrics"
	"github.com/rasparac/rekreativko-api/shared/logger"
	"github.com/rasparac/rekreativko-api/shared/store/postgres"
	"github.com/rasparac/rekreativko-api/shared/telemetry"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// MaxListedBlocks caps GET /blocks: a block list is short, so it is one page.
const MaxListedBlocks = 500

// ReportParams is a user's report; Note is optional free text (never logged).
type ReportParams struct {
	ReporterID uuid.UUID
	TargetType string
	TargetID   uuid.UUID
	Reason     string
	Note       string
}

// ModerationService handles reports of users, groups and sessions, and
// blocks between accounts (rekreativko-api-ep8.8).
type ModerationService struct {
	logger      *logger.Logger
	txManager   *postgres.TransactionManager
	moderation  ModerationRepository
	groupRepo   ActivityGroupRepository
	memberRepo  MemberRepository
	sessionRepo SessionRepository
	tracer      trace.Tracer
	metrics     *metrics.Metrics
}

func NewModerationService(
	logger *logger.Logger,
	txManager *postgres.TransactionManager,
	moderation ModerationRepository,
	groupRepo ActivityGroupRepository,
	memberRepo MemberRepository,
	sessionRepo SessionRepository,
	metrics *metrics.Metrics,
) *ModerationService {
	return &ModerationService{
		logger:      logger.WithName("activity.moderation_service"),
		txManager:   txManager,
		moderation:  moderation,
		groupRepo:   groupRepo,
		memberRepo:  memberRepo,
		sessionRepo: sessionRepo,
		tracer:      telemetry.Tracer(telemetry.TracerActivityService),
		metrics:     metrics,
	}
}

// Report stores a report for an operator to review. A group or session the
// reporter can't see is not found, as if it didn't exist. Reporting the same
// target again while the first report is open is accepted and changes nothing.
func (s *ModerationService) Report(ctx context.Context, params ReportParams) error {
	ctx, span := s.tracer.Start(ctx, "activity.service.Report")
	defer span.End()

	log := s.logger.WithValues(
		"method", "Report",
		"reporter_id", params.ReporterID,
		"target_type", params.TargetType,
		"target_id", params.TargetID,
	)
	span.SetAttributes(
		attribute.String("target_type", params.TargetType),
		attribute.String("reason", params.Reason),
	)

	report, err := domain.NewReport(
		params.ReporterID,
		domain.ReportTargetType(params.TargetType),
		params.TargetID,
		domain.ReportReason(params.Reason),
		params.Note,
	)
	if err != nil {
		span.SetStatus(codes.Error, err.Error())
		return MapErrToAppError(err)
	}

	var created bool
	err = s.txManager.WithTransaction(ctx, func(tCtx context.Context) error {
		if err := s.requireVisibleTarget(tCtx, report); err != nil {
			return err
		}

		created, err = s.moderation.CreateReport(tCtx, report)
		if err != nil {
			return fmt.Errorf("persist report: %w", err)
		}

		return nil
	})
	if err != nil {
		span.SetStatus(codes.Error, err.Error())
		log.Warn(ctx, "report not stored", "error", err)
		return mapToAppErr(err)
	}

	if created {
		s.metrics.ReportsCreatedTotal.WithLabelValues(string(report.TargetType()), string(report.Reason())).Inc()
		log.Info(ctx, "report received", "report_id", report.ID(), "reason", report.Reason())
	} else {
		log.Debug(ctx, "repeat of an open report")
	}

	span.SetStatus(codes.Ok, "report stored")
	return nil
}

// requireVisibleTarget: a reported group or session must exist and be
// visible to the reporter. An account can't be checked here (activity keeps
// no account list), so any id is taken.
func (s *ModerationService) requireVisibleTarget(ctx context.Context, report *domain.Report) error {
	switch report.TargetType() {
	case domain.ReportTargetGroup:
		_, err := visibleGroup(ctx, s.groupRepo, s.memberRepo, report.TargetID(), report.ReporterID())
		return err
	case domain.ReportTargetSession:
		visible, err := s.sessionRepo.IsSessionVisibleTo(ctx, report.TargetID(), report.ReporterID())
		if err != nil {
			return fmt.Errorf("check session visibility: %w", err)
		}
		if !visible {
			return domain.ErrSessionNotFound
		}
	}

	return nil
}

// Block blocks an account for the blocker; blocking it again is a no-op.
func (s *ModerationService) Block(ctx context.Context, blockerID, blockedID uuid.UUID) error {
	ctx, span := s.tracer.Start(ctx, "activity.service.Block")
	defer span.End()

	log := s.logger.WithValues("method", "Block", "account_id", blockerID, "blocked_id", blockedID)

	block, err := domain.NewAccountBlock(blockerID, blockedID)
	if err != nil {
		span.SetStatus(codes.Error, err.Error())
		return MapErrToAppError(err)
	}

	if err := s.moderation.Block(ctx, block); err != nil {
		span.SetStatus(codes.Error, err.Error())
		log.Error(ctx, "failed to block account", "error", err)
		return mapToAppErr(err)
	}

	span.SetStatus(codes.Ok, "account blocked")
	log.Info(ctx, "account blocked")
	return nil
}

// Unblock removes a block; unblocking an account that isn't blocked is a no-op.
func (s *ModerationService) Unblock(ctx context.Context, blockerID, blockedID uuid.UUID) error {
	ctx, span := s.tracer.Start(ctx, "activity.service.Unblock")
	defer span.End()

	log := s.logger.WithValues("method", "Unblock", "account_id", blockerID, "blocked_id", blockedID)

	if err := s.moderation.Unblock(ctx, blockerID, blockedID); err != nil {
		span.SetStatus(codes.Error, err.Error())
		log.Error(ctx, "failed to unblock account", "error", err)
		return mapToAppErr(err)
	}

	span.SetStatus(codes.Ok, "account unblocked")
	log.Info(ctx, "account unblocked")
	return nil
}

// ListBlocks returns the accounts the blocker has blocked, newest first, at
// most MaxListedBlocks.
func (s *ModerationService) ListBlocks(ctx context.Context, blockerID uuid.UUID) ([]*domain.AccountBlock, error) {
	ctx, span := s.tracer.Start(ctx, "activity.service.ListBlocks")
	defer span.End()

	blocks, err := s.moderation.ListBlocked(ctx, blockerID, MaxListedBlocks)
	if err != nil {
		span.SetStatus(codes.Error, err.Error())
		s.logger.Error(ctx, "failed to list blocks", "account_id", blockerID, "error", err)
		return nil, mapToAppErr(err)
	}

	if len(blocks) == MaxListedBlocks {
		s.logger.Warn(ctx, "block list cut at its cap", "account_id", blockerID, "cap", MaxListedBlocks)
	}

	span.SetAttributes(attribute.Int("blocks", len(blocks)))
	span.SetStatus(codes.Ok, "blocks listed")
	return blocks, nil
}
