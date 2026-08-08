package domain

import (
	"time"

	"github.com/google/uuid"
	"github.com/rasparac/rekreativko-api/shared/domainevent"
)

type SessionStatus string

const (
	// SessionStatusCollectiong is a transient status used when
	// creating a session with auto-confirmed attendees.
	SessionStatusCollectiong SessionStatus = "collecting"
	SessionStatusScheduled   SessionStatus = "scheduled"
	SessionStatusStarted     SessionStatus = "started"
	SessionStatusCanceled    SessionStatus = "canceled"
	SessionStatusCompleted   SessionStatus = "completed"
)

func (s SessionStatus) IsValid() bool {
	switch s {
	case SessionStatusScheduled, SessionStatusStarted, SessionStatusCanceled, SessionStatusCompleted:
		return true
	default:
		return false
	}
}

func (s SessionStatus) String() string {
	return string(s)
}

type SessionLocation struct {
	city      string
	country   string
	latitude  float64
	longitude float64
}

func NewSessionLocation(
	city, country string,
	lat, lng float64,
) (SessionLocation, error) {
	if city == "" {
		return SessionLocation{}, ErrSessionLocationCityRequired
	}

	if country == "" {
		return SessionLocation{}, ErrSessionLocationCountryRequired
	}

	if lat < -90 || lat > 90 {
		return SessionLocation{}, ErrSessionLocationLatitudeInvalid
	}

	if lng < -180 || lng > 180 {
		return SessionLocation{}, ErrSessionLocationLongitudeInvalid
	}

	return SessionLocation{
		city:      city,
		country:   country,
		latitude:  lat,
		longitude: lng,
	}, nil
}

func (sl SessionLocation) City() string {
	return sl.city
}

func (sl SessionLocation) Country() string {
	return sl.country
}

func (sl SessionLocation) Latitude() float64 {
	return sl.latitude
}

func (sl SessionLocation) Longitude() float64 {
	return sl.longitude
}

type SessionSchedule struct {
	startTime time.Time
	endTime   *time.Time
}

func NewSessionSchedule(startTime time.Time, endTime *time.Time) (SessionSchedule, error) {
	startTime = startTime.UTC()
	if endTime != nil {
		*endTime = endTime.UTC()
	}

	if endTime != nil && !endTime.After(startTime) {
		return SessionSchedule{}, ErrInvalidScheduleTime
	}

	if startTime.Before(time.Now().UTC()) {
		return SessionSchedule{}, ErrSessionStartTimeInPast
	}

	return SessionSchedule{
		startTime: startTime,
		endTime:   endTime,
	}, nil
}

func (ss SessionSchedule) StartTime() time.Time {
	return ss.startTime
}

func (ss SessionSchedule) EndTime() *time.Time {
	return ss.endTime
}

// Session represents a single occurrence of an activity group event.
// Two creation paths:
// 1. Manual on-off sessions
// 2. Recurring sessions generated from a session template
type Session struct {
	id              uuid.UUID
	activityGroupID uuid.UUID
	createdByID     uuid.UUID

	// TemplateID is nil for manula on-off sessions
	templateID *uuid.UUID

	location    SessionLocation
	schedule    SessionSchedule
	capacity    *int // nil means unlimited capacity
	status      SessionStatus
	isRecurring bool // true if this session is part of a recurring series, false otherwise
	note        string

	// openAt is the UTC timestamp when this session transitions from "collecting" to "scheduled" status.
	openAt *time.Time

	createdAt   time.Time
	updatedAt   time.Time
	cancelledAt *time.Time
	startedAt   *time.Time
	completedAt *time.Time

	events []domainevent.Event
}

type SessionInput struct {
	ActivityGroupID uuid.UUID
	CreatedByID     uuid.UUID
	Location        SessionLocation
	Schedule        SessionSchedule
	Capacity        *int
	Note            string
	IsRecurring     bool
	AutoAttendeeIDs []uuid.UUID
}

func NewSession(
	input SessionInput,
) (*Session, []Attendee, error) {

	if input.Capacity != nil && *input.Capacity <= 0 {
		return nil, nil, ErrInvalidSessionCapacity
	}

	s := &Session{
		id:              uuid.New(),
		activityGroupID: input.ActivityGroupID,
		createdByID:     input.CreatedByID,
		location:        input.Location,
		schedule:        input.Schedule,
		capacity:        input.Capacity,
		status:          SessionStatusScheduled,
		isRecurring:     input.IsRecurring,
		note:            input.Note,
		createdAt:       time.Now().UTC(),
	}

	// First N auto-confirmed attendees based on capacity, then the rest are auto-pending
	attendees := s.processAutoAtendees(input.AutoAttendeeIDs)

	s.addEvent(NewSessionCreatedEvent(s, input.CreatedByID))

	return s, attendees, nil
}

func (s *Session) ID() uuid.UUID {
	return s.id
}

func (s *Session) ActivityGroupID() uuid.UUID {
	return s.activityGroupID
}

func (s *Session) Location() SessionLocation {
	return s.location
}

func (s *Session) Schedule() SessionSchedule {
	return s.schedule
}

func (s *Session) Capacity() *int {
	return s.capacity
}

func (s *Session) Status() SessionStatus {
	return s.status
}

func (s *Session) IsRecurring() bool {
	return s.isRecurring
}

func (s *Session) CreatedAt() time.Time {
	return s.createdAt
}

func (s *Session) UpdatedAt() time.Time {
	return s.updatedAt
}

