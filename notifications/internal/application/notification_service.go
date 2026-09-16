package application

import (
	"context"
	"errors"
	"fmt"

	"github.com/rasparac/rekreativko-api/notifications/internal/domain"
	"github.com/rasparac/rekreativko-api/notifications/internal/infrastructure/persistence"
	"github.com/rasparac/rekreativko-api/notifications/internal/metrics"
	"github.com/rasparac/rekreativko-api/shared/domainerror"
	"github.com/rasparac/rekreativko-api/shared/logger"
	"github.com/rasparac/rekreativko-api/shared/store/postgres"
	"github.com/rasparac/rekreativko-api/shared/telemetry"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// NotificationService handles business logic for notifications
type NotificationService struct {
	logger    *logger.Logger
	txManager *postgres.TransactionManager
	repo      persistence.NotificationRepository
	tracer    trace.Tracer
	metrics   *metrics.Metrics
}

// NewNotificationService creates a new notification service
func NewNotificationService(
	logger *logger.Logger,
	txManager *postgres.TransactionManager,
	repo persistence.NotificationRepository,
	metrics *metrics.Metrics,
) *NotificationService {
	return &NotificationService{
		logger:    logger.WithName("notifications.notification_service"),
		txManager: txManager,
		repo:      repo,
		tracer:    telemetry.Tracer(telemetry.TracerNotificationsService),
		metrics:   metrics,
	}
}

// CreateNotification creates a single notification for a single recipient.
// Called once per recipient by the event consumers - fan-out to multiple
// recipients (e.g. every manager of a group) happens at the call site.
func (s *NotificationService) CreateNotification(
	ctx context.Context,
	params CreateNotificationParams,
) (*domain.Notification, error) {
	ctx, span := s.tracer.Start(ctx, "notifications.service.CreateNotification")
	defer span.End()

	log := s.logger.WithValues(
		"method", "CreateNotification",
		"recipient_account_id", params.RecipientAccountID,
		"type", params.Type,
	)

	span.SetAttributes(
		attribute.String("recipient_account_id", params.RecipientAccountID.String()),
		attribute.String("type", params.Type),
	)

	notification := domain.New(
		params.RecipientAccountID,
		domain.NotificationType(params.Type),
		params.Data,
	)

	if err := s.repo.Create(ctx, notification); err != nil {
		span.SetStatus(codes.Error, err.Error())
		log.Error(ctx, "failed to create notification", "error", err)
		return nil, mapToAppErr(err)
	}

	s.metrics.NotificationCreated.Inc()

	span.SetStatus(codes.Ok, "notification created")
	log.Debug(ctx, "notification created", "notification_id", notification.ID())

	return notification, nil
}

// ListNotifications returns a recipient's notification feed alongside their
// current unread count - the two numbers a home-screen badge needs in one call.
func (s *NotificationService) ListNotifications(
	ctx context.Context,
	params ListNotificationsParams,
) (notifications []*domain.Notification, unreadCount int, nextPageToken string, err error) {
	ctx, span := s.tracer.Start(ctx, "notifications.service.ListNotifications")
	defer span.End()

	log := s.logger.WithValues(
		"method", "ListNotifications",
		"recipient_account_id", params.RecipientAccountID,
	)

	span.SetAttributes(attribute.String("recipient_account_id", params.RecipientAccountID.String()))

	notifications, nextPageToken, err = s.repo.ListByRecipient(ctx, persistence.NotificationFilter{
		RecipientAccountID: params.RecipientAccountID,
		UnreadOnly:         params.UnreadOnly,
		Limit:              params.Limit,
		PageToken:          params.PageToken,
	})
	if err != nil {
		span.SetStatus(codes.Error, err.Error())
		log.Error(ctx, "failed to list notifications", "error", err)
		return nil, 0, "", mapToAppErr(err)
	}

	unreadCount, err = s.repo.CountUnread(ctx, params.RecipientAccountID)
	if err != nil {
		span.SetStatus(codes.Error, err.Error())
		log.Error(ctx, "failed to count unread notifications", "error", err)
		return nil, 0, "", mapToAppErr(err)
	}

	span.SetStatus(codes.Ok, "notifications listed")
	log.Debug(ctx, "notifications listed", "count", len(notifications), "unread_count", unreadCount)

	return notifications, unreadCount, nextPageToken, nil
}

// MarkAsRead marks a single notification as read. Only the notification's own
// recipient may do this.
func (s *NotificationService) MarkAsRead(ctx context.Context, params MarkAsReadParams) error {
	ctx, span := s.tracer.Start(ctx, "notifications.service.MarkAsRead")
	defer span.End()

	log := s.logger.WithValues(
		"method", "MarkAsRead",
		"notification_id", params.NotificationID,
		"requester_id", params.RequesterID,
	)

	span.SetAttributes(
		attribute.String("notification_id", params.NotificationID.String()),
		attribute.String("requester_id", params.RequesterID.String()),
	)

	err := s.txManager.WithTransaction(ctx, func(tCtx context.Context) error {
		notification, err := s.repo.GetByID(tCtx, params.NotificationID)
		if err != nil {
			return fmt.Errorf("get notification: %w", err)
		}

		if notification.RecipientAccountID() != params.RequesterID {
			return domain.ErrUnauthorized
		}

		notification.MarkRead()

		if err := s.repo.MarkRead(tCtx, notification); err != nil {
			return fmt.Errorf("persist mark read: %w", err)
		}

		return nil
	})
	if err != nil {
		span.SetStatus(codes.Error, err.Error())
		log.Error(ctx, "failed to mark notification read", "error", err)
		return mapToAppErr(err)
	}

	s.metrics.NotificationRead.Inc()

	span.SetStatus(codes.Ok, "notification marked read")
	log.Debug(ctx, "notification marked read")

	return nil
}

func mapToAppErr(err error) *domainerror.AppError {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, domain.ErrNotificationNotFound):
		return domainerror.NotFound("notification_not_found", "Notification not found", err)
	case errors.Is(err, domain.ErrUnauthorized):
		return domainerror.Forbidden("unauthorized", "You cannot access this notification", err)
	default:
		return domainerror.InternalWithErr(err)
	}
}
