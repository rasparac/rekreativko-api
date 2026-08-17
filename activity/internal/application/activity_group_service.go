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

// ActivityGroupService handles business logic for activity groups
type ActivityGroupService struct {
	logger      *logger.Logger
	txManager   *postgres.TransactionManager
	groupRepo   ActivityGroupRepository
	eventWriter domainevent.EventWriter
	tracer      trace.Tracer
	metrics     *metrics.Metrics
}

// NewActivityGroupService creates a new activity group service
func NewActivityGroupService(
	logger *logger.Logger,
	txManager *postgres.TransactionManager,
	groupRepo ActivityGroupRepository,
	eventWriter domainevent.EventWriter,
	metrics *metrics.Metrics,
) *ActivityGroupService {
	return &ActivityGroupService{
		logger:      logger.WithName("activity.activity_group_service"),
		txManager:   txManager,
		groupRepo:   groupRepo,
		eventWriter: eventWriter,
		tracer:      telemetry.Tracer(telemetry.TracerActivityService),
		metrics:     metrics,
	}
}

// CreateActivityGroup creates a new activity group
func (s *ActivityGroupService) CreateActivityGroup(
	ctx context.Context,
	params CreateActivityGroupParams,
) (*domain.ActivityGroup, error) {
	ctx, span := s.tracer.Start(
		ctx,
		"activity.service.CreateActivityGroup",
	)
	defer span.End()

	log := s.logger.WithValues(
		"method", "CreateActivityGroup",
		"creator_id", params.CreatorID,
		"title", params.Title,
	)

	span.SetAttributes(
		attribute.String("creator_id", params.CreatorID.String()),
		attribute.String("title", params.Title),
		attribute.String("activity_type", params.ActivityType),
	)

	// Parse and validate parameters
	title, err := domain.NewTitle(params.Title)
	if err != nil {
		span.SetStatus(codes.Error, err.Error())
		log.Error(ctx, "invalid title", "error", err)
		return nil, MapErrToAppError(err)
	}

	location, err := domain.NewActivityGroupLocation(params.LocationCity, params.LocationCountry)
	if err != nil {
		span.SetStatus(codes.Error, err.Error())
		log.Error(ctx, "invalid location", "error", err)
		return nil, MapErrToAppError(err)
	}

	activityType := domain.ActivityType(params.ActivityType)
	if !activityType.IsValid() {
		err := fmt.Errorf("invalid activity type: %s", params.ActivityType)
		span.SetStatus(codes.Error, err.Error())
		log.Error(ctx, "invalid activity type", "error", err)
		return nil, MapErrToAppError(err)
	}

	difficultyLevel := domain.DifficultyLevel(params.DifficultyLevel)
	if !difficultyLevel.IsValid() {
		err := fmt.Errorf("invalid difficulty level: %s", params.DifficultyLevel)
		span.SetStatus(codes.Error, err.Error())
		log.Error(ctx, "invalid difficulty level", "error", err)
		return nil, MapErrToAppError(err)
	}

	visibility := domain.ActivityGroupVisibility(params.Visibility)
	if !visibility.IsValid() {
		err := fmt.Errorf("invalid visibility: %s", params.Visibility)
		span.SetStatus(codes.Error, err.Error())
		log.Error(ctx, "invalid visibility", "error", err)
		return nil, MapErrToAppError(err)
	}

	// Build capacity if provided
	var capacity *domain.Capacity
	if params.DefaultCapacity != nil {
		cap, err := domain.NewCapacity(*params.DefaultCapacity)
		if err != nil {
			span.SetStatus(codes.Error, err.Error())
			log.Error(ctx, "invalid capacity", "error", err)
			return nil, MapErrToAppError(err)
		}
		capacity = &cap
	}

	var group *domain.ActivityGroup
	err = s.txManager.WithTransaction(ctx, func(tCtx context.Context) error {
		// Create the domain object
		group, err = domain.NewActivityGroup(
			params.CreatorID,
			title,
			params.Description,
			activityType,
			location,
			difficultyLevel,
			visibility,
			params.Timezone,
			capacity,
		)
		if err != nil {
			return fmt.Errorf("create activity group domain object: %w", err)
		}

		// Persist to database
		err = s.groupRepo.CreateActivityGroup(tCtx, group)
		if err != nil {
			return fmt.Errorf("persist activity group: %w", err)
		}

		// Publish domain events
		err = s.eventWriter.InsertEvents(
			tCtx,
			activitySchema,
			group.Events(),
		)
		if err != nil {
			return fmt.Errorf("insert domain events: %w", err)
		}

		group.ClearEvents()

		return nil
	})
	if err != nil {
		span.SetStatus(codes.Error, err.Error())
		log.Error(ctx, "failed to create activity group", "error", err)
		return nil, mapToAppErr(err)
	}

	span.SetStatus(codes.Ok, "activity group created")
	log.Info(ctx, "activity group created", "group_id", group.ID())

	return group, nil
}