func (s *Session) CancelledAt() *time.Time {
	return s.cancelledAt
}

func (s *Session) StartedAt() *time.Time {
	return s.startedAt
}

func (s *Session) CompletedAt() *time.Time {
	return s.completedAt
}

func (s *Session) CreatedByID() uuid.UUID {
	return s.createdByID
}

func (s *Session) TemplateID() *uuid.UUID {
	return s.templateID
}

func (s *Session) Note() string {
	return s.note
}

func (s *Session) OpenAt() *time.Time {
	return s.openAt
}

func (s *Session) HasStarted() bool {
	return s.status == SessionStatusStarted
}

func (s *Session) ClearEvents() {
	s.events = make([]domainevent.Event, 0)
}

func (s *Session) Events() []domainevent.Event {
	return s.events
}

type SessionUpdateInput struct {
	requesterID   uuid.UUID
	requesterRole MemberRole
	Location      SessionLocation
	Schedule      SessionSchedule
	Capacity      *int
	Note          string
}

func (s *Session) Update(
	input SessionUpdateInput,
) error {
	if err := s.requireSchedule(); err != nil {
		return err
	}

	if !s.canManageSession(
		input.requesterID,
		input.requesterRole,
	) {
		return ErrUnauthorized
	}

	if input.Capacity != nil && *input.Capacity <= 0 {
		return ErrInvalidSessionCapacity
	}

	s.location = input.Location
	s.schedule = input.Schedule
	s.capacity = input.Capacity
	s.note = input.Note
	s.updatedAt = time.Now().UTC()

	s.addEvent(NewSessionUpdatedEvent(s, input.requesterID))

	return nil
}

func (s *Session) Cancel(
	requesterID uuid.UUID,
	requesterRole MemberRole,
	reason string,
) error {
	if !s.canManageSession(requesterID, requesterRole) {
		return ErrUnauthorized
	}

	if s.isCompleted() {
		return ErrSessionCompleted
	}

	if s.isCanceled() {
		return ErrSessionCanceled
	}

	now := time.Now().UTC()
	s.status = SessionStatusCanceled
	s.cancelledAt = &now

	s.addEvent(NewSessionCancelledEvent(s, requesterID, reason))

	return nil
}

func (s *Session) CancelFromGroup(reason string) error {
	if s.isCompleted() {
		return ErrSessionCompleted
	}

	if s.isCanceled() {
		return ErrSessionCanceled
	}

	now := time.Now().UTC()
	s.status = SessionStatusCanceled
	s.cancelledAt = &now

	s.addEvent(NewSessionCancelledEvent(s, uuid.Nil, reason))

	return nil
}

func (s *Session) Start(
	requesterID uuid.UUID,
	requesterRole MemberRole,
) error {
	if err := s.requireSchedule(); err != nil {
		return err
	}

	if !s.canManageSession(requesterID, requesterRole) {
		return ErrUnauthorized
	}

	now := time.Now().UTC()
	s.status = SessionStatusStarted
	s.updatedAt = now
	s.startedAt = &now

	s.addEvent(NewSessionStartedEvent(s, requesterID))

	return nil
}

func (s *Session) Complete(
	requesterID uuid.UUID,
	requesterRole MemberRole,
) error {
	if !s.HasStarted() {
		return ErrSessionNotStarted
	}

	if !requesterRole.CanManageMembers() {
		return ErrUnauthorized
	}

	now := time.Now().UTC()
	s.status = SessionStatusCompleted
	s.updatedAt = now

	s.addEvent(NewSessionCompletedEvent(s, requesterID))

	return nil
}

func (s *Session) requireSchedule() error {
	switch s.status {
	case SessionStatusCanceled:
		return ErrSessionCanceled
	case SessionStatusCompleted:
		return ErrSessionCompleted
	case SessionStatusStarted:
		return ErrSessionAlreadyStarted
	default:
		return nil
	}
}

func (s *Session) hasCapacity(currentConfirmedCount int) bool {
	if s.capacity == nil {
		return true
	}

	return currentConfirmedCount < *s.capacity
}

func (s *Session) canManageSession(userID uuid.UUID, userRole MemberRole) bool {
	if userRole.CanManageMembers() {
		return true
	}

	return s.createdByID == userID
}

func (s *Session) isCanceled() bool {
	return s.status == SessionStatusCanceled
}

func (s *Session) isCompleted() bool {
	return s.status == SessionStatusCompleted
}

func (s *Session) addEvent(event domainevent.Event) {
	s.events = append(s.events, event)
}

func (s *Session) processAutoAtendees(autoAttendeeIDs []uuid.UUID) []Attendee {
	if len(autoAttendeeIDs) == 0 {
		return []Attendee{}
	}

	var (
		attendees      = make([]Attendee, 0, len(autoAttendeeIDs))
		confirmedCount int
	)

	for _, userID := range autoAttendeeIDs {
		var attendee Attendee
		if s.hasCapacity(confirmedCount) {
			//atendeee = newAutoConfirmedAttendee(s.id, s.activityID, userID)
			confirmedCount++
			s.addEvent(NewSessionAttendeeAutoConfirmedEvent(
				s.id,
				s.activityGroupID,
				userID,
			))
		} else {
			//attendee = newAutoPendingAttendee(s.id, s.activityID, userID)
			s.addEvent(NewSessionAttendeeAutoPendingEvent(
				s.id,
				s.activityGroupID,
				userID,
			))
		}
		attendees = append(attendees, attendee)
	}

	return attendees
}
