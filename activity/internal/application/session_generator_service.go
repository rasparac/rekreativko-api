package application

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/rasparac/rekreativko-api/activity/internal/domain"
	"github.com/rasparac/rekreativko-api/activity/internal/infrastructure/persistence"
	"github.com/rasparac/rekreativko-api/activity/internal/metrics"
	"github.com/rasparac/rekreativko-api/shared/domainevent"
	"github.com/rasparac/rekreativko-api/shared/logger"
	"github.com/rasparac/rekreativko-api/shared/store/postgres"
	"github.com/rasparac/rekreativko-api/shared/telemetry"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// SessionGeneratorService handles generating sessions from recurring templates
type SessionGeneratorService struct {
	logger          *logger.Logger
	txManager       *postgres.TransactionManager
	templateRepo    SessionTemplateRepository
	sessionRepo     persistence.SessionRepository
	eventWriter     domainevent.EventWriter
	tracer          trace.Tracer
	metrics         *metrics.Metrics
	lookaheadWindow time.Duration
}

// NewSessionGeneratorService creates a new session generator service
func NewSessionGeneratorService(
	logger *logger.Logger,
	txManager *postgres.TransactionManager,
	templateRepo SessionTemplateRepository,
	sessionRepo persistence.SessionRepository,
	eventWriter domainevent.EventWriter,
	metrics *metrics.Metrics,
	lookaheadWindow time.Duration,
) *SessionGeneratorService {
	return &SessionGeneratorService{
		logger:          logger,
		txManager:       txManager,
		templateRepo:    templateRepo,
		sessionRepo:     sessionRepo,
		eventWriter:     eventWriter,
		tracer:          telemetry.Tracer(telemetry.TracerActivityService),
		metrics:         metrics,
		lookaheadWindow: lookaheadWindow,
	}
}

// GenerateSessionsFromTemplates finds recurring templates and generates sessions for them
func (s *SessionGeneratorService) GenerateSessionsFromTemplates(ctx context.Context) error {
	ctx, span := s.tracer.Start(ctx, "GenerateSessionsFromTemplates")
	defer span.End()

	// Find templates that need session generation
	templates, err := s.templateRepo.FindRecurringTemplatesToGenerate(ctx, s.lookaheadWindow)
	if err != nil {
		span.RecordError(err)
		s.logger.Error(ctx, "failed to find templates for generation", "error", err)
		return fmt.Errorf("failed to find templates: %w", err)
	}

	span.SetAttributes(attribute.Int("templates_count", len(templates)))
	s.logger.Info(ctx, "found templates for session generation", "count", len(templates))

	// Process each template
	var totalGenerated int
	for _, template := range templates {
		generated, err := s.generateSessionsForTemplate(ctx, template)
		if err != nil {
			// Log error but continue with other templates
			s.logger.Error(ctx,
				"failed to generate sessions for template",
				"template_id", template.ID(),
				"error", err,
			)
			continue
		}

		totalGenerated += generated
		s.logger.Info(ctx,
			"generated sessions for template",
			"template_id", template.ID(),
			"sessions_generated", generated,
		)
	}

	span.SetAttributes(attribute.Int("total_sessions_generated", totalGenerated))
	s.logger.Info(ctx, "session generation complete", "total_generated", totalGenerated)

	return nil
}

