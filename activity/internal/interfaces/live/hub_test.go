package live

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/rasparac/rekreativko-api/activity/internal/application"
	"github.com/rasparac/rekreativko-api/activity/internal/domain"
	"github.com/rasparac/rekreativko-api/shared/domainerror"
	"github.com/rasparac/rekreativko-api/shared/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeSnapshots returns a snapshot with an ever-increasing version per call.
type fakeSnapshots struct {
	mu      sync.Mutex
	version int64
	calls   int
	err     error
}

func (f *fakeSnapshots) Snapshot(_ context.Context, sessionID uuid.UUID) (*application.TeamFormationSnapshot, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	f.version++
	return &application.TeamFormationSnapshot{Version: f.version}, nil
}

func (f *fakeSnapshots) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

// fakeViewer reports every user in hidden as unable to see the session.
type fakeViewer struct {
	hidden map[uuid.UUID]struct{}
}

func (f *fakeViewer) GetSession(_ context.Context, _, requesterID uuid.UUID) (*domain.Session, error) {
	if _, ok := f.hidden[requesterID]; ok {
		return nil, domainerror.NotFound("session_not_found", "Session not found", domain.ErrSessionNotFound)
	}
	return nil, nil
}

func newTestHub(t *testing.T) (*Hub, *fakeSnapshots, *fakeViewer) {
	t.Helper()

	snapshots := &fakeSnapshots{}
	viewer := &fakeViewer{hidden: map[uuid.UUID]struct{}{}}

	return NewHub(snapshots, viewer, logger.New("error", "json")), snapshots, viewer
}

func connect(t *testing.T, h *Hub, sessionID, viewerID uuid.UUID) *Client {
	t.Helper()

	client, err := h.Connect(context.Background(), sessionID, viewerID)
	require.NoError(t, err)
	t.Cleanup(func() { h.Disconnect(client) })

	first := next(t, client)
	require.Equal(t, "snapshot", first.Event)
	require.Nil(t, first.Change)

	return client
}

func next(t *testing.T, c *Client) Update {
	t.Helper()

	select {
	case u := <-c.Updates():
		return u
	case <-time.After(time.Second):
		t.Fatal("no update arrived")
		return Update{}
	}
}

func requireNoUpdate(t *testing.T, c *Client) {
	t.Helper()

	select {
	case u := <-c.Updates():
		t.Fatalf("unexpected update %q", u.Event)
	default:
	}
}

func requireClosed(t *testing.T, c *Client, reason string) {
	t.Helper()

	select {
	case <-c.Done():
		assert.Equal(t, reason, c.Reason())
	case <-time.After(time.Second):
		t.Fatal("client was not disconnected")
	}
}

func payload(t *testing.T, fields map[string]any) []byte {
	t.Helper()

	b, err := json.Marshal(fields)
	require.NoError(t, err)
	return b
}

func TestHub_PushesChangesToEveryClientOfTheSession(t *testing.T) {
	h, snapshots, _ := newTestHub(t)
	sessionID := uuid.New()
	a := connect(t, h, sessionID, uuid.New())
	b := connect(t, h, sessionID, uuid.New())
	other := connect(t, h, uuid.New(), uuid.New())
	buildsBefore := snapshots.callCount()

	event := payload(t, map[string]any{"session_id": sessionID, "user_id": uuid.New(), "pick_number": 3})
	h.HandleEvent(context.Background(), "activity.session.draft.player_picked", event)

	assert.Equal(t, buildsBefore+1, snapshots.callCount(), "one snapshot per event, shared by the session's clients")
	for _, c := range []*Client{a, b} {
		u := next(t, c)
		assert.Equal(t, "draft.player_picked", u.Event)
		assert.JSONEq(t, string(event), string(u.Change))
	}
	requireNoUpdate(t, other)
}

func TestHub_IgnoresSessionsWithoutClients(t *testing.T) {
	h, snapshots, _ := newTestHub(t)

	h.HandleEvent(context.Background(), "activity.session.voting.vote_cast", payload(t, map[string]any{"session_id": uuid.New()}))
	h.HandleEvent(context.Background(), "activity.group.cancelled", payload(t, map[string]any{"aggregate_id": uuid.New()}))
	h.HandleEvent(context.Background(), "activity.session.draft.started", []byte("not json"))

	assert.Zero(t, snapshots.callCount())
}

func TestHub_SessionLevelEventsUseTheAggregateID(t *testing.T) {
	h, _, _ := newTestHub(t)
	sessionID := uuid.New()
	c := connect(t, h, sessionID, uuid.New())

	h.HandleEvent(context.Background(), "activity.session.cancelled", payload(t, map[string]any{"aggregate_id": sessionID}))

	assert.Equal(t, "cancelled", next(t, c).Event)
}

func TestHub_RemovedAttendeeIsDisconnected(t *testing.T) {
	h, _, _ := newTestHub(t)
	sessionID, removed := uuid.New(), uuid.New()
	gone := connect(t, h, sessionID, removed)
	stays := connect(t, h, sessionID, uuid.New())

	h.HandleEvent(context.Background(), "activity.session.attendee.removed", payload(t, map[string]any{"session_id": sessionID, "user_id": removed}))

	requireClosed(t, gone, ReasonRemoved)
	assert.Equal(t, "attendee.removed", next(t, stays).Event)
}

