package domain

import (
	"fmt"
	"time"
)

type RecurrenceFrequency string

const (
	RecurrenceFrequencyDaily   RecurrenceFrequency = "daily"
	RecurrenceFrequencyWeekly  RecurrenceFrequency = "weekly"
	RecurrenceFrequencyMonthly RecurrenceFrequency = "monthly"
)

func (f RecurrenceFrequency) IsValid() bool {
	switch f {
	case RecurrenceFrequencyDaily, RecurrenceFrequencyWeekly, RecurrenceFrequencyMonthly:
		return true
	default:
		return false
	}
}

func (f RecurrenceFrequency) String() string {
	return string(f)
}

type TimeOfDay struct {
	hour   int // 0-23
	minute int // 0-59
}

func NewTimeOfDay(hour, minute int) (TimeOfDay, error) {
	if hour < 0 || hour > 23 {
		return TimeOfDay{}, ErrInvalidTimeOfDayHour
	}
	if minute < 0 || minute > 59 {
		return TimeOfDay{}, ErrInvalidTimeOfDayMinute
	}
	return TimeOfDay{hour: hour, minute: minute}, nil
}

func (t TimeOfDay) String() string {
	return fmt.Sprintf("%02d:%02d", t.hour, t.minute)
}

func (t TimeOfDay) Hour() int {
	return t.hour
}

func (t TimeOfDay) Minute() int {
	return t.minute
}

type RecurrenceRule struct {
	frequency  RecurrenceFrequency
	interval   int           // every N frequency units (1 = every week, 2 = every 2 weeks, etc.)
	dayOfWeek  *time.Weekday // only for weekly frequency, specifies the day of the week (0 = Sunday, 1 = Monday, ..., 6 = Saturday)
	timeOfDay  TimeOfDay
	dayOfMonth *int       // only for monthly frequency, specifies the day of the month (1-31)
	endsAt     *time.Time // optional end date for the recurrence pattern
}

func NewRecurrenceRule(
	frequency RecurrenceFrequency,
	timeOfDay TimeOfDay,
	interval int,
	dayOfWeek *time.Weekday,
	dayOfMonth *int,
	endsAt *time.Time,
) (RecurrenceRule, error) {
	if !frequency.IsValid() {
		return RecurrenceRule{}, ErrInvalidRecurrenceRule
	}

	if interval < 1 {
		return RecurrenceRule{}, ErrRecurrenceRuleMissingInterval
	}

	switch frequency {
	case RecurrenceFrequencyWeekly:
		if dayOfWeek == nil {
			return RecurrenceRule{}, ErrRecurrenceRuleMissingTimeOfDay
		}
		if *dayOfWeek < 0 || *dayOfWeek > 6 {
			return RecurrenceRule{}, ErrRecurrenceRuleInvalidDayOfWeek
		}
	case RecurrenceFrequencyMonthly:
		if dayOfMonth == nil {
			return RecurrenceRule{}, ErrRecurrenceRuleMissingDayOfMonth
		}
		if *dayOfMonth < 1 || *dayOfMonth > 31 {
			return RecurrenceRule{}, ErrRecurrenceRuleInvalidDayOfMonth
		}
	}

	if endsAt != nil && endsAt.Before(time.Now()) {
		return RecurrenceRule{}, ErrRecurrenceEndDateInPast
	}

	return RecurrenceRule{
		frequency:  frequency,
		timeOfDay:  timeOfDay,
		interval:   interval,
		dayOfWeek:  dayOfWeek,
		dayOfMonth: dayOfMonth,
		endsAt:     endsAt,
	}, nil
}

func (r RecurrenceRule) Frequency() RecurrenceFrequency {
	return r.frequency
}

func (r RecurrenceRule) Interval() int {
	return r.interval
}

func (r RecurrenceRule) DayOfWeek() *time.Weekday {
	return r.dayOfWeek
}

func (r RecurrenceRule) DayOfMonth() *int {
	return r.dayOfMonth
}

func (r RecurrenceRule) EndsAt() *time.Time {
	return r.endsAt
}

func (r RecurrenceRule) TimeOfDay() TimeOfDay {
	return r.timeOfDay
}

func (r RecurrenceRule) NextOccurrenceAfter(after time.Time) *time.Time {
	afterUTC := after.UTC()

	var next time.Time
	switch r.frequency {
	case RecurrenceFrequencyDaily:
		next = r.nextDaily(afterUTC)
	case RecurrenceFrequencyWeekly:
		next = r.nextWeekly(afterUTC)
	case RecurrenceFrequencyMonthly:
		next = r.nextMonthly(afterUTC)
	}

	if r.endsAt != nil && next.After(*r.endsAt) {
		return nil
	}

	return &next
}

func (r RecurrenceRule) nextDaily(after time.Time) time.Time {
	// Calculate the next daily occurrence after the given time.
	// This is a simplified example and does not account for all edge cases.
	candidate := time.Date(
		after.Year(),
		after.Month(),
		after.Day(),
		r.timeOfDay.Hour(),
		r.timeOfDay.Minute(),
		0,
		0,
		time.UTC,
	)

	if !candidate.After(after) {
		candidate = candidate.AddDate(0, 0, r.interval)
	}

	return candidate
}

func (r RecurrenceRule) nextWeekly(after time.Time) time.Time {
	targetDay := *r.dayOfWeek

	// find days until the next target day of week
	daysUntil := int(targetDay - after.Weekday())
	if daysUntil <= 0 {
		daysUntil += 7
	}

	candidate := time.Date(
		after.Year(),
		after.Month(),
		after.Day(),
		r.TimeOfDay().Hour(),
		r.TimeOfDay().Minute(),
		0,
		0,
		time.UTC,
	).AddDate(0, 0, daysUntil)

	// if the candidate is in the past, move to the next week
	if !candidate.After(after) {
		candidate = candidate.AddDate(0, 0, 7*r.interval)
	}

	return candidate
}

func (r RecurrenceRule) nextMonthly(after time.Time) time.Time {
	targetDay := *r.dayOfMonth

	candidate := time.Date(
		after.Year(),
		after.Month(),
		targetDay,
		r.TimeOfDay().Hour(),
		r.TimeOfDay().Minute(),
		0,
		0,
		time.UTC,
	)

	if !candidate.After(after) || candidate.Day() != targetDay {
		candidate = r.nextValidMonth(after.Year(), int(after.Month())*r.interval, targetDay)
	}

	return candidate
}

func (r RecurrenceRule) nextValidMonth(
	year,
	month,
	targetDay int,
) time.Time {
	for {
		candidate := time.Date(
			year,
			time.Month(month),
			targetDay,
			r.TimeOfDay().Hour(),
			r.TimeOfDay().Minute(),
			0,
			0,
			time.UTC,
		)

		// If the candidate date is valid and matches the target day, return it.
		// Go normalize invalid dates (e.g., Feb 30 becomes March 2), so we check if the day is correct.
		if candidate.Day() == targetDay {
			return candidate
		}

		month += r.interval
	}
}

func (r RecurrenceRule) HasEnded(t time.Time) bool {
	if r.endsAt == nil {
		return false
	}
	return time.Now().UTC().After(*r.endsAt)
}