// generateSessionsForTemplate generates sessions for a single template
func (s *SessionGeneratorService) generateSessionsForTemplate(
	ctx context.Context,
	template *domain.SessionTemplate,
) (int, error) {
	ctx, span := s.tracer.Start(ctx, "generateSessionsForTemplate")
	defer span.End()

	span.SetAttributes(
		attribute.String("template_id", template.ID().String()),
		attribute.String("template_title", template.Title()),
	)

	// Determine generation start point
	startFrom := time.Now().UTC()
	if generatedUpTo := template.GeneratedUpTo(); generatedUpTo != nil {
		startFrom = *generatedUpTo
	}

	// Generate sessions up to lookahead window
	generateUntil := time.Now().UTC().Add(s.lookaheadWindow)

	// Get recurrence rule from template
	recurrenceRule := template.RecurrenceRule()
	if recurrenceRule == nil {
		// Template is not recurring (should not happen since we filter by recurring templates)
		s.logger.Warn(ctx, "template has no recurrence rule", "template_id", template.ID())
		return 0, nil
	}

	// If recurrence has ended, skip
	if recurrenceRule.HasEnded(time.Now().UTC()) {
		s.logger.Info(ctx, "template recurrence has ended", "template_id", template.ID())
		return 0, nil
	}

	// Generate sessions
	var sessionsGenerated int
	currentTime := startFrom

	for {
		// Get next occurrence
		nextOccurrence := recurrenceRule.NextOccurrenceAfter(currentTime)
		if nextOccurrence == nil {
			// No more occurrences (recurrence ended)
			break
		}

		// Stop if beyond lookahead window
		if nextOccurrence.After(generateUntil) {
			break
		}

		// Create session at this occurrence
		err := s.txManager.WithTransaction(ctx, func(txCtx context.Context) error {
			// Build session location
			location, err := domain.NewSessionLocation(
				template.LocationCity(),
				template.LocationCountry(),
				0, // latitude - TODO: add lat/lng to template
				0, // longitude
			)
			if err != nil {
				return fmt.Errorf("failed to create location: %w", err)
			}

			// Build session schedule
			// For now, using a 2-hour default duration
			endTime := nextOccurrence.Add(2 * time.Hour)
			schedule, err := domain.NewSessionSchedule(*nextOccurrence, &endTime)
			if err != nil {
				return fmt.Errorf("failed to create schedule: %w", err)
			}

			// Create session
			sessionInput := domain.SessionInput{
				ActivityGroupID: template.ActivityGroupID(),
				CreatedByID:     template.CreatedByID(),
				Location:        location,
				Schedule:        schedule,
				Capacity:        template.DefaultCapacity(),
				Note:            fmt.Sprintf("Generated from template: %s", template.Title()),
				IsRecurring:     true,
				AutoAttendeeIDs: nil, // No auto-attendees for generated sessions
			}

			session, _, err := domain.NewSession(sessionInput)
			if err != nil {
				return fmt.Errorf("failed to create session: %w", err)
			}

			// Persist session
			if err := s.sessionRepo.CreateSession(txCtx, session); err != nil {
				return fmt.Errorf("failed to persist session: %w", err)
			}

			// Publish domain events
			if len(session.Events()) > 0 {
				if err := s.eventWriter.InsertEvents(txCtx, "activity", session.Events()); err != nil {
					return fmt.Errorf("failed to insert events: %w", err)
				}
			}

			// Increment metrics
			s.metrics.SessionCreatedTotal.Inc()

			return nil
		})

		if err != nil {
			s.logger.Error(ctx,
				"failed to create session",
				"template_id", template.ID(),
				"occurrence", nextOccurrence,
				"error", err,
			)
			// Continue with next occurrence despite error
			currentTime = *nextOccurrence
			continue
		}

		sessionsGenerated++
		currentTime = *nextOccurrence
	}

	// Update template's generated_up_to timestamp
	if sessionsGenerated > 0 {
		updateErr := s.txManager.WithTransaction(ctx, func(txCtx context.Context) error {
			return s.templateRepo.UpdateGeneratedUpTo(txCtx, template.ID(), generateUntil)
		})

		if updateErr != nil {
			s.logger.Error(ctx,
				"failed to update generated_up_to",
				"template_id", template.ID(),
				"error", updateErr,
			)
			// Don't fail the whole operation if this update fails
			// Sessions were already created successfully
		}
	}

	span.SetAttributes(attribute.Int("sessions_generated", sessionsGenerated))

	return sessionsGenerated, nil
}

// GenerateSessionsForTemplate generates sessions for a specific template (useful for manual triggers)
func (s *SessionGeneratorService) GenerateSessionsForTemplate(
	ctx context.Context,
	templateID uuid.UUID,
) (int, error) {
	ctx, span := s.tracer.Start(ctx, "GenerateSessionsForTemplate")
	defer span.End()

	span.SetAttributes(attribute.String("template_id", templateID.String()))

	// Get template
	template, err := s.templateRepo.GetSessionTemplateByID(ctx, templateID)
	if err != nil {
		span.RecordError(err)
		return 0, MapErrToAppError(err)
	}

	// Ensure template is recurring
	if !template.IsRecurring() {
		return 0, MapErrToAppError(domain.ErrSessionTemplateNotRecurring)
	}

	// Generate sessions
	generated, err := s.generateSessionsForTemplate(ctx, template)
	if err != nil {
		span.RecordError(err)
		return 0, fmt.Errorf("failed to generate sessions: %w", err)
	}

	return generated, nil
}
