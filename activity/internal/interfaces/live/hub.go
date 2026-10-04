// Package live pushes a session's team-formation state to connected clients
// as it changes. It knows nothing about the transport: the HTTP layer turns
// Updates into server-sent events.
package live

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"

	"github.com/google/uuid"
	"github.com/rasparac/rekreativko-api/activity/internal/application"
	"github.com/rasparac/rekreativko-api/activity/internal/domain"
	"github.com/rasparac/rekreativko-api/shared/logger"
)

// TopicPattern is the event pattern the hub listens to: everything about a
// session - draft, voting, teams, attendance and the session itself.
const TopicPattern = "activity.session.>"

const topicPrefix = "activity.session."

// sendBuffer is how many updates a client may fall behind before it is
// disconnected. Every update carries the full state, so a client that
// reconnects loses nothing.
const sendBuffer = 16

// Disconnect reasons, sent to the client before its stream is closed.
const (
	ReasonRemoved      = "removed"         // the viewer was removed from the session or can no longer see it
	ReasonNotFound     = "not_found"       // the session is gone
	ReasonSlowConsumer = "slow_consumer"   // the client could not keep up
	ReasonShutdown     = "server_shutdown" // this instance is shutting down; reconnect to another one
	ReasonTokenExpired = "token_expired"   // the viewer's access token ran out; refresh it and reconnect
)

type (
	snapshotBuilder interface {
		Snapshot(ctx context.Context, sessionID uuid.UUID) (*application.TeamFormationSnapshot, error)
	}

	// sessionViewer reports whether userID may still see the session.
	sessionViewer interface {
		GetSession(ctx context.Context, sessionID, requesterID uuid.UUID) (*domain.Session, error)
	}
)

// Update is one change pushed to a client.
type Update struct {
	// Event names the change: "snapshot" for the first update after
	// connecting, otherwise the event type without the "activity.session."
	// prefix, e.g. "draft.player_picked" or "voting.vote_cast".
	Event string
	// Change is the raw domain event (ids of everything involved); nil for
	// the initial snapshot.
	Change json.RawMessage
	// Snapshot is the whole state after the change. Only the voting round's
	// my_vote depends on the viewer, so it is shared by all of a session's
	// clients and personalised when written.
	Snapshot *application.TeamFormationSnapshot
}

// Client is one connected viewer of one session.
type Client struct {
	SessionID uuid.UUID
	ViewerID  uuid.UUID

	updates chan Update
	done    chan struct{}

	mu          sync.Mutex
	lastVersion int64
	reason      string
	closed      bool
}

// Updates delivers the client's updates, in increasing version order.
func (c *Client) Updates() <-chan Update { return c.updates }

// Done is closed when the hub disconnects the client; Reason says why.
func (c *Client) Done() <-chan struct{} { return c.done }

// Reason is why the hub disconnected the client, once Done is closed.
func (c *Client) Reason() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.reason
}

// offer queues u unless it is not newer than what the client already has.
// A full buffer disconnects the client.
func (c *Client) offer(u Update) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed || u.Snapshot.Version <= c.lastVersion {
		return
	}

	select {
	case c.updates <- u:
		c.lastVersion = u.Snapshot.Version
	default:
		c.closeLocked(ReasonSlowConsumer)
	}
}

func (c *Client) close(reason string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.closeLocked(reason)
}

func (c *Client) closeLocked(reason string) {
	if c.closed {
		return
	}
	c.closed = true
	c.reason = reason
	close(c.done)
}

// Hub fans session events out to the clients connected to this instance.
// Every instance runs its own hub and receives every event (a broadcast
// subscription), so a client can be connected to any instance.
type Hub struct {
	snapshots snapshotBuilder
	sessions  sessionViewer
	logger    *logger.Logger

	mu       sync.Mutex
	clients  map[uuid.UUID]map[*Client]struct{}
	shutdown bool
}

func NewHub(snapshots snapshotBuilder, sessions sessionViewer, logger *logger.Logger) *Hub {
	return &Hub{
		snapshots: snapshots,
		sessions:  sessions,
		logger:    logger.WithName("activity.live.hub"),
		clients:   map[uuid.UUID]map[*Client]struct{}{},
	}
}

// Connect registers a viewer of sessionID and queues the current snapshot as
// its first update. The caller has already checked the viewer may see the
// session, and must call Disconnect when the connection ends. Registering
// before building the snapshot means no change can slip in between.
func (h *Hub) Connect(ctx context.Context, sessionID, viewerID uuid.UUID) (*Client, error) {
	client := &Client{
		SessionID: sessionID,
		ViewerID:  viewerID,
		updates:   make(chan Update, sendBuffer),
		done:      make(chan struct{}),
	}

	h.mu.Lock()
	if h.shutdown {
		// A request that slipped in while the server was closing: end it at once.
		client.closeLocked(ReasonShutdown)
	} else {
		if h.clients[sessionID] == nil {
			h.clients[sessionID] = map[*Client]struct{}{}
		}
		h.clients[sessionID][client] = struct{}{}
	}
	h.mu.Unlock()

	snapshot, err := h.snapshots.Snapshot(ctx, sessionID)
	if err != nil {
		h.Disconnect(client)
		return nil, err
	}

	client.offer(Update{Event: "snapshot", Snapshot: snapshot})

	return client, nil
}

