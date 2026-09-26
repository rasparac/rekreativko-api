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
	AttendeeSourceRequested     AttendeeSource = "requested"      // RSVPed "going" on a session that requires creator/admin approval
	AttendeeSourceInvited       AttendeeSource = "invited"        // accepted a session invite from the creator (standalone sessions)
)

type Attendee struct {
	id         uuid.UUID
	activityID *uuid.UUID // nil for a standalone session with no group
	sessionID  uuid.UUID
	userID     uuid.UUID
	status     AttendeeStatus
	source     AttendeeSource
	teamID     *uuid.UUID // nil = not on a team (or the session has no teams)

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

func (a *Attendee) ActivityID() *uuid.UUID {
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

func (a *Attendee) TeamID() *uuid.UUID {
	return a.teamID
}

func (a *Attendee) CreatedAt() time.Time {
	return a.createdAt
}

func (a *Attendee) UpdatedAt() time.Time {
	return a.updatedAt
}

func newAutoConfirmedAttendee(
	sessionID uuid.UUID,
	activityID *uuid.UUID,
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
	sessionID uuid.UUID,
	activityID *uuid.UUID,
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

// NewRSVPManualAttendee creates a brand-new manual RSVP. For status=Going,
// currentConfirmedCount is checked against the session's capacity up front -
// unlike UpdateRSVP's identical check, there is no "old status" here for a
// freshly-created attendee to differ from, so the capacity check must happen
// during construction rather than via a follow-up UpdateRSVP call (which
// would be a no-op since newStatus would already equal a.status).
func NewRSVPManualAttendee(
	session *Session,
	activityID *uuid.UUID,
	userID uuid.UUID,
	status AttendeeStatus,
	currentConfirmedCount int,
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
		if !session.hasCapacity(currentConfirmedCount) {
			a.status = AttendeeStatusPending
			a.addEvent(NewAttendeeRSVPAutoPendingEvent(
				a,
				session,
			))
			break
		}
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

// NewInvitedAttendee creates the attendee for an accepted session invite.
// Capacity is handled exactly like a "going" RSVP: a full session puts the
// invitee on the waitlist (pending) rather than rejecting the acceptance.
// Invites bypass RequiresApproval, since the creator already vetted the user
// by inviting them.
func NewInvitedAttendee(
	session *Session,
	userID uuid.UUID,
	currentConfirmedCount int,
) *Attendee {
	// Going is always a valid status, so the error branch is unreachable.
	a, _ := NewRSVPManualAttendee(
		session,
		session.ActivityGroupID(),
		userID,
		AttendeeStatusGoing,
		currentConfirmedCount,
	)
	a.source = AttendeeSourceInvited

	return a
}

// NewRequestedAttendee creates a pending join request for a session that
// requires creator/admin approval - mirrors NewJoinRequest for groups. Unlike
// NewRSVPManualAttendee, this never resolves straight to "going": approval is
// a separate, explicit step (see Attendee.Approve).
func NewRequestedAttendee(
	session *Session,
	activityID *uuid.UUID,
	userID uuid.UUID,
	managerUserIDs []uuid.UUID,
) *Attendee {
	now := time.Now().UTC()

	a := &Attendee{
		id:         uuid.New(),
		sessionID:  session.ID(),
		activityID: activityID,
		userID:     userID,
		status:     AttendeeStatusPending,
		source:     AttendeeSourceRequested,
		createdAt:  now,
		updatedAt:  now,
	}

	a.addEvent(NewAttendeeJoinRequestedEvent(a, session, managerUserIDs))

	return a
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
		a.releaseTeam(TeamUnassignReasonLeft, uuid.Nil)

	case AttendeeStatusMaybe:
		a.status = AttendeeStatusMaybe
		a.updatedAt = time.Now().UTC()
		a.addEvent(NewAttendeeRSVPMaybeEvent(
			a,
			session,
			oldStatus.HoldSpot(), // signals service layer to promote next pending attendee
		))
		a.releaseTeam(TeamUnassignReasonLeft, uuid.Nil)
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

// Approve accepts a pending join request, moving the attendee to "going".
// Authorization goes through session.canManageSession rather than a bare
// role check, since a standalone session's creator has no group role at all
// (empty string) and must still be able to approve their own session's
// requests - the exact gap that caused the DELETE /sessions/{id} 500 bug
// fixed earlier for Session.Cancel/Start/Complete.
func (a *Attendee) Approve(session *Session, approverID uuid.UUID, approverRole MemberRole) error {
	if !session.canManageSession(approverID, approverRole) {
		return ErrUnauthorized
	}

	if a.status != AttendeeStatusPending || a.source != AttendeeSourceRequested {
		return ErrAttendeeNotAwaitingApproval
	}

	a.status = AttendeeStatusGoing
	a.updatedAt = time.Now().UTC()

	a.addEvent(NewAttendeeJoinApprovedEvent(a, session, approverID))

	return nil
}

// Remove removes an already-confirmed attendee from a session - a manager
// kicking someone out, as opposed to CancelRSVP (application layer only),
// which lets a user cancel only their own RSVP. Authorization goes through
// session.canManageSession, same as Approve/Reject, so a standalone
// session's creator (no group role) can still remove attendees from their
// own session. Like CancelRSVP, removal doesn't change the attendee's
// status - "removed" means soft-deleted (see AttendeeRepository.
// DeleteAttendee) - this method only validates the action and records who
// did it, for notification/audit purposes.
func (a *Attendee) Remove(session *Session, removerID uuid.UUID, removerRole MemberRole) error {
	if !session.canManageSession(removerID, removerRole) {
		return ErrUnauthorized
	}

	if a.userID == session.CreatedByID() {
		return ErrCannotRemoveCreator
	}

	if !a.status.IsConfirmed() {
		return ErrAttendeeNotGoing
	}

	a.addEvent(NewAttendeeRemovedEvent(a, session, removerID))
	a.releaseTeam(TeamUnassignReasonRemoved, removerID)

	return nil
}

// AssignToTeam puts a confirmed attendee on one of the session's teams, or
// moves them there from another team. currentTeamSize is the number of
// attendees already on teamID - the caller must read it under the session
// capacity lock so concurrent assignments can't both squeeze into the last
// slot. Assigning to the team the attendee is already on is a no-op.
func (a *Attendee) AssignToTeam(
	session *Session,
	teamID uuid.UUID,
	requesterID uuid.UUID,
	requesterRole MemberRole,
	currentTeamSize int,
) error {
	if !session.canManageSession(requesterID, requesterRole) {
		return ErrUnauthorized
	}

	if err := session.requireTeamsEditable(); err != nil {
		return err
	}

	if _, ok := session.Team(teamID); !ok {
		return ErrTeamNotFound
	}

	if !a.status.IsConfirmed() {
		return ErrAttendeeNotGoing
	}

	if a.teamID != nil && *a.teamID == teamID {
		return nil
	}

	if !session.teamConfig.hasRoomFor(currentTeamSize) {
		return ErrTeamFull
	}

	previous := a.teamID
	a.teamID = &teamID
	a.updatedAt = time.Now().UTC()

	if previous == nil {
		a.addEvent(NewAttendeeTeamAssignedEvent(a, teamID, requesterID))
	} else {
		a.addEvent(NewAttendeeTeamChangedEvent(a, *previous, teamID, requesterID))
	}

	return nil
}

// UnassignFromTeam takes an attendee off their team, back to the unassigned
// pool. A no-op when they aren't on a team.
func (a *Attendee) UnassignFromTeam(
	session *Session,
	requesterID uuid.UUID,
	requesterRole MemberRole,
) error {
	if !session.canManageSession(requesterID, requesterRole) {
		return ErrUnauthorized
	}

	if err := session.requireTeamsEditable(); err != nil {
		return err
	}

	a.releaseTeam(TeamUnassignReasonManual, requesterID)

	return nil
}

// UnassignForReplacedTeams takes the attendee off their team because the
// session's teams are being recreated (Session.CreateTeams) - the old team is
// about to be deleted. Authorization is checked once, by CreateTeams.
func (a *Attendee) UnassignForReplacedTeams(replacedBy uuid.UUID) {
	a.releaseTeam(TeamUnassignReasonTeamsReplaced, replacedBy)
}

// LeaveTeam frees the attendee's team slot when their RSVP is cancelled
// outright (the row is deleted, so there is no status change to hook into).
func (a *Attendee) LeaveTeam() {
	a.releaseTeam(TeamUnassignReasonLeft, uuid.Nil)
}

func (a *Attendee) releaseTeam(reason TeamUnassignReason, by uuid.UUID) {
	if a.teamID == nil {
		return
	}

	teamID := *a.teamID
	a.teamID = nil
	a.updatedAt = time.Now().UTC()

	a.addEvent(NewAttendeeTeamUnassignedEvent(a, teamID, reason, by))
}

// Reject declines a pending join request, moving the attendee to "not_going".
func (a *Attendee) Reject(session *Session, rejectorID uuid.UUID, rejectorRole MemberRole) error {
	if !session.canManageSession(rejectorID, rejectorRole) {
		return ErrUnauthorized
	}

	if a.status != AttendeeStatusPending || a.source != AttendeeSourceRequested {
		return ErrAttendeeNotAwaitingApproval
	}

	a.status = AttendeeStatusNotGoing
	a.updatedAt = time.Now().UTC()

	a.addEvent(NewAttendeeJoinRejectedEvent(a, session, rejectorID))

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

// ReconstructAttendee reconstitutes an Attendee from persistence without running creation validations
// This is used by the repository layer to load existing attendees from the database
func ReconstructAttendee(
	id uuid.UUID,
	sessionID uuid.UUID,
	activityID *uuid.UUID,
	userID uuid.UUID,
	status AttendeeStatus,
	source AttendeeSource,
	teamID *uuid.UUID,
	createdAt time.Time,
	updatedAt time.Time,
) *Attendee {
	return &Attendee{
		id:         id,
		sessionID:  sessionID,
		activityID: activityID,
		userID:     userID,
		status:     status,
		source:     source,
		teamID:     teamID,
		createdAt:  createdAt,
		updatedAt:  updatedAt,
		events:     make([]domainevent.Event, 0),
	}
}
