package domain

import (
	"time"

	"github.com/google/uuid"
	"github.com/rasparac/rekreativko-api/shared/domainevent"
)

type AttendeeStatus string

const (
	AttendeeStatusGoing    AttendeeStatus = "going"     // confirmed, has spot
	AttendeeStatusPending  AttendeeStatus = "pending"   // waitlisted, no spot yet
	AttendeeStatusNotGoing AttendeeStatus = "not_going" // explicitly declined, no spot
	AttendeeStatusMaybe    AttendeeStatus = "maybe"     // interested but not confirmed, no spot
	AttendeeStatusPromoted AttendeeStatus = "promoted"  // was pending but got promoted to going due to cancellation, has spot
)

func (s AttendeeStatus) IsValid() bool {
	switch s {
	case AttendeeStatusGoing,
		AttendeeStatusPending,
		AttendeeStatusNotGoing,
		AttendeeStatusMaybe,
		AttendeeStatusPromoted:
		return true
	default:
		return false
	}
}

func (s AttendeeStatus) IsConfirmed() bool {
	return s == AttendeeStatusGoing || s == AttendeeStatusPromoted
}

func (s AttendeeStatus) HoldSpot() bool {
	return s == AttendeeStatusGoing || s == AttendeeStatusPromoted
}

type AttendeeSource string

const (
	AttendeeSourceAutoConfirmed AttendeeSource = "auto_confirmed" // added automatically by the system and confirmed (e.g. creator or auto-confirmed from capacity)
	AttendeeSourceAutoPending   AttendeeSource = "auto_pending"   // added automatically by the system but pending (e.g. auto-pending from capacity)
	AttendeeSourceRSVPManual    AttendeeSource = "rsvp_manual"    // member RSVPed themselves manually
)

type Attendee struct {
	id         uuid.UUID
	activityID uuid.UUID
	sessionID  uuid.UUID
	userID     uuid.UUID
	status     AttendeeStatus
	source     AttendeeSource

	createdAt time.Time
	updatedAt time.Time

	events []domainevent.Event
}

func NewAttendee(
	sessionID,
	userID uuid.UUID,
) *Attendee {
	return &Attendee{
		sessionID: sessionID,
		userID:    userID,
	}
}

func (a *Attendee) ID() uuid.UUID {
	return a.id
}

func (a *Attendee) ActivityID() uuid.UUID {
	return a.activityID
}

func (a *Attendee) SessionID() uuid.UUID {
	return a.sessionID
}

func (a *Attendee) UserID() uuid.UUID {
	return a.userID
}

func (a *Attendee) Status() AttendeeStatus {
	return a.status
}

func (a *Attendee) Source() AttendeeSource {
	return a.source
}

func (a *Attendee) CreatedAt() time.Time {
	return a.createdAt
}

func (a *Attendee) UpdatedAt() time.Time {
	return a.updatedAt
}

func newAutoConfirmedAttendee(
	sessionID,
	activityID,
	userID uuid.UUID,
) *Attendee {
	now := time.Now().UTC()

	return &Attendee{
		id:         uuid.New(),
		sessionID:  sessionID,
		activityID: activityID,
		userID:     userID,
		status:     AttendeeStatusGoing,
		source:     AttendeeSourceAutoConfirmed,
		createdAt:  now,
		updatedAt:  now,
	}
}

func newAutoPendingAttendee(
	sessionID,
	activityID,
	userID uuid.UUID,
) *Attendee {
	now := time.Now().UTC()

	return &Attendee{
		id:         uuid.New(),
		sessionID:  sessionID,
		activityID: activityID,
		userID:     userID,
		status:     AttendeeStatusPending,
		source:     AttendeeSourceAutoPending,
		createdAt:  now,
		updatedAt:  now,
	}
}

