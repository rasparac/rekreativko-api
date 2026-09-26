package application

import (
	"context"
	"fmt"
	"slices"

	"github.com/google/uuid"
	"github.com/rasparac/rekreativko-api/activity/internal/domain"
	"github.com/rasparac/rekreativko-api/activity/internal/infrastructure/persistence"
	"github.com/rasparac/rekreativko-api/activity/internal/metrics"
	"github.com/rasparac/rekreativko-api/shared/domainevent"
	"github.com/rasparac/rekreativko-api/shared/logger"
	"github.com/rasparac/rekreativko-api/shared/store/postgres"
	"github.com/rasparac/rekreativko-api/shared/telemetry"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// InviteService handles business logic for group invites
type InviteService struct {
	logger      *logger.Logger
	txManager   *postgres.TransactionManager
	inviteRepo  GroupInviteRepository
	memberRepo  MemberRepository
	eventWriter domainevent.EventWriter
	tracer      trace.Tracer
	metrics     *metrics.Metrics
}

// NewInviteService creates a new invite service
func NewInviteService(
	logger *logger.Logger,
	txManager *postgres.TransactionManager,
	inviteRepo GroupInviteRepository,
	memberRepo MemberRepository,
	eventWriter domainevent.EventWriter,
	metrics *metrics.Metrics,
) *InviteService {
	return &InviteService{
		logger:      logger.WithName("activity.invite_service"),
		txManager:   txManager,
		inviteRepo:  inviteRepo,
		memberRepo:  memberRepo,
		eventWriter: eventWriter,
		tracer:      telemetry.Tracer(telemetry.TracerActivityService),
		metrics:     metrics,
	}
}

// SendInvite sends an invite for a user to join an activity group
func (s *InviteService) SendInvite(
	ctx context.Context,
	params SendInviteParams,
) (*domain.GroupInvite, error) {
	ctx, span := s.tracer.Start(
		ctx,
		"activity.service.SendInvite",
	)
	defer span.End()

	log := s.logger.WithValues(
		"method", "SendInvite",
		"activity_group_id", params.ActivityGroupID,
		"requester_id", params.RequesterID,
		"invited_user_id", params.InvitedUserID,
	)

	span.SetAttributes(
		attribute.String("activity_group_id", params.ActivityGroupID.String()),
		attribute.String("requester_id", params.RequesterID.String()),
		attribute.String("invited_user_id", params.InvitedUserID.String()),
	)

	requesterRole, err := parseMemberRole(params.RequesterRole)
	if err != nil {
		span.SetStatus(codes.Error, err.Error())
		log.Error(ctx, "invalid requester role", "error", err)
		return nil, MapErrToAppError(err)
	}

	var invite *domain.GroupInvite

	err = s.txManager.WithTransaction(ctx, func(tCtx context.Context) error {
		// Reject if the invited user is already a member
		existingMember, err := s.memberRepo.GetMemberByGroupAndUser(
			tCtx,
			params.ActivityGroupID,
			params.InvitedUserID,
		)
		if err == nil && existingMember != nil {
			return domain.ErrUserAlreadyMember
		}

		// Reject if there's already a pending invite for this user in this group
		existingInvite, err := s.inviteRepo.GetPendingInviteByGroupAndUser(
			tCtx,
			params.ActivityGroupID,
			params.InvitedUserID,
		)
		if err == nil && existingInvite != nil {
			return domain.ErrAlreadyInvited
		}

		invite, err = domain.NewInvite(
			params.ActivityGroupID,
			params.InvitedUserID,
			params.RequesterID,
			requesterRole,
		)
		if err != nil {
			return fmt.Errorf("create invite: %w", err)
		}

		if err := s.inviteRepo.CreateInvite(tCtx, invite); err != nil {
			return fmt.Errorf("persist invite: %w", err)
		}

		if err := s.eventWriter.InsertEvents(tCtx, activitySchema, invite.Events()); err != nil {
			return fmt.Errorf("insert domain events: %w", err)
		}

		invite.ClearEvents()

		return nil
	})
	if err != nil {
		span.SetStatus(codes.Error, err.Error())
		log.Error(ctx, "failed to send invite", "error", err)
		return nil, mapToAppErr(err)
	}

	span.SetStatus(codes.Ok, "invite sent")
	log.Debug(ctx, "invite sent", "invite_id", invite.ID())

	return invite, nil
}

// AcceptInvite accepts a pending invite, creating a confirmed membership for the invited user
func (s *InviteService) AcceptInvite(
	ctx context.Context,
	inviteID uuid.UUID,
	userID uuid.UUID,
) (*domain.Member, error) {
	ctx, span := s.tracer.Start(
		ctx,
		"activity.service.AcceptInvite",
	)
	defer span.End()

	log := s.logger.WithValues(
		"method", "AcceptInvite",
		"invite_id", inviteID,
		"user_id", userID,
	)

	span.SetAttributes(
		attribute.String("invite_id", inviteID.String()),
		attribute.String("user_id", userID.String()),
	)

	var member *domain.Member

	err := s.txManager.WithTransaction(ctx, func(tCtx context.Context) error {
		invite, err := s.inviteRepo.GetInviteByID(tCtx, inviteID)
		if err != nil {
			return fmt.Errorf("get invite: %w", err)
		}

		if err := invite.Accept(userID); err != nil {
			return fmt.Errorf("accept invite: %w", err)
		}

		if err := s.inviteRepo.UpdateInvite(tCtx, invite); err != nil {
			return fmt.Errorf("persist invite acceptance: %w", err)
		}

		member = domain.NewMemberFromInvite(invite.ActivityGroupID(), userID)

		if err := s.memberRepo.CreateMember(tCtx, member); err != nil {
			return fmt.Errorf("persist member: %w", err)
		}

		// One insert for the invite's accepted event and the new member's
		// joined event.
		events := slices.Concat(invite.Events(), member.Events())
		if err := s.eventWriter.InsertEvents(tCtx, activitySchema, events); err != nil {
			return fmt.Errorf("insert domain events: %w", err)
		}
		invite.ClearEvents()
		member.ClearEvents()

		return nil
	})
	if err != nil {
		span.SetStatus(codes.Error, err.Error())
		log.Error(ctx, "failed to accept invite", "error", err)
		return nil, mapToAppErr(err)
	}

	span.SetStatus(codes.Ok, "invite accepted")
	log.Debug(ctx, "invite accepted", "member_id", member.ID())

	return member, nil
}

// DeclineInvite declines a pending invite; no membership is created
func (s *InviteService) DeclineInvite(
	ctx context.Context,
	inviteID uuid.UUID,
	userID uuid.UUID,
) error {
	ctx, span := s.tracer.Start(
		ctx,
		"activity.service.DeclineInvite",
	)
	defer span.End()

	log := s.logger.WithValues(
		"method", "DeclineInvite",
		"invite_id", inviteID,
		"user_id", userID,
	)

	span.SetAttributes(
		attribute.String("invite_id", inviteID.String()),
		attribute.String("user_id", userID.String()),
	)

	err := s.txManager.WithTransaction(ctx, func(tCtx context.Context) error {
		invite, err := s.inviteRepo.GetInviteByID(tCtx, inviteID)
		if err != nil {
			return fmt.Errorf("get invite: %w", err)
		}

		if err := invite.Decline(userID); err != nil {
			return fmt.Errorf("decline invite: %w", err)
		}

		if err := s.inviteRepo.UpdateInvite(tCtx, invite); err != nil {
			return fmt.Errorf("persist invite decline: %w", err)
		}

		if err := s.eventWriter.InsertEvents(tCtx, activitySchema, invite.Events()); err != nil {
			return fmt.Errorf("insert domain events: %w", err)
		}
		invite.ClearEvents()

		return nil
	})
	if err != nil {
		span.SetStatus(codes.Error, err.Error())
		log.Error(ctx, "failed to decline invite", "error", err)
		return mapToAppErr(err)
	}

	span.SetStatus(codes.Ok, "invite declined")
	log.Debug(ctx, "invite declined")

	return nil
}

// ExpireStaleInvites finds every pending invite past its expiry and marks it
// expired. Meant to be run periodically by a cron job - accepting/declining
// an expired invite already fails on its own, but nothing else ever flips
// its status, which otherwise leaves it stuck as "pending" forever (still
// shown by ListMyInvites, and blocking SendInvite's duplicate-invite check).
func (s *InviteService) ExpireStaleInvites(ctx context.Context) (int, error) {
	ctx, span := s.tracer.Start(
		ctx,
		"activity.service.ExpireStaleInvites",
	)
	defer span.End()

	log := s.logger.WithValues("method", "ExpireStaleInvites")

	invites, err := s.inviteRepo.FindExpiredPendingInvites(ctx)
	if err != nil {
		span.SetStatus(codes.Error, err.Error())
		log.Error(ctx, "failed to find expired invites", "error", err)
		return 0, mapToAppErr(err)
	}

	var expiredCount int
	for _, invite := range invites {
		err := s.txManager.WithTransaction(ctx, func(tCtx context.Context) error {
			if err := invite.Expire(); err != nil {
				return fmt.Errorf("expire invite: %w", err)
			}

			if err := s.inviteRepo.UpdateInvite(tCtx, invite); err != nil {
				return fmt.Errorf("persist invite expiry: %w", err)
			}

			if err := s.eventWriter.InsertEvents(tCtx, activitySchema, invite.Events()); err != nil {
				return fmt.Errorf("insert domain events: %w", err)
			}
			invite.ClearEvents()

			return nil
		})
		if err != nil {
			log.Error(ctx, "failed to expire invite", "invite_id", invite.ID(), "error", err)
			continue
		}
		expiredCount++
	}

	span.SetAttributes(attribute.Int("expired_count", expiredCount))
	log.Info(ctx, "expired stale invites", "count", expiredCount, "found", len(invites))

	return expiredCount, nil
}

// ListMyInvites retrieves the caller's own pending invites
func (s *InviteService) ListMyInvites(
	ctx context.Context,
	userID uuid.UUID,
	params ListMyInvitesParams,
) ([]*domain.GroupInvite, string, error) {
	ctx, span := s.tracer.Start(
		ctx,
		"activity.service.ListMyInvites",
	)
	defer span.End()

	log := s.logger.WithValues(
		"method", "ListMyInvites",
		"user_id", userID,
	)

	span.SetAttributes(attribute.String("user_id", userID.String()))

	invites, nextPageToken, err := s.inviteRepo.ListPendingInvitesForUser(ctx, persistence.ListPendingInvitesFilter{
		InvitedUserID: userID,
		Limit:         params.Limit,
		PageToken:     params.PageToken,
	})
	if err != nil {
		span.SetStatus(codes.Error, err.Error())
		log.Error(ctx, "failed to list invites", "error", err)
		return nil, "", mapToAppErr(err)
	}

	span.SetStatus(codes.Ok, "invites listed")
	log.Debug(ctx, "invites listed", "count", len(invites))

	return invites, nextPageToken, nil
}
