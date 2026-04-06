package domain

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func fixedTime() time.Time {
	return time.Date(2024, 3, 15, 10, 0, 0, 0, time.UTC)
}

func TestNewAccountStatistics(t *testing.T) {
	accountID := uuid.New()

	us := NewAccountStatistics(accountID)

	require.NotNil(t, us)
	assert.Equal(t, accountID, us.AccountID())
	assert.Zero(t, us.TotalActivitiesJoined())
	assert.Zero(t, us.TotalActivitiesCreated())
	assert.Zero(t, us.TotalActivitiesCompleted())
	assert.Nil(t, us.LastActiveAt())
	assert.NotZero(t, us.UpdatedAt())

	events := us.Events()
	require.Len(t, events, 1)

	createdEvent, ok := events[0].(*AccountStatisticsCreated)
	require.True(t, ok)
	assert.Equal(t, accountID, createdEvent.AccountID)
}

func TestRecordActivityJoined(t *testing.T) {
	accountID := uuid.New()
	us := NewAccountStatistics(accountID)
	occurredAt := fixedTime()

	err := us.RecordActivityJoined("running", occurredAt)
	require.NoError(t, err)

	assert.Equal(t, 1, us.TotalActivitiesJoined())
	assert.Equal(t, occurredAt, *us.LastActiveAt())

	stats, ok := us.GetActivityTypeStats("running")
	require.True(t, ok)
	assert.Equal(t, 1, stats.TotalJoined)
	assert.Equal(t, occurredAt, stats.LastActivity)

	month := occurredAt.Format("2006-01")
	monthly, ok := us.GetMonthlyActivityStats(month)
	require.True(t, ok)
	assert.Equal(t, 1, monthly.ActivitiesJoined)

	events := us.Events()
	require.Len(t, events, 2)

	ev, ok := events[1].(*AccountStatisticsActivityJoined)
	require.True(t, ok)
	assert.Equal(t, accountID, ev.AccountID)
	assert.Equal(t, "running", ev.ActivityType)
	assert.Equal(t, month, ev.Month)
}

func TestRecordActivityCreated(t *testing.T) {
	accountID := uuid.New()
	us := NewAccountStatistics(accountID)
	occurredAt := fixedTime()

	err := us.RecordActivityCreated("cycling", occurredAt)
	require.NoError(t, err)

	assert.Equal(t, 1, us.TotalActivitiesCreated())

	stats, ok := us.GetActivityTypeStats("cycling")
	require.True(t, ok)
	assert.Equal(t, 1, stats.TotalCreated)

	month := occurredAt.Format("2006-01")
	monthly, ok := us.GetMonthlyActivityStats(month)
	require.True(t, ok)
	assert.Equal(t, 1, monthly.ActivitiesCreated)

	events := us.Events()
	require.Len(t, events, 2)

	ev, ok := events[1].(*AccountStatisticsActivityCreated)
	require.True(t, ok)
	assert.Equal(t, accountID, ev.AccountID)
	assert.Equal(t, "cycling", ev.ActivityType)
	assert.Equal(t, month, ev.Month)
}

func TestRecordActivityCompleted(t *testing.T) {
	accountID := uuid.New()
	us := NewAccountStatistics(accountID)
	occurredAt := fixedTime()

	err := us.RecordActivityCompleted("tennis", occurredAt)
	require.NoError(t, err)

	assert.Equal(t, 1, us.TotalActivitiesCompleted())

	stats, ok := us.GetActivityTypeStats("tennis")
	require.True(t, ok)
	assert.Equal(t, 1, stats.TotalCompleted)

	month := occurredAt.Format("2006-01")
	monthly, ok := us.GetMonthlyActivityStats(month)
	require.True(t, ok)
	assert.Equal(t, 1, monthly.ActivitiesCompleted)

	events := us.Events()
	require.Len(t, events, 2)

	ev, ok := events[1].(*AccountStatisticsActivityCompleted)
	require.True(t, ok)
	assert.Equal(t, accountID, ev.AccountID)
	assert.Equal(t, "tennis", ev.ActivityType)
	assert.Equal(t, month, ev.Month)
}

func TestRecordActivity_InvalidActivityType(t *testing.T) {
	us := NewAccountStatistics(uuid.New())

	err := us.RecordActivityJoined("", fixedTime())

	assert.ErrorIs(t, err, ErrInvalidActivityType)
	assert.Zero(t, us.TotalActivitiesJoined())
	assert.Len(t, us.Events(), 1) // only created event
}

func TestMultipleActivitiesSameType(t *testing.T) {
	us := NewAccountStatistics(uuid.New())
	t1 := fixedTime()
	t2 := t1.Add(2 * time.Hour)

	require.NoError(t, us.RecordActivityJoined("running", t1))
	require.NoError(t, us.RecordActivityJoined("running", t2))

	stats, ok := us.GetActivityTypeStats("running")
	require.True(t, ok)
	assert.Equal(t, 2, stats.TotalJoined)
	assert.Equal(t, t2, stats.LastActivity)

	assert.Equal(t, 2, us.TotalActivitiesJoined())
}

func TestGetRecentMonths(t *testing.T) {
	us := NewAccountStatistics(uuid.New())

	now := time.Now().UTC()
	lastMonth := now.AddDate(0, -1, 0)

	require.NoError(t, us.RecordActivityJoined("running", now))
	require.NoError(t, us.RecordActivityJoined("running", lastMonth))

	months := us.GetRecentMonths(2)

	require.Len(t, months, 2)
	assert.Equal(t, now.Format("2006-01"), months[0].Month)
	assert.Equal(t, lastMonth.Format("2006-01"), months[1].Month)
}

func Test_AccountStatistics_ClearEvents(t *testing.T) {
	us := NewAccountStatistics(uuid.New())

	require.NotEmpty(t, us.Events())

	us.ClearEvents()

	assert.Empty(t, us.Events())
}