// GetActivityGroup retrieves an activity group by ID
func (s *ActivityGroupService) GetActivityGroup(
	ctx context.Context,
	groupID uuid.UUID,
) (*domain.ActivityGroup, error) {
	ctx, span := s.tracer.Start(
		ctx,
		"activity.service.GetActivityGroup",
	)
	defer span.End()

	log := s.logger.WithValues(
		"method", "GetActivityGroup",
		"group_id", groupID,
	)

	span.SetAttributes(attribute.String("group_id", groupID.String()))

	group, err := s.groupRepo.GetActivityGroupByID(ctx, groupID)
	if err != nil {
		span.SetStatus(codes.Error, err.Error())
		log.Error(ctx, "failed to get activity group", "error", err)
		return nil, mapToAppErr(err)
	}

	span.SetStatus(codes.Ok, "activity group found")
	log.Debug(ctx, "activity group found")

	return group, nil
}

// UpdateActivityGroup updates an existing activity group
func (s *ActivityGroupService) UpdateActivityGroup(
	ctx context.Context,
	groupID uuid.UUID,
	params UpdateActivityGroupParams,
) error {
	ctx, span := s.tracer.Start(
		ctx,
		"activity.service.UpdateActivityGroup",
	)
	defer span.End()

	log := s.logger.WithValues(
		"method", "UpdateActivityGroup",
		"group_id", groupID,
		"requester_id", params.RequesterID,
	)

	span.SetAttributes(
		attribute.String("group_id", groupID.String()),
		attribute.String("requester_id", params.RequesterID.String()),
	)

	// Parse and validate parameters
	title, err := domain.NewTitle(params.Title)
	if err != nil {
		span.SetStatus(codes.Error, err.Error())
		log.Error(ctx, "invalid title", "error", err)
		return MapErrToAppError(err)
	}

	location, err := domain.NewActivityGroupLocation(params.LocationCity, params.LocationCountry)
	if err != nil {
		span.SetStatus(codes.Error, err.Error())
		log.Error(ctx, "invalid location", "error", err)
		return MapErrToAppError(err)
	}

	activityType := domain.ActivityType(params.ActivityType)
	if !activityType.IsValid() {
		err := fmt.Errorf("invalid activity type: %s", params.ActivityType)
		span.SetStatus(codes.Error, err.Error())
		log.Error(ctx, "invalid activity type", "error", err)
		return MapErrToAppError(err)
	}

	difficultyLevel := domain.DifficultyLevel(params.DifficultyLevel)
	if !difficultyLevel.IsValid() {
		err := fmt.Errorf("invalid difficulty level: %s", params.DifficultyLevel)
		span.SetStatus(codes.Error, err.Error())
		log.Error(ctx, "invalid difficulty level", "error", err)
		return MapErrToAppError(err)
	}

	err = s.txManager.WithTransaction(ctx, func(tCtx context.Context) error {
		// Fetch existing group
		group, err := s.groupRepo.GetActivityGroupByID(tCtx, groupID)
		if err != nil {
			return fmt.Errorf("get activity group: %w", err)
		}

		// Update the domain object
		err = group.Update(
			params.RequesterID,
			title,
			params.Description,
			activityType,
			location,
			difficultyLevel,
			params.Timezone,
		)
		if err != nil {
			return fmt.Errorf("update activity group: %w", err)
		}

		// Persist changes
		err = s.groupRepo.UpdateActivityGroup(tCtx, group)
		if err != nil {
			return fmt.Errorf("persist activity group updates: %w", err)
		}

		// Publish domain events
		err = s.eventWriter.InsertEvents(
			tCtx,
			activitySchema,
			group.Events(),
		)
		if err != nil {
			return fmt.Errorf("insert domain events: %w", err)
		}

		group.ClearEvents()

		return nil
	})
	if err != nil {
		span.SetStatus(codes.Error, err.Error())
		log.Error(ctx, "failed to update activity group", "error", err)
		return mapToAppErr(err)
	}

	span.SetStatus(codes.Ok, "activity group updated")
	log.Info(ctx, "activity group updated")

	return nil
}