func NewRSVPManualAttendee(
	session *Session,
	activityID,
	userID uuid.UUID,
	status AttendeeStatus,
) (*Attendee, error) {
	if !status.IsValid() {
		return nil, ErrInvalidAttendeeStatus
	}

	now := time.Now().UTC()

	a := &Attendee{
		id:         uuid.New(),
		sessionID:  session.ID(),
		activityID: activityID,
		userID:     userID,
		status:     status,
		source:     AttendeeSourceRSVPManual,
		createdAt:  now,
		updatedAt:  now,
	}

	switch status {
	case AttendeeStatusGoing:
		a.addEvent(NewAttendeeRSVPGoingEvent(
			a,
			session,
		))
	case AttendeeStatusNotGoing:
		a.addEvent(NewAttendeeRSVPNotGoingEvent(
			a,
			session,
			a.status.HoldSpot(),
		))
	case AttendeeStatusMaybe:
		a.addEvent(NewAttendeeRSVPMaybeEvent(
			a,
			session,
			a.status.HoldSpot(),
		))
	}

	return a, nil
}

func (a *Attendee) UpdateRSVP(
	newStatus AttendeeStatus,
	session *Session,
	currentConfirmedCount int,
) error {
	if newStatus == a.status {
		return nil // no change
	}

	if err := a.validateRSVPTransition(newStatus); err != nil {
		return err
	}

	oldStatus := a.status

	switch newStatus {
	case AttendeeStatusGoing:
		if !session.hasCapacity(currentConfirmedCount) {
			a.status = AttendeeStatusPending
			a.updatedAt = time.Now().UTC()

			a.addEvent(NewAttendeeRSVPAutoPendingEvent(
				a,
				session,
			))
			return nil
		}

		a.status = AttendeeStatusGoing
		a.updatedAt = time.Now().UTC()
		a.addEvent(NewAttendeeRSVPGoingEvent(
			a,
			session,
		))
	case AttendeeStatusNotGoing:
		a.status = AttendeeStatusNotGoing
		a.updatedAt = time.Now().UTC()
		a.addEvent(NewAttendeeRSVPNotGoingEvent(
			a,
			session,
			oldStatus.HoldSpot(), // signals service layer to promote next pending attendee
		))

	case AttendeeStatusMaybe:
		a.status = AttendeeStatusMaybe
		a.updatedAt = time.Now().UTC()
		a.addEvent(NewAttendeeRSVPMaybeEvent(
			a,
			session,
			oldStatus.HoldSpot(), // signals service layer to promote next pending attendee
		))

	}

	return nil
}

func (a *Attendee) Promote() error {
	if a.status != AttendeeStatusPending {
		return ErrInvalidAttendeeStatus
	}

	a.status = AttendeeStatusPromoted
	a.updatedAt = time.Now().UTC()

	a.addEvent(NewAttendeePromotedEvent(a))

	return nil
}

func (a *Attendee) validateRSVPTransition(newStatus AttendeeStatus) error {
	if !newStatus.IsValid() {
		return ErrInvalidAttendeeStatus
	}

	switch a.Status() {
	case AttendeeStatusNotGoing:
		// can change from not_going to going/maybe
		if newStatus == AttendeeStatusGoing {
			return ErrInvalidAttendeeTransition
		}
	case AttendeeStatusMaybe:
		// can change to going, not_going
		if newStatus == AttendeeStatusPending {
			return ErrInvalidAttendeeTransition
		}
	case AttendeeStatusGoing, AttendeeStatusPromoted:
		// can change to not_going or maybe (releases spot)
		// cannot go directily to pending (use not_going first)
		if newStatus == AttendeeStatusPending {
			return ErrInvalidAttendeeTransition
		}
	case AttendeeStatusPending:
		// can change to not_going or mabye (removes from waitlist)
		// cannot go directily to going (must wait for promotion)
		if newStatus == AttendeeStatusGoing {
			return ErrInvalidAttendeeTransition
		}
	}

	return nil
}

func (a *Attendee) addEvent(event domainevent.Event) {
	a.events = append(a.events, event)
}

func (a *Attendee) Events() []domainevent.Event {
	return a.events
}

func (a *Attendee) ClearEvents() {
	a.events = make([]domainevent.Event, 0)
}