// Disconnect unregisters the client. Safe to call more than once.
func (h *Hub) Disconnect(client *Client) {
	h.mu.Lock()
	if clients := h.clients[client.SessionID]; clients != nil {
		delete(clients, client)
		if len(clients) == 0 {
			delete(h.clients, client.SessionID)
		}
	}
	h.mu.Unlock()

	client.close("")
}

// Shutdown disconnects every client with ReasonShutdown so their streams end
// and they reconnect to another instance, and turns away clients connecting
// afterwards. Streams never end on their own, so call it when the server
// starts shutting down, or http.Server.Shutdown waits out its whole grace
// period. Safe to call more than once.
func (h *Hub) Shutdown() {
	h.mu.Lock()
	h.shutdown = true
	clients := h.clients
	h.clients = map[uuid.UUID]map[*Client]struct{}{}
	h.mu.Unlock()

	for _, sessionClients := range clients {
		for c := range sessionClients {
			c.close(ReasonShutdown)
		}
	}
}

func (h *Hub) clientsOf(sessionID uuid.UUID) []*Client {
	h.mu.Lock()
	defer h.mu.Unlock()

	clients := make([]*Client, 0, len(h.clients[sessionID]))
	for c := range h.clients[sessionID] {
		clients = append(clients, c)
	}
	return clients
}

// eventRef is the part of a session event the hub needs.
type eventRef struct {
	SessionID   uuid.UUID `json:"session_id"`
	AggregateID uuid.UUID `json:"aggregate_id"`
	UserID      uuid.UUID `json:"user_id"`
}

// HandleEvent is the broadcast handler for TopicPattern. It rebuilds the
// session's snapshot once and pushes it to every client of the session, after
// disconnecting a viewer the event removed. Events run one at a time, in
// order, so snapshots go out in the order they were built.
func (h *Hub) HandleEvent(ctx context.Context, topic string, payload []byte) {
	if !strings.HasPrefix(topic, topicPrefix) {
		return
	}

	var ref eventRef
	if err := json.Unmarshal(payload, &ref); err != nil {
		h.logger.Warn(ctx, "undecodable session event", "topic", topic, "error", err)
		return
	}

	// Session-level events (cancelled, completed, ...) carry the session only
	// as their aggregate.
	sessionID := ref.SessionID
	if sessionID == uuid.Nil {
		sessionID = ref.AggregateID
	}

	clients := h.clientsOf(sessionID)
	if len(clients) == 0 {
		return
	}

	event := strings.TrimPrefix(topic, topicPrefix)
	if strings.HasPrefix(event, "attendee.") && ref.UserID != uuid.Nil {
		clients = h.dropRemovedViewer(ctx, clients, event, sessionID, ref.UserID)
	}

	snapshot, err := h.snapshots.Snapshot(ctx, sessionID)
	if errors.Is(err, domain.ErrSessionNotFound) {
		for _, c := range clients {
			c.close(ReasonNotFound)
		}
		return
	} else if err != nil {
		// Clients keep their last state; the next event or a reconnect resyncs.
		h.logger.Error(ctx, "failed to build team-formation snapshot", "session_id", sessionID, "topic", topic, "error", err)
		return
	}

	update := Update{Event: event, Change: json.RawMessage(payload), Snapshot: snapshot}
	for _, c := range clients {
		c.offer(update)
	}
}

// dropRemovedViewer disconnects userID's clients when the attendance change
// removed them from the session or left them unable to see it (e.g. no
// longer attending a private session). It returns the clients still connected.
func (h *Hub) dropRemovedViewer(ctx context.Context, clients []*Client, event string, sessionID, userID uuid.UUID) []*Client {
	var (
		remaining []*Client
		checked   bool
		removed   = event == "attendee.removed"
	)

	for _, c := range clients {
		if c.ViewerID != userID {
			remaining = append(remaining, c)
			continue
		}

		if !checked && !removed {
			// Only "you can't see it" counts; a failed lookup keeps the viewer.
			_, err := h.sessions.GetSession(ctx, sessionID, userID)
			removed = errors.Is(err, domain.ErrSessionNotFound)
			checked = true
		}

		if removed {
			c.close(ReasonRemoved)
			continue
		}
		remaining = append(remaining, c)
	}

	return remaining
}
