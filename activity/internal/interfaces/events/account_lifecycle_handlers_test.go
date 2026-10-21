package events

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/rasparac/rekreativko-api/shared/events"
	"github.com/rasparac/rekreativko-api/shared/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeAccountLifecycle struct {
	deactivated, erased []uuid.UUID
	err                 error
}

func (f *fakeAccountLifecycle) DeactivateAccount(_ context.Context, accountID uuid.UUID) error {
	f.deactivated = append(f.deactivated, accountID)
	return f.err
}

func (f *fakeAccountLifecycle) EraseAccount(_ context.Context, accountID uuid.UUID) error {
	f.erased = append(f.erased, accountID)
	return f.err
}

func TestAccountLifecycleHandlers_DeletedDeactivatesTheAggregate(t *testing.T) {
	accounts := &fakeAccountLifecycle{}
	h := NewAccountLifecycleHandlers(accounts, logger.New("error", "json"))
	accountID := uuid.New()

	// identity.account.deleted carries the account as its aggregate id.
	err := h.HandleAccountDeleted(context.Background(),
		[]byte(`{"event_id":"`+uuid.NewString()+`","aggregate_id":"`+accountID.String()+`"}`))

	require.NoError(t, err)
	assert.Equal(t, []uuid.UUID{accountID}, accounts.deactivated)
}

func TestAccountLifecycleHandlers_ErasedErasesTheAccount(t *testing.T) {
	accounts := &fakeAccountLifecycle{}
	h := NewAccountLifecycleHandlers(accounts, logger.New("error", "json"))
	accountID := uuid.New()

	err := h.HandleAccountErased(context.Background(),
		[]byte(`{"event_id":"`+uuid.NewString()+`","account_id":"`+accountID.String()+`"}`))

	require.NoError(t, err)
	assert.Equal(t, []uuid.UUID{accountID}, accounts.erased)
}

func TestAccountLifecycleHandlers_BadEventsArePermanent(t *testing.T) {
	h := NewAccountLifecycleHandlers(&fakeAccountLifecycle{}, logger.New("error", "json"))

	for name, handle := range map[string]func(context.Context, []byte) error{
		"deleted": h.HandleAccountDeleted,
		"erased":  h.HandleAccountErased,
	} {
		for kind, payload := range map[string]string{
			"not json":      `{`,
			"no account id": `{"event_id":"` + uuid.NewString() + `"}`,
		} {
			assert.ErrorIs(t, handle(context.Background(), []byte(payload)), events.ErrPermanent, name+"/"+kind)
		}
	}
}

func TestAccountLifecycleHandlers_OtherFailuresAreRetried(t *testing.T) {
	h := NewAccountLifecycleHandlers(&fakeAccountLifecycle{err: errors.New("connection reset")}, logger.New("error", "json"))

	err := h.HandleAccountDeleted(context.Background(), []byte(`{"aggregate_id":"`+uuid.NewString()+`"}`))

	require.Error(t, err)
	assert.NotErrorIs(t, err, events.ErrPermanent)
}
