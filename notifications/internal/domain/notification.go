package domain

import (
	"time"

	"github.com/google/uuid"
)

// NotificationType identifies what kind of event produced a notification.
// The set is intentionally open (just a string) rather than a closed enum -
// new event consumers can introduce new types without a migration.
type NotificationType string

const (
	NotificationTypeJoinRequestCreated  NotificationType = "join_request_created"
	NotificationTypeJoinRequestApproved NotificationType = "join_request_approved"
	NotificationTypeJoinRequestRejected NotificationType = "join_request_rejected"
	NotificationTypeAttendeePromoted    NotificationType = "attendee_promoted"
	NotificationTypeInviteSent          NotificationType = "invite_sent"
	NotificationTypeInviteAccepted      NotificationType = "invite_accepted"
	NotificationTypeInviteDeclined      NotificationType = "invite_declined"
	NotificationTypeInviteExpired       NotificationType = "invite_expired"
	// Session join requests - kept distinct from the group join_request_*
	// types above since the Data payload carries session_id, not group_id.
	NotificationTypeSessionJoinRequestCreated  NotificationType = "session_join_request_created"
	NotificationTypeSessionJoinRequestApproved NotificationType = "session_join_request_approved"
	NotificationTypeSessionJoinRequestRejected NotificationType = "session_join_request_rejected"
	NotificationTypeSessionAttendeeRemoved     NotificationType = "session_attendee_removed"
	NotificationTypeMemberRemoved              NotificationType = "member_removed"
	NotificationTypeSessionCancelled           NotificationType = "session_cancelled"
)

// Notification is a single item in a recipient's notification feed. Data
// carries whatever fields that notification type's consumer decided the
// client needs (group_id, session_id, actor id, etc.) - kept as a free-form
// map rather than per-type columns so new notification types don't need a
// migration.
type Notification struct {
	id                uuid.UUID
	recipientAccontID uuid.UUID
	notificationType  NotificationType
	data              map[string]any
	readAt            *time.Time
	createdAt         time.Time
}

// New creates a new, unread Notification.
func New(
	recipientAccountID uuid.UUID,
	notificationType NotificationType,
	data map[string]any,
) *Notification {
	return &Notification{
		id:                uuid.New(),
		recipientAccontID: recipientAccountID,
		notificationType:  notificationType,
		data:              data,
		createdAt:         time.Now().UTC(),
	}
}

// Reconstruct rebuilds a Notification from persisted data.
func Reconstruct(
	id uuid.UUID,
	recipientAccountID uuid.UUID,
	notificationType NotificationType,
	data map[string]any,
	readAt *time.Time,
	createdAt time.Time,
) *Notification {
	return &Notification{
		id:                id,
		recipientAccontID: recipientAccountID,
		notificationType:  notificationType,
		data:              data,
		readAt:            readAt,
		createdAt:         createdAt,
	}
}

func (n *Notification) ID() uuid.UUID {
	return n.id
}

func (n *Notification) RecipientAccountID() uuid.UUID {
	return n.recipientAccontID
}

func (n *Notification) Type() NotificationType {
	return n.notificationType
}

func (n *Notification) Data() map[string]any {
	return n.data
}

func (n *Notification) ReadAt() *time.Time {
	return n.readAt
}

func (n *Notification) IsRead() bool {
	return n.readAt != nil
}

func (n *Notification) CreatedAt() time.Time {
	return n.createdAt
}

// MarkRead marks the notification as read, if it isn't already. Idempotent -
// re-marking an already-read notification is a no-op, not an error.
func (n *Notification) MarkRead() {
	if n.readAt != nil {
		return
	}
	now := time.Now().UTC()
	n.readAt = &now
}
