package application

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/rasparac/rekreativko-api/notifications/internal/domain"
	"github.com/rasparac/rekreativko-api/notifications/internal/infrastructure/persistence"
	"github.com/rasparac/rekreativko-api/notifications/internal/metrics"
	"github.com/rasparac/rekreativko-api/shared/logger"
	"github.com/rasparac/rekreativko-api/shared/telemetry"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeRepo embeds the interface so only Create needs implementing here.
type fakeRepo struct {
	persistence.NotificationRepository
	seen map[[2]uuid.UUID]bool
}

func (f *fakeRepo) Create(_ context.Context, n *domain.Notification) (bool, error) {
	key := [2]uuid.UUID{n.EventID(), n.RecipientAccountID()}
	if f.seen[key] {
		return false, nil
	}
	f.seen[key] = true
	return true, nil
}

func TestCreateNotification_RedeliveredEventDoesNotCountTwice(t *testing.T) {
	m := &metrics.Metrics{
		NotificationCreated: prometheus.NewCounter(prometheus.CounterOpts{Name: "created"}),
	}
	reg := prometheus.NewRegistry()
	reg.MustRegister(m.NotificationCreated)
	svc := &NotificationService{
		logger:  logger.New("error", "json"),
		repo:    &fakeRepo{seen: map[[2]uuid.UUID]bool{}},
		tracer:  telemetry.Tracer(telemetry.TracerNotificationsService),
		metrics: m,
	}

	params := CreateNotificationParams{
		EventID:            uuid.New(),
		RecipientAccountID: uuid.New(),
		Type:               string(domain.NotificationTypeInviteSent),
	}

	for range 2 {
		n, err := svc.CreateNotification(context.Background(), params)
		require.NoError(t, err)
		assert.Equal(t, params.EventID, n.EventID())
	}

	families, err := reg.Gather()
	require.NoError(t, err)
	require.Len(t, families, 1)
	assert.Equal(t, 1.0, families[0].GetMetric()[0].GetCounter().GetValue())
}
