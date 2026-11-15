package main

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/rasparac/rekreativko-api/identity/internal/domain"
	"github.com/rasparac/rekreativko-api/shared/domainerror"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseRedeletions(t *testing.T) {
	now := time.Date(2026, 11, 15, 12, 0, 0, 0, time.UTC)
	withTime, withoutTime := uuid.New(), uuid.New()

	t.Run("an id with its deletion time, and one without", func(t *testing.T) {
		got, err := parseRedeletions([]string{
			withTime.String() + "@2026-11-10T15:05:00+01:00",
			withoutTime.String(),
		}, now)

		require.NoError(t, err)
		assert.Equal(t, []redeletion{
			{accountID: withTime, deletedAt: time.Date(2026, 11, 10, 14, 5, 0, 0, time.UTC)},
			{accountID: withoutTime, deletedAt: now},
		}, got)
	})

	for name, args := range map[string][]string{
		"nothing":            nil,
		"not an id":          {"alice"},
		"not RFC 3339":       {withTime.String() + "@2026-11-10"},
		"deleted in future":  {withTime.String() + "@2026-11-16T00:00:00Z"},
		"an account twice":   {withTime.String(), withTime.String() + "@2026-11-10T00:00:00Z"},
		"one bad among good": {withoutTime.String(), "nope"},
	} {
		t.Run("rejects "+name, func(t *testing.T) {
			_, err := parseRedeletions(args, now)
			assert.Error(t, err)
		})
	}
}

type fakeRedeleter struct {
	errs    map[uuid.UUID]error
	deleted map[uuid.UUID]time.Time
}

func (f *fakeRedeleter) RedoAccountDeletion(_ context.Context, accountID uuid.UUID, deletedAt time.Time) error {
	if err := f.errs[accountID]; err != nil {
		return err
	}
	f.deleted[accountID] = deletedAt
	return nil
}

func TestRedeleteAll(t *testing.T) {
	asked := time.Date(2026, 11, 10, 14, 5, 0, 0, time.UTC)
	active, deleted, broken := uuid.New(), uuid.New(), uuid.New()
	svc := &fakeRedeleter{
		deleted: map[uuid.UUID]time.Time{},
		errs: map[uuid.UUID]error{
			// the service returns mapped errors; the sentinel is still inside
			deleted: domainerror.Forbidden("identity_account_deleted", "Account is deleted", domain.ErrAccountDeleted),
			broken:  errors.New("connection reset"),
		},
	}
	var out bytes.Buffer

	err := redeleteAll(context.Background(), svc, []redeletion{
		{accountID: active, deletedAt: asked},
		{accountID: deleted, deletedAt: asked},
		{accountID: broken, deletedAt: asked},
	}, &out)

	assert.EqualError(t, err, "1 of 3 accounts not deleted", "an account already deleted is not a failure")
	assert.Equal(t, map[uuid.UUID]time.Time{active: asked}, svc.deleted)
	assert.Contains(t, out.String(), "deleted  "+active.String()+"  as of 2026-11-10T14:05:00Z")
	assert.Contains(t, out.String(), "skipped  "+deleted.String())
	assert.Contains(t, out.String(), "FAILED   "+broken.String())
}
