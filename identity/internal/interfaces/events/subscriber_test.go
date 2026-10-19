package events

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rasparac/rekreativko-api/shared/events"
	"github.com/rasparac/rekreativko-api/shared/logger"
)

type fakeBroker struct {
	handlers map[string]events.MessageHandler
}

func (f *fakeBroker) Publish(context.Context, string, []byte) error { return nil }

func (f *fakeBroker) Subscribe(_ context.Context, topic string, handler events.MessageHandler) error {
	f.handlers[topic] = handler
	return nil
}

func (f *fakeBroker) Close(context.Context) error { return nil }

type recorded struct {
	accountID uuid.UUID
	context   string
}

type fakeRecorder struct {
	calls []recorded
	err   error
}

func (f *fakeRecorder) RecordContextErasure(_ context.Context, accountID uuid.UUID, context string) error {
	f.calls = append(f.calls, recorded{accountID, context})
	return f.err
}

func subscribe(t *testing.T, recorder *fakeRecorder) *fakeBroker {
	t.Helper()

	broker := &fakeBroker{handlers: map[string]events.MessageHandler{}}
	subscriber := NewSubscriber(broker, recorder, []string{"activity", "notifications"}, logger.New("error", "json"))
	require.NoError(t, subscriber.Subscribe(context.Background()))
	return broker
}

func TestSubscriber_SubscribesToEveryContextsConfirmation(t *testing.T) {
	broker := subscribe(t, &fakeRecorder{})

	assert.Len(t, broker.handlers, 2)
	assert.Contains(t, broker.handlers, "activity.account.erased")
	assert.Contains(t, broker.handlers, "notifications.account.erased")
}

func TestSubscriber_RecordsTheConfirmationUnderItsContext(t *testing.T) {
	recorder := &fakeRecorder{}
	broker := subscribe(t, recorder)
	accountID := uuid.New()

	err := broker.handlers["notifications.account.erased"](context.Background(),
		[]byte(`{"event_id":"`+uuid.NewString()+`","account_id":"`+accountID.String()+`"}`))

	require.NoError(t, err)
	assert.Equal(t, []recorded{{accountID, "notifications"}}, recorder.calls)
}

func TestSubscriber_RejectsBadEventsPermanently(t *testing.T) {
	for name, payload := range map[string]string{
		"not json":           `{`,
		"missing account_id": `{"event_id":"` + uuid.NewString() + `"}`,
	} {
		recorder := &fakeRecorder{}
		broker := subscribe(t, recorder)

		err := broker.handlers["activity.account.erased"](context.Background(), []byte(payload))

		assert.ErrorIs(t, err, events.ErrPermanent, name)
		assert.Empty(t, recorder.calls, name)
	}
}

func TestSubscriber_UnknownAccountIsPermanent(t *testing.T) {
	recorder := &fakeRecorder{err: &pgconn.PgError{Code: pgerrcode.ForeignKeyViolation}}
	broker := subscribe(t, recorder)

	err := broker.handlers["activity.account.erased"](context.Background(),
		[]byte(`{"account_id":"`+uuid.NewString()+`"}`))

	assert.ErrorIs(t, err, events.ErrPermanent)
}

func TestSubscriber_OtherFailuresAreRetried(t *testing.T) {
	recorder := &fakeRecorder{err: errors.New("connection reset")}
	broker := subscribe(t, recorder)

	err := broker.handlers["activity.account.erased"](context.Background(),
		[]byte(`{"account_id":"`+uuid.NewString()+`"}`))

	require.Error(t, err)
	assert.NotErrorIs(t, err, events.ErrPermanent)
}
