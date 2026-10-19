package events

import (
	"context"
	"fmt"
	"time"

	"github.com/rasparac/rekreativko-api/shared/logger"
)

type (
	// outboxDeleter deletes expired outbox rows. DeletePublishedEvents keeps
	// pending and dead-lettered rows, DeleteDeadLetteredEvents keeps pending
	// and published ones; pending rows are never deleted.
	outboxDeleter interface {
		DeletePublishedEvents(ctx context.Context, schema string, publishedBefore time.Time, limit int) (int64, error)
		DeleteDeadLetteredEvents(ctx context.Context, schema string, failedBefore time.Time, limit int) (int64, error)
	}

	// outboxPurger deletes outbox rows that were published, or dead-lettered,
	// longer ago than their retention window. Published rows are not an event
	// history (the NATS stream keeps messages for the same window), they only
	// make the tables grow. Dead-lettered rows are kept long enough to be
	// inspected and replayed, but not forever: payloads can carry personal
	// data. Deleting is idempotent and skips rows another purger holds, so
	// every replica of the publisher can run it.
	outboxPurger struct {
		deleter             outboxDeleter
		logger              *logger.Logger
		metrics             *Metrics
		schemas             []string
		retention           time.Duration
		deadLetterRetention time.Duration
		interval            time.Duration
		batch               int
		now                 func() time.Time
	}

	// purgeKind is one class of rows the purger deletes.
	purgeKind struct {
		label     string // the metric's kind label
		rows      string // for log lines
		retention time.Duration
		delete    func(ctx context.Context, schema string, before time.Time, limit int) (int64, error)
	}
)

// NewOutboxPurger returns a purger for the schemas: every interval it deletes
// rows published more than retention ago and rows dead-lettered more than
// deadLetterRetention ago, batch rows at a time so no single statement holds
// many locks.
func NewOutboxPurger(
	deleter outboxDeleter,
	logger *logger.Logger,
	metrics *Metrics,
	schemas []string,
	retention time.Duration,
	deadLetterRetention time.Duration,
	interval time.Duration,
	batch int,
) (*outboxPurger, error) {
	if retention <= 0 || deadLetterRetention <= 0 || interval <= 0 || batch <= 0 {
		return nil, fmt.Errorf(
			"outbox purge needs a positive retention, dead-letter retention, interval and batch (got %s, %s, %s, %d)",
			retention, deadLetterRetention, interval, batch,
		)
	}

	return &outboxPurger{
		deleter:             deleter,
		logger:              logger,
		metrics:             metrics,
		schemas:             schemas,
		retention:           retention,
		deadLetterRetention: deadLetterRetention,
		interval:            interval,
		batch:               batch,
		now:                 time.Now,
	}, nil
}

// Run purges once at start and then every interval until ctx ends.
func (p *outboxPurger) Run(ctx context.Context) {
	p.PurgeOnce(ctx)

	ticker := time.NewTicker(p.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			p.PurgeOnce(ctx)
		}
	}
}

// PurgeOnce deletes the expired published and dead-lettered rows of every
// schema, batch by batch. A failing schema or kind is logged and does not stop
// the others.
func (p *outboxPurger) PurgeOnce(ctx context.Context) {
	now := p.now()
	kinds := []purgeKind{
		{label: "published", rows: "published", retention: p.retention, delete: p.deleter.DeletePublishedEvents},
		{label: "dead_lettered", rows: "dead-lettered", retention: p.deadLetterRetention, delete: p.deleter.DeleteDeadLetteredEvents},
	}

	for _, schema := range p.schemas {
		for _, kind := range kinds {
			p.purge(ctx, schema, kind, now.Add(-kind.retention))
		}
	}
}

// purge deletes one kind of expired rows of one schema until a batch comes
// back short.
func (p *outboxPurger) purge(ctx context.Context, schema string, kind purgeKind, cutoff time.Time) {
	var total int64

	for ctx.Err() == nil {
		deleted, err := kind.delete(ctx, schema, cutoff, p.batch)
		if err != nil {
			p.logger.Error(ctx, "failed to purge "+kind.rows+" outbox rows", "schema", schema, "error", err)
			break
		}

		total += deleted
		if deleted < int64(p.batch) {
			break
		}
	}

	if total > 0 {
		p.metrics.OutboxPurgedTotal.WithLabelValues(schema, kind.label).Add(float64(total))
		p.logger.Info(ctx, "purged "+kind.rows+" outbox rows", "schema", schema, "deleted", total, "before", cutoff)
	}
}
