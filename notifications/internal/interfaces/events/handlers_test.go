package events

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/rasparac/rekreativko-api/notifications/internal/application"
	"github.com/rasparac/rekreativko-api/notifications/internal/domain"
	"github.com/rasparac/rekreativko-api/shared/events"
	"github.com/rasparac/rekreativko-api/shared/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type recordingCreator struct {
	calls []application.CreateNotificationParams
}

func (r *recordingCreator) CreateNotification(_ context.Context, p application.CreateNotificationParams) (*domain.Notification, error) {
	r.calls = append(r.calls, p)
	return nil, nil
}

func TestMemberJoinRequestedHandler_PassesEventIDToEveryRecipient(t *testing.T) {
	creator := &recordingCreator{}
	h := &memberJoinRequestedHandler{notifications: creator, logger: logger.New("error", "json")}

	event := memberJoinRequestedEvent{
		EventID:        uuid.New(),
		UserID:         uuid.New(),
		ManagerUserIDs: []uuid.UUID{uuid.New(), uuid.New()},
	}
	payload, err := json.Marshal(event)
	require.NoError(t, err)

	require.NoError(t, h.Handle(context.Background(), payload))

	require.Len(t, creator.calls, 2)
	for i, call := range creator.calls {
		assert.Equal(t, event.EventID, call.EventID, "call %d must carry the event id", i)
		assert.Equal(t, event.ManagerUserIDs[i], call.RecipientAccountID)
	}
}

func TestSingleRecipientHandler_PassesEventID(t *testing.T) {
	creator := &recordingCreator{}
	h := &inviteSentHandler{notifications: creator, logger: logger.New("error", "json")}

	event := inviteSentEvent{EventID: uuid.New(), InvitedUserID: uuid.New()}
	payload, err := json.Marshal(event)
	require.NoError(t, err)

	require.NoError(t, h.Handle(context.Background(), payload))

	require.Len(t, creator.calls, 1)
	assert.Equal(t, event.EventID, creator.calls[0].EventID)
}

// sessionInvitePayload builds the wire shape activity publishes for
// activity.session_invite.* - BaseEvent fields plus the invite fields - so the
// tests catch a drift between the upstream JSON tags and our local copies.
func sessionInvitePayload(t *testing.T, eventType string, eventID, sessionID, invitedBy, invitedUserID uuid.UUID, expiresAt time.Time) []byte {
	t.Helper()

	payload, err := json.Marshal(map[string]any{
		"event_id":        eventID,
		"event_type":      eventType,
		"occurred_at":     time.Now().UTC(),
		"aggregate_id":    uuid.New(),
		"session_id":      sessionID,
		"invited_by":      invitedBy,
		"invited_user_id": invitedUserID,
		"expires_at":      expiresAt,
	})
	require.NoError(t, err)

	return payload
}

func TestSessionInviteSentHandler_NotifiesInvitee(t *testing.T) {
	creator := &recordingCreator{}
	h := &sessionInviteSentHandler{notifications: creator, logger: logger.New("error", "json")}

	eventID, sessionID, invitedBy, invitee := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	expiresAt := time.Now().Add(7 * 24 * time.Hour).UTC().Truncate(time.Second)
	payload := sessionInvitePayload(t, "activity.session_invite.sent", eventID, sessionID, invitedBy, invitee, expiresAt)

	require.NoError(t, h.Handle(context.Background(), payload))

	require.Len(t, creator.calls, 1)
	call := creator.calls[0]
	assert.Equal(t, eventID, call.EventID)
	assert.Equal(t, invitee, call.RecipientAccountID)
	assert.Equal(t, string(domain.NotificationTypeSessionInviteSent), call.Type)
	assert.Equal(t, sessionID, call.Data["session_id"])
	assert.Equal(t, invitedBy, call.Data["invited_by"])
	assert.True(t, expiresAt.Equal(call.Data["expires_at"].(time.Time)))
	assert.NotContains(t, call.Data, "group_id")
}

func TestSessionInviteResponseHandlers_NotifyInviter(t *testing.T) {
	log := logger.New("error", "json")

	tests := []struct {
		eventType string
		newHandle func(notificationCreator) func(context.Context, []byte) error
		wantType  domain.NotificationType
	}{
		{
			eventType: "activity.session_invite.accepted",
			newHandle: func(c notificationCreator) func(context.Context, []byte) error {
				return (&sessionInviteAcceptedHandler{notifications: c, logger: log}).Handle
			},
			wantType: domain.NotificationTypeSessionInviteAccepted,
		},
		{
			eventType: "activity.session_invite.declined",
			newHandle: func(c notificationCreator) func(context.Context, []byte) error {
				return (&sessionInviteDeclinedHandler{notifications: c, logger: log}).Handle
			},
			wantType: domain.NotificationTypeSessionInviteDeclined,
		},
		{
			eventType: "activity.session_invite.expired",
			newHandle: func(c notificationCreator) func(context.Context, []byte) error {
				return (&sessionInviteExpiredHandler{notifications: c, logger: log}).Handle
			},
			wantType: domain.NotificationTypeSessionInviteExpired,
		},
	}

	for _, tt := range tests {
		t.Run(tt.eventType, func(t *testing.T) {
			creator := &recordingCreator{}
			handle := tt.newHandle(creator)

			eventID, sessionID, invitedBy, invitee := uuid.New(), uuid.New(), uuid.New(), uuid.New()
			payload := sessionInvitePayload(t, tt.eventType, eventID, sessionID, invitedBy, invitee, time.Time{})

			require.NoError(t, handle(context.Background(), payload))

			require.Len(t, creator.calls, 1)
			call := creator.calls[0]
			assert.Equal(t, eventID, call.EventID)
			assert.Equal(t, invitedBy, call.RecipientAccountID)
			assert.Equal(t, string(tt.wantType), call.Type)
			assert.Equal(t, sessionID, call.Data["session_id"])
			assert.Equal(t, invitee, call.Data["invited_user_id"])
		})

		t.Run(tt.eventType+"/no inviter is a no-op", func(t *testing.T) {
			creator := &recordingCreator{}
			handle := tt.newHandle(creator)

			payload := sessionInvitePayload(t, tt.eventType, uuid.New(), uuid.New(), uuid.Nil, uuid.New(), time.Time{})

			require.NoError(t, handle(context.Background(), payload))
			assert.Empty(t, creator.calls)
		})
	}
}

func TestSessionInviteHandlers_MalformedPayloadIsPermanent(t *testing.T) {
	creator := &recordingCreator{}
	log := logger.New("error", "json")

	handlers := map[string]func(context.Context, []byte) error{
		"sent":     (&sessionInviteSentHandler{notifications: creator, logger: log}).Handle,
		"accepted": (&sessionInviteAcceptedHandler{notifications: creator, logger: log}).Handle,
		"declined": (&sessionInviteDeclinedHandler{notifications: creator, logger: log}).Handle,
		"expired":  (&sessionInviteExpiredHandler{notifications: creator, logger: log}).Handle,
	}

	for name, handle := range handlers {
		t.Run(name, func(t *testing.T) {
			err := handle(context.Background(), []byte(`{"session_id":`))
			require.Error(t, err)
			assert.ErrorIs(t, err, events.ErrPermanent, "a bad payload must not be retried")
		})
	}
	assert.Empty(t, creator.calls)
}
