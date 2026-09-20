package events

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/rasparac/rekreativko-api/notifications/internal/application"
	"github.com/rasparac/rekreativko-api/notifications/internal/domain"
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
