package events

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/rasparac/rekreativko-api/notifications/internal/application"
	"github.com/rasparac/rekreativko-api/notifications/internal/domain"
	"github.com/rasparac/rekreativko-api/shared/api"
	"github.com/rasparac/rekreativko-api/shared/logger"
)

// notificationCreator is the subset of NotificationService every handler needs.
type notificationCreator interface {
	CreateNotification(ctx context.Context, params application.CreateNotificationParams) (*domain.Notification, error)
}

type memberJoinRequestedHandler struct {
	notifications notificationCreator
	logger        *logger.Logger
}

func (h *memberJoinRequestedHandler) Handle(ctx context.Context, payload []byte) error {
	var event memberJoinRequestedEvent
	if err := json.Unmarshal(payload, &event); err != nil {
		return fmt.Errorf("decode payload: %w", err)
	}
	ctx = api.WithEventID(ctx, event.EventID.String())

	// Fan-out: every manager of the group gets their own notification row.
	for _, managerID := range event.ManagerUserIDs {
		_, err := h.notifications.CreateNotification(ctx, application.CreateNotificationParams{
			RecipientAccountID: managerID,
			Type:               string(domain.NotificationTypeJoinRequestCreated),
			Data: map[string]any{
				"group_id": event.ActivityGroupID,
				"user_id":  event.UserID,
			},
		})
		if err != nil {
			h.logger.Error(ctx, "failed to create join_request_created notification",
				"error", err, "recipient_account_id", managerID)
			return err
		}
	}

	return nil
}

type memberApprovedHandler struct {
	notifications notificationCreator
	logger        *logger.Logger
}

func (h *memberApprovedHandler) Handle(ctx context.Context, payload []byte) error {
	var event memberApprovedEvent
	if err := json.Unmarshal(payload, &event); err != nil {
		return fmt.Errorf("decode payload: %w", err)
	}
	ctx = api.WithEventID(ctx, event.EventID.String())

	_, err := h.notifications.CreateNotification(ctx, application.CreateNotificationParams{
		RecipientAccountID: event.UserID,
		Type:               string(domain.NotificationTypeJoinRequestApproved),
		Data: map[string]any{
			"group_id":    event.ActivityID,
			"approved_by": event.ApprovedBy,
		},
	})
	if err != nil {
		h.logger.Error(ctx, "failed to create join_request_approved notification", "error", err)
	}
	return err
}

type memberRejectedHandler struct {
	notifications notificationCreator
	logger        *logger.Logger
}

func (h *memberRejectedHandler) Handle(ctx context.Context, payload []byte) error {
	var event memberRejectedEvent
	if err := json.Unmarshal(payload, &event); err != nil {
		return fmt.Errorf("decode payload: %w", err)
	}
	ctx = api.WithEventID(ctx, event.EventID.String())

	_, err := h.notifications.CreateNotification(ctx, application.CreateNotificationParams{
		RecipientAccountID: event.UserID,
		Type:               string(domain.NotificationTypeJoinRequestRejected),
		Data: map[string]any{
			"group_id":    event.ActivityID,
			"rejected_by": event.RejectedBy,
		},
	})
	if err != nil {
		h.logger.Error(ctx, "failed to create join_request_rejected notification", "error", err)
	}
	return err
}

type attendeePromotedHandler struct {
	notifications notificationCreator
	logger        *logger.Logger
}

func (h *attendeePromotedHandler) Handle(ctx context.Context, payload []byte) error {
	var event attendeePromotedEvent
	if err := json.Unmarshal(payload, &event); err != nil {
		return fmt.Errorf("decode payload: %w", err)
	}
	ctx = api.WithEventID(ctx, event.EventID.String())

	// UserID is empty for events published before AttendeePromotedEvent
	// carried a payload - skip rather than notify uuid.Nil.
	if event.UserID == uuid.Nil {
		return nil
	}

	_, err := h.notifications.CreateNotification(ctx, application.CreateNotificationParams{
		RecipientAccountID: event.UserID,
		Type:               string(domain.NotificationTypeAttendeePromoted),
		Data: map[string]any{
			"session_id": event.SessionID,
		},
	})
	if err != nil {
		h.logger.Error(ctx, "failed to create attendee_promoted notification", "error", err)
	}
	return err
}

type attendeeJoinRequestedHandler struct {
	notifications notificationCreator
	logger        *logger.Logger
}

