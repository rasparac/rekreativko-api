package persistence

import (
	"time"

	"github.com/google/uuid"
	"github.com/rasparac/rekreativko-api/activity/internal/domain"
	testutil "github.com/rasparac/rekreativko-api/shared/testing"
)

// SessionTemplateBuilder provides a fluent API for building test session templates
type SessionTemplateBuilder struct {
	id              uuid.UUID
	activityGroupID uuid.UUID
	createdByID     uuid.UUID
	title           string
	description     string
	capacity        *int
	locationCity    *string
	locationCountry *string
	status          domain.SessionTemplateStatus
	recurrenceRule  *domain.RecurrenceRule
}

// NewSessionTemplateBuilder creates a new builder with sensible defaults
func NewSessionTemplateBuilder() *SessionTemplateBuilder {
	return &SessionTemplateBuilder{
		id:              uuid.New(),
		activityGroupID: uuid.New(),
		createdByID:     uuid.New(),
		title:           "Test Session Template",
		description:     "Test description",
		capacity:        testutil.Ptr(20),
		status:          domain.SessionTemplateStatusActive,
	}
}

// WithID sets the template ID
func (b *SessionTemplateBuilder) WithID(id uuid.UUID) *SessionTemplateBuilder {
	b.id = id
	return b
}

// WithActivityGroup sets the activity group ID
func (b *SessionTemplateBuilder) WithActivityGroup(groupID uuid.UUID) *SessionTemplateBuilder {
	b.activityGroupID = groupID
	return b
}

// WithCreator sets the creator ID
func (b *SessionTemplateBuilder) WithCreator(creatorID uuid.UUID) *SessionTemplateBuilder {
	b.createdByID = creatorID
	return b
}

// WithTitle sets the title
func (b *SessionTemplateBuilder) WithTitle(title string) *SessionTemplateBuilder {
	b.title = title
	return b
}

// WithDescription sets the description
func (b *SessionTemplateBuilder) WithDescription(desc string) *SessionTemplateBuilder {
	b.description = desc
	return b
}

// WithCapacity sets the capacity
func (b *SessionTemplateBuilder) WithCapacity(capacity int) *SessionTemplateBuilder {
	b.capacity = &capacity
	return b
}

// WithLocation sets the location
func (b *SessionTemplateBuilder) WithLocation(city, country string) *SessionTemplateBuilder {
	b.locationCity = &city
	b.locationCountry = &country
	return b
}

// WithStatus sets the status
func (b *SessionTemplateBuilder) WithStatus(status domain.SessionTemplateStatus) *SessionTemplateBuilder {
	b.status = status
	return b
}

// AsInactive sets the status to inactive
func (b *SessionTemplateBuilder) AsInactive() *SessionTemplateBuilder {
	b.status = domain.SessionTemplateStatusInactive
	return b
}

// WithWeeklyRecurrence sets up a weekly recurrence pattern
func (b *SessionTemplateBuilder) WithWeeklyRecurrence(dayOfWeek time.Weekday, hour, minute int) *SessionTemplateBuilder {
	timeOfDay, _ := domain.NewTimeOfDay(hour, minute)
	rule, _ := domain.NewRecurrenceRule(
		domain.RecurrenceFrequencyWeekly,
		timeOfDay,
		1, // every week
		testutil.Ptr(dayOfWeek),
		nil, // no day of month for weekly
		nil, // no end date
	)
	b.recurrenceRule = &rule
	return b
}

// WithMonthlyRecurrence sets up a monthly recurrence pattern
func (b *SessionTemplateBuilder) WithMonthlyRecurrence(dayOfMonth, hour, minute int) *SessionTemplateBuilder {
	timeOfDay, _ := domain.NewTimeOfDay(hour, minute)
	rule, _ := domain.NewRecurrenceRule(
		domain.RecurrenceFrequencyMonthly,
		timeOfDay,
		1, // every month
		nil, // no day of week for monthly
		testutil.Ptr(dayOfMonth),
		nil, // no end date
	)
	b.recurrenceRule = &rule
	return b
}

// WithRecurrenceEndDate sets the recurrence end date
func (b *SessionTemplateBuilder) WithRecurrenceEndDate(endDate time.Time) *SessionTemplateBuilder {
	if b.recurrenceRule != nil {
		// Recreate rule with end date
		rule, _ := domain.NewRecurrenceRule(
			b.recurrenceRule.Frequency(),
			b.recurrenceRule.TimeOfDay(),
			b.recurrenceRule.Interval(),
			b.recurrenceRule.DayOfWeek(),
			b.recurrenceRule.DayOfMonth(),
			&endDate,
		)
		b.recurrenceRule = &rule
	}
	return b
}

// Build creates the session template
func (b *SessionTemplateBuilder) Build() *domain.SessionTemplate {
	var location *domain.Location
	if b.locationCity != nil && b.locationCountry != nil {
		loc, _ := domain.NewLocation(*b.locationCity, *b.locationCountry, 0.0, 0.0)
		location = &loc
	}

	// Use ReconstructSessionTemplate to set ID and status
	return domain.ReconstructSessionTemplate(
		b.id,
		b.activityGroupID,
		b.createdByID,
		b.title,
		b.description,
		b.status,
		b.recurrenceRule,
		b.capacity,
		location,
		nil, // generatedUpTo
		testutil.NowUTC(),
		testutil.NowUTC(),
		nil, // deletedAt
	)
}

// BuildMultiple creates multiple templates with sequential titles
func (b *SessionTemplateBuilder) BuildMultiple(count int) []*domain.SessionTemplate {
	templates := make([]*domain.SessionTemplate, count)
	for i := 0; i < count; i++ {
		builder := *b // copy
		builder.id = uuid.New()
		builder.title = builder.title + " " + string(rune('A'+i))
		templates[i] = builder.Build()
	}
	return templates
}
