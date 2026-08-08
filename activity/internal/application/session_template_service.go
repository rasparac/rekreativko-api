package application

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/rasparac/rekreativko-api/activity/internal/domain"
	"github.com/rasparac/rekreativko-api/activity/internal/infrastructure/persistence"
	"github.com/rasparac/rekreativko-api/activity/internal/metrics"
	"github.com/rasparac/rekreativko-api/shared/domainerror"
	"github.com/rasparac/rekreativko-api/shared/domainevent"
	"github.com/rasparac/rekreativko-api/shared/logger"
	"github.com/rasparac/rekreativko-api/shared/store/postgres"
	"github.com/rasparac/rekreativko-api/shared/telemetry"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

const activitySchema = "activity"

type (
	// SessionTemplateRepository defines the interface for session template persistence
	SessionTemplateRepository interface {
		CreateSessionTemplate(ctx context.Context, template *domain.SessionTemplate) error
		UpdateSessionTemplate(ctx context.Context, template *domain.SessionTemplate) error
		UpdateGeneratedUpTo(ctx context.Context, templateID uuid.UUID, generatedUpTo time.Time) error
		DeleteSessionTemplate(ctx context.Context, templateID uuid.UUID) error
		GetSessionTemplateByID(ctx context.Context, templateID uuid.UUID) (*domain.SessionTemplate, error)
		ListSessionTemplates(ctx context.Context, filter persistence.SessionTemplateFilter) ([]*domain.SessionTemplate, error)
		FindRecurringTemplatesToGenerate(ctx context.Context, lookaheadWindow time.Duration) ([]*domain.SessionTemplate, error)
	}

	// SessionTemplateService handles business logic for session templates
	SessionTemplateService struct {
		logger            *logger.Logger
		txManager         *postgres.TransactionManager
		templateRepo      SessionTemplateRepository
		eventWriter       domainevent.EventWriter
		tracer            trace.Tracer
		metrics           *metrics.Metrics
	}
)

// NewSessionTemplateService creates a new session template service
func NewSessionTemplateService(
	logger *logger.Logger,
	txManager *postgres.TransactionManager,
	templateRepo SessionTemplateRepository,
	eventWriter domainevent.EventWriter,
	metrics *metrics.Metrics,
) *SessionTemplateService {
	return &SessionTemplateService{
		logger:       logger.WithName("activity.session_template_service"),
		txManager:    txManager,
		templateRepo: templateRepo,
		eventWriter:  eventWriter,
		tracer:       telemetry.Tracer(telemetry.TracerActivityService),
		metrics:      metrics,
	}
}

// CreateSessionTemplate creates a new session template
func (s *SessionTemplateService) CreateSessionTemplate(
	ctx context.Context,
	params CreateSessionTemplateParams,
) (*domain.SessionTemplate, error) {
	ctx, span := s.tracer.Start(
		ctx,
		"activity.service.CreateSessionTemplate",
	)
	defer span.End()

	log := s.logger.WithValues(
		"method", "CreateSessionTemplate",
		"activity_group_id", params.ActivityGroupID,
		"created_by_id", params.CreatedByID,
	)

	span.SetAttributes(
		attribute.String("activity_group_id", params.ActivityGroupID.String()),
		attribute.String("created_by_id", params.CreatedByID.String()),
	)

	// Build recurrence rule if provided
	var recurrenceRule *domain.RecurrenceRule
	var err error
	if params.RecurrenceFrequency != nil {
		recurrenceRule, err = s.buildRecurrenceRule(params)
		if err != nil {
			span.SetStatus(codes.Error, err.Error())
			log.Error(ctx, "failed to build recurrence rule", "error", err)
			return nil, MapErrToAppError(err)
		}
	}

	// Build location if provided
	var location *domain.Location
	if params.LocationCity != nil && params.LocationCountry != nil {
		// Templates don't need coordinates, use 0,0
		loc, err := domain.NewLocation(*params.LocationCity, *params.LocationCountry, 0, 0)
		if err != nil {
			span.SetStatus(codes.Error, err.Error())
			log.Error(ctx, "failed to create location", "error", err)
			return nil, MapErrToAppError(err)
		}
		location = &loc
	}

	var template *domain.SessionTemplate
	err = s.txManager.WithTransaction(ctx, func(tCtx context.Context) error {
		// Create the domain object
		template, err = domain.NewSessionTemplate(
			params.ActivityGroupID,
			params.CreatedByID,
			params.Title,
			params.Description,
			recurrenceRule,
			params.DefaultCapacity,
			location,
		)
		if err != nil {
			return fmt.Errorf("create session template domain object: %w", err)
		}

		// Persist to database
		err = s.templateRepo.CreateSessionTemplate(tCtx, template)
		if err != nil {
			return fmt.Errorf("persist session template: %w", err)
		}

		// Publish domain events
		err = s.eventWriter.InsertEvents(
			tCtx,
			activitySchema,
			template.Events(),
		)
		if err != nil {
			return fmt.Errorf("insert domain events: %w", err)
		}

		template.ClearEvents()

		return nil
	})
	if err != nil {
		span.SetStatus(codes.Error, err.Error())
		log.Error(ctx, "failed to create session template", "error", err)
		return nil, mapToAppErr(err)
	}

	span.SetStatus(codes.Ok, "session template created")
	log.Info(ctx, "session template created", "template_id", template.ID())

	return template, nil
}

