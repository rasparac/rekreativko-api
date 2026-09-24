package domain

import (
	"time"

	"github.com/google/uuid"
	"github.com/rasparac/rekreativko-api/shared/domainevent"
)

// SessionInvite is a creator's invitation for a specific user to attend a
// standalone session (one with no activity group). Group sessions don't need
// it - their audience is the group's members - so a standalone private
// session would otherwise have no way to gain attendees beyond its creator.
// The invitee must accept before becoming an attendee, so nobody is added to
// a session without knowing about it.
type SessionInvite struct {
	id            uuid.UUID
	sessionID     uuid.UUID
	invitedUserID uuid.UUID
	invitedByID   uuid.UUID
	status        InviteStatus
	createdAt     time.Time
	expiresAt     time.Time
	respondedAt   *time.Time

	events []domainevent.Event
}

// NewSessionInvite creates a pending invite. Only the session's creator can
// invite; a standalone session has no group roles, so canManageSession
// reduces to a creator check when given an empty role.
func NewSessionInvite(
	session *Session,
	invitedUserID uuid.UUID,
	invitedByID uuid.UUID,
) (*SessionInvite, error) {
	if !session.IsStandalone() {
		return nil, ErrSessionNotStandalone
	}

	if !session.canManageSession(invitedByID, "") {
		return nil, ErrUnauthorized
	}

	if session.Status() != SessionStatusScheduled {
		return nil, ErrSessionNotScheduled
	}

	if invitedByID == invitedUserID {
		return nil, ErrCannotInviteCreator
	}

	now := time.Now().UTC()

	invite := &SessionInvite{
		id:            uuid.New(),
		sessionID:     session.ID(),
		invitedUserID: invitedUserID,
		invitedByID:   invitedByID,
		status:        InviteStatusPending,
		createdAt:     now,
		expiresAt:     now.Add(InviteExpiryDuration),
	}

	invite.addEvent(NewSessionInviteSentEvent(invite))

	return invite, nil
}

// ReconstructSessionInvite rebuilds a SessionInvite from persisted data
// without running creation-time validations.
func ReconstructSessionInvite(
	id uuid.UUID,
	sessionID uuid.UUID,
	invitedUserID uuid.UUID,
	invitedByID uuid.UUID,
	status InviteStatus,
	createdAt time.Time,
	expiresAt time.Time,
	respondedAt *time.Time,
) *SessionInvite {
	return &SessionInvite{
		id:            id,
		sessionID:     sessionID,
		invitedUserID: invitedUserID,
		invitedByID:   invitedByID,
		status:        status,
		createdAt:     createdAt,
		expiresAt:     expiresAt,
		respondedAt:   respondedAt,
	}
}

func (i *SessionInvite) ID() uuid.UUID {
	return i.id
}

func (i *SessionInvite) SessionID() uuid.UUID {
	return i.sessionID
}

func (i *SessionInvite) InvitedUserID() uuid.UUID {
	return i.invitedUserID
}

func (i *SessionInvite) InvitedByID() uuid.UUID {
	return i.invitedByID
}

func (i *SessionInvite) Status() InviteStatus {
	return i.status
}

func (i *SessionInvite) CreatedAt() time.Time {
	return i.createdAt
}

func (i *SessionInvite) ExpiresAt() time.Time {
	return i.expiresAt
}

func (i *SessionInvite) RespondedAt() *time.Time {
	return i.respondedAt
}

func (i *SessionInvite) IsExpired() bool {
	return time.Now().UTC().After(i.expiresAt)
}

func (i *SessionInvite) Events() []domainevent.Event {
	return i.events
}

func (i *SessionInvite) ClearEvents() {
	i.events = make([]domainevent.Event, 0)
}

func (i *SessionInvite) addEvent(event domainevent.Event) {
	i.events = append(i.events, event)
}

// Accept marks the invite accepted. Creating the attendee (and the capacity
// check that goes with it) is the service layer's job.
func (i *SessionInvite) Accept(userID uuid.UUID) error {
	if err := i.validateResponse(userID); err != nil {
		return err
	}

	now := time.Now().UTC()
	i.status = InviteStatusAccepted
	i.respondedAt = &now

	i.addEvent(NewSessionInviteAcceptedEvent(i))

	return nil
}

func (i *SessionInvite) Decline(userID uuid.UUID) error {
	if err := i.validateResponse(userID); err != nil {
		return err
	}

	now := time.Now().UTC()
	i.status = InviteStatusDeclined
	i.respondedAt = &now

	i.addEvent(NewSessionInviteDeclinedEvent(i))

	return nil
}

// Expire marks a still-pending invite expired once past its expiry time.
// Called by a background job.
func (i *SessionInvite) Expire() error {
	if !i.IsExpired() {
		return ErrInviteHasNotExpiredYet
	}

	if i.status == InviteStatusPending {
		i.status = InviteStatusExpired
		i.addEvent(NewSessionInviteExpiredEvent(i))
	}

	return nil
}

func (i *SessionInvite) validateResponse(userID uuid.UUID) error {
	if i.invitedUserID != userID {
		return ErrUnauthorized
	}

	if i.IsExpired() {
		return ErrInviteExpired
	}

	if i.status != InviteStatusPending {
		return ErrInviteAlreadyProcessed
	}

	return nil
}
