package domain

import (
	"time"

	"github.com/google/uuid"
	"github.com/rasparac/rekreativko-api/internal/shared/domainevent"
)

type (
	ActivityTypeStats struct {
		TotalJoined    int
		TotalCreated   int
		TotalCompleted int
		LastActivity   time.Time
	}

	MonthlyActivityStats struct {
		Month               string
		ActivitiesJoined    int
		ActivitiesCreated   int
		ActivitiesCompleted int
	}

	AccountStatistics struct {
		accountID                uuid.UUID
		totalActivitiesJoined    int
		totalActivitiesCreated   int
		totalActivitiesCompleted int
		activityTypeStats        map[string]*ActivityTypeStats
		monthlyActivityStats     map[string]*MonthlyActivityStats
		lastActiveAt             *time.Time
		updatedAt                time.Time

		events []domainevent.Event
	}
)

func NewAccountStatistics(accountID uuid.UUID) *AccountStatistics {
	us := &AccountStatistics{
		accountID:            accountID,
		activityTypeStats:    make(map[string]*ActivityTypeStats),
		monthlyActivityStats: make(map[string]*MonthlyActivityStats),
		updatedAt:            time.Now().UTC(),
		events:               []domainevent.Event{},
	}

	us.addEvent(NewAccountStatisticsCreatedEvent(us))

	return us
}

func (us *AccountStatistics) ID() uuid.UUID {
	return us.accountID
}

func (us *AccountStatistics) AccountID() uuid.UUID {
	return us.accountID
}

func (us *AccountStatistics) TotalActivitiesJoined() int {
	return us.totalActivitiesJoined
}

func (us *AccountStatistics) MonthlyActivityStats() map[string]*MonthlyActivityStats {
	return us.monthlyActivityStats
}

func (us *AccountStatistics) ActivityTypeStats() map[string]*ActivityTypeStats {
	return us.activityTypeStats
}

func (us *AccountStatistics) LastActiveAt() *time.Time {
	return us.lastActiveAt
}

func (us *AccountStatistics) TotalActivitiesCompleted() int {
	return us.totalActivitiesCompleted
}

func (us *AccountStatistics) TotalActivitiesCreated() int {
	return us.totalActivitiesCreated
}

func (us *AccountStatistics) UpdatedAt() time.Time {
	return us.updatedAt
}

func (us *AccountStatistics) Events() []domainevent.Event {
	return us.events
}

func (us *AccountStatistics) addEvent(event domainevent.Event) {
	us.events = append(us.events, event)
}

func (us *AccountStatistics) ClearEvents() {
	us.events = []domainevent.Event{}
}

func (us *AccountStatistics) GetActivityTypeStats(activityType string) (*ActivityTypeStats, bool) {
	stats, exists := us.activityTypeStats[activityType]
	return stats, exists
}

func (us *AccountStatistics) GetMonthlyActivityStats(month string) (*MonthlyActivityStats, bool) {
	stats, exists := us.monthlyActivityStats[month]
	return stats, exists
}

func (us *AccountStatistics) RecordActivityJoined(activityType string, occurredAt time.Time) error {
	if activityType == "" {
		return ErrInvalidActivityType
	}

	us.totalActivitiesJoined++

	us.updateActivityTypeStats(activityType, func(stats *ActivityTypeStats) {
		stats.TotalJoined++
		stats.LastActivity = occurredAt
	})

	month := occurredAt.Format("2006-01")
	us.updateMonthlyActivityStats(month, func(stats *MonthlyActivityStats) {
		stats.ActivitiesJoined++
	})

	us.setLastActiveAt(occurredAt)
	us.updatedAt = time.Now().UTC()

	us.addEvent(NewAccountStatisticsActivityJoinedEvent(us.accountID, activityType, month))

	return nil
}

func (us *AccountStatistics) RecordActivityCreated(activityType string, occurredAt time.Time) error {
	if activityType == "" {
		return ErrInvalidActivityType
	}

	us.totalActivitiesCreated++

	us.updateActivityTypeStats(activityType, func(stats *ActivityTypeStats) {
		stats.TotalCreated++
		stats.LastActivity = occurredAt
	})

	month := occurredAt.Format("2006-01")
	us.updateMonthlyActivityStats(month, func(stats *MonthlyActivityStats) {
		stats.ActivitiesCreated++
	})

	us.setLastActiveAt(occurredAt)
	us.updatedAt = time.Now().UTC()

	us.addEvent(NewAccountStatisticsActivityCreatedEvent(us.accountID, activityType, month))

	return nil
}

func (us *AccountStatistics) RecordActivityCompleted(activityType string, occurredAt time.Time) error {
	if activityType == "" {
		return ErrInvalidActivityType
	}

	us.totalActivitiesCompleted++

	us.updateActivityTypeStats(activityType, func(stats *ActivityTypeStats) {
		stats.TotalCompleted++
		stats.LastActivity = occurredAt
	})

	month := occurredAt.Format("2006-01")
	us.updateMonthlyActivityStats(month, func(stats *MonthlyActivityStats) {
		stats.ActivitiesCompleted++
	})

	us.setLastActiveAt(occurredAt)
	us.updatedAt = time.Now().UTC()

	us.addEvent(NewAccountStatisticsActivityCompletedEvent(us.accountID, activityType, month))

	return nil
}

func (us *AccountStatistics) GetRecentMonths(count int) []MonthlyActivityStats {
	if count <= 0 {
		return []MonthlyActivityStats{}
	}

	now := time.Now().UTC()
	months := make([]MonthlyActivityStats, 0, count)

	for i := range count {
		monthStr := now.AddDate(0, -i, 0).Format("2006-01")
		if stats, exists := us.monthlyActivityStats[monthStr]; exists {
			months = append(months, *stats)
		} else {
			months = append(months, MonthlyActivityStats{Month: monthStr})
		}
	}

	return months
}

func (us *AccountStatistics) updateActivityTypeStats(activityType string, updateFn func(*ActivityTypeStats)) {
	stats, exists := us.activityTypeStats[activityType]
	if !exists {
		stats = &ActivityTypeStats{}
		us.activityTypeStats[activityType] = stats
	}

	updateFn(stats)
}

func (us *AccountStatistics) updateMonthlyActivityStats(month string, updateFn func(*MonthlyActivityStats)) {
	stats, exists := us.monthlyActivityStats[month]
	if !exists {
		stats = &MonthlyActivityStats{Month: month}
		us.monthlyActivityStats[month] = stats
	}

	updateFn(stats)
}

func (us *AccountStatistics) setLastActiveAt(t time.Time) {
	us.lastActiveAt = &t
}