// GetSessionTemplate retrieves a session template by ID
func (s *SessionTemplateService) GetSessionTemplate(
	ctx context.Context,
	templateID uuid.UUID,
) (*domain.SessionTemplate, error) {
	ctx, span := s.tracer.Start(
		ctx,
		"activity.service.GetSessionTemplate",
	)
	defer span.End()

	log := s.logger.WithValues(
		"method", "GetSessionTemplate",
		"template_id", templateID,
	)

	span.SetAttributes(attribute.String("template_id", templateID.String()))

	template, err := s.templateRepo.GetSessionTemplateByID(ctx, templateID)
	if err != nil {
		span.SetStatus(codes.Error, err.Error())
		log.Error(ctx, "failed to get session template", "error", err)
		return nil, mapToAppErr(err)
	}

	span.SetStatus(codes.Ok, "session template found")
	log.Debug(ctx, "session template found")

	return template, nil
}

// UpdateSessionTemplate updates an existing session template
func (s *SessionTemplateService) UpdateSessionTemplate(
	ctx context.Context,
	templateID uuid.UUID,
	params UpdateSessionTemplateParams,
) error {
	ctx, span := s.tracer.Start(
		ctx,
		"activity.service.UpdateSessionTemplate",
	)
	defer span.End()

	log := s.logger.WithValues(
		"method", "UpdateSessionTemplate",
		"template_id", templateID,
	)

	span.SetAttributes(attribute.String("template_id", templateID.String()))

	err := s.txManager.WithTransaction(ctx, func(tCtx context.Context) error {
		// Fetch existing template
		template, err := s.templateRepo.GetSessionTemplateByID(tCtx, templateID)
		if err != nil {
			return fmt.Errorf("get session template: %w", err)
		}

		// Build recurrence rule if provided
		var recurrenceRule *domain.RecurrenceRule
		if params.RecurrenceFrequency != nil {
			recurrenceRule, err = s.buildRecurrenceRuleFromUpdate(params)
			if err != nil {
				return fmt.Errorf("build recurrence rule: %w", err)
			}
		}

		// Build location if provided
		var location *domain.Location
		if params.LocationCity != nil && params.LocationCountry != nil {
			loc, err := domain.NewLocation(*params.LocationCity, *params.LocationCountry, 0, 0)
			if err != nil {
				return fmt.Errorf("create location: %w", err)
			}
			location = &loc
		}

		// Update the domain object
		err = template.Update(
			params.Title,
			params.Description,
			recurrenceRule,
			params.DefaultCapacity,
			location,
		)
		if err != nil {
			return fmt.Errorf("update session template: %w", err)
		}

		// Persist changes
		err = s.templateRepo.UpdateSessionTemplate(tCtx, template)
		if err != nil {
			return fmt.Errorf("persist session template updates: %w", err)
		}

		// Publish domain events
		err = s.eventWriter.InsertEvents(
			tCtx,
			activitySchema,
			template.Events(),
		)
		if err != nil {
			return fmt.Errorf("insert domain events: %w", err)
		}

		template.ClearEvents()

		return nil
	})
	if err != nil {
		span.SetStatus(codes.Error, err.Error())
		log.Error(ctx, "failed to update session template", "error", err)
		return mapToAppErr(err)
	}

	span.SetStatus(codes.Ok, "session template updated")
	log.Info(ctx, "session template updated")

	return nil
}

