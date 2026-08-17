package events

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/rasparac/rekreativko-api/shared/domainevent"
	"github.com/rasparac/rekreativko-api/shared/logger"
)

type (
	outboxPublisher struct {
		eventReader   eventOutboxReader
		logger        *logger.Logger
		broker        MessageBroker
		readLimit     int
		pollIntervalS time.Duration
		metrics       *Metrics
		schemas       []string
	}

	eventOutboxReader interface {
		ReadEvents(ctx context.Context, schema string, limit int) ([]domainevent.BrokerEvent, error)
		MarkEventAsPublished(ctx context.Context, schema string, eventID uuid.UUID) error
	}
)

func NewOutboxPublisher(
	eventReader eventOutboxReader,
	broker MessageBroker,
	logger *logger.Logger,
	readLimit int,
	pollIntervalS time.Duration,
	metrics *Metrics,
	schemas []string,
) *outboxPublisher {
	return &outboxPublisher{
		eventReader:   eventReader,
		logger:        logger,
		broker:        broker,
		readLimit:     readLimit,
		pollIntervalS: pollIntervalS,
		metrics:       metrics,
		schemas:       schemas,
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
		totalFailed    int
		totalPublished int
	)

	// Poll events from configured bounded context schemas
	for _, schema := range op.schemas {
		failedCount, publishedCount, err := op.publishFromSchema(ctx, schema)
		if err != nil {
			op.logger.Error(ctx, "failed to publish from schema", "schema", schema, "error", err)
			totalFailed += failedCount
			continue
		}
		totalFailed += failedCount
		totalPublished += publishedCount
	}

	duration := time.Since(start).Seconds()
	op.metrics.EventPublishDuration.WithLabelValues(
		"outbox",
	).Observe(duration)

	op.metrics.EventsPublishedTotal.WithLabelValues("outbox").Add(float64(totalPublished))
	op.metrics.EventProcessedTotal.WithLabelValues("outbox", "success").Add(float64(totalPublished))

	if totalFailed > 0 {
		op.metrics.EventProcessedTotal.WithLabelValues("outbox", "failed").Add(float64(totalFailed))
	}

	return nil
}

func (op *outboxPublisher) publishFromSchema(ctx context.Context, schema string) (failedCount, publishedCount int, err error) {
	events, err := op.eventReader.ReadEvents(ctx, schema, op.readLimit)
	if err != nil {
		return 0, 0, err
	}

	for _, event := range events {
		err := op.broker.Publish(ctx, event.EventType, event.Payload)
		if err != nil {
			failedCount++
			op.logger.Error(ctx, "failed to publish outbox event", "schema", schema, "error", err)
			continue
		}

		err = op.eventReader.MarkEventAsPublished(ctx, schema, event.EventID)
		if err != nil {
			failedCount++
			op.logger.Error(ctx, "failed to mark as published outbox event", "schema", schema, "error", err)
			continue
		}
		publishedCount++
	}

	return failedCount, publishedCount, nil
}
