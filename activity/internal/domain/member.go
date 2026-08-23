package domain

import (
	"time"

	"github.com/google/uuid"
	"github.com/rasparac/rekreativko-api/shared/domainevent"
)

// Member aggregate

type MemberRole string

const (
	MemberRoleCreator MemberRole = "creator"
	MemberRoleAdmin   MemberRole = "admin"
	MemberRoleMember  MemberRole = "member"
)

func (r MemberRole) IsValid() bool {
	switch r {
	case MemberRoleCreator, MemberRoleAdmin, MemberRoleMember:
		return true
	default:
		return false
	}
}

func (r MemberRole) String() string {
	return string(r)
}

func (r MemberRole) CanManageMembers() bool {
	return r == MemberRoleCreator || r == MemberRoleAdmin
}

type MemberStatus string

const (
	MemberStatusPending   MemberStatus = "pending"
	MemberStatusConfirmed MemberStatus = "confirmed"
	MemberStatusRejected  MemberStatus = "rejected"
	MemberStatusLeft      MemberStatus = "left"
	MemberStatusRemoved   MemberStatus = "removed"
)

func (ms MemberStatus) IsValid() bool {
	switch ms {
	case MemberStatusPending,
		MemberStatusConfirmed,
		MemberStatusRejected,
		MemberStatusLeft,
		MemberStatusRemoved:
		return true
	default:
		return false
	}
}

func (ms MemberStatus) String() string {
	return string(ms)
}

func (ms MemberStatus) IsActive() bool {
	return ms == MemberStatusPending || ms == MemberStatusConfirmed
}

func (ms MemberStatus) CanReJoin() bool {
	return ms == MemberStatusLeft || ms == MemberStatusRejected
}

type Member struct {
	id              uuid.UUID
	activityGroupID uuid.UUID
	userID          uuid.UUID
	role            MemberRole
	status          MemberStatus
	isPriority      bool // VIP member with early session access

	joinedAt  time.Time
	decidedAt *time.Time // Time when the Member accepted or rejected the invitation
	leftAt    *time.Time // Time when the Member left the activity group
	events    []domainevent.Event
}

func (p *Member) ID() uuid.UUID {
	return p.id
}

func (p *Member) ActivityGroupID() uuid.UUID {
	return p.activityGroupID
}

func (p *Member) UserID() uuid.UUID {
	return p.userID
}

func (p *Member) Role() MemberRole {
	return p.role
}

func (p *Member) Status() MemberStatus {
	return p.status
}

func (p *Member) JoinedAt() time.Time {
	return p.joinedAt
}

func (p *Member) DecidedAt() *time.Time {
	return p.decidedAt
}

func (p *Member) IsPending() bool {
	return p.status == MemberStatusPending
}

func (p *Member) IsConfirmed() bool {
	return p.status == MemberStatusConfirmed
}

func (p *Member) IsPriority() bool {
	return p.isPriority
}

func NewCreatorMember(activityGroupID, userID uuid.UUID) *Member {
	now := time.Now().UTC()
	p := &Member{
		id:              uuid.New(),
		activityGroupID: activityGroupID,
		userID:          userID,
		role:            MemberRoleCreator,
		status:          MemberStatusConfirmed,
		joinedAt:        now,
		decidedAt:       &now,
	}

	return p
}

func NewJoinRequest(activityGroupID, userID uuid.UUID) *Member {
	m := &Member{
		id:              uuid.New(),
		activityGroupID: activityGroupID,
		userID:          userID,
		role:            MemberRoleMember,
		status:          MemberStatusPending,
		joinedAt:        time.Now().UTC(),
	}

	m.addEvent(NewMemberJoinRequestedEvent(m))

	return m
}

func NewMemberFromInvite(activityGroupID, userID uuid.UUID) *Member {
	m := &Member{
		id:              uuid.New(),
		activityGroupID: activityGroupID,
		userID:          userID,
		role:            MemberRoleMember,
		status:          MemberStatusConfirmed,
		joinedAt:        time.Now().UTC(),
	}

	m.addEvent(NewMemberJoinedEvent(m, true))

	return m
}

func NewMemberFromInviteLink(activityGroupID, userID uuid.UUID) *Member {
	m := &Member{
		id:              uuid.New(),
		activityGroupID: activityGroupID,
		userID:          userID,
		role:            MemberRoleMember,
		status:          MemberStatusConfirmed,
		joinedAt:        time.Now().UTC(),
	}

	m.addEvent(NewMemberJoinedEvent(m, true))

	return m
}

func (p *Member) Approve(approvedBy uuid.UUID, approverRole MemberRole) error {
	if !approverRole.CanManageMembers() {
		return ErrUnauthorized
	}

	if p.Status() != MemberStatusPending {
		return ErrNotPendingStatus
	}

	now := time.Now().UTC()
	p.status = MemberStatusConfirmed
	p.decidedAt = &now

	p.addEvent(NewMemberApprovedEvent(p, approvedBy))

	return nil
}

