package domain

import (
	"time"

	"github.com/google/uuid"
	"github.com/rasparac/rekreativko-api/internal/shared/domainevent"
)

const (
	AccountStatisticsCreatedEvent           = "account_profile.account_statistics.created"
	AccountStatisticsActivityCreatedEvent   = "account_profile.account_statistics.activity_created"
	AccountStatisticsActivityJoinedEvent    = "account_profile.account_statistics.activity_joined"
	AccountStatisticsActivityCompletedEvent = "account_profile.account_statistics.activity_completed"
)

type AccountStatisticsCreated struct {
	domainevent.BaseEvent
	AccountID uuid.UUID `json:"account_id"`
}

func NewAccountStatisticsCreatedEvent(us *AccountStatistics) *AccountStatisticsCreated {
	return &AccountStatisticsCreated{
		BaseEvent: domainevent.BaseEvent{
			EventID:     uuid.New(),
			EventType:   AccountStatisticsCreatedEvent,
			OccurredAt:  time.Now().UTC(),
			AggregateID: us.AccountID(),
		},
		AccountID: us.AccountID(),
	}
}

type AccountStatisticsActivityCreated struct {
	domainevent.BaseEvent
	AccountID    uuid.UUID `json:"account_id"`
	ActivityType string    `json:"activity_type"`
	Month        string    `json:"month"`
}

func NewAccountStatisticsActivityCreatedEvent(accountID uuid.UUID, activityType, month string) *AccountStatisticsActivityCreated {
	return &AccountStatisticsActivityCreated{
		BaseEvent: domainevent.BaseEvent{
			EventID:     uuid.New(),
			EventType:   AccountStatisticsActivityCreatedEvent,
			OccurredAt:  time.Now().UTC(),
			AggregateID: accountID,
		},
		AccountID:    accountID,
		ActivityType: activityType,
		Month:        month,
	}
}

type AccountStatisticsActivityJoined struct {
	domainevent.BaseEvent
	AccountID    uuid.UUID `json:"account_id"`
	ActivityType string    `json:"activity_type"`
	Month        string    `json:"month"`
}

func NewAccountStatisticsActivityJoinedEvent(accountID uuid.UUID, activityType, month string) *AccountStatisticsActivityJoined {
	return &AccountStatisticsActivityJoined{
		BaseEvent: domainevent.BaseEvent{
			EventID:     uuid.New(),
			EventType:   AccountStatisticsActivityJoinedEvent,
			OccurredAt:  time.Now().UTC(),
			AggregateID: accountID,
		},
		AccountID:    accountID,
		ActivityType: activityType,
		Month:        month,
	}
}

type AccountStatisticsActivityCompleted struct {
	domainevent.BaseEvent
	AccountID    uuid.UUID `json:"account_id"`
	ActivityType string    `json:"activity_type"`
	Month        string    `json:"month"`
}

func NewAccountStatisticsActivityCompletedEvent(accountID uuid.UUID, activityType, month string) *AccountStatisticsActivityCompleted {
	return &AccountStatisticsActivityCompleted{
		BaseEvent: domainevent.BaseEvent{
			EventID:     uuid.New(),
			EventType:   AccountStatisticsActivityCompletedEvent,
			OccurredAt:  time.Now().UTC(),
			AggregateID: accountID,
		},
		AccountID:    accountID,
		ActivityType: activityType,
		Month:        month,
	}
}
