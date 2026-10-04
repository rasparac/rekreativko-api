package events

import (
	"time"

	"github.com/google/uuid"
)

// Local copies of the upstream activity-service event payloads this service
// consumes. Deliberately not importing activity's domain package - services
// stay decoupled and only agree on the JSON wire shape, same convention
// account-profile already uses for its own identity.account.verified consumer.

type memberJoinRequestedEvent struct {
	EventID         uuid.UUID   `json:"event_id"`
	ActivityGroupID uuid.UUID   `json:"activity_id"`
	UserID          uuid.UUID   `json:"user_id"`
	ManagerUserIDs  []uuid.UUID `json:"manager_user_ids"`
}

type memberApprovedEvent struct {
	EventID    uuid.UUID `json:"event_id"`
	ActivityID uuid.UUID `json:"activity_id"`
	ApprovedBy uuid.UUID `json:"approved_by"`
	UserID     uuid.UUID `json:"user_id"`
}

type memberRejectedEvent struct {
	EventID    uuid.UUID `json:"event_id"`
	ActivityID uuid.UUID `json:"activity_id"`
	RejectedBy uuid.UUID `json:"rejected_by"`
	UserID     uuid.UUID `json:"user_id"`
}

type attendeePromotedEvent struct {
	EventID   uuid.UUID `json:"event_id"`
	SessionID uuid.UUID `json:"session_id"`
	UserID    uuid.UUID `json:"user_id"`
}

type attendeeJoinRequestedEvent struct {
	EventID        uuid.UUID   `json:"event_id"`
	SessionID      uuid.UUID   `json:"session_id"`
	UserID         uuid.UUID   `json:"user_id"`
	ManagerUserIDs []uuid.UUID `json:"manager_user_ids"`
}

type attendeeJoinApprovedEvent struct {
	EventID    uuid.UUID `json:"event_id"`
	SessionID  uuid.UUID `json:"session_id"`
	UserID     uuid.UUID `json:"user_id"`
	ApprovedBy uuid.UUID `json:"approved_by"`
}

type attendeeJoinRejectedEvent struct {
	EventID    uuid.UUID `json:"event_id"`
	SessionID  uuid.UUID `json:"session_id"`
	UserID     uuid.UUID `json:"user_id"`
	RejectedBy uuid.UUID `json:"rejected_by"`
}

type attendeeRemovedEvent struct {
	EventID   uuid.UUID `json:"event_id"`
	SessionID uuid.UUID `json:"session_id"`
	UserID    uuid.UUID `json:"user_id"`
	RemovedBy uuid.UUID `json:"removed_by"`
}

type memberRemovedEvent struct {
	EventID    uuid.UUID `json:"event_id"`
	ActivityID uuid.UUID `json:"activity_id"`
	UserID     uuid.UUID `json:"user_id"`
	RemovedBy  uuid.UUID `json:"removed_by"`
}

type sessionCancelledEvent struct {
	EventID uuid.UUID `json:"event_id"`
	// SessionID is the cancelled session's ID, carried on every event as the
	// aggregate ID (SessionCancelledEvent has no separate session_id field).
	SessionID          uuid.UUID   `json:"aggregate_id"`
	ActivityID         *uuid.UUID  `json:"activity_id"`
	CancelledBy        uuid.UUID   `json:"cancelled_by"`
	CancellationReason string      `json:"cancellation_reason"`
	AttendeeUserIDs    []uuid.UUID `json:"attendee_user_ids"`
}

type inviteSentEvent struct {
	EventID       uuid.UUID `json:"event_id"`
	ActivityID    uuid.UUID `json:"activity_id"`
	InvitedBy     uuid.UUID `json:"invited_by"`
	InvitedUserID uuid.UUID `json:"invited_user_id"`
	ExpiresAt     time.Time `json:"expires_at"`
}

type inviteAcceptedEvent struct {
	EventID       uuid.UUID `json:"event_id"`
	ActivityID    uuid.UUID `json:"activity_id"`
	InvitedBy     uuid.UUID `json:"invited_by"`
	InvitedUserID uuid.UUID `json:"invited_user_id"`
}

type inviteDeclinedEvent struct {
	EventID       uuid.UUID `json:"event_id"`
	ActivityID    uuid.UUID `json:"activity_id"`
	InvitedBy     uuid.UUID `json:"invited_by"`
	InvitedUserID uuid.UUID `json:"invited_user_id"`
}

type inviteExpiredEvent struct {
	EventID       uuid.UUID `json:"event_id"`
	ActivityID    uuid.UUID `json:"activity_id"`
	InvitedBy     uuid.UUID `json:"invited_by"`
	InvitedUserID uuid.UUID `json:"invited_user_id"`
}

type sessionInviteSentEvent struct {
	EventID       uuid.UUID `json:"event_id"`
	SessionID     uuid.UUID `json:"session_id"`
	InvitedBy     uuid.UUID `json:"invited_by"`
	InvitedUserID uuid.UUID `json:"invited_user_id"`
	ExpiresAt     time.Time `json:"expires_at"`
}

type sessionInviteAcceptedEvent struct {
	EventID       uuid.UUID `json:"event_id"`
	SessionID     uuid.UUID `json:"session_id"`
	InvitedBy     uuid.UUID `json:"invited_by"`
	InvitedUserID uuid.UUID `json:"invited_user_id"`
}

