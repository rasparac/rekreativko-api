package events

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rasparac/rekreativko-api/shared/domainevent"
	"github.com/rasparac/rekreativko-api/shared/logger"
)

type fakeReader struct {
	events       []domainevent.BrokerEvent
	published    []uuid.UUID
	failed       map[uuid.UUID]int
	retries      map[uuid.UUID]int
	markPubErr   error
	readCalls    int
	maxRetriesIn int
}

func (f *fakeReader) ReadEvents(_ context.Context, _ string, _ int) ([]domainevent.BrokerEvent, error) {
	f.readCalls++
	return f.events, nil
}

func (f *fakeReader) MarkEventAsPublished(_ context.Context, _ string, id uuid.UUID) error {
	if f.markPubErr != nil {
		return f.markPubErr
	}
	f.published = append(f.published, id)
	return nil
}

func (f *fakeReader) MarkEventAsFailed(_ context.Context, _ string, id uuid.UUID, _ error, maxRetries int) (bool, error) {
	f.maxRetriesIn = maxRetries
	if f.retries == nil {
		f.retries = map[uuid.UUID]int{}
	}
	f.retries[id]++
	return f.retries[id] >= maxRetries, nil
}

type fakeBroker struct {
	failFor map[string]bool
	sent    []string
}

func (b *fakeBroker) Publish(_ context.Context, topic string, _ []byte) error {
	if b.failFor[topic] {
		return errors.New("broker rejected payload")
	}
	b.sent = append(b.sent, topic)
	return nil
}

func (b *fakeBroker) Subscribe(context.Context, string, MessageHandler) error { return nil }

func (b *fakeBroker) Close(context.Context) error { return nil }

type fakeTx struct {
	calls int
}

func (f *fakeTx) WithTransaction(ctx context.Context, fn func(ctx context.Context) error) error {
	f.calls++
	return fn(ctx)
}

func newTestMetrics() *Metrics {
	return &Metrics{
		EventsPublishedTotal: prometheus.NewCounterVec(prometheus.CounterOpts{Name: "p"}, []string{"event_type", "schema"}),
		EventPublishDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{Name: "d"}, []string{"event_type", "schema"}),
		EventProcessedTotal:  prometheus.NewCounterVec(prometheus.CounterOpts{Name: "e"}, []string{"event_type", "schema", "status"}),
	}
}

func newTestPublisher(r *fakeReader, b *fakeBroker, tx *fakeTx, maxRetries int) *outboxPublisher {
	return NewOutboxPublisher(r, tx, b, logger.New("error", "json"), 10, maxRetries, time.Second, newTestMetrics(), []string{"identity"})
}

func TestPublishFromSchema_PublishesAndMarksEvents(t *testing.T) {
	a, b := uuid.New(), uuid.New()
	r := &fakeReader{events: []domainevent.BrokerEvent{
		{EventID: a, EventType: "identity.account.verified"},
		{EventID: b, EventType: "identity.account.locked"},
	}}
	br := &fakeBroker{}
	tx := &fakeTx{}

	failed, published, err := newTestPublisher(r, br, tx, 3).publishFromSchema(context.Background(), "identity")

	require.NoError(t, err)
	assert.Equal(t, 0, failed)
	assert.Equal(t, 2, published)
	assert.Equal(t, []uuid.UUID{a, b}, r.published)
	assert.Equal(t, 1, tx.calls, "batch must run in a single transaction")
}

func TestPublishFromSchema_FailedEventDoesNotBlockLaterEvents(t *testing.T) {
	poison, ok := uuid.New(), uuid.New()
	r := &fakeReader{events: []domainevent.BrokerEvent{
		{EventID: poison, EventType: "bad.event"},
		{EventID: ok, EventType: "good.event"},
	}}
	br := &fakeBroker{failFor: map[string]bool{"bad.event": true}}

	failed, published, err := newTestPublisher(r, br, &fakeTx{}, 3).publishFromSchema(context.Background(), "identity")

	require.NoError(t, err)
	assert.Equal(t, 1, failed)
	assert.Equal(t, 1, published)
	assert.Equal(t, []uuid.UUID{ok}, r.published)
	assert.Equal(t, 1, r.retries[poison])
	assert.Equal(t, 3, r.maxRetriesIn)
}

func TestPublishFromSchema_MarkPublishedErrorRollsBackBatch(t *testing.T) {
	r := &fakeReader{
		events:     []domainevent.BrokerEvent{{EventID: uuid.New(), EventType: "e"}},
		markPubErr: errors.New("db down"),
	}

	_, _, err := newTestPublisher(r, &fakeBroker{}, &fakeTx{}, 3).publishFromSchema(context.Background(), "identity")

	assert.ErrorContains(t, err, "db down")
}
