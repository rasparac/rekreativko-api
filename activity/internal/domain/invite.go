package domain

import (
	"time"

	"github.com/google/uuid"
	"github.com/rasparac/rekreativko-api/shared/domainevent"
)

const InviteExpiryDuration = 7 * 24 * time.Hour // 7 days

type InviteStatus string

const (
	InviteStatusPending  InviteStatus = "pending"
	InviteStatusAccepted InviteStatus = "accepted"
	InviteStatusDeclined InviteStatus = "declined"
	InviteStatusExpired  InviteStatus = "expired"
)

func (s InviteStatus) IsValid() bool {
	switch s {
	case InviteStatusPending, InviteStatusAccepted, InviteStatusDeclined, InviteStatusExpired:
		return true
	default:
		return false
	}
}

func (s InviteStatus) String() string {
	return string(s)
}

type GroupInvite struct {
	id              uuid.UUID
	activityGroupID uuid.UUID
	invitedUserID   uuid.UUID
	invitedByID     uuid.UUID
	status          InviteStatus
	createdAt       time.Time
	expiresAt       time.Time
	respondedAt     *time.Time

	events []domainevent.Event
}

func NewInvite(
	activityGroupID uuid.UUID,
	invitedUserID uuid.UUID,
	invitedByID uuid.UUID,
	senderRole MemberRole,
) (*GroupInvite, error) {
	if !senderRole.CanManageMembers() {
		return nil, ErrUnauthorized
	}

	if invitedByID == invitedUserID {
		return nil, ErrCannotInviteCreator
	}

	now := time.Now().UTC()

	invite := &GroupInvite{
		id:              uuid.New(),
		activityGroupID: activityGroupID,
		invitedUserID:   invitedUserID,
		invitedByID:     invitedByID,
		status:          InviteStatusPending,
		createdAt:       now,
		expiresAt:       now.Add(InviteExpiryDuration),
	}

	invite.addEvent(NewInviteSentEvent(invite))

	return invite, nil
}

// ReconstructGroupInvite rebuilds a GroupInvite from persisted data without
// running creation-time validations (e.g. permission checks), which only
// apply when an invite is first sent, not when loading an existing one.
func ReconstructGroupInvite(
	id uuid.UUID,
	activityGroupID uuid.UUID,
	invitedUserID uuid.UUID,
	invitedByID uuid.UUID,
	status InviteStatus,
	createdAt time.Time,
	expiresAt time.Time,
	respondedAt *time.Time,
) *GroupInvite {
	return &GroupInvite{
		id:              id,
		activityGroupID: activityGroupID,
		invitedUserID:   invitedUserID,
		invitedByID:     invitedByID,
		status:          status,
		createdAt:       createdAt,
		expiresAt:       expiresAt,
		respondedAt:     respondedAt,
	}
}

func (i *GroupInvite) ID() uuid.UUID {
	return i.id
}

func (i *GroupInvite) ActivityGroupID() uuid.UUID {
	return i.activityGroupID
}

func (i *GroupInvite) InvitedUserID() uuid.UUID {
	return i.invitedUserID
}

func (i *GroupInvite) InvitedByID() uuid.UUID {
	return i.invitedByID
}

func (i *GroupInvite) Status() InviteStatus {
	return i.status
}

func (i *GroupInvite) CreatedAt() time.Time {
	return i.createdAt
}

func (i *GroupInvite) ExpiresAt() time.Time {
	return i.expiresAt
}

func (i *GroupInvite) RespondedAt() *time.Time {
	return i.respondedAt
}

func (i *GroupInvite) ClearEvents() {
	i.events = make([]domainevent.Event, 0)
}

func (i *GroupInvite) IsExpired() bool {
	return time.Now().UTC().After(i.expiresAt)
}

func (i *GroupInvite) Events() []domainevent.Event {
	return i.events
}

func (i *GroupInvite) addEvent(event domainevent.Event) {
	i.events = append(i.events, event)
}

func (i *GroupInvite) Accept(userID uuid.UUID) error {
	if i.InvitedUserID() != userID {
		return ErrUnauthorized
	}

	if i.IsExpired() {
		return ErrInviteExpired
	}

	if i.Status() != InviteStatusPending {
		return ErrInviteAlreadyProcessed
	}

	now := time.Now().UTC()
	i.status = InviteStatusAccepted
	i.respondedAt = &now

	i.addEvent(NewInviteAcceptedEvent(i))

	return nil
}

func (i *GroupInvite) Decline(userID uuid.UUID) error {
	if i.InvitedUserID() != userID {
		return ErrUnauthorized
	}

	if i.IsExpired() {
		return ErrInviteExpired
	}

	if i.Status() != InviteStatusPending {
		return ErrInviteAlreadyProcessed
	}

	now := time.Now().UTC()
	i.status = InviteStatusDeclined
	i.respondedAt = &now

	i.addEvent(NewInviteDeclinedEvent(i))

	return nil
}

// Expire marks the invite as expired if it is still pending and has passed its expiry time.
// Called by a background job to clean up expired invites.
func (i *GroupInvite) Expire() error {
	if !i.IsExpired() {
		return ErrInviteHasNotExpiredYet
	}

	if i.status == InviteStatusPending {
		i.status = InviteStatusExpired
		i.addEvent(NewInviteExpiredEvent(i))
	}

	return nil
}
