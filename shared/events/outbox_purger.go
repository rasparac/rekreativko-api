package events

import (
	"context"
	"fmt"
	"time"

	"github.com/rasparac/rekreativko-api/shared/logger"
)

type (
	// outboxDeleter deletes published outbox rows. DeletePublishedEvents keeps
	// pending and dead-lettered rows.
	outboxDeleter interface {
		DeletePublishedEvents(ctx context.Context, schema string, publishedBefore time.Time, limit int) (int64, error)
	}

	// outboxPurger deletes outbox rows that were published longer ago than the
	// retention window. Published rows are not an event history (the NATS stream
	// keeps messages for the same window), they only make the tables grow.
	// Deleting is idempotent and skips rows another purger holds, so every
	// replica of the publisher can run it.
	outboxPurger struct {
		deleter   outboxDeleter
		logger    *logger.Logger
		metrics   *Metrics
		schemas   []string
		retention time.Duration
		interval  time.Duration
		batch     int
		now       func() time.Time
	}
)

// NewOutboxPurger returns a purger for the schemas: every interval it deletes
// rows published more than retention ago, batch rows at a time so no single
// statement holds many locks.
func NewOutboxPurger(
	deleter outboxDeleter,
	logger *logger.Logger,
	metrics *Metrics,
	schemas []string,
	retention time.Duration,
	interval time.Duration,
	batch int,
) (*outboxPurger, error) {
	if retention <= 0 || interval <= 0 || batch <= 0 {
		return nil, fmt.Errorf("outbox purge needs a positive retention, interval and batch (got %s, %s, %d)", retention, interval, batch)
	}

	return &outboxPurger{
		deleter:   deleter,
		logger:    logger,
		metrics:   metrics,
		schemas:   schemas,
		retention: retention,
		interval:  interval,
		batch:     batch,
		now:       time.Now,
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

// PurgeOnce deletes the expired rows of every schema, batch by batch. A failing
// schema is logged and does not stop the others.
func (p *outboxPurger) PurgeOnce(ctx context.Context) {
	cutoff := p.now().Add(-p.retention)

	for _, schema := range p.schemas {
		var total int64

		for ctx.Err() == nil {
			deleted, err := p.deleter.DeletePublishedEvents(ctx, schema, cutoff, p.batch)
			if err != nil {
				p.logger.Error(ctx, "failed to purge published outbox rows", "schema", schema, "error", err)
				break
			}

			total += deleted
			if deleted < int64(p.batch) {
				break
			}
		}

		if total > 0 {
			p.metrics.OutboxPurgedTotal.WithLabelValues(schema).Add(float64(total))
			p.logger.Info(ctx, "purged published outbox rows", "schema", schema, "deleted", total, "published_before", cutoff)
		}
	}
}