func TestHub_AttendanceChangeDisconnectsOnlyWhoCanNoLongerSee(t *testing.T) {
	h, _, viewer := newTestHub(t)
	sessionID, leaver, watcher := uuid.New(), uuid.New(), uuid.New()
	leaverClient := connect(t, h, sessionID, leaver)
	watcherClient := connect(t, h, sessionID, watcher)

	// A public session: leaving doesn't hide it.
	h.HandleEvent(context.Background(), "activity.session.attendee.rsvp_not_going", payload(t, map[string]any{"session_id": sessionID, "user_id": watcher}))
	assert.Equal(t, "attendee.rsvp_not_going", next(t, watcherClient).Event)
	assert.Equal(t, "attendee.rsvp_not_going", next(t, leaverClient).Event)

	// A private one: the leaver can no longer see it.
	viewer.hidden[leaver] = struct{}{}
	h.HandleEvent(context.Background(), "activity.session.attendee.rsvp_not_going", payload(t, map[string]any{"session_id": sessionID, "user_id": leaver}))

	requireClosed(t, leaverClient, ReasonRemoved)
	assert.Equal(t, "attendee.rsvp_not_going", next(t, watcherClient).Event)
}

func TestHub_DeletedSessionDisconnectsEveryone(t *testing.T) {
	h, snapshots, _ := newTestHub(t)
	sessionID := uuid.New()
	a := connect(t, h, sessionID, uuid.New())
	b := connect(t, h, sessionID, uuid.New())

	snapshots.err = domainerror.NotFound("session_not_found", "Session not found", domain.ErrSessionNotFound)
	h.HandleEvent(context.Background(), "activity.session.cancelled", payload(t, map[string]any{"aggregate_id": sessionID}))

	requireClosed(t, a, ReasonNotFound)
	requireClosed(t, b, ReasonNotFound)
}

func TestHub_FailedSnapshotKeepsClientsConnected(t *testing.T) {
	h, snapshots, _ := newTestHub(t)
	sessionID := uuid.New()
	c := connect(t, h, sessionID, uuid.New())

	snapshots.err = assert.AnError
	h.HandleEvent(context.Background(), "activity.session.voting.vote_cast", payload(t, map[string]any{"session_id": sessionID}))

	requireNoUpdate(t, c)
	select {
	case <-c.Done():
		t.Fatal("a transient failure must not disconnect")
	default:
	}
}

func TestClient_DropsSnapshotsOlderThanWhatItHas(t *testing.T) {
	c := &Client{updates: make(chan Update, sendBuffer), done: make(chan struct{})}

	c.offer(Update{Event: "newer", Snapshot: &application.TeamFormationSnapshot{Version: 10}})
	c.offer(Update{Event: "older", Snapshot: &application.TeamFormationSnapshot{Version: 9}})
	c.offer(Update{Event: "same", Snapshot: &application.TeamFormationSnapshot{Version: 10}})

	assert.Equal(t, "newer", next(t, c).Event)
	requireNoUpdate(t, c)
}

func TestClient_SlowConsumerIsDisconnected(t *testing.T) {
	c := &Client{updates: make(chan Update, sendBuffer), done: make(chan struct{})}

	for v := int64(1); v <= sendBuffer+1; v++ {
		c.offer(Update{Snapshot: &application.TeamFormationSnapshot{Version: v}})
	}

	requireClosed(t, c, ReasonSlowConsumer)
}

func TestHub_DisconnectedClientGetsNothingMore(t *testing.T) {
	h, snapshots, _ := newTestHub(t)
	sessionID := uuid.New()
	c := connect(t, h, sessionID, uuid.New())

	h.Disconnect(c)
	h.Disconnect(c) // idempotent
	builds := snapshots.callCount()

	h.HandleEvent(context.Background(), "activity.session.draft.started", payload(t, map[string]any{"session_id": sessionID}))

	assert.Equal(t, builds, snapshots.callCount(), "no clients left, no snapshot built")
	requireNoUpdate(t, c)
}

func TestHub_ShutdownDisconnectsEveryClient(t *testing.T) {
	h, _, _ := newTestHub(t)
	a := connect(t, h, uuid.New(), uuid.New())
	b := connect(t, h, uuid.New(), uuid.New())

	h.Shutdown()
	h.Shutdown() // idempotent

	requireClosed(t, a, ReasonShutdown)
	requireClosed(t, b, ReasonShutdown)
}

func TestHub_ConnectAfterShutdownIsClosedAtOnce(t *testing.T) {
	h, _, _ := newTestHub(t)
	h.Shutdown()

	c, err := h.Connect(context.Background(), uuid.New(), uuid.New())
	require.NoError(t, err)

	requireClosed(t, c, ReasonShutdown)
	requireNoUpdate(t, c)
}
