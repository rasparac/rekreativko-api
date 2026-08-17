package application

import (
	"context"
	"fmt"

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

// MemberService handles business logic for members
type MemberService struct {
	logger      *logger.Logger
	txManager   *postgres.TransactionManager
	memberRepo  MemberRepository
	eventWriter domainevent.EventWriter
	tracer      trace.Tracer
	metrics     *metrics.Metrics
}

// NewMemberService creates a new member service
func NewMemberService(
	logger *logger.Logger,
	txManager *postgres.TransactionManager,
	memberRepo MemberRepository,
	eventWriter domainevent.EventWriter,
	metrics *metrics.Metrics,
) *MemberService {
	return &MemberService{
		logger:      logger.WithName("activity.member_service"),
		txManager:   txManager,
		memberRepo:  memberRepo,
		eventWriter: eventWriter,
		tracer:      telemetry.Tracer(telemetry.TracerActivityService),
		metrics:     metrics,
	}
}

// InviteMember invites a user to join an activity group
func (s *MemberService) InviteMember(
	ctx context.Context,
	params InviteMemberParams,
) (*domain.Member, error) {
	ctx, span := s.tracer.Start(
		ctx,
		"activity.service.InviteMember",
	)
	defer span.End()

	log := s.logger.WithValues(
		"method", "InviteMember",
		"activity_group_id", params.ActivityGroupID,
		"requester_id", params.RequesterID,
		"user_id", params.UserID,
	)

	span.SetAttributes(
		attribute.String("activity_group_id", params.ActivityGroupID.String()),
		attribute.String("requester_id", params.RequesterID.String()),
		attribute.String("user_id", params.UserID.String()),
	)

	// Parse requester role
	requesterRole, err := parseMemberRole(params.RequesterRole)
	if err != nil {
		span.SetStatus(codes.Error, err.Error())
		log.Error(ctx, "invalid requester role", "error", err)
		return nil, MapErrToAppError(err)
	}

	// Validate requester has permission to invite
	if !requesterRole.CanManageMembers() {
		span.SetStatus(codes.Error, "unauthorized")
		log.Error(ctx, "requester cannot manage members")
		return nil, MapErrToAppError(domain.ErrUnauthorized)
	}

	var member *domain.Member

	err = s.txManager.WithTransaction(ctx, func(tCtx context.Context) error {
		// Check if user is already a member
		existingMember, err := s.memberRepo.GetMemberByGroupAndUser(
			tCtx,
			params.ActivityGroupID,
			params.UserID,
		)
		if err == nil && existingMember != nil {
			return fmt.Errorf("user is already a member of this group")
		}

		// Create new member from invite
		member = domain.NewMemberFromInvite(params.ActivityGroupID, params.UserID)

		// Persist to database
		err = s.memberRepo.CreateMember(tCtx, member)
		if err != nil {
			return fmt.Errorf("persist member: %w", err)
		}

		// Publish domain events
		err = s.eventWriter.InsertEvents(
			tCtx,
			activitySchema,
			member.Events(),
		)
		if err != nil {
			return fmt.Errorf("insert domain events: %w", err)
		}

		member.ClearEvents()

		return nil
	})
	if err != nil {
		span.SetStatus(codes.Error, err.Error())
		log.Error(ctx, "failed to invite member", "error", err)
		return nil, mapToAppErr(err)
	}

	span.SetStatus(codes.Ok, "member invited")
	log.Info(ctx, "member invited", "member_id", member.ID())

	return member, nil
}

// RemoveMember removes a member from an activity group
func (s *MemberService) RemoveMember(
	ctx context.Context,
	params RemoveMemberParams,
) error {
	ctx, span := s.tracer.Start(
		ctx,
		"activity.service.RemoveMember",
	)
	defer span.End()

	log := s.logger.WithValues(
		"method", "RemoveMember",
		"activity_group_id", params.ActivityGroupID,
		"requester_id", params.RequesterID,
		"user_id", params.UserID,
	)

	span.SetAttributes(
		attribute.String("activity_group_id", params.ActivityGroupID.String()),
		attribute.String("requester_id", params.RequesterID.String()),
		attribute.String("user_id", params.UserID.String()),
	)

	// Parse requester role
	requesterRole, err := parseMemberRole(params.RequesterRole)
	if err != nil {
		span.SetStatus(codes.Error, err.Error())
		log.Error(ctx, "invalid requester role", "error", err)
		return MapErrToAppError(err)
	}

	err = s.txManager.WithTransaction(ctx, func(tCtx context.Context) error {
		// Get the member to remove
		member, err := s.memberRepo.GetMemberByGroupAndUser(
			tCtx,
			params.ActivityGroupID,
			params.UserID,
		)
		if err != nil {
			return fmt.Errorf("get member: %w", err)
		}

		// Remove the member (domain validates permissions)
		err = member.Remove(params.RequesterID, requesterRole)
		if err != nil {
			return fmt.Errorf("remove member: %w", err)
		}

		// Persist changes
		err = s.memberRepo.UpdateMember(tCtx, member)
		if err != nil {
			return fmt.Errorf("persist member removal: %w", err)
		}

		// Publish domain events
		err = s.eventWriter.InsertEvents(
			tCtx,
			activitySchema,
			member.Events(),
		)
		if err != nil {
			return fmt.Errorf("insert domain events: %w", err)
		}

		member.ClearEvents()

		return nil
	})
	if err != nil {
		span.SetStatus(codes.Error, err.Error())
		log.Error(ctx, "failed to remove member", "error", err)
		return mapToAppErr(err)
	}

	span.SetStatus(codes.Ok, "member removed")
	log.Info(ctx, "member removed")

	return nil
}

// PromoteMember promotes a member to admin role
func (s *MemberService) PromoteMember(
	ctx context.Context,
	params UpdateMemberRoleParams,
) error {
	ctx, span := s.tracer.Start(
		ctx,
		"activity.service.PromoteMember",
	)
	defer span.End()

	log := s.logger.WithValues(
		"method", "PromoteMember",
		"activity_group_id", params.ActivityGroupID,
		"requester_id", params.RequesterID,
		"user_id", params.UserID,
	)

	span.SetAttributes(
		attribute.String("activity_group_id", params.ActivityGroupID.String()),
		attribute.String("requester_id", params.RequesterID.String()),
		attribute.String("user_id", params.UserID.String()),
	)

	// Parse requester role
	requesterRole, err := parseMemberRole(params.RequesterRole)
	if err != nil {
		span.SetStatus(codes.Error, err.Error())
		log.Error(ctx, "invalid requester role", "error", err)
		return MapErrToAppError(err)
	}

	err = s.txManager.WithTransaction(ctx, func(tCtx context.Context) error {
		// Get the member to promote
		member, err := s.memberRepo.GetMemberByGroupAndUser(
			tCtx,
			params.ActivityGroupID,
			params.UserID,
		)
		if err != nil {
			return fmt.Errorf("get member: %w", err)
		}

		// Promote to admin (domain validates permissions)
		err = member.PromoteToAdmin(params.RequesterID, requesterRole)
		if err != nil {
			return fmt.Errorf("promote member: %w", err)
		}

		// Persist changes
		err = s.memberRepo.UpdateMember(tCtx, member)
		if err != nil {
			return fmt.Errorf("persist member promotion: %w", err)
		}

		// Publish domain events
		err = s.eventWriter.InsertEvents(
			tCtx,
			activitySchema,
			member.Events(),
		)
		if err != nil {
			return fmt.Errorf("insert domain events: %w", err)
		}

		member.ClearEvents()

		return nil
	})
	if err != nil {
		span.SetStatus(codes.Error, err.Error())
		log.Error(ctx, "failed to promote member", "error", err)
		return mapToAppErr(err)
	}

	span.SetStatus(codes.Ok, "member promoted")
	log.Info(ctx, "member promoted to admin")

	return nil
}

// DemoteMember demotes an admin to member role
func (s *MemberService) DemoteMember(
	ctx context.Context,
	params UpdateMemberRoleParams,
) error {
	ctx, span := s.tracer.Start(
		ctx,
		"activity.service.DemoteMember",
	)
	defer span.End()

	log := s.logger.WithValues(
		"method", "DemoteMember",
		"activity_group_id", params.ActivityGroupID,
		"requester_id", params.RequesterID,
		"user_id", params.UserID,
	)

	span.SetAttributes(
		attribute.String("activity_group_id", params.ActivityGroupID.String()),
		attribute.String("requester_id", params.RequesterID.String()),
		attribute.String("user_id", params.UserID.String()),
	)

	// Parse requester role
	requesterRole, err := parseMemberRole(params.RequesterRole)
	if err != nil {
		span.SetStatus(codes.Error, err.Error())
		log.Error(ctx, "invalid requester role", "error", err)
		return MapErrToAppError(err)
	}

	err = s.txManager.WithTransaction(ctx, func(tCtx context.Context) error {
		// Get the member to demote
		member, err := s.memberRepo.GetMemberByGroupAndUser(
			tCtx,
			params.ActivityGroupID,
			params.UserID,
		)
		if err != nil {
			return fmt.Errorf("get member: %w", err)
		}

		// Demote to member (domain validates permissions)
		err = member.DemoteToMember(params.RequesterID, requesterRole)
		if err != nil {
			return fmt.Errorf("demote member: %w", err)
		}

		// Persist changes
		err = s.memberRepo.UpdateMember(tCtx, member)
		if err != nil {
			return fmt.Errorf("persist member demotion: %w", err)
		}

		// Publish domain events
		err = s.eventWriter.InsertEvents(
			tCtx,
			activitySchema,
			member.Events(),
		)
		if err != nil {
			return fmt.Errorf("insert domain events: %w", err)
		}

		member.ClearEvents()

		return nil
	})
	if err != nil {
		span.SetStatus(codes.Error, err.Error())
		log.Error(ctx, "failed to demote member", "error", err)
		return mapToAppErr(err)
	}

	span.SetStatus(codes.Ok, "member demoted")
	log.Info(ctx, "member demoted to member role")

	return nil
}

// ApproveMember approves a pending join request
func (s *MemberService) ApproveMember(
	ctx context.Context,
	params ApproveMemberParams,
) error {
	ctx, span := s.tracer.Start(
		ctx,
		"activity.service.ApproveMember",
	)
	defer span.End()

	log := s.logger.WithValues(
		"method", "ApproveMember",
		"activity_group_id", params.ActivityGroupID,
		"requester_id", params.RequesterID,
		"user_id", params.UserID,
	)

	span.SetAttributes(
		attribute.String("activity_group_id", params.ActivityGroupID.String()),
		attribute.String("requester_id", params.RequesterID.String()),
		attribute.String("user_id", params.UserID.String()),
	)

	// Parse requester role
	requesterRole, err := parseMemberRole(params.RequesterRole)
	if err != nil {
		span.SetStatus(codes.Error, err.Error())
		log.Error(ctx, "invalid requester role", "error", err)
		return MapErrToAppError(err)
	}

	err = s.txManager.WithTransaction(ctx, func(tCtx context.Context) error {
		// Get the member to approve
		member, err := s.memberRepo.GetMemberByGroupAndUser(
			tCtx,
			params.ActivityGroupID,
			params.UserID,
		)
		if err != nil {
			return fmt.Errorf("get member: %w", err)
		}

		// Approve the member (domain validates permissions)
		err = member.Approve(params.RequesterID, requesterRole)
		if err != nil {
			return fmt.Errorf("approve member: %w", err)
		}

		// Persist changes
		err = s.memberRepo.UpdateMember(tCtx, member)
		if err != nil {
			return fmt.Errorf("persist member approval: %w", err)
		}

		// Publish domain events
		err = s.eventWriter.InsertEvents(
			tCtx,
			activitySchema,
			member.Events(),
		)
		if err != nil {
			return fmt.Errorf("insert domain events: %w", err)
		}

		member.ClearEvents()

		return nil
	})
	if err != nil {
		span.SetStatus(codes.Error, err.Error())
		log.Error(ctx, "failed to approve member", "error", err)
		return mapToAppErr(err)
	}

	span.SetStatus(codes.Ok, "member approved")
	log.Info(ctx, "member approved")

	return nil
}

// RejectMember rejects a pending join request
func (s *MemberService) RejectMember(
	ctx context.Context,
	params RejectMemberParams,
) error {
	ctx, span := s.tracer.Start(
		ctx,
		"activity.service.RejectMember",
	)
	defer span.End()

	log := s.logger.WithValues(
		"method", "RejectMember",
		"activity_group_id", params.ActivityGroupID,
		"requester_id", params.RequesterID,
		"user_id", params.UserID,
	)

	span.SetAttributes(
		attribute.String("activity_group_id", params.ActivityGroupID.String()),
		attribute.String("requester_id", params.RequesterID.String()),
		attribute.String("user_id", params.UserID.String()),
	)

	// Parse requester role
	requesterRole, err := parseMemberRole(params.RequesterRole)
	if err != nil {
		span.SetStatus(codes.Error, err.Error())
		log.Error(ctx, "invalid requester role", "error", err)
		return MapErrToAppError(err)
	}

	err = s.txManager.WithTransaction(ctx, func(tCtx context.Context) error {
		// Get the member to reject
		member, err := s.memberRepo.GetMemberByGroupAndUser(
			tCtx,
			params.ActivityGroupID,
			params.UserID,
		)
		if err != nil {
			return fmt.Errorf("get member: %w", err)
		}

		// Reject the member (domain validates permissions)
		err = member.Reject(params.RequesterID, requesterRole)
		if err != nil {
			return fmt.Errorf("reject member: %w", err)
		}

		// Persist changes
		err = s.memberRepo.UpdateMember(tCtx, member)
		if err != nil {
			return fmt.Errorf("persist member rejection: %w", err)
		}

		// Publish domain events
		err = s.eventWriter.InsertEvents(
			tCtx,
			activitySchema,
			member.Events(),
		)
		if err != nil {
			return fmt.Errorf("insert domain events: %w", err)
		}

		member.ClearEvents()

		return nil
	})
	if err != nil {
		span.SetStatus(codes.Error, err.Error())
		log.Error(ctx, "failed to reject member", "error", err)
		return mapToAppErr(err)
	}

	span.SetStatus(codes.Ok, "member rejected")
	log.Info(ctx, "member rejected")

	return nil
}

// LeaveMember allows a member to leave an activity group
func (s *MemberService) LeaveMember(
	ctx context.Context,
	activityGroupID uuid.UUID,
	userID uuid.UUID,
) error {
	ctx, span := s.tracer.Start(
		ctx,
		"activity.service.LeaveMember",
	)
	defer span.End()

	log := s.logger.WithValues(
		"method", "LeaveMember",
		"activity_group_id", activityGroupID,
		"user_id", userID,
	)

	span.SetAttributes(
		attribute.String("activity_group_id", activityGroupID.String()),
		attribute.String("user_id", userID.String()),
	)

	err := s.txManager.WithTransaction(ctx, func(tCtx context.Context) error {
		// Get the member
		member, err := s.memberRepo.GetMemberByGroupAndUser(
			tCtx,
			activityGroupID,
			userID,
		)
		if err != nil {
			return fmt.Errorf("get member: %w", err)
		}

		// Leave the group
		err = member.Leave()
		if err != nil {
			return fmt.Errorf("leave group: %w", err)
		}

		// Persist changes
		err = s.memberRepo.UpdateMember(tCtx, member)
		if err != nil {
			return fmt.Errorf("persist member leave: %w", err)
		}

		// Publish domain events
		err = s.eventWriter.InsertEvents(
			tCtx,
			activitySchema,
			member.Events(),
		)
		if err != nil {
			return fmt.Errorf("insert domain events: %w", err)
		}

		member.ClearEvents()

		return nil
	})
	if err != nil {
		span.SetStatus(codes.Error, err.Error())
		log.Error(ctx, "failed to leave group", "error", err)
		return mapToAppErr(err)
	}

	span.SetStatus(codes.Ok, "member left")
	log.Info(ctx, "member left group")

	return nil
}

// GetMember retrieves a member by activity group and user ID
func (s *MemberService) GetMember(
	ctx context.Context,
	activityGroupID uuid.UUID,
	userID uuid.UUID,
) (*domain.Member, error) {
	ctx, span := s.tracer.Start(
		ctx,
		"activity.service.GetMember",
	)
	defer span.End()

	log := s.logger.WithValues(
		"method", "GetMember",
		"activity_group_id", activityGroupID,
		"user_id", userID,
	)

	span.SetAttributes(
		attribute.String("activity_group_id", activityGroupID.String()),
		attribute.String("user_id", userID.String()),
	)

	member, err := s.memberRepo.GetMemberByGroupAndUser(ctx, activityGroupID, userID)
	if err != nil {
		span.SetStatus(codes.Error, err.Error())
		log.Error(ctx, "failed to get member", "error", err)
		return nil, mapToAppErr(err)
	}

	span.SetStatus(codes.Ok, "member found")
	log.Debug(ctx, "member found")

	return member, nil
}

// ListMembers retrieves members with optional filters
func (s *MemberService) ListMembers(
	ctx context.Context,
	params ListMembersParams,
) ([]*domain.Member, error) {
	ctx, span := s.tracer.Start(
		ctx,
		"activity.service.ListMembers",
	)
	defer span.End()

	log := s.logger.WithValues(
		"method", "ListMembers",
	)

	if params.ActivityGroupID != nil {
		span.SetAttributes(attribute.String("activity_group_id", params.ActivityGroupID.String()))
	}

	// Convert params to repository filter
	var status *domain.MemberStatus
	if params.Status != nil {
		s := domain.MemberStatus(*params.Status)
		status = &s
	}

	var role *domain.MemberRole
	if params.Role != nil {
		r := domain.MemberRole(*params.Role)
		role = &r
	}

	filter := persistence.MemberFilter{
		ActivityGroupID: params.ActivityGroupID,
		UserID:          params.UserID,
		Status:          status,
		Role:            role,
		Limit:           params.Limit,
		Offset:          params.Offset,
	}

	members, err := s.memberRepo.ListMembers(ctx, filter)
	if err != nil {
		span.SetStatus(codes.Error, err.Error())
		log.Error(ctx, "failed to list members", "error", err)
		return nil, mapToAppErr(err)
	}

	span.SetStatus(codes.Ok, "members listed")
	log.Debug(ctx, "members listed", "count", len(members))

	return members, nil
}
