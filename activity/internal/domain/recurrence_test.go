package domain

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func utc(year int, month time.Month, day, hour, minute int) time.Time {
	return time.Date(year, month, day, hour, minute, 0, 0, time.UTC)
}

func mustTimeOfDay(t *testing.T, hour, minute int) TimeOfDay {
	t.Helper()
	tod, err := NewTimeOfDay(hour, minute)
	require.NoError(t, err)
	return tod
}

func weekdayPtr(d time.Weekday) *time.Weekday { return &d }
func dayPtr(d int) *int                       { return &d }

func daily(t *testing.T, hour, minute, interval int) RecurrenceRule {
	t.Helper()
	rule, err := NewRecurrenceRule(RecurrenceFrequencyDaily, mustTimeOfDay(t, hour, minute), interval, nil, nil, nil)
	require.NoError(t, err)
	return rule
}

func weekly(t *testing.T, day time.Weekday, hour, minute, interval int) RecurrenceRule {
	t.Helper()
	rule, err := NewRecurrenceRule(RecurrenceFrequencyWeekly, mustTimeOfDay(t, hour, minute), interval, weekdayPtr(day), nil, nil)
	require.NoError(t, err)
	return rule
}

func monthly(t *testing.T, day, hour, minute, interval int) RecurrenceRule {
	t.Helper()
	rule, err := NewRecurrenceRule(RecurrenceFrequencyMonthly, mustTimeOfDay(t, hour, minute), interval, nil, dayPtr(day), nil)
	require.NoError(t, err)
	return rule
}

func TestRecurrenceFrequency(t *testing.T) {
	for _, f := range []RecurrenceFrequency{RecurrenceFrequencyDaily, RecurrenceFrequencyWeekly, RecurrenceFrequencyMonthly} {
		assert.True(t, f.IsValid(), f)
	}
	assert.False(t, RecurrenceFrequency("").IsValid())
	assert.False(t, RecurrenceFrequency("hourly").IsValid())
	assert.Equal(t, "weekly", RecurrenceFrequencyWeekly.String())
}

func TestNewTimeOfDay(t *testing.T) {
	tod, err := NewTimeOfDay(7, 5)
	require.NoError(t, err)
	assert.Equal(t, "07:05", tod.String())
	assert.Equal(t, 7, tod.Hour())
	assert.Equal(t, 5, tod.Minute())

	for _, tc := range [][2]int{{-1, 0}, {24, 0}, {0, -1}, {0, 60}} {
		_, err := NewTimeOfDay(tc[0], tc[1])
		assert.Error(t, err, tc)
	}
	_, err = NewTimeOfDay(23, 59)
	assert.NoError(t, err)
}

func TestNewRecurrenceRule_Validation(t *testing.T) {
	tod := mustTimeOfDay(t, 18, 0)
	past := time.Now().Add(-time.Hour)
	future := time.Now().Add(24 * time.Hour)

	tests := []struct {
		name     string
		freq     RecurrenceFrequency
		interval int
		dow      *time.Weekday
		dom      *int
		endsAt   *time.Time
		wantErr  bool
	}{
		{"daily", RecurrenceFrequencyDaily, 1, nil, nil, nil, false},
		{"weekly with a day", RecurrenceFrequencyWeekly, 1, weekdayPtr(time.Monday), nil, nil, false},
		{"monthly with a day", RecurrenceFrequencyMonthly, 1, nil, dayPtr(15), nil, false},
		{"an unknown frequency", RecurrenceFrequency("hourly"), 1, nil, nil, nil, true},
		{"an interval below 1", RecurrenceFrequencyDaily, 0, nil, nil, nil, true},
		{"weekly without a day of week", RecurrenceFrequencyWeekly, 1, nil, nil, nil, true},
		{"weekly with an impossible day of week", RecurrenceFrequencyWeekly, 1, weekdayPtr(time.Weekday(7)), nil, nil, true},
		{"monthly without a day of month", RecurrenceFrequencyMonthly, 1, nil, nil, nil, true},
		{"monthly day 0", RecurrenceFrequencyMonthly, 1, nil, dayPtr(0), nil, true},
		{"monthly day 32", RecurrenceFrequencyMonthly, 1, nil, dayPtr(32), nil, true},
		{"monthly day 31 is allowed", RecurrenceFrequencyMonthly, 1, nil, dayPtr(31), nil, false},
		{"an end date in the past", RecurrenceFrequencyDaily, 1, nil, nil, &past, true},
		{"an end date in the future", RecurrenceFrequencyDaily, 1, nil, nil, &future, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rule, err := NewRecurrenceRule(tt.freq, tod, tt.interval, tt.dow, tt.dom, tt.endsAt)

			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.freq, rule.Frequency())
			assert.Equal(t, tt.interval, rule.Interval())
			assert.Equal(t, tod, rule.TimeOfDay())
		})
	}
}

