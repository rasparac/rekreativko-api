package domain

import (
	"time"

	"github.com/google/uuid"
	"github.com/rasparac/rekreativko-api/internal/shared/domainevent"
)

const (
	UserStatisticsCreatedEvent           = "user_profile.user_statistics.created"
	UserStatisticsActivityCreatedEvent   = "user_profile.user_statistics.activity_created"
	UserStatisticsActivityJoinedEvent    = "user_profile.user_statistics.activity_joined"
	UserStatisticsActivityCompletedEvent = "user_profile.user_statistics.activity_completed"
)

type UserStatisticsCreated struct {
	domainevent.BaseEvent
	AccountID uuid.UUID `json:"account_id"`
}

func NewUserStatisticsCreatedEvent(us *UserStatistics) *UserStatisticsCreated {
	return &UserStatisticsCreated{
		BaseEvent: domainevent.BaseEvent{
			EventID:     uuid.New(),
			EventType:   UserStatisticsCreatedEvent,
			OccurredAt:  time.Now().UTC(),
			AggregateID: us.AccountID(),
		},
		AccountID: us.AccountID(),
	}
}

type UserStatisticsActivityCreated struct {
	domainevent.BaseEvent
	AccountID    uuid.UUID `json:"account_id"`
	ActivityType string    `json:"activity_type"`
	Month        string    `json:"month"`
}

func NewUserStatisticsActivityCreatedEvent(accountID uuid.UUID, activityType, month string) *UserStatisticsActivityCreated {
	return &UserStatisticsActivityCreated{
		BaseEvent: domainevent.BaseEvent{
			EventID:     uuid.New(),
			EventType:   UserStatisticsActivityCreatedEvent,
			OccurredAt:  time.Now().UTC(),
			AggregateID: accountID,
		},
		AccountID:    accountID,
		ActivityType: activityType,
		Month:        month,
	}
}

type UserStatisticsActivityJoined struct {
	domainevent.BaseEvent
	AccountID    uuid.UUID `json:"account_id"`
	ActivityType string    `json:"activity_type"`
	Month        string    `json:"month"`
}

func NewUserStatisticsActivityJoinedEvent(accountID uuid.UUID, activityType, month string) *UserStatisticsActivityJoined {
	return &UserStatisticsActivityJoined{
		BaseEvent: domainevent.BaseEvent{
			EventID:     uuid.New(),
			EventType:   UserStatisticsActivityJoinedEvent,
			OccurredAt:  time.Now().UTC(),
			AggregateID: accountID,
		},
		AccountID:    accountID,
		ActivityType: activityType,
		Month:        month,
	}
}

type UserStatisticsActivityCompleted struct {
	domainevent.BaseEvent
	AccountID    uuid.UUID `json:"account_id"`
	ActivityType string    `json:"activity_type"`
	Month        string    `json:"month"`
}

func NewUserStatisticsActivityCompletedEvent(accountID uuid.UUID, activityType, month string) *UserStatisticsActivityCompleted {
	return &UserStatisticsActivityCompleted{
		BaseEvent: domainevent.BaseEvent{
			EventID:     uuid.New(),
			EventType:   UserStatisticsActivityCompletedEvent,
			OccurredAt:  time.Now().UTC(),
			AggregateID: accountID,
		},
		AccountID:    accountID,
		ActivityType: activityType,
		Month:        month,
	}
}