// ActivateActivityGroup activates a draft activity group
func (s *ActivityGroupService) ActivateActivityGroup(
	ctx context.Context,
	groupID uuid.UUID,
	requesterID uuid.UUID,
) error {
	ctx, span := s.tracer.Start(
		ctx,
		"activity.service.ActivateActivityGroup",
	)
	defer span.End()

	log := s.logger.WithValues(
		"method", "ActivateActivityGroup",
		"group_id", groupID,
		"requester_id", requesterID,
	)

	span.SetAttributes(
		attribute.String("group_id", groupID.String()),
		attribute.String("requester_id", requesterID.String()),
	)

	err := s.txManager.WithTransaction(ctx, func(tCtx context.Context) error {
		group, err := s.groupRepo.GetActivityGroupByID(tCtx, groupID)
		if err != nil {
			return fmt.Errorf("get activity group: %w", err)
		}

		err = group.Activate(requesterID)
		if err != nil {
			return fmt.Errorf("activate activity group: %w", err)
		}

		err = s.groupRepo.UpdateActivityGroup(tCtx, group)
		if err != nil {
			return fmt.Errorf("persist activation: %w", err)
		}

		err = s.eventWriter.InsertEvents(
			tCtx,
			activitySchema,
			group.Events(),
		)
		if err != nil {
			return fmt.Errorf("insert domain events: %w", err)
		}

		group.ClearEvents()

		return nil
	})
	if err != nil {
		span.SetStatus(codes.Error, err.Error())
		log.Error(ctx, "failed to activate activity group", "error", err)
		return mapToAppErr(err)
	}

	span.SetStatus(codes.Ok, "activity group activated")
	log.Info(ctx, "activity group activated")

	return nil
}

// CancelActivityGroup cancels an activity group (soft delete)
func (s *ActivityGroupService) CancelActivityGroup(
	ctx context.Context,
	groupID uuid.UUID,
	requesterID uuid.UUID,
	reason string,
) error {
	ctx, span := s.tracer.Start(
		ctx,
		"activity.service.CancelActivityGroup",
	)
	defer span.End()

	log := s.logger.WithValues(
		"method", "CancelActivityGroup",
		"group_id", groupID,
		"requester_id", requesterID,
	)

	span.SetAttributes(
		attribute.String("group_id", groupID.String()),
		attribute.String("requester_id", requesterID.String()),
	)

	err := s.txManager.WithTransaction(ctx, func(tCtx context.Context) error {
		group, err := s.groupRepo.GetActivityGroupByID(tCtx, groupID)
		if err != nil {
			return fmt.Errorf("get activity group: %w", err)
		}

		err = group.Cancel(requesterID, reason)
		if err != nil {
			return fmt.Errorf("cancel activity group: %w", err)
		}

		err = s.groupRepo.CancelActivityGroup(tCtx, group)
		if err != nil {
			return fmt.Errorf("persist cancellation: %w", err)
		}

		err = s.eventWriter.InsertEvents(
			tCtx,
			activitySchema,
			group.Events(),
		)
		if err != nil {
			return fmt.Errorf("insert domain events: %w", err)
		}

		group.ClearEvents()

		return nil
	})
	if err != nil {
		span.SetStatus(codes.Error, err.Error())
		log.Error(ctx, "failed to cancel activity group", "error", err)
		return mapToAppErr(err)
	}

	span.SetStatus(codes.Ok, "activity group cancelled")
	log.Info(ctx, "activity group cancelled")

	return nil
}

