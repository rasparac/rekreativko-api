package domainevent

import (
	"time"

	"github.com/google/uuid"
)

type (
	Event interface {
		GetEventID() uuid.UUID
		GetEventType() string
		GetOccurredAt() time.Time
		GetAggregateID() uuid.UUID
	}

	BaseEvent struct {
		EventID     uuid.UUID `json:"event_id"`
		EventType   string    `json:"event_type"`
		OccurredAt  time.Time `json:"occurred_at"`
		AggregateID uuid.UUID `json:"aggregate_id"`
	}
)

func (e *BaseEvent) GetEventID() uuid.UUID {
	return e.EventID
}

func (e *BaseEvent) GetEventType() string {
	return e.EventType
}

func (e *BaseEvent) GetOccurredAt() time.Time {
	return e.OccurredAt
}

func (e *BaseEvent) GetAggregateID() uuid.UUID {
	return e.AggregateID
}