func TestRecurrenceRule_NextOccurrenceAfter_Daily(t *testing.T) {
	rule := daily(t, 18, 0, 1)

	tests := []struct {
		name  string
		after time.Time
		want  time.Time
	}{
		{"before today's time: today", utc(2026, 3, 10, 9, 0), utc(2026, 3, 10, 18, 0)},
		{"after today's time: tomorrow", utc(2026, 3, 10, 19, 0), utc(2026, 3, 11, 18, 0)},
		{"exactly on an occurrence: the next one", utc(2026, 3, 10, 18, 0), utc(2026, 3, 11, 18, 0)},
		{"over a month end", utc(2026, 3, 31, 19, 0), utc(2026, 4, 1, 18, 0)},
		{"over a year end", utc(2026, 12, 31, 19, 0), utc(2027, 1, 1, 18, 0)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := rule.NextOccurrenceAfter(tt.after)
			require.NotNil(t, got)
			assert.Equal(t, tt.want, *got)
		})
	}

	t.Run("every other day, walking from an occurrence", func(t *testing.T) {
		got := daily(t, 18, 0, 2).NextOccurrenceAfter(utc(2026, 3, 10, 18, 0))
		require.NotNil(t, got)
		assert.Equal(t, utc(2026, 3, 12, 18, 0), *got)
	})

	t.Run("a time in another zone is read as UTC", func(t *testing.T) {
		zone := time.FixedZone("UTC+2", 2*60*60)
		got := rule.NextOccurrenceAfter(time.Date(2026, 3, 10, 21, 0, 0, 0, zone)) // 19:00 UTC
		require.NotNil(t, got)
		assert.Equal(t, utc(2026, 3, 11, 18, 0), *got)
	})
}

func TestRecurrenceRule_NextOccurrenceAfter_Weekly(t *testing.T) {
	monday := weekly(t, time.Monday, 18, 0, 1) // 2026-03-09 is a Monday

	tests := []struct {
		name  string
		after time.Time
		want  time.Time
	}{
		{"the day before", utc(2026, 3, 8, 12, 0), utc(2026, 3, 9, 18, 0)},
		{"the same day, earlier than the time: today", utc(2026, 3, 9, 8, 0), utc(2026, 3, 9, 18, 0)},
		{"the same day, later than the time: next week", utc(2026, 3, 9, 19, 0), utc(2026, 3, 16, 18, 0)},
		{"exactly on an occurrence: next week", utc(2026, 3, 9, 18, 0), utc(2026, 3, 16, 18, 0)},
		{"mid-week", utc(2026, 3, 11, 12, 0), utc(2026, 3, 16, 18, 0)},
		{"over a year end", utc(2026, 12, 29, 12, 0), utc(2027, 1, 4, 18, 0)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := monday.NextOccurrenceAfter(tt.after)
			require.NotNil(t, got)
			assert.Equal(t, tt.want, *got)
			assert.Equal(t, time.Monday, got.Weekday())
		})
	}

	t.Run("every other week, walking from an occurrence", func(t *testing.T) {
		got := weekly(t, time.Monday, 18, 0, 2).NextOccurrenceAfter(utc(2026, 3, 9, 18, 0))
		require.NotNil(t, got)
		assert.Equal(t, utc(2026, 3, 23, 18, 0), *got)
	})

	t.Run("a Sunday rule", func(t *testing.T) {
		got := weekly(t, time.Sunday, 10, 30, 1).NextOccurrenceAfter(utc(2026, 3, 9, 12, 0))
		require.NotNil(t, got)
		assert.Equal(t, utc(2026, 3, 15, 10, 30), *got)
	})
}