func (p *Member) Reject(rejectedBy uuid.UUID, rejectorRole MemberRole) error {
	if !rejectorRole.CanManageMembers() {
		return ErrUnauthorized
	}

	if p.Status() != MemberStatusPending {
		return ErrNotPendingStatus
	}

	now := time.Now().UTC()
	p.status = MemberStatusRejected
	p.decidedAt = &now

	p.addEvent(NewMemberRejectedEvent(p, rejectedBy))

	return nil
}

func (p *Member) Leave() error {
	if p.Role() == MemberRoleCreator {
		return ErrCreatorCannotLeave
	}

	if p.Status() != MemberStatusConfirmed {
		return ErrNotConfirmed
	}

	now := time.Now().UTC()
	p.status = MemberStatusLeft
	p.decidedAt = &now

	p.addEvent(NewMemberLeftEvent(p))

	return nil
}

func (p *Member) Remove(
	removerID uuid.UUID,
	removerRole MemberRole,
) error {
	if p.Role() == MemberRoleCreator {
		return ErrCreatorCannotLeave
	}

	if removerRole == MemberRoleAdmin && p.Role() == MemberRoleAdmin {
		return ErrUnauthorized
	}

	if !removerRole.CanManageMembers() {
		return ErrUnauthorized
	}

	if p.Status() != MemberStatusConfirmed {
		return ErrNotConfirmed
	}

	now := time.Now().UTC()
	p.status = MemberStatusRemoved
	p.decidedAt = &now

	p.addEvent(NewMemberRemovedEvent(p, removerID))

	return nil
}

func (p *Member) PromoteToAdmin(
	requesterID uuid.UUID,
	requesterRole MemberRole,
) error {
	return p.changeRole(requesterID, requesterRole, MemberRoleAdmin)
}

func (p *Member) DemoteToMember(
	requesterID uuid.UUID,
	requesterRole MemberRole,
) error {
	return p.changeRole(requesterID, requesterRole, MemberRoleMember)
}

func (p *Member) changeRole(
	requesterID uuid.UUID,
	requesterRole MemberRole,
	newRole MemberRole,
) error {
	if requesterRole != MemberRoleCreator {
		return ErrUnauthorized
	}

	if p.Role() == MemberRoleCreator {
		return ErrCannotDemoteCreator
	}

	if p.Status() != MemberStatusConfirmed {
		return ErrNotConfirmed
	}

	if newRole == MemberRoleMember && p.Role() != MemberRoleAdmin {
		return ErrMemberNotAdmin
	}

	if newRole == MemberRoleAdmin && p.Role() == MemberRoleAdmin {
		return ErrMemberAlreadyAdmin
	}

	oldRole := p.role
	p.role = newRole

	p.addEvent(NewMemberRoleChangedEvent(
		p,
		requesterID,
		requesterRole,
		oldRole,
	))

	return nil
}

func (p *Member) CanReJoin() bool {
	return p.status == MemberStatusLeft || p.status == MemberStatusRemoved
}

func (p *Member) SetPriority(
	requesterID uuid.UUID,
	requesterRole MemberRole,
) error {
	if !requesterRole.CanManageMembers() {
		return ErrUnauthorized
	}

	if p.Status() != MemberStatusConfirmed {
		return ErrNotConfirmed
	}

	if p.isPriority {
		return ErrMemberAlreadyPriority
	}

	p.isPriority = true

	p.addEvent(NewMemberPrioritySetEvent(p, requesterID))

	return nil
}

func (p *Member) RemovePriority(
	requesterID uuid.UUID,
	requesterRole MemberRole,
) error {
	if !requesterRole.CanManageMembers() {
		return ErrUnauthorized
	}

	if p.Status() != MemberStatusConfirmed {
		return ErrNotConfirmed
	}

	if !p.isPriority {
		return ErrMemberNotPriority
	}

	p.isPriority = false

	p.addEvent(NewMemberPriorityRemovedEvent(p, requesterID))

	return nil
}

func (p *Member) addEvent(event domainevent.Event) {
	p.events = append(p.events, event)
}

func (p *Member) ClearEvents() {
	p.events = make([]domainevent.Event, 0)
}

func (p *Member) Events() []domainevent.Event {
	return p.events
}

// ReconstructMember reconstitutes a Member from persistence without running creation validations
// This is used by the repository layer to load existing members from the database
func ReconstructMember(
	id uuid.UUID,
	activityGroupID uuid.UUID,
	userID uuid.UUID,
	role MemberRole,
	status MemberStatus,
	isPriority bool,
	joinedAt time.Time,
	decidedAt *time.Time,
	leftAt *time.Time,
) *Member {
	return &Member{
		id:              id,
		activityGroupID: activityGroupID,
		userID:          userID,
		role:            role,
		status:          status,
		isPriority:      isPriority,
		joinedAt:        joinedAt,
		decidedAt:       decidedAt,
		leftAt:          leftAt,
		events:          make([]domainevent.Event, 0),
	}
}
