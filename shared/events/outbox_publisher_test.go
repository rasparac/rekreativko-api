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
	failFor map[string]struct{}
	sent    []string
}

func (b *fakeBroker) Publish(_ context.Context, topic string, _ []byte) error {
	if _, fail := b.failFor[topic]; fail {
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
	br := &fakeBroker{failFor: map[string]struct{}{"bad.event": {}}}

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

func TestPublishWoken_PublishesOnlyTheNamedSchemas(t *testing.T) {
	newPublisher := func(r *fakeReader, wakeups chan string) *outboxPublisher {
		return NewOutboxPublisher(r, &fakeTx{}, &fakeBroker{}, logger.New("error", "json"), 10, 3, time.Hour,
			newTestMetrics(), []string{"activity", "identity"}).WithWakeups(wakeups)
	}

	tests := []struct {
		name      string
		woken     []string
		wantReads int
	}{
		{"named schema", []string{"activity"}, 1},
		{"unknown schema is ignored", []string{"other"}, 0},
		{"empty means every schema", []string{""}, 2},
		{"a burst is one pass per schema", []string{"activity", "activity", "identity"}, 2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &fakeReader{}
			wakeups := make(chan string, 8)
			for _, schema := range tt.woken[1:] {
				wakeups <- schema
			}

			newPublisher(r, wakeups).publishWoken(context.Background(), tt.woken[0])

			assert.Equal(t, tt.wantReads, r.readCalls)
		})
	}
}

// batchReader serves its batches one per read, then nothing.
type batchReader struct {
	fakeReader
	batches [][]domainevent.BrokerEvent
}

func (b *batchReader) ReadEvents(context.Context, string, int) ([]domainevent.BrokerEvent, error) {
	b.readCalls++
	if len(b.batches) == 0 {
		return nil, nil
	}
	next := b.batches[0]
	b.batches = b.batches[1:]
	return next, nil
}

func TestPublishSchemaBacklog_DrainsFullBatches(t *testing.T) {
	batch := func(n int) []domainevent.BrokerEvent {
		events := make([]domainevent.BrokerEvent, n)
		for i := range events {
			events[i] = domainevent.BrokerEvent{EventID: uuid.New(), EventType: "e"}
		}
		return events
	}

	// read limit is 10: two full batches mean there may be more, the short
	// one is the end.
	r := &batchReader{batches: [][]domainevent.BrokerEvent{batch(10), batch(10), batch(3)}}
	op := newTestPublisher(&r.fakeReader, &fakeBroker{}, &fakeTx{}, 3)
	op.eventReader = r

	op.publishSchemaBacklog(context.Background(), "identity")

	assert.Equal(t, 3, r.readCalls)
	assert.Len(t, r.published, 23)
}

func TestPublishSchemaBacklog_StopsAtFirstFailure(t *testing.T) {
	events := make([]domainevent.BrokerEvent, 10)
	for i := range events {
		events[i] = domainevent.BrokerEvent{EventID: uuid.New(), EventType: "bad.event"}
	}
	r := &batchReader{batches: [][]domainevent.BrokerEvent{events, events}}
	op := newTestPublisher(&r.fakeReader, &fakeBroker{failFor: map[string]struct{}{"bad.event": {}}}, &fakeTx{}, 3)
	op.eventReader = r

	op.publishSchemaBacklog(context.Background(), "identity")

	assert.Equal(t, 1, r.readCalls, "a failing event is retried on the poll's schedule")
}

// signalReader is safe to use from the publisher's goroutine while the test
// waits on it: reads are reported on a channel, nothing else is shared.
type signalReader struct {
	reads chan string
}

func (s *signalReader) ReadEvents(_ context.Context, schema string, _ int) ([]domainevent.BrokerEvent, error) {
	s.reads <- schema
	return nil, nil
}

func (s *signalReader) MarkEventAsPublished(context.Context, string, uuid.UUID) error { return nil }

func (s *signalReader) MarkEventAsFailed(context.Context, string, uuid.UUID, error, int) (bool, error) {
	return false, nil
}

func startPublisher(t *testing.T, poll time.Duration, wakeups chan string) (reads chan string, stop func()) {
	t.Helper()

	reader := &signalReader{reads: make(chan string, 64)}
	op := NewOutboxPublisher(reader, &fakeTx{}, &fakeBroker{}, logger.New("error", "json"), 10, 3,
		poll, newTestMetrics(), []string{"activity", "identity"}).WithWakeups(wakeups)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- op.Start(ctx) }()

	return reader.reads, func() {
		cancel()
		select {
		case err := <-done:
			assert.ErrorIs(t, err, context.Canceled)
		case <-time.After(3 * time.Second):
			t.Fatal("Start did not stop")
		}
	}
}

func expectRead(t *testing.T, reads <-chan string, want string) {
	t.Helper()

	select {
	case got := <-reads:
		assert.Equal(t, want, got)
	case <-time.After(3 * time.Second):
		t.Fatalf("no read of %q", want)
	}
}

// With the poll an hour away, a wake-up is the only thing that can make Start
// publish. Run with -race: the wake-up comes from another goroutine.
func TestStart_PublishesOnWakeup(t *testing.T) {
	wakeups := make(chan string)
	reads, stop := startPublisher(t, time.Hour, wakeups)
	defer stop()

	// Start's first pass reads every schema.
	expectRead(t, reads, "activity")
	expectRead(t, reads, "identity")

	wakeups <- "identity"
	expectRead(t, reads, "identity")

	select {
	case extra := <-reads:
		t.Fatalf("unexpected read of %q: only the woken schema is published", extra)
	case <-time.After(200 * time.Millisecond):
	}
}

// A closed wake-up channel (the listener stopped) must not stop or spin the
// publisher: it keeps polling.
func TestStart_KeepsPollingWhenTheListenerStops(t *testing.T) {
	wakeups := make(chan string)
	reads, stop := startPublisher(t, 20*time.Millisecond, wakeups)
	defer stop()

	close(wakeups)

	// First pass plus several polls: 2 schemas each.
	for range 8 {
		select {
		case <-reads:
		case <-time.After(3 * time.Second):
			t.Fatal("the publisher stopped polling")
		}
	}
}