func TestRecurrenceRule_NextOccurrenceAfter_Monthly(t *testing.T) {
	tests := []struct {
		name  string
		rule  RecurrenceRule
		after time.Time
		want  time.Time
	}{
		{"before the day this month: this month", monthly(t, 15, 18, 0, 1), utc(2026, 3, 10, 9, 0), utc(2026, 3, 15, 18, 0)},
		{"after the day: next month", monthly(t, 15, 18, 0, 1), utc(2026, 3, 20, 9, 0), utc(2026, 4, 15, 18, 0)},
		{"the day itself, earlier than the time: today", monthly(t, 15, 18, 0, 1), utc(2026, 3, 15, 8, 0), utc(2026, 3, 15, 18, 0)},
		{"exactly on an occurrence: the next month's", monthly(t, 15, 18, 0, 1), utc(2026, 3, 15, 18, 0), utc(2026, 4, 15, 18, 0)},
		{"over a year end", monthly(t, 15, 18, 0, 1), utc(2026, 12, 20, 9, 0), utc(2027, 1, 15, 18, 0)},
		{"every other month", monthly(t, 15, 18, 0, 2), utc(2026, 3, 15, 18, 0), utc(2026, 5, 15, 18, 0)},
		{"every other month over a year end", monthly(t, 15, 18, 0, 2), utc(2026, 11, 15, 18, 0), utc(2027, 1, 15, 18, 0)},
		{"the 31st skips a short month", monthly(t, 31, 18, 0, 1), utc(2026, 1, 31, 18, 0), utc(2026, 3, 31, 18, 0)},
		{"the 30th skips February", monthly(t, 30, 18, 0, 1), utc(2026, 1, 30, 18, 0), utc(2026, 3, 30, 18, 0)},
		{"the 29th of a leap February", monthly(t, 29, 18, 0, 1), utc(2028, 1, 29, 18, 0), utc(2028, 2, 29, 18, 0)},
		{"the 29th skips a non-leap February", monthly(t, 29, 18, 0, 1), utc(2026, 1, 29, 18, 0), utc(2026, 3, 29, 18, 0)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.rule.NextOccurrenceAfter(tt.after)
			require.NotNil(t, got)
			assert.Equal(t, tt.want, *got)
		})
	}
}

// A day that does not exist in the months the rule lands on (the 31st every 12
// months, starting from February) means no occurrence - not a search that never
// ends.
func TestRecurrenceRule_MonthlyWithADayThatNeverComes(t *testing.T) {
	rule := monthly(t, 31, 18, 0, 12)

	assert.Nil(t, rule.NextOccurrenceAfter(utc(2026, 2, 10, 9, 0)))
	assert.NotNil(t, rule.NextOccurrenceAfter(utc(2026, 1, 10, 9, 0)), "from January the 31st does exist")
}

// The session generator walks occurrences with currentTime = next;
// NextOccurrenceAfter(currentTime). If an occurrence is ever not strictly after
// its predecessor it never advances.
func TestRecurrenceRule_WalkingAlwaysMovesForward(t *testing.T) {
	rules := map[string]RecurrenceRule{
		"daily":             daily(t, 18, 0, 1),
		"every 3 days":      daily(t, 6, 30, 3),
		"weekly":            weekly(t, time.Friday, 20, 0, 1),
		"every 2 weeks":     weekly(t, time.Tuesday, 9, 0, 2),
		"monthly":           monthly(t, 15, 18, 0, 1),
		"monthly on the 31": monthly(t, 31, 18, 0, 1),
		"every 3 months":    monthly(t, 1, 8, 0, 3),
	}

	for name, rule := range rules {
		t.Run(name, func(t *testing.T) {
			current := utc(2026, 1, 1, 0, 0)
			for range 40 {
				next := rule.NextOccurrenceAfter(current)
				require.NotNil(t, next)
				require.True(t, next.After(current), "%s: %s is not after %s", name, next, current)
				current = *next
			}
		})
	}

	t.Run("monthly keeps its day of the month", func(t *testing.T) {
		rule := monthly(t, 31, 18, 0, 1)
		current := utc(2026, 1, 1, 0, 0)
		for range 24 {
			next := rule.NextOccurrenceAfter(current)
			require.NotNil(t, next)
			assert.Equal(t, 31, next.Day())
			current = *next
		}
	})
}

func TestRecurrenceRule_EndsAt(t *testing.T) {
	endsAt := time.Now().UTC().Add(10 * 24 * time.Hour)
	rule, err := NewRecurrenceRule(RecurrenceFrequencyDaily, mustTimeOfDay(t, 18, 0), 1, nil, nil, &endsAt)
	require.NoError(t, err)

	t.Run("occurrences stop at the end date", func(t *testing.T) {
		assert.NotNil(t, rule.NextOccurrenceAfter(time.Now().UTC()))
		assert.Nil(t, rule.NextOccurrenceAfter(endsAt.Add(24*time.Hour)))
	})

	t.Run("not ended yet", func(t *testing.T) {
		assert.False(t, rule.HasEnded(time.Now().UTC()))
	})

	t.Run("no end date never ends", func(t *testing.T) {
		assert.False(t, daily(t, 18, 0, 1).HasEnded(time.Now().UTC()))
	})
}
