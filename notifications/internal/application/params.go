package application

import "github.com/google/uuid"

// CreateNotificationParams contains parameters for creating a single notification.
// Fan-out (one event -> many recipients) is the event handler's job, not this
// service's - each call here creates exactly one row for exactly one recipient.
type CreateNotificationParams struct {
	RecipientAccountID uuid.UUID
	Type               string
	Data               map[string]any
}

// ListNotificationsParams contains parameters for listing a recipient's notifications
type ListNotificationsParams struct {
	RecipientAccountID uuid.UUID
	UnreadOnly         bool
	Limit              int
	PageToken          string
}

// MarkAsReadParams contains parameters for marking a notification as read
type MarkAsReadParams struct {
	NotificationID uuid.UUID
	RequesterID    uuid.UUID
}
