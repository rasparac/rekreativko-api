package application

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/rasparac/rekreativko-api/notifications/internal/domain"
	"github.com/rasparac/rekreativko-api/notifications/internal/infrastructure/persistence"
	"github.com/rasparac/rekreativko-api/shared/logger"
	"github.com/rasparac/rekreativko-api/shared/telemetry"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// listRepo returns up to filter.Limit of its rows and records the arguments it got.
type listRepo struct {
	persistence.NotificationRepository
	rows       int
	gotFilter  persistence.NotificationFilter
	readBefore time.Time
	allBefore  time.Time
}

func (f *listRepo) ListByRecipient(_ context.Context, filter persistence.NotificationFilter) ([]*domain.Notification, error) {
	f.gotFilter = filter
	out := make([]*domain.Notification, min(f.rows, filter.Limit))
	for i := range out {
		out[i] = domain.New(uuid.New(), filter.RecipientAccountID, domain.NotificationTypeInviteSent, nil)
	}
	return out, nil
}

func (f *listRepo) CountUnread(context.Context, uuid.UUID) (int, error) { return f.rows, nil }

func (f *listRepo) DeleteExpired(_ context.Context, readBefore, allBefore time.Time) (int64, error) {
	f.readBefore, f.allBefore = readBefore, allBefore
	return 3, nil
}

func newListService(repo persistence.NotificationRepository) *NotificationService {
	return &NotificationService{
		logger:           logger.New("error", "json"),
		repo:             repo,
		inactiveAccounts: fakeInactiveAccounts{},
		tracer:           telemetry.Tracer(telemetry.TracerNotificationsService),
	}
}

func TestListNotifications_IsCapped(t *testing.T) {
	for name, tc := range map[string]struct {
		rows, want int
	}{
		"under the cap: everything": {rows: 7, want: 7},
		"at the cap: everything":    {rows: MaxListedNotifications, want: MaxListedNotifications},
		"over the cap: the newest":  {rows: MaxListedNotifications + 50, want: MaxListedNotifications},
	} {
		t.Run(name, func(t *testing.T) {
			repo := &listRepo{rows: tc.rows}
			params := ListNotificationsParams{RecipientAccountID: uuid.New(), UnreadOnly: true}

			got, unread, err := newListService(repo).ListNotifications(context.Background(), params)
			require.NoError(t, err)

			assert.Len(t, got, tc.want)
			// The badge counts every unread notification, listed or not.
			assert.Equal(t, tc.rows, unread)
			assert.Equal(t, params.RecipientAccountID, repo.gotFilter.RecipientAccountID)
			assert.True(t, repo.gotFilter.UnreadOnly)
		})
	}
}

func TestDeleteExpiredNotifications_CutoffsFollowThePolicy(t *testing.T) {
	repo := &listRepo{}
	now := time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC)

	deleted, err := newListService(repo).DeleteExpiredNotifications(
		context.Background(),
		RetentionPolicy{Read: 30 * 24 * time.Hour, All: 90 * 24 * time.Hour},
		now,
	)
	require.NoError(t, err)

	assert.EqualValues(t, 3, deleted)
	assert.Equal(t, now.AddDate(0, 0, -30), repo.readBefore)
	assert.Equal(t, now.AddDate(0, 0, -90), repo.allBefore)
}