// ListActivityGroups retrieves activity groups with optional filters
func (s *ActivityGroupService) ListActivityGroups(
	ctx context.Context,
	params ListActivityGroupsParams,
) ([]*domain.ActivityGroup, error) {
	ctx, span := s.tracer.Start(
		ctx,
		"activity.service.ListActivityGroups",
	)
	defer span.End()

	log := s.logger.WithValues(
		"method", "ListActivityGroups",
	)

	if params.CreatorID != nil {
		span.SetAttributes(attribute.String("creator_id", params.CreatorID.String()))
	}

	// Convert params to repository filter
	var status domain.ActivityGroupStatus
	if params.Status != nil {
		status = domain.ActivityGroupStatus(*params.Status)
	}

	filter := persistence.ActivityGroupFilter{
		CreatorID: uuid.Nil,
		Status:    status,
		Title:     "",
		Limit:     params.Limit,
		Offset:    params.Offset,
	}

	if params.CreatorID != nil {
		filter.CreatorID = *params.CreatorID
	}

	if params.Title != nil {
		filter.Title = *params.Title
	}

	groups, err := s.groupRepo.ListActivityGroups(ctx, filter)
	if err != nil {
		span.SetStatus(codes.Error, err.Error())
		log.Error(ctx, "failed to list activity groups", "error", err)
		return nil, mapToAppErr(err)
	}

	span.SetStatus(codes.Ok, "activity groups listed")
	log.Debug(ctx, "activity groups listed", "count", len(groups))

	return groups, nil
}

// DiscoverActivityGroups retrieves public activity groups for discovery
func (s *ActivityGroupService) DiscoverActivityGroups(
	ctx context.Context,
	params DiscoverActivityGroupsParams,
) ([]*domain.ActivityGroup, error) {
	ctx, span := s.tracer.Start(
		ctx,
		"activity.service.DiscoverActivityGroups",
	)
	defer span.End()

	log := s.logger.WithValues(
		"method", "DiscoverActivityGroups",
	)

	// Convert params to repository filter
	filter := persistence.DiscoveryFilter{
		Limit:  params.Limit,
		Offset: params.Offset,
	}

	if params.City != nil {
		filter.City = *params.City
		span.SetAttributes(attribute.String("city", *params.City))
	}

	if params.Country != nil {
		filter.Country = *params.Country
		span.SetAttributes(attribute.String("country", *params.Country))
	}

	if params.ActivityType != nil {
		activityType := domain.ActivityType(*params.ActivityType)
		if !activityType.IsValid() {
			err := fmt.Errorf("invalid activity type: %s", *params.ActivityType)
			span.SetStatus(codes.Error, err.Error())
			log.Error(ctx, "invalid activity type", "error", err)
			return nil, MapErrToAppError(err)
		}
		filter.ActivityType = activityType
		span.SetAttributes(attribute.String("activity_type", *params.ActivityType))
	}

	groups, err := s.groupRepo.DiscoverGroups(ctx, filter)
	if err != nil {
		span.SetStatus(codes.Error, err.Error())
		log.Error(ctx, "failed to discover activity groups", "error", err)
		return nil, mapToAppErr(err)
	}

	span.SetStatus(codes.Ok, "activity groups discovered")
	log.Debug(ctx, "activity groups discovered", "count", len(groups))

	return groups, nil
}
