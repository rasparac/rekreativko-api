package application

import (
	"context"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
)

// RetentionPolicy says how long notifications are kept. Read ones go first;
// unread ones are kept longer but not forever, so the feed stays small enough
// to list without pagination.
type RetentionPolicy struct {
	// Read is how long a read notification is kept, counted from its creation.
	Read time.Duration
	// All is how long any notification is kept, read or not.
	All time.Duration
}

// DeleteExpiredNotifications deletes the notifications the policy no longer
// keeps, as of now. Safe to run from several instances at once: deleting an
// already deleted row is a no-op.
func (s *NotificationService) DeleteExpiredNotifications(
	ctx context.Context,
	policy RetentionPolicy,
	now time.Time,
) (int64, error) {
	ctx, span := s.tracer.Start(ctx, "notifications.service.DeleteExpiredNotifications")
	defer span.End()

	log := s.logger.WithValues("method", "DeleteExpiredNotifications")

	deleted, err := s.repo.DeleteExpired(ctx, now.Add(-policy.Read), now.Add(-policy.All))
	if err != nil {
		span.SetStatus(codes.Error, err.Error())
		log.Error(ctx, "failed to delete expired notifications", "error", err)
		return 0, mapToAppErr(err)
	}

	span.SetAttributes(attribute.Int64("deleted", deleted))
	span.SetStatus(codes.Ok, "expired notifications deleted")
	log.Debug(ctx, "expired notifications deleted", "deleted", deleted)

	return deleted, nil
}

// RunRetention deletes expired notifications right away and then every
// interval, until ctx is done. A failed run is logged and retried on the next tick.
func (s *NotificationService) RunRetention(ctx context.Context, policy RetentionPolicy, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		// The error is already logged; the next tick tries again.
		_, _ = s.DeleteExpiredNotifications(ctx, policy, time.Now())

		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