// ActivateSessionTemplate activates a session template
func (s *SessionTemplateService) ActivateSessionTemplate(
	ctx context.Context,
	templateID uuid.UUID,
) error {
	ctx, span := s.tracer.Start(
		ctx,
		"activity.service.ActivateSessionTemplate",
	)
	defer span.End()

	log := s.logger.WithValues(
		"method", "ActivateSessionTemplate",
		"template_id", templateID,
	)

	span.SetAttributes(attribute.String("template_id", templateID.String()))

	err := s.txManager.WithTransaction(ctx, func(tCtx context.Context) error {
		template, err := s.templateRepo.GetSessionTemplateByID(tCtx, templateID)
		if err != nil {
			return fmt.Errorf("get session template: %w", err)
		}

		err = template.Activate()
		if err != nil {
			return fmt.Errorf("activate session template: %w", err)
		}

		err = s.templateRepo.UpdateSessionTemplate(tCtx, template)
		if err != nil {
			return fmt.Errorf("persist activation: %w", err)
		}

		err = s.eventWriter.InsertEvents(
			tCtx,
			activitySchema,
			template.Events(),
		)
		if err != nil {
			return fmt.Errorf("insert domain events: %w", err)
		}

		template.ClearEvents()

		return nil
	})
	if err != nil {
		span.SetStatus(codes.Error, err.Error())
		log.Error(ctx, "failed to activate session template", "error", err)
		return mapToAppErr(err)
	}

	span.SetStatus(codes.Ok, "session template activated")
	log.Info(ctx, "session template activated")

	return nil
}

// DeactivateSessionTemplate deactivates a session template
func (s *SessionTemplateService) DeactivateSessionTemplate(
	ctx context.Context,
	templateID uuid.UUID,
) error {
	ctx, span := s.tracer.Start(
		ctx,
		"activity.service.DeactivateSessionTemplate",
	)
	defer span.End()

	log := s.logger.WithValues(
		"method", "DeactivateSessionTemplate",
		"template_id", templateID,
	)

	span.SetAttributes(attribute.String("template_id", templateID.String()))

	err := s.txManager.WithTransaction(ctx, func(tCtx context.Context) error {
		template, err := s.templateRepo.GetSessionTemplateByID(tCtx, templateID)
		if err != nil {
			return fmt.Errorf("get session template: %w", err)
		}

		err = template.Deactivate()
		if err != nil {
			return fmt.Errorf("deactivate session template: %w", err)
		}

		err = s.templateRepo.UpdateSessionTemplate(tCtx, template)
		if err != nil {
			return fmt.Errorf("persist deactivation: %w", err)
		}

		err = s.eventWriter.InsertEvents(
			tCtx,
			activitySchema,
			template.Events(),
		)
		if err != nil {
			return fmt.Errorf("insert domain events: %w", err)
		}

		template.ClearEvents()

		return nil
	})
	if err != nil {
		span.SetStatus(codes.Error, err.Error())
		log.Error(ctx, "failed to deactivate session template", "error", err)
		return mapToAppErr(err)
	}

	span.SetStatus(codes.Ok, "session template deactivated")
	log.Info(ctx, "session template deactivated")

	return nil
}

// DeleteSessionTemplate soft-deletes a session template
func (s *SessionTemplateService) DeleteSessionTemplate(
	ctx context.Context,
	templateID uuid.UUID,
) error {
	ctx, span := s.tracer.Start(
		ctx,
		"activity.service.DeleteSessionTemplate",
	)
	defer span.End()

	log := s.logger.WithValues(
		"method", "DeleteSessionTemplate",
		"template_id", templateID,
	)

	span.SetAttributes(attribute.String("template_id", templateID.String()))

	err := s.txManager.WithTransaction(ctx, func(tCtx context.Context) error {
		template, err := s.templateRepo.GetSessionTemplateByID(tCtx, templateID)
		if err != nil {
			return fmt.Errorf("get session template: %w", err)
		}

		template.Delete()

		err = s.templateRepo.DeleteSessionTemplate(tCtx, templateID)
		if err != nil {
			return fmt.Errorf("delete session template: %w", err)
		}

		err = s.eventWriter.InsertEvents(
			tCtx,
			activitySchema,
			template.Events(),
		)
		if err != nil {
			return fmt.Errorf("insert domain events: %w", err)
		}

		template.ClearEvents()

		return nil
	})
	if err != nil {
		span.SetStatus(codes.Error, err.Error())
		log.Error(ctx, "failed to delete session template", "error", err)
		return mapToAppErr(err)
	}

	span.SetStatus(codes.Ok, "session template deleted")
	log.Info(ctx, "session template deleted")

	return nil
}

