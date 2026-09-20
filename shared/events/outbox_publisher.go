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
		txRunner      transactionRunner
		logger        *logger.Logger
		broker        MessageBroker
		readLimit     int
		maxRetries    int
		pollIntervalS time.Duration
		metrics       *Metrics
		schemas       []string
	}

	eventOutboxReader interface {
		ReadEvents(ctx context.Context, schema string, limit int) ([]domainevent.BrokerEvent, error)
		MarkEventAsPublished(ctx context.Context, schema string, eventID uuid.UUID) error
		MarkEventAsFailed(ctx context.Context, schema string, eventID uuid.UUID, publishErr error, maxRetries int) (deadLettered bool, err error)
	}

	// transactionRunner runs fn in a transaction. ReadEvents locks the rows it
	// returns, so a batch must be read, published and marked in one transaction.
	transactionRunner interface {
		WithTransaction(ctx context.Context, fn func(ctx context.Context) error) error
	}
)

func NewOutboxPublisher(
	eventReader eventOutboxReader,
	txRunner transactionRunner,
	broker MessageBroker,
	logger *logger.Logger,
	readLimit int,
	maxRetries int,
	pollIntervalS time.Duration,
	metrics *Metrics,
	schemas []string,
) *outboxPublisher {
	return &outboxPublisher{
		eventReader:   eventReader,
		txRunner:      txRunner,
		logger:        logger,
		broker:        broker,
		readLimit:     readLimit,
		maxRetries:    maxRetries,
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
	// Poll events from configured bounded context schemas
	for _, schema := range op.schemas {
		_, _, err := op.publishFromSchema(ctx, schema)
		if err != nil {
			op.logger.Error(ctx, "failed to publish from schema", "schema", schema, "error", err)
		}
	}

	return nil
}

// publishFromSchema publishes one batch of pending events from the schema. The
// batch runs in a single transaction so the row locks taken by ReadEvents are
// held until every event is marked, keeping concurrent publishers from picking
// up the same events. A failed publish is recorded and the event is
// dead-lettered after maxRetries so it can't block younger events forever.
func (op *outboxPublisher) publishFromSchema(ctx context.Context, schema string) (failedCount, publishedCount int, err error) {
	err = op.txRunner.WithTransaction(ctx, func(ctx context.Context) error {
		failedCount, publishedCount = 0, 0

		events, err := op.eventReader.ReadEvents(ctx, schema, op.readLimit)
		if err != nil {
			return err
		}

		for _, event := range events {
			start := time.Now()
			err := op.broker.Publish(ctx, event.EventType, event.Payload)
			op.metrics.EventPublishDuration.WithLabelValues(event.EventType, schema).Observe(time.Since(start).Seconds())
			if err != nil {
				failedCount++
				op.metrics.EventProcessedTotal.WithLabelValues(event.EventType, schema, "failed").Inc()
				op.logger.Error(ctx, "failed to publish outbox event", "schema", schema, "event_id", event.EventID, "error", err)

				deadLettered, markErr := op.eventReader.MarkEventAsFailed(ctx, schema, event.EventID, err, op.maxRetries)
				if markErr != nil {
					// The transaction is unusable after a failed statement; roll back and retry next poll.
					return markErr
				}
				if deadLettered {
					op.metrics.EventProcessedTotal.WithLabelValues(event.EventType, schema, "dead_lettered").Inc()
					op.logger.Error(ctx, "outbox event dead-lettered after max retries", "schema", schema, "event_id", event.EventID, "max_retries", op.maxRetries)
				}
				continue
			}

			err = op.eventReader.MarkEventAsPublished(ctx, schema, event.EventID)
			if err != nil {
				// Already on the broker but the mark failed: roll back so the batch is
				// retried next poll (at-least-once delivery).
				op.metrics.EventProcessedTotal.WithLabelValues(event.EventType, schema, "failed").Inc()
				return err
			}

			publishedCount++
			op.metrics.EventsPublishedTotal.WithLabelValues(event.EventType, schema).Inc()
			op.metrics.EventProcessedTotal.WithLabelValues(event.EventType, schema, "success").Inc()
		}

		return nil
	})

	return failedCount, publishedCount, err
}
