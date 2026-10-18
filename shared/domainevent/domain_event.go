package domainevent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/rasparac/rekreativko-api/shared/api"
	"github.com/rasparac/rekreativko-api/shared/logger"
	"github.com/rasparac/rekreativko-api/shared/store/postgres"
)

type (
	domainEventManager struct {
		txManager *postgres.TransactionManager
		logger    *logger.Logger
	}

	// EventWriter represents an interface for writing domain events to a persistent store.
	// It defines a method for inserting events into the store.
	// This interface can be implemented by various storage backends, such as databases or message queues.
	// Insert MUST be called within a transaction to ensure consistency.
	EventWriter interface {
		// InsertEvent inserts a domain events into the store.
		InsertEvents(
			ctx context.Context,
			schema string,
			event []Event,
		) error
	}
)

func NewDomainEventManager(
	txManager *postgres.TransactionManager,
	logger *logger.Logger,
) *domainEventManager {
	return &domainEventManager{
		txManager: txManager,
		logger:    logger.WithName("domainevent"),
	}
}

func (tm *domainEventManager) InsertEvents(
	ctx context.Context,
	schema string,
	events []Event,
) error {
	if len(events) == 0 {
		return nil
	}

	var (
		eventValues  = make([]any, 0, len(events)*5)
		placeHolders = make([]string, 0)
	)
	for i, event := range events {
		eventID := event.GetEventID()
		eventType := event.GetEventType()
		aggregateID := event.GetAggregateID()
		eventData, err := json.Marshal(event)

		eventOccurredAt := event.GetOccurredAt()
		if err != nil {
			return err
		}

		eventValues = append(eventValues,
			eventID,
			eventType,
			aggregateID,
			eventData,
			eventOccurredAt,
		)
		placeHolders = append(placeHolders,
			fmt.Sprintf("($%d, $%d, $%d, $%d, $%d)", i*5+1, i*5+2, i*5+3, i*5+4, i*5+5),
		)
	}
	const insertEventQueryBlueprint = `
		INSERT INTO %s.event_outbox (
			event_id,
			event_type,
			aggregate_id,
			payload,
			created_at
		)
		VALUES %s
	`
	insertEventQuery := fmt.Sprintf(
		insertEventQueryBlueprint,
		schema,
		strings.Join(placeHolders, ","),
	)

	_, err := tm.txManager.Querier(ctx).Exec(ctx, insertEventQuery, eventValues...)
	if err != nil {
		return err
	}

	// The first hop of an event's trail: the request_id in ctx ties the
	// event_id to the request that raised it. The transaction may still roll
	// back - the event exists only if "outbox event published" follows.
	for _, event := range events {
		tm.logger.Debug(api.WithEventID(ctx, event.GetEventID().String()), "event stored in outbox",
			"schema", schema, "event_type", event.GetEventType(), "aggregate_id", event.GetAggregateID())
	}

	return nil
}

type BrokerEvent struct {
	EventID   uuid.UUID
	EventType string
	Payload   json.RawMessage
}

// ReadEvents returns up to limit pending events (not published and not
// dead-lettered), oldest first. The rows are locked with FOR UPDATE SKIP LOCKED
// so concurrent publishers never pick up the same events; the locks are held
// until the surrounding transaction ends, so it MUST be called within one.
func (dem *domainEventManager) ReadEvents(
	ctx context.Context,
	schema string,
	limit int,
) ([]BrokerEvent, error) {
	query := fmt.Sprintf(`
		SELECT
			event_id,
			event_type,
			payload
		FROM %s.event_outbox
		WHERE
			published_at IS NULL
			AND failed_at IS NULL
		ORDER BY created_at ASC
		LIMIT $1
		FOR UPDATE SKIP LOCKED
	`, schema)

	rows, err := dem.txManager.Querier(ctx).Query(ctx, query, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []BrokerEvent
	for rows.Next() {
		var be BrokerEvent

		if err := rows.Scan(&be.EventID, &be.EventType, &be.Payload); err != nil {
			return nil, err
		}
		events = append(events, be)
	}

	return events, rows.Err()
}

func (dem *domainEventManager) MarkEventAsPublished(
	ctx context.Context,
	schema string,
	eventID uuid.UUID,
) error {
	query := fmt.Sprintf(`
		UPDATE %s.event_outbox
		SET
			published_at = NOW()
		WHERE event_id = $1
	`, schema)

	_, err := dem.txManager.Querier(ctx).Exec(ctx, query, eventID)
	return err
}

// MarkEventAsFailed records a failed publish attempt. Once the event has failed
// maxRetries times it is dead-lettered (failed_at is set) and no longer read by
// ReadEvents. It reports whether the event was dead-lettered by this call.
func (dem *domainEventManager) MarkEventAsFailed(
	ctx context.Context,
	schema string,
	eventID uuid.UUID,
	publishErr error,
	maxRetries int,
) (deadLettered bool, err error) {
	query := fmt.Sprintf(`
		UPDATE %s.event_outbox
		SET
			retry_count = retry_count + 1,
			last_error = $2,
			failed_at = CASE WHEN retry_count + 1 >= $3 THEN NOW() ELSE NULL END
		WHERE event_id = $1
		RETURNING failed_at IS NOT NULL
	`, schema)

	err = dem.txManager.Querier(ctx).
		QueryRow(ctx, query, eventID, publishErr.Error(), maxRetries).
		Scan(&deadLettered)
	return deadLettered, err
}

// DeletePublishedEvents deletes up to limit events that were published before
// the given time, oldest first, and returns how many it deleted. Pending
// events and dead-lettered ones (failed_at set, never published) are kept. Rows
// another purger holds are skipped, so several publishers can run it together.
func (dem *domainEventManager) DeletePublishedEvents(
	ctx context.Context,
	schema string,
	publishedBefore time.Time,
	limit int,
) (int64, error) {
	query := fmt.Sprintf(`
		DELETE FROM %[1]s.event_outbox
		WHERE event_id IN (
			SELECT event_id
			FROM %[1]s.event_outbox
			WHERE
				published_at IS NOT NULL
				AND failed_at IS NULL
				AND published_at < $1
			ORDER BY published_at ASC
			LIMIT $2
			FOR UPDATE SKIP LOCKED
		)
	`, schema)

	tag, err := dem.txManager.Querier(ctx).Exec(ctx, query, publishedBefore, limit)
	if err != nil {
		return 0, err
	}

	return tag.RowsAffected(), nil
}