// ListSessionTemplates retrieves session templates with optional filters
func (s *SessionTemplateService) ListSessionTemplates(
	ctx context.Context,
	params ListSessionTemplatesParams,
) ([]*domain.SessionTemplate, error) {
	ctx, span := s.tracer.Start(
		ctx,
		"activity.service.ListSessionTemplates",
	)
	defer span.End()

	log := s.logger.WithValues(
		"method", "ListSessionTemplates",
		"activity_group_id", params.ActivityGroupID,
	)

	if params.ActivityGroupID != nil {
		span.SetAttributes(attribute.String("activity_group_id", params.ActivityGroupID.String()))
	}

	// Convert params to repository filter
	var status *domain.SessionTemplateStatus
	if params.Status != nil {
		s := domain.SessionTemplateStatus(*params.Status)
		status = &s
	}

	filter := persistence.SessionTemplateFilter{
		ActivityGroupID: params.ActivityGroupID,
		CreatedByID:     params.CreatedByID,
		Status:          status,
		IsRecurring:     params.IsRecurring,
		NeedsGeneration: params.NeedsGeneration,
		LookaheadWindow: params.LookaheadWindow,
		Limit:           params.Limit,
		Offset:          params.Offset,
	}

	templates, err := s.templateRepo.ListSessionTemplates(ctx, filter)
	if err != nil {
		span.SetStatus(codes.Error, err.Error())
		log.Error(ctx, "failed to list session templates", "error", err)
		return nil, mapToAppErr(err)
	}

	span.SetStatus(codes.Ok, "session templates listed")
	log.Debug(ctx, "session templates listed", "count", len(templates))

	return templates, nil
}

// buildRecurrenceRule constructs a RecurrenceRule from create params
func (s *SessionTemplateService) buildRecurrenceRule(
	params CreateSessionTemplateParams,
) (*domain.RecurrenceRule, error) {
	if params.RecurrenceFrequency == nil {
		return nil, nil
	}

	freq := domain.RecurrenceFrequency(*params.RecurrenceFrequency)

	timeOfDay, err := domain.NewTimeOfDay(params.RecurrenceTimeHour, params.RecurrenceTimeMinute)
	if err != nil {
		return nil, err
	}

	interval := 1
	if params.RecurrenceInterval != nil {
		interval = *params.RecurrenceInterval
	}

	rule, err := domain.NewRecurrenceRule(
		freq,
		timeOfDay,
		interval,
		params.RecurrenceDayOfWeek,
		params.RecurrenceDayOfMonth,
		params.RecurrenceEndsAt,
	)
	if err != nil {
		return nil, err
	}

	return &rule, nil
}

// buildRecurrenceRuleFromUpdate constructs a RecurrenceRule from update params
func (s *SessionTemplateService) buildRecurrenceRuleFromUpdate(
	params UpdateSessionTemplateParams,
) (*domain.RecurrenceRule, error) {
	if params.RecurrenceFrequency == nil {
		return nil, nil
	}

	freq := domain.RecurrenceFrequency(*params.RecurrenceFrequency)

	timeOfDay, err := domain.NewTimeOfDay(params.RecurrenceTimeHour, params.RecurrenceTimeMinute)
	if err != nil {
		return nil, err
	}

	interval := 1
	if params.RecurrenceInterval != nil {
		interval = *params.RecurrenceInterval
	}

	rule, err := domain.NewRecurrenceRule(
		freq,
		timeOfDay,
		interval,
		params.RecurrenceDayOfWeek,
		params.RecurrenceDayOfMonth,
		params.RecurrenceEndsAt,
	)
	if err != nil {
		return nil, err
	}

	return &rule, nil
}

// mapToAppErr maps repository/domain errors to application errors
func mapToAppErr(err error) *domainerror.AppError {
	pgErr := postgres.GetPgxError(err)
	if pgErr != nil {
		return MapPostgresError(pgErr)
	}

	return MapErrToAppError(err)
}
