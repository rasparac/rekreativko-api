package domainevent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/rasparac/rekreativko-api/internal/shared/store/postgres"
)

type (
	domainEventManager struct {
		txManager *postgres.TransactionManager
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
) *domainEventManager {
	return &domainEventManager{
		txManager: txManager,
	}
}

func (tm *domainEventManager) InsertEvents(
	ctx context.Context,
	_ string,
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
	schema := "public" // TODO: read from config
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
	return err
}

type BrokerEvent struct {
	EventID   uuid.UUID
	EventType string
	Payload   json.RawMessage
}

func (dem *domainEventManager) ReadEvents(
	ctx context.Context,
	limit int,
) ([]BrokerEvent, error) {
	rows, err := dem.txManager.Querier(ctx).Query(ctx, `
		SELECT
			event_id,
			event_type,
			payload
		FROM public.event_outbox
		WHERE
			published_at IS NULL
		ORDER BY created_at ASC
		LIMIT $1
	`, limit)
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

	return events, nil
}

func (dem *domainEventManager) MarkEventAsPublished(
	ctx context.Context,
	eventID uuid.UUID,
) error {
	_, err := dem.txManager.Querier(ctx).Exec(ctx, `
		UPDATE public.event_outbox
		SET
			published_at = NOW()
		WHERE event_id = $1
	`, eventID)
	return err
}
