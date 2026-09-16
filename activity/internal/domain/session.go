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

type SessionVisibility string

const (
	SessionVisibilityPrivate SessionVisibility = "private"
	SessionVisibilityPublic  SessionVisibility = "public"
)

func (v SessionVisibility) IsValid() bool {
	switch v {
	case SessionVisibilityPrivate, SessionVisibilityPublic:
		return true
	default:
		return false
	}
}

func (v SessionVisibility) String() string {
	return string(v)
}

type SessionLocation struct {
	city      string
	country   string
	street    string // optional - venue/address line, e.g. "Ada Ciganlija bb, Court 3"
	latitude  float64
	longitude float64
}

func NewSessionLocation(
	city, country, street string,
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
		street:    street,
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

func (sl SessionLocation) Street() string {
	return sl.street
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

// ReconstructSessionSchedule rebuilds a schedule from persisted data. Unlike
// NewSessionSchedule, it does not require startTime to be in the future, since
// historical sessions loaded from the database will always have a start time
// in the past once they've occurred.
func ReconstructSessionSchedule(startTime time.Time, endTime *time.Time) (SessionSchedule, error) {
	if endTime != nil && !endTime.After(startTime) {
		return SessionSchedule{}, ErrInvalidScheduleTime
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
	activityGroupID *uuid.UUID // nil for a standalone session with no group
	createdByID     uuid.UUID

	// TemplateID is nil for manula on-off sessions
	templateID *uuid.UUID

	title           Title
	activityType    ActivityType
	difficultyLevel DifficultyLevel

	location         SessionLocation
	schedule         SessionSchedule
	capacity         *int // nil means unlimited capacity
	status           SessionStatus
	visibility       SessionVisibility
	requiresApproval bool // if true, RSVPing "going" creates a pending join request instead of joining immediately
	isRecurring      bool // true if this session is part of a recurring series, false otherwise
	note             string

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
	ActivityGroupID *uuid.UUID // nil for a standalone session with no group
	CreatedByID     uuid.UUID
	TemplateID      *uuid.UUID // nil for manual one-off sessions, set for sessions generated from a template
	Title           Title
	ActivityType    ActivityType
	DifficultyLevel DifficultyLevel
	Location        SessionLocation
	Schedule        SessionSchedule
	Capacity        *int
	Note            string
	IsRecurring     bool
	// Visibility applies to both group-scoped and standalone sessions -
	// nil means "use the default" (private).
	Visibility       *SessionVisibility
	RequiresApproval bool
	OpenAt           *time.Time // When regular members can start RSVPing (nil = immediately open)
	AutoAttendeeIDs  []uuid.UUID
}

func NewSession(
	input SessionInput,
) (*Session, []*Attendee, error) {

	if input.Capacity != nil && *input.Capacity <= 0 {
		return nil, nil, ErrInvalidSessionCapacity
	}

	visibility := SessionVisibilityPrivate
	if input.Visibility != nil {
		if !input.Visibility.IsValid() {
			return nil, nil, ErrInvalidSessionVisibility
		}
		visibility = *input.Visibility
	}

	now := time.Now().UTC()
	s := &Session{
		id:               uuid.New(),
		activityGroupID:  input.ActivityGroupID,
		createdByID:      input.CreatedByID,
		templateID:       input.TemplateID,
		title:            input.Title,
		activityType:     input.ActivityType,
		difficultyLevel:  input.DifficultyLevel,
		location:         input.Location,
		schedule:         input.Schedule,
		capacity:         input.Capacity,
		status:           SessionStatusScheduled,
		visibility:       visibility,
		requiresApproval: input.RequiresApproval,
		isRecurring:      input.IsRecurring,
		note:             input.Note,
		openAt:           input.OpenAt,
		createdAt:        now,
		updatedAt:        now,
	}

	// First N auto-confirmed attendees based on capacity, then the rest are auto-pending
	attendees := s.processAutoAtendees(input.AutoAttendeeIDs)

	s.addEvent(NewSessionCreatedEvent(s, input.CreatedByID))

	return s, attendees, nil
}

// ReconstructSession rebuilds a Session from persisted data without running
// creation-time validations (e.g. NewSession's future-start-time check), which
// would otherwise reject any already-occurred session loaded from the database.
func ReconstructSession(
	id uuid.UUID,
	activityGroupID *uuid.UUID,
	createdByID uuid.UUID,
	templateID *uuid.UUID,
	title Title,
	activityType ActivityType,
	difficultyLevel DifficultyLevel,
	location SessionLocation,
	schedule SessionSchedule,
	capacity *int,
	status SessionStatus,
	visibility SessionVisibility,
	requiresApproval bool,
	isRecurring bool,
	note string,
	openAt *time.Time,
	createdAt time.Time,
	updatedAt time.Time,
	cancelledAt *time.Time,
	startedAt *time.Time,
	completedAt *time.Time,
) *Session {
	return &Session{
		id:               id,
		activityGroupID:  activityGroupID,
		createdByID:      createdByID,
		templateID:       templateID,
		title:            title,
		activityType:     activityType,
		difficultyLevel:  difficultyLevel,
		location:         location,
		schedule:         schedule,
		capacity:         capacity,
		status:           status,
		visibility:       visibility,
		requiresApproval: requiresApproval,
		isRecurring:      isRecurring,
		note:             note,
		openAt:           openAt,
		createdAt:        createdAt,
		updatedAt:        updatedAt,
		cancelledAt:      cancelledAt,
		startedAt:        startedAt,
		completedAt:      completedAt,
	}
}

func (s *Session) ID() uuid.UUID {
	return s.id
}

func (s *Session) ActivityGroupID() *uuid.UUID {
	return s.activityGroupID
}

func (s *Session) IsStandalone() bool {
	return s.activityGroupID == nil
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

func (s *Session) Visibility() SessionVisibility {
	return s.visibility
}

func (s *Session) RequiresApproval() bool {
	return s.requiresApproval
}

func (s *Session) IsPublic() bool {
	return s.visibility == SessionVisibilityPublic
}

// IsVisibleTo reports whether this session should be shown to userID -
// mirrors ActivityGroup.IsVisibleTo. isRelated is computed by the caller:
// true if userID is a confirmed member of the owning group (creator/admin
// included, since a group-scoped session's creator must already be one) or
// an attendee of this session - a standalone session has no group, so only
// the attendee check ever contributes to isRelated for it.
func (s *Session) IsVisibleTo(userID uuid.UUID, isRelated bool) bool {
	if s.visibility == SessionVisibilityPublic {
		return true
	}

	if s.createdByID == userID {
		return true
	}

	return isRelated
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

func (s *Session) Title() Title {
	return s.title
}

func (s *Session) ActivityType() ActivityType {
	return s.activityType
}

func (s *Session) DifficultyLevel() DifficultyLevel {
	return s.difficultyLevel
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
	RequesterID   uuid.UUID
	RequesterRole MemberRole
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
		input.RequesterID,
		input.RequesterRole,
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

	s.addEvent(NewSessionUpdatedEvent(s, input.RequesterID))

	return nil
}

func (s *Session) SetVisibility(
	requesterID uuid.UUID,
	requesterRole MemberRole,
	visibility SessionVisibility,
) error {
	if !s.canManageSession(requesterID, requesterRole) {
		return ErrUnauthorized
	}

	if !visibility.IsValid() {
		return ErrInvalidSessionVisibility
	}

	if visibility == s.visibility {
		return nil
	}

	old := s.visibility
	s.visibility = visibility
	s.updatedAt = time.Now().UTC()

	s.addEvent(NewSessionVisibilityChangedEvent(s, old, requesterID))

	return nil
}

func (s *Session) Cancel(
	requesterID uuid.UUID,
	requesterRole MemberRole,
	reason string,
	attendeeUserIDs []uuid.UUID,
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

	s.addEvent(NewSessionCancelledEvent(s, requesterID, reason, attendeeUserIDs))

	return nil
}

func (s *Session) CancelFromGroup(reason string, attendeeUserIDs []uuid.UUID) error {
	if s.isCompleted() {
		return ErrSessionCompleted
	}

	if s.isCanceled() {
		return ErrSessionCanceled
	}

	now := time.Now().UTC()
	s.status = SessionStatusCanceled
	s.cancelledAt = &now

	s.addEvent(NewSessionCancelledEvent(s, uuid.Nil, reason, attendeeUserIDs))

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

// ExpireSchedule auto-completes a session whose end time has passed. Unlike
// Complete, this is a system-driven time transition, not a user action - no
// requester/role check, mirroring GroupInvite.Expire.
func (s *Session) ExpireSchedule(now time.Time) error {
	endTime := s.Schedule().EndTime()
	if endTime == nil || !now.After(*endTime) {
		return ErrSessionNotExpiredYet
	}

	if s.status != SessionStatusScheduled && s.status != SessionStatusStarted {
		return nil
	}

	s.status = SessionStatusCompleted
	s.completedAt = &now
	s.updatedAt = now

	s.addEvent(NewSessionExpiredEvent(s))

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

// HasCapacity reports whether the session can accept one more confirmed
// attendee on top of currentConfirmedCount. Exported so the application layer
// can enforce capacity up front for join requests, before a slot is ever
// consumed (mirrored again, more strictly, at approval time).
func (s *Session) HasCapacity(currentConfirmedCount int) bool {
	return s.hasCapacity(currentConfirmedCount)
}

func (s *Session) canManageSession(userID uuid.UUID, userRole MemberRole) bool {
	if userRole.CanManageMembers() {
		return true
	}

	return s.createdByID == userID
}

// CanRSVP checks if a member can RSVP to this session based on priority status and opening time
func (s *Session) CanRSVP(isPriorityMember bool) error {
	// Check session status
	if s.status != SessionStatusScheduled {
		return ErrSessionNotScheduled
	}

	// Priority members can always RSVP (no time restriction)
	if isPriorityMember {
		return nil
	}

	// Regular members must wait until openAt time
	if s.openAt != nil {
		now := time.Now().UTC()
		if now.Before(*s.openAt) {
			return ErrSessionNotOpen
		}
	}

	return nil
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

func (s *Session) processAutoAtendees(autoAttendeeIDs []uuid.UUID) []*Attendee {
	if len(autoAttendeeIDs) == 0 {
		return []*Attendee{}
	}

	var (
		attendees      = make([]*Attendee, 0, len(autoAttendeeIDs))
		confirmedCount int
	)

	for _, userID := range autoAttendeeIDs {
		var attendee *Attendee
		if s.hasCapacity(confirmedCount) {
			attendee = newAutoConfirmedAttendee(s.id, s.activityGroupID, userID)
			confirmedCount++
			s.addEvent(NewSessionAttendeeAutoConfirmedEvent(
				s.id,
				s.activityGroupID,
				userID,
			))
		} else {
			attendee = newAutoPendingAttendee(s.id, s.activityGroupID, userID)
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
