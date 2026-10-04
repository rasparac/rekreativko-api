package events

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgxlisten"
	"github.com/rasparac/rekreativko-api/shared/logger"
)

// OutboxChannel is the Postgres NOTIFY channel the event_outbox triggers
// signal on; the payload is the schema that got events.
const OutboxChannel = "outbox_events"

const (
	// listenReconnectDelay is how long the listener waits before replacing a
	// failed connection. The poll covers the gap.
	listenReconnectDelay = 2 * time.Second

	// wakeupBuffer is how many notifications can wait for the publisher. A
	// full buffer drops the notification: the poll picks the events up.
	wakeupBuffer = 64
)

// ListenOutbox LISTENs for outbox notifications on a connection of its own
// (not the pool's: it is held for as long as the process runs) and returns the
// schemas that got events. After every (re)connect it sends "" - meaning
// every schema - because notifications sent while it was down are lost. The
// channel is closed when ctx ends.
func ListenOutbox(ctx context.Context, connConfig *pgx.ConnConfig, log *logger.Logger) <-chan string {
	wakeups := make(chan string, wakeupBuffer)

	listener := &pgxlisten.Listener{
		Connect: func(ctx context.Context) (*pgx.Conn, error) {
			cfg := connConfig.Copy()
			cfg.Tracer = nil // waiting for a notification is not a query to time

			return pgx.ConnectConfig(ctx, cfg)
		},
		ReconnectDelay: listenReconnectDelay,
		LogError: func(ctx context.Context, err error) {
			if ctx.Err() == nil {
				log.Error(ctx, "outbox listener: polling covers until it is back", "error", err)
			}
		},
	}
	listener.Handle(OutboxChannel, &outboxWakeups{wakeups: wakeups})

	go func() {
		defer close(wakeups)

		if err := listener.Listen(ctx); err != nil && !errors.Is(err, context.Canceled) {
			log.Error(ctx, "outbox listener stopped", "error", err)
		}
	}()

	return wakeups
}

// outboxWakeups turns notifications into schema names on a channel, without
// ever blocking the listener.
type outboxWakeups struct {
	wakeups chan<- string
}

func (h *outboxWakeups) HandleNotification(_ context.Context, n *pgconn.Notification, _ *pgx.Conn) error {
	h.wake(n.Payload)
	return nil
}

// HandleBacklog runs after every successful LISTEN: notifications sent while
// the connection was down are gone, so ask for a full pass.
func (h *outboxWakeups) HandleBacklog(context.Context, string, *pgx.Conn) error {
	h.wake("")
	return nil
}

func (h *outboxWakeups) wake(schema string) {
	select {
	case h.wakeups <- schema:
	default:
	}
}