func (h *attendeeJoinRequestedHandler) Handle(ctx context.Context, payload []byte) error {
	var event attendeeJoinRequestedEvent
	if err := json.Unmarshal(payload, &event); err != nil {
		return fmt.Errorf("decode payload: %w", err)
	}
	ctx = api.WithEventID(ctx, event.EventID.String())

	// Fan-out: every manager of the session (or just the creator, for a
	// standalone session) gets their own notification row.
	for _, managerID := range event.ManagerUserIDs {
		_, err := h.notifications.CreateNotification(ctx, application.CreateNotificationParams{
			RecipientAccountID: managerID,
			Type:               string(domain.NotificationTypeSessionJoinRequestCreated),
			Data: map[string]any{
				"session_id": event.SessionID,
				"user_id":    event.UserID,
			},
		})
		if err != nil {
			h.logger.Error(ctx, "failed to create session_join_request_created notification",
				"error", err, "recipient_account_id", managerID)
			return err
		}
	}

	return nil
}

type attendeeJoinApprovedHandler struct {
	notifications notificationCreator
	logger        *logger.Logger
}

func (h *attendeeJoinApprovedHandler) Handle(ctx context.Context, payload []byte) error {
	var event attendeeJoinApprovedEvent
	if err := json.Unmarshal(payload, &event); err != nil {
		return fmt.Errorf("decode payload: %w", err)
	}
	ctx = api.WithEventID(ctx, event.EventID.String())

	_, err := h.notifications.CreateNotification(ctx, application.CreateNotificationParams{
		RecipientAccountID: event.UserID,
		Type:               string(domain.NotificationTypeSessionJoinRequestApproved),
		Data: map[string]any{
			"session_id":  event.SessionID,
			"approved_by": event.ApprovedBy,
		},
	})
	if err != nil {
		h.logger.Error(ctx, "failed to create session_join_request_approved notification", "error", err)
	}
	return err
}

type attendeeJoinRejectedHandler struct {
	notifications notificationCreator
	logger        *logger.Logger
}

func (h *attendeeJoinRejectedHandler) Handle(ctx context.Context, payload []byte) error {
	var event attendeeJoinRejectedEvent
	if err := json.Unmarshal(payload, &event); err != nil {
		return fmt.Errorf("decode payload: %w", err)
	}
	ctx = api.WithEventID(ctx, event.EventID.String())

	_, err := h.notifications.CreateNotification(ctx, application.CreateNotificationParams{
		RecipientAccountID: event.UserID,
		Type:               string(domain.NotificationTypeSessionJoinRequestRejected),
		Data: map[string]any{
			"session_id":  event.SessionID,
			"rejected_by": event.RejectedBy,
		},
	})
	if err != nil {
		h.logger.Error(ctx, "failed to create session_join_request_rejected notification", "error", err)
	}
	return err
}

type attendeeRemovedHandler struct {
	notifications notificationCreator
	logger        *logger.Logger
}

func (h *attendeeRemovedHandler) Handle(ctx context.Context, payload []byte) error {
	var event attendeeRemovedEvent
	if err := json.Unmarshal(payload, &event); err != nil {
		return fmt.Errorf("decode payload: %w", err)
	}
	ctx = api.WithEventID(ctx, event.EventID.String())

	_, err := h.notifications.CreateNotification(ctx, application.CreateNotificationParams{
		RecipientAccountID: event.UserID,
		Type:               string(domain.NotificationTypeSessionAttendeeRemoved),
		Data: map[string]any{
			"session_id": event.SessionID,
			"removed_by": event.RemovedBy,
		},
	})
	if err != nil {
		h.logger.Error(ctx, "failed to create session_attendee_removed notification", "error", err)
	}
	return err
}

type memberRemovedHandler struct {
	notifications notificationCreator
	logger        *logger.Logger
}

func (h *memberRemovedHandler) Handle(ctx context.Context, payload []byte) error {
	var event memberRemovedEvent
	if err := json.Unmarshal(payload, &event); err != nil {
		return fmt.Errorf("decode payload: %w", err)
	}
	ctx = api.WithEventID(ctx, event.EventID.String())

	_, err := h.notifications.CreateNotification(ctx, application.CreateNotificationParams{
		RecipientAccountID: event.UserID,
		Type:               string(domain.NotificationTypeMemberRemoved),
		Data: map[string]any{
			"group_id":   event.ActivityID,
			"removed_by": event.RemovedBy,
		},
	})
	if err != nil {
		h.logger.Error(ctx, "failed to create member_removed notification", "error", err)
	}
	return err
}

type sessionCancelledHandler struct {
	notifications notificationCreator
	logger        *logger.Logger
}

