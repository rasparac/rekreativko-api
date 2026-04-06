package events

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/rasparac/rekreativko-api/internal/shared/domainevent"
	"github.com/rasparac/rekreativko-api/internal/shared/logger"
)

type (
	outboxPublisher struct {
		eventReader   eventOutboxReader
		logger        *logger.Logger
		broker        MessageBroker
		readLimit     int
		pollIntervalS time.Duration
		metrics       *Metrics
	}

	eventOutboxReader interface {
		ReadEvents(ctx context.Context, limit int) ([]domainevent.BrokerEvent, error)
		MarkEventAsPublished(ctx context.Context, eventID uuid.UUID) error
	}
)

func NewOutboxPublisher(
	eventReader eventOutboxReader,
	broker MessageBroker,
	logger *logger.Logger,
	readLimit int,
	pollIntervalS time.Duration,
	metrics *Metrics,
) *outboxPublisher {
	return &outboxPublisher{
		eventReader:   eventReader,
		logger:        logger,
		broker:        broker,
		readLimit:     readLimit,
		pollIntervalS: pollIntervalS,
		metrics:       metrics,
	}
}

func (op *outboxPublisher) Start(ctx context.Context) error {

	err := op.publish(ctx)
	if err != nil {
		return err
	}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(op.pollIntervalS):
			err := op.publish(ctx)
			if err != nil {
				op.logger.Error(ctx, "failed to publish outbox event", "error", err)
			}
		}
	}
}

func (op *outboxPublisher) publish(ctx context.Context) error {
	var (
		start          = time.Now()
		failedCount    int
		publishedCount int
	)
	events, err := op.eventReader.ReadEvents(ctx, op.readLimit)
	if err != nil {
		return err
	}

	for _, event := range events {
		err := op.broker.Publish(ctx, event.EventType, event.Payload)
		if err != nil {
			failedCount++
			op.logger.Error(ctx, "failed to publish outbox event", "error", err)
			continue
		}

		err = op.eventReader.MarkEventAsPublished(ctx, event.EventID)
		if err != nil {
			failedCount++
			op.logger.Error(ctx, "failed to mark as published outbox event", "error", err)
			continue
		}
		publishedCount++
	}

	duration := time.Since(start).Seconds()
	op.metrics.EventPublishDuration.WithLabelValues(
		"outbox", // TODO think about this name
	).Observe(duration)

	op.metrics.EventsPublishedTotal.WithLabelValues("outbox").Add(float64(publishedCount))
	op.metrics.EventProcessedTotal.WithLabelValues("outbox", "success").Add(float64(publishedCount))

	if failedCount > 0 {
		op.metrics.EventProcessedTotal.WithLabelValues("outbox", "failed").Add(float64(failedCount))
	}

	return nil
}