type sessionInviteDeclinedEvent struct {
	EventID       uuid.UUID `json:"event_id"`
	SessionID     uuid.UUID `json:"session_id"`
	InvitedBy     uuid.UUID `json:"invited_by"`
	InvitedUserID uuid.UUID `json:"invited_user_id"`
}

type sessionInviteExpiredEvent struct {
	EventID       uuid.UUID `json:"event_id"`
	SessionID     uuid.UUID `json:"session_id"`
	InvitedBy     uuid.UUID `json:"invited_by"`
	InvitedUserID uuid.UUID `json:"invited_user_id"`
}

// Team formation events (activity.session.draft.*, activity.session.voting.*,
// activity.session.attendee.team_changed). Team positions are 0 (Team A) and
// 1 (Team B).

type draftStartedEvent struct {
	EventID    uuid.UUID    `json:"event_id"`
	SessionID  uuid.UUID    `json:"session_id"`
	DraftID    uuid.UUID    `json:"draft_id"`
	CaptainIDs [2]uuid.UUID `json:"captain_ids"`
	StartedBy  uuid.UUID    `json:"started_by"`
}

type draftTurnChangedEvent struct {
	EventID      uuid.UUID `json:"event_id"`
	SessionID    uuid.UUID `json:"session_id"`
	DraftID      uuid.UUID `json:"draft_id"`
	CaptainID    uuid.UUID `json:"captain_id"`
	TeamPosition int       `json:"team_position"`
	PickNumber   int       `json:"pick_number"`
}

type draftPausedEvent struct {
	EventID      uuid.UUID `json:"event_id"`
	SessionID    uuid.UUID `json:"session_id"`
	DraftID      uuid.UUID `json:"draft_id"`
	Reason       string    `json:"reason"`
	TeamPosition int       `json:"team_position"`
	CaptainID    uuid.UUID `json:"captain_id"` // the captain who left
	StartedBy    uuid.UUID `json:"started_by"` // the organizer who can resolve it
}

type draftCompletedEvent struct {
	EventID   uuid.UUID      `json:"event_id"`
	SessionID uuid.UUID      `json:"session_id"`
	DraftID   uuid.UUID      `json:"draft_id"`
	Teams     [2][]uuid.UUID `json:"teams"` // index = team position
}

type draftCancelledEvent struct {
	EventID            uuid.UUID   `json:"event_id"`
	SessionID          uuid.UUID   `json:"session_id"`
	DraftID            uuid.UUID   `json:"draft_id"`
	Reason             string      `json:"reason"`
	CancelledBy        uuid.UUID   `json:"cancelled_by"`
	ParticipantUserIDs []uuid.UUID `json:"participant_user_ids"`
}

type votingOpenedEvent struct {
	EventID            uuid.UUID   `json:"event_id"`
	SessionID          uuid.UUID   `json:"session_id"`
	RoundID            uuid.UUID   `json:"round_id"`
	ProposalID         uuid.UUID   `json:"proposal_id"`
	AuthorID           uuid.UUID   `json:"author_id"`
	ParticipantUserIDs []uuid.UUID `json:"participant_user_ids"`
}

type votingClosedEvent struct {
	EventID            uuid.UUID   `json:"event_id"`
	SessionID          uuid.UUID   `json:"session_id"`
	RoundID            uuid.UUID   `json:"round_id"`
	WinnerProposalID   *uuid.UUID  `json:"winner_proposal_id"`
	KeptCurrent        bool        `json:"kept_current"`
	ClosedBy           uuid.UUID   `json:"closed_by"`
	ParticipantUserIDs []uuid.UUID `json:"participant_user_ids"`
}

type votingCancelledEvent struct {
	EventID            uuid.UUID   `json:"event_id"`
	SessionID          uuid.UUID   `json:"session_id"`
	RoundID            uuid.UUID   `json:"round_id"`
	Reason             string      `json:"reason"`
	ParticipantUserIDs []uuid.UUID `json:"participant_user_ids"`
}

type votingTiedEvent struct {
	EventID         uuid.UUID   `json:"event_id"`
	SessionID       uuid.UUID   `json:"session_id"`
	RoundID         uuid.UUID   `json:"round_id"`
	TiedProposalIDs []uuid.UUID `json:"tied_proposal_ids"`
	KeepCurrentTied bool        `json:"keep_current_tied"`
	ClosedBy        uuid.UUID   `json:"closed_by"`
	ManagerUserIDs  []uuid.UUID `json:"manager_user_ids"`
}

type teamsResetEvent struct {
	EventID            uuid.UUID   `json:"event_id"`
	SessionID          uuid.UUID   `json:"session_id"`
	Reason             string      `json:"reason"`
	ResetBy            uuid.UUID   `json:"reset_by"`
	ParticipantUserIDs []uuid.UUID `json:"participant_user_ids"`
}

type attendeeTeamChangedEvent struct {
	EventID        uuid.UUID `json:"event_id"`
	SessionID      uuid.UUID `json:"session_id"`
	UserID         uuid.UUID `json:"user_id"`
	TeamID         uuid.UUID `json:"team_id"`
	PreviousTeamID uuid.UUID `json:"previous_team_id"`
	ChangedBy      uuid.UUID `json:"changed_by"`
}