func (h *sessionCancelledHandler) Handle(ctx context.Context, payload []byte) error {
	var event sessionCancelledEvent
	if err := json.Unmarshal(payload, &event); err != nil {
		return fmt.Errorf("decode payload: %w", err)
	}
	ctx = api.WithEventID(ctx, event.EventID.String())

	// Fan-out: every attendee still interested in the session gets their own
	// notification row.
	for _, userID := range event.AttendeeUserIDs {
		_, err := h.notifications.CreateNotification(ctx, application.CreateNotificationParams{
			RecipientAccountID: userID,
			Type:               string(domain.NotificationTypeSessionCancelled),
			Data: map[string]any{
				"session_id":   event.SessionID,
				"cancelled_by": event.CancelledBy,
				"reason":       event.CancellationReason,
			},
		})
		if err != nil {
			h.logger.Error(ctx, "failed to create session_cancelled notification",
				"error", err, "recipient_account_id", userID)
			return err
		}
	}

	return nil
}

type inviteSentHandler struct {
	notifications notificationCreator
	logger        *logger.Logger
}

func (h *inviteSentHandler) Handle(ctx context.Context, payload []byte) error {
	var event inviteSentEvent
	if err := json.Unmarshal(payload, &event); err != nil {
		return fmt.Errorf("decode payload: %w", err)
	}
	ctx = api.WithEventID(ctx, event.EventID.String())

	_, err := h.notifications.CreateNotification(ctx, application.CreateNotificationParams{
		RecipientAccountID: event.InvitedUserID,
		Type:               string(domain.NotificationTypeInviteSent),
		Data: map[string]any{
			"group_id":   event.ActivityID,
			"invited_by": event.InvitedBy,
			"expires_at": event.ExpiresAt,
		},
	})
	if err != nil {
		h.logger.Error(ctx, "failed to create invite_sent notification", "error", err)
	}
	return err
}

type inviteAcceptedHandler struct {
	notifications notificationCreator
	logger        *logger.Logger
}

func (h *inviteAcceptedHandler) Handle(ctx context.Context, payload []byte) error {
	var event inviteAcceptedEvent
	if err := json.Unmarshal(payload, &event); err != nil {
		return fmt.Errorf("decode payload: %w", err)
	}
	ctx = api.WithEventID(ctx, event.EventID.String())

	if event.InvitedBy == uuid.Nil {
		return nil
	}

	_, err := h.notifications.CreateNotification(ctx, application.CreateNotificationParams{
		RecipientAccountID: event.InvitedBy,
		Type:               string(domain.NotificationTypeInviteAccepted),
		Data: map[string]any{
			"group_id":        event.ActivityID,
			"invited_user_id": event.InvitedUserID,
		},
	})
	if err != nil {
		h.logger.Error(ctx, "failed to create invite_accepted notification", "error", err)
	}
	return err
}

type inviteDeclinedHandler struct {
	notifications notificationCreator
	logger        *logger.Logger
}

func (h *inviteDeclinedHandler) Handle(ctx context.Context, payload []byte) error {
	var event inviteDeclinedEvent
	if err := json.Unmarshal(payload, &event); err != nil {
		return fmt.Errorf("decode payload: %w", err)
	}
	ctx = api.WithEventID(ctx, event.EventID.String())

	if event.InvitedBy == uuid.Nil {
		return nil
	}

	_, err := h.notifications.CreateNotification(ctx, application.CreateNotificationParams{
		RecipientAccountID: event.InvitedBy,
		Type:               string(domain.NotificationTypeInviteDeclined),
		Data: map[string]any{
			"group_id":        event.ActivityID,
			"invited_user_id": event.InvitedUserID,
		},
	})
	if err != nil {
		h.logger.Error(ctx, "failed to create invite_declined notification", "error", err)
	}
	return err
}

type inviteExpiredHandler struct {
	notifications notificationCreator
	logger        *logger.Logger
}

func (h *inviteExpiredHandler) Handle(ctx context.Context, payload []byte) error {
	var event inviteExpiredEvent
	if err := json.Unmarshal(payload, &event); err != nil {
		return fmt.Errorf("decode payload: %w", err)
	}
	ctx = api.WithEventID(ctx, event.EventID.String())

	if event.InvitedBy == uuid.Nil {
		return nil
	}

	_, err := h.notifications.CreateNotification(ctx, application.CreateNotificationParams{
		RecipientAccountID: event.InvitedBy,
		Type:               string(domain.NotificationTypeInviteExpired),
		Data: map[string]any{
			"group_id":        event.ActivityID,
			"invited_user_id": event.InvitedUserID,
		},
	})
	if err != nil {
		h.logger.Error(ctx, "failed to create invite_expired notification", "error", err)
	}
	return err
}
