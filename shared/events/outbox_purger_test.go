package events

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rasparac/rekreativko-api/shared/logger"
)

type purgeCall struct {
	schema string
	kind   string
	before time.Time
	limit  int
}

// fakeDeleter answers each schema's calls from a script of results, keyed by
// "schema/kind" (kind is published or dead_lettered).
type fakeDeleter struct {
	script map[string][]int64
	errs   map[string]error
	calls  []purgeCall
}

func (f *fakeDeleter) DeletePublishedEvents(ctx context.Context, schema string, before time.Time, limit int) (int64, error) {
	return f.delete(ctx, schema, "published", before, limit)
}

func (f *fakeDeleter) DeleteDeadLetteredEvents(ctx context.Context, schema string, before time.Time, limit int) (int64, error) {
	return f.delete(ctx, schema, "dead_lettered", before, limit)
}

func (f *fakeDeleter) delete(_ context.Context, schema, kind string, before time.Time, limit int) (int64, error) {
	f.calls = append(f.calls, purgeCall{schema, kind, before, limit})
	key := schema + "/" + kind
	if err := f.errs[key]; err != nil {
		return 0, err
	}
	results := f.script[key]
	if len(results) == 0 {
		return 0, nil
	}
	f.script[key] = results[1:]
	return results[0], nil
}

// callsOf returns the calls of one kind.
func (f *fakeDeleter) callsOf(kind string) []purgeCall {
	var calls []purgeCall
	for _, call := range f.calls {
		if call.kind == kind {
			calls = append(calls, call)
		}
	}
	return calls
}

func counterValue(c prometheus.Counter) float64 {
	var m dto.Metric
	if err := c.Write(&m); err != nil {
		panic(err)
	}
	return m.GetCounter().GetValue()
}

func newTestPurger(t *testing.T, deleter outboxDeleter, schemas ...string) (*outboxPurger, *Metrics, *bytes.Buffer) {
	t.Helper()

	metrics := &Metrics{OutboxPurgedTotal: prometheus.NewCounterVec(prometheus.CounterOpts{Name: "purged"}, []string{"schema", "kind"})}
	var logs bytes.Buffer
	purger, err := NewOutboxPurger(deleter, logger.NewWithWriter("info", "json", &logs), metrics, schemas, 7*24*time.Hour, 14*24*time.Hour, time.Hour, 100)
	require.NoError(t, err)

	now := time.Date(2026, 10, 17, 12, 0, 0, 0, time.UTC)
	purger.now = func() time.Time { return now }

	return purger, metrics, &logs
}

func TestNewOutboxPurger_RejectsNonPositiveSettings(t *testing.T) {
	for name, args := range map[string][4]any{
		"retention":             {time.Duration(0), time.Hour, time.Hour, 100},
		"dead-letter retention": {time.Hour, time.Duration(0), time.Hour, 100},
		"interval":              {time.Hour, time.Hour, time.Duration(0), 100},
		"batch":                 {time.Hour, time.Hour, time.Hour, 0},
	} {
		_, err := NewOutboxPurger(&fakeDeleter{}, logger.New("error", "json"), nil, []string{"a"},
			args[0].(time.Duration), args[1].(time.Duration), args[2].(time.Duration), args[3].(int))
		assert.Error(t, err, name)
	}
}

func TestOutboxPurger_DeletesInBatchesUntilOneComesBackShort(t *testing.T) {
	deleter := &fakeDeleter{script: map[string][]int64{"activity/published": {100, 100, 37}}}
	purger, metrics, logs := newTestPurger(t, deleter, "activity")

	purger.PurgeOnce(context.Background())

	published := deleter.callsOf("published")
	require.Len(t, published, 3, "full batch, full batch, short batch: stop")
	cutoff := time.Date(2026, 10, 10, 12, 0, 0, 0, time.UTC)
	for _, call := range published {
		assert.Equal(t, "activity", call.schema)
		assert.Equal(t, 100, call.limit)
		assert.True(t, call.before.Equal(cutoff), "7 days before now, got %s", call.before)
	}
	assert.InDelta(t, 237, counterValue(metrics.OutboxPurgedTotal.WithLabelValues("activity", "published")), 0)
	assert.Contains(t, logs.String(), "purged published outbox rows")
}

func TestOutboxPurger_DeletesDeadLetteredRowsPastTheirOwnWindow(t *testing.T) {
	deleter := &fakeDeleter{script: map[string][]int64{"identity/dead_lettered": {100, 3}}}
	purger, metrics, logs := newTestPurger(t, deleter, "identity")

	purger.PurgeOnce(context.Background())

	deadLettered := deleter.callsOf("dead_lettered")
	require.Len(t, deadLettered, 2, "full batch, short batch: stop")
	cutoff := time.Date(2026, 10, 3, 12, 0, 0, 0, time.UTC)
	for _, call := range deadLettered {
		assert.Equal(t, "identity", call.schema)
		assert.Equal(t, 100, call.limit)
		assert.True(t, call.before.Equal(cutoff), "14 days before now, got %s", call.before)
	}
	assert.InDelta(t, 103, counterValue(metrics.OutboxPurgedTotal.WithLabelValues("identity", "dead_lettered")), 0)
	assert.Zero(t, counterValue(metrics.OutboxPurgedTotal.WithLabelValues("identity", "published")))
	assert.Contains(t, logs.String(), "purged dead-lettered outbox rows")
	assert.NotContains(t, logs.String(), "purged published")
}

func TestOutboxPurger_NothingToDeleteIsQuiet(t *testing.T) {
	deleter := &fakeDeleter{}
	purger, metrics, logs := newTestPurger(t, deleter, "identity", "activity")

	purger.PurgeOnce(context.Background())

	assert.Len(t, deleter.calls, 4, "one call per schema and kind")
	assert.Zero(t, counterValue(metrics.OutboxPurgedTotal.WithLabelValues("identity", "published")))
	assert.Zero(t, counterValue(metrics.OutboxPurgedTotal.WithLabelValues("identity", "dead_lettered")))
	assert.NotContains(t, logs.String(), "purged")
}

func TestOutboxPurger_AFailingSchemaDoesNotStopTheOthers(t *testing.T) {
	deleter := &fakeDeleter{
		errs:   map[string]error{"identity/published": errors.New("boom")},
		script: map[string][]int64{"identity/dead_lettered": {2}, "activity/published": {5}},
	}
	purger, metrics, logs := newTestPurger(t, deleter, "identity", "activity")

	purger.PurgeOnce(context.Background())

	assert.Contains(t, logs.String(), "failed to purge published outbox rows")
	assert.InDelta(t, 2, counterValue(metrics.OutboxPurgedTotal.WithLabelValues("identity", "dead_lettered")), 0,
		"a failing kind does not stop the other kind of the same schema")
	assert.InDelta(t, 5, counterValue(metrics.OutboxPurgedTotal.WithLabelValues("activity", "published")), 0)
	assert.Len(t, deleter.calls, 4, "each schema and kind once (no retry loop on error)")
}

func TestOutboxPurger_StopsWhenTheContextEnds(t *testing.T) {
	deleter := &fakeDeleter{script: map[string][]int64{"activity/published": {100, 100, 100}}}
	purger, _, _ := newTestPurger(t, deleter, "activity")

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	purger.PurgeOnce(ctx)

	assert.Empty(t, deleter.calls)
}
