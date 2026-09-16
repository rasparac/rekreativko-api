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
