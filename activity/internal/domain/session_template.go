package domain

import (
	"time"

	"github.com/google/uuid"
	"github.com/rasparac/rekreativko-api/shared/domainevent"
)

type SessionTemplateStatus string

const (
	SessionTemplateStatusActive   SessionTemplateStatus = "active"
	SessionTemplateStatusInactive SessionTemplateStatus = "inactive"
	// Future states:
	// SessionTemplateStatusPaused   SessionTemplateStatus = "paused"
	// SessionTemplateStatusArchived SessionTemplateStatus = "archived"
)

// Invalid markers for recurrence fields (used by repository layer)
const (
	InvalidDayOfWeek  = -1
	InvalidTimeValue  = -1
	InvalidDayOfMonth = 0
)

func (s SessionTemplateStatus) IsValid() bool {
	switch s {
	case SessionTemplateStatusActive, SessionTemplateStatusInactive:
		return true
	default:
		return false
	}
}

func (s SessionTemplateStatus) String() string {
	return string(s)
}

// SessionTemplate is the blueprint for recurring or scheduled sessions.
// A group can have multiple session templates, each with its own recurrence rule and default settings.
// (eg. "Monday morning run", "Saturday long ride", "weekly yoga class", "monthly hiking trip")
type SessionTemplate struct {
	id              uuid.UUID
	activityGroupID uuid.UUID
	createdByID     uuid.UUID
	title           string
	status          SessionTemplateStatus
	description     string

	recurrenceRule  *RecurrenceRule
	defaultCapacity *int
	defaultLocation *Location   // required; inherited by every session generated from this template
	teamConfig      *TeamConfig // optional; inherited by every session generated from this template
	generatedUpTo   *time.Time  // tracks how far ahead sessions have been generated

	createdAt time.Time
	updatedAt time.Time
	deletedAt *time.Time

	events []domainevent.Event
}

// validateFields checks common validation rules for title, capacity, location, and recurrence
func validateFields(title string, defaultCapacity *int, defaultLocation *Location, recurrenceRule *RecurrenceRule) error {
	if title == "" {
		return ErrSessionTemplateTitleRequired
	}

	if defaultLocation == nil {
		return ErrSessionTemplateLocationRequired
	}

	if defaultCapacity != nil && *defaultCapacity < 0 {
		return ErrInvalidDefaultCapacity
	}

	if recurrenceRule != nil && !recurrenceRule.Frequency().IsValid() {
		return ErrInvalidRecurrenceFrequency
	}

	return nil
}

func NewSessionTemplate(
	activityGroupID uuid.UUID,
	createdByID uuid.UUID,
	title string,
	description string,
	recurrenceRule *RecurrenceRule,
	defaultCapacity *int,
	defaultLocation *Location,
	teamConfig *TeamConfig,
) (*SessionTemplate, error) {
	if err := validateFields(title, defaultCapacity, defaultLocation, recurrenceRule); err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	st := &SessionTemplate{
		id:              uuid.New(),
		activityGroupID: activityGroupID,
		createdByID:     createdByID,
		title:           title,
		description:     description,
		status:          SessionTemplateStatusActive,
		recurrenceRule:  recurrenceRule,
		defaultCapacity: defaultCapacity,
		defaultLocation: defaultLocation,
		teamConfig:      teamConfig,
		createdAt:       now,
		updatedAt:       now,
	}

	st.addEvent(NewSessionTemplateCreatedEvent(st))
	return st, nil
}

// ReconstructSessionTemplate reconstructs a SessionTemplate from persistence layer
// Used by repository to recreate domain objects from database
func ReconstructSessionTemplate(
	id uuid.UUID,
	activityGroupID uuid.UUID,
	createdByID uuid.UUID,
	title string,
	description string,
	status SessionTemplateStatus,
	recurrenceRule *RecurrenceRule,
	defaultCapacity *int,
	defaultLocation *Location,
	teamConfig *TeamConfig,
	generatedUpTo *time.Time,
	createdAt time.Time,
	updatedAt time.Time,
	deletedAt *time.Time,
) *SessionTemplate {
	return &SessionTemplate{
		id:              id,
		activityGroupID: activityGroupID,
		createdByID:     createdByID,
		title:           title,
		description:     description,
		status:          status,
		recurrenceRule:  recurrenceRule,
		defaultCapacity: defaultCapacity,
		defaultLocation: defaultLocation,
		teamConfig:      teamConfig,
		generatedUpTo:   generatedUpTo,
		createdAt:       createdAt,
		updatedAt:       updatedAt,
		deletedAt:       deletedAt,
		events:          nil, // no events when reconstructing from DB
	}
}

func (st *SessionTemplate) Update(
	title string,
	description string,
	recurrenceRule *RecurrenceRule,
	defaultCapacity *int,
	defaultLocation *Location,
	teamConfig *TeamConfig,
) error {
	if err := st.requiredNotDeleted(); err != nil {
		return err
	}

	if err := validateFields(title, defaultCapacity, defaultLocation, recurrenceRule); err != nil {
		return err
	}

	st.title = title
	st.description = description
	st.recurrenceRule = recurrenceRule
	st.defaultCapacity = defaultCapacity
	st.defaultLocation = defaultLocation
	st.teamConfig = teamConfig
	st.updatedAt = time.Now().UTC()

	st.addEvent(NewSessionTemplateUpdatedEvent(st))

	return nil
}

func (st *SessionTemplate) Activate() error {
	if err := st.requiredNotDeleted(); err != nil {
		return err
	}

	if st.status == SessionTemplateStatusActive {
		return nil // already active
	}

	st.status = SessionTemplateStatusActive
	st.updatedAt = time.Now().UTC()

	st.addEvent(NewSessionTemplateActivatedEvent(st))

	return nil
}

func (st *SessionTemplate) Deactivate() error {
	if err := st.requiredNotDeleted(); err != nil {
		return err
	}

	if st.status == SessionTemplateStatusInactive {
		return nil // already inactive
	}

	st.status = SessionTemplateStatusInactive
	st.updatedAt = time.Now().UTC()

	st.addEvent(NewSessionTemplateDeactivatedEvent(st))

	return nil
}

// UpdateGeneratedUpTo updates the timestamp tracking how far ahead sessions have been generated
// This is called by the cron job after generating sessions
func (st *SessionTemplate) UpdateGeneratedUpTo(generatedUpTo time.Time) error {
	if err := st.requiredNotDeleted(); err != nil {
		return err
	}

	st.generatedUpTo = &generatedUpTo
	st.updatedAt = time.Now().UTC()

	return nil
}

func (st *SessionTemplate) Delete() {
	if st.deletedAt != nil {
		return
	}

	now := time.Now().UTC()
	st.deletedAt = &now
	st.updatedAt = now

	st.addEvent(NewSessionTemplateDeletedEvent(st))
}

func (st *SessionTemplate) ID() uuid.UUID {
	return st.id
}

func (st *SessionTemplate) ActivityGroupID() uuid.UUID {
	return st.activityGroupID
}

func (st *SessionTemplate) CreatedByID() uuid.UUID {
	return st.createdByID
}

func (st *SessionTemplate) Title() string {
	return st.title
}

func (st *SessionTemplate) Description() string {
	return st.description
}

func (st *SessionTemplate) Status() SessionTemplateStatus {
	return st.status
}

func (st *SessionTemplate) IsActive() bool {
	return st.status == SessionTemplateStatusActive
}

func (st *SessionTemplate) RecurrenceRule() *RecurrenceRule {
	return st.recurrenceRule
}

func (st *SessionTemplate) DefaultCapacity() *int {
	return st.defaultCapacity
}

func (st *SessionTemplate) DefaultLocation() *Location {
	return st.defaultLocation
}

// TeamConfig is the team setup inherited by generated sessions - nil means
// generated sessions have no teams. Changing it only affects sessions
// generated afterwards.
func (st *SessionTemplate) TeamConfig() *TeamConfig {
	return st.teamConfig
}

func (st *SessionTemplate) GeneratedUpTo() *time.Time {
	return st.generatedUpTo
}

func (st *SessionTemplate) LocationCity() string {
	if st.defaultLocation == nil {
		return ""
	}
	return st.defaultLocation.City()
}

func (st *SessionTemplate) LocationCountry() string {
	if st.defaultLocation == nil {
		return ""
	}
	return st.defaultLocation.Country()
}

// Recurrence field getters for repository persistence
func (st *SessionTemplate) RecurrenceFrequency() RecurrenceFrequency {
	if st.recurrenceRule == nil {
		return ""
	}
	return st.recurrenceRule.Frequency()
}

func (st *SessionTemplate) RecurrenceInterval() int {
	if st.recurrenceRule == nil {
		return 0
	}
	return st.recurrenceRule.Interval()
}

func (st *SessionTemplate) RecurrenceDayOfWeek() int {
	if st.recurrenceRule == nil || st.recurrenceRule.DayOfWeek() == nil {
		return InvalidDayOfWeek
	}
	return int(*st.recurrenceRule.DayOfWeek())
}

func (st *SessionTemplate) RecurrenceDayOfMonth() int {
	if st.recurrenceRule == nil || st.recurrenceRule.DayOfMonth() == nil {
		return InvalidDayOfMonth
	}
	return *st.recurrenceRule.DayOfMonth()
}

func (st *SessionTemplate) RecurrenceTimeHour() int {
	if st.recurrenceRule == nil {
		return InvalidTimeValue
	}
	return st.recurrenceRule.TimeOfDay().Hour()
}

func (st *SessionTemplate) RecurrenceTimeMinute() int {
	if st.recurrenceRule == nil {
		return InvalidTimeValue
	}
	return st.recurrenceRule.TimeOfDay().Minute()
}

func (st *SessionTemplate) RecurrenceEndsAt() *time.Time {
	if st.recurrenceRule == nil {
		return nil
	}
	return st.recurrenceRule.EndsAt()
}

func (st *SessionTemplate) CreatedAt() time.Time {
	return st.createdAt
}

func (st *SessionTemplate) UpdatedAt() time.Time {
	return st.updatedAt
}

func (st *SessionTemplate) DeletedAt() *time.Time {
	return st.deletedAt
}

func (st *SessionTemplate) addEvent(event domainevent.Event) {
	st.events = append(st.events, event)
}

func (st *SessionTemplate) Events() []domainevent.Event {
	return st.events
}

func (st *SessionTemplate) ClearEvents() {
	st.events = nil
}

// cron job helpers

func (st *SessionTemplate) IsRecurring() bool {
	return st.recurrenceRule != nil
}

func (st *SessionTemplate) NextSessionTime(after time.Time) *time.Time {
	if st.recurrenceRule == nil {
		return nil
	}

	return st.recurrenceRule.NextOccurrenceAfter(after)
}

func (st *SessionTemplate) requiredNotDeleted() error {
	if st.deletedAt != nil {
		return ErrSessionTemplateDeleted
	}
	return nil
}
