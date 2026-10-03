package http

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/rasparac/rekreativko-api/activity/internal/application"
	"github.com/rasparac/rekreativko-api/activity/internal/domain"
	"github.com/rasparac/rekreativko-api/activity/internal/interfaces/http/dtos"
	"github.com/rasparac/rekreativko-api/activity/internal/interfaces/live"
	"github.com/rasparac/rekreativko-api/shared/authcontext"
	"github.com/rasparac/rekreativko-api/shared/domainerror"
	"github.com/rasparac/rekreativko-api/shared/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// streamSessions is the part of sessionService the stream uses; any other
// method panics (the embedded interface is nil).
type streamSessions struct {
	sessionService

	session *domain.Session
	mu      sync.Mutex
	hidden  map[uuid.UUID]struct{}
}

func (s *streamSessions) GetSession(_ context.Context, sessionID, requesterID uuid.UUID) (*domain.Session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.hidden[requesterID]; ok || sessionID != s.session.ID() {
		return nil, domainerror.NotFound("session_not_found", "Session not found", domain.ErrSessionNotFound)
	}
	return s.session, nil
}

func (s *streamSessions) hide(userID uuid.UUID) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.hidden[userID] = struct{}{}
}

type streamSnapshots struct {
	session *domain.Session

	mu      sync.Mutex
	version int64
}

func (s *streamSnapshots) Snapshot(context.Context, uuid.UUID) (*application.TeamFormationSnapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.version++
	return &application.TeamFormationSnapshot{
		Session:     s.session,
		TeamMembers: map[uuid.UUID][]uuid.UUID{},
		Voting:      &application.TeamVotingState{},
		Version:     s.version,
	}, nil
}

func newStreamTestSession(t *testing.T) *domain.Session {
	t.Helper()

	title, err := domain.NewTitle("Pickup Game")
	require.NoError(t, err)
	location, err := domain.NewSessionLocation("Belgrade", "RS", "", 44.8, 20.4)
	require.NoError(t, err)
	schedule, err := domain.NewSessionSchedule(time.Now().Add(time.Hour), nil)
	require.NoError(t, err)

	visibility := domain.SessionVisibilityPublic
	session, _, err := domain.NewSession(domain.SessionInput{
		CreatedByID:     uuid.New(),
		Title:           title,
		ActivityType:    domain.ActivityTypeBasketball,
		DifficultyLevel: domain.DifficultyLevelBeginner,
		Location:        location,
		Schedule:        schedule,
		Visibility:      &visibility,
	})
	require.NoError(t, err)

	return session
}

type streamEnv struct {
	server   *httptest.Server
	hub      *live.Hub
	sessions *streamSessions
	session  *domain.Session
}

// newStreamEnv serves the stream through a real server whose WriteTimeout is
// far shorter than the tests, as in production (15s vs a long-lived stream).
func newStreamEnv(t *testing.T) *streamEnv {
	t.Helper()

	orig := sseHeartbeatInterval
	sseHeartbeatInterval = 50 * time.Millisecond
	t.Cleanup(func() { sseHeartbeatInterval = orig })

	log := logger.New("error", "json")
	session := newStreamTestSession(t)
	sessions := &streamSessions{session: session, hidden: map[uuid.UUID]struct{}{}}
	hub := live.NewHub(&streamSnapshots{session: session}, sessions, log)

	h := &Handler{sessionService: sessions, live: hub, logger: log}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/sessions/{id}/events", func(w http.ResponseWriter, r *http.Request) {
		// Stands in for ExtractUserContext: the viewer comes from a test header.
		viewer, _ := uuid.Parse(r.Header.Get("X-Test-Viewer"))
		h.StreamTeamFormation(w, r.WithContext(authcontext.WithAccountID(r.Context(), viewer)))
	})

	server := httptest.NewUnstartedServer(mux)
	server.Config.WriteTimeout = 200 * time.Millisecond
	server.Start()
	t.Cleanup(server.Close)

	return &streamEnv{server: server, hub: hub, sessions: sessions, session: session}
}

func (e *streamEnv) open(t *testing.T, sessionID, viewer uuid.UUID) *http.Response {
	t.Helper()

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, e.server.URL+"/api/v1/sessions/"+sessionID.String()+"/events", nil)
	require.NoError(t, err)
	req.Header.Set("X-Test-Viewer", viewer.String())

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	t.Cleanup(func() { _ = resp.Body.Close() })

	return resp
}

type sseEvent struct {
	id, event, data string
}

// sseReader reads events off a stream, reporting heartbeats separately.
type sseReader struct {
	events chan sseEvent
	pings  chan struct{}
}

func readSSE(resp *http.Response) *sseReader {
	r := &sseReader{events: make(chan sseEvent, 16), pings: make(chan struct{}, 16)}

	go func() {
		defer close(r.events)
		scanner := bufio.NewScanner(resp.Body)
		var ev sseEvent
		for scanner.Scan() {
			line := scanner.Text()
			switch {
			case line == ": ping":
				select {
				case r.pings <- struct{}{}:
				default:
				}
			case strings.HasPrefix(line, "id: "):
				ev.id = strings.TrimPrefix(line, "id: ")
			case strings.HasPrefix(line, "event: "):
				ev.event = strings.TrimPrefix(line, "event: ")
			case strings.HasPrefix(line, "data: "):
				ev.data = strings.TrimPrefix(line, "data: ")
			case line == "" && ev.event != "":
				r.events <- ev
				ev = sseEvent{}
			}
		}
	}()

	return r
}

func (r *sseReader) next(t *testing.T) sseEvent {
	t.Helper()

	select {
	case ev, ok := <-r.events:
		require.True(t, ok, "stream ended")
		return ev
	case <-time.After(3 * time.Second):
		t.Fatal("no event arrived")
		return sseEvent{}
	}
}

func (r *sseReader) requireEnded(t *testing.T) {
	t.Helper()

	select {
	case _, ok := <-r.events:
		require.False(t, ok, "expected the stream to end")
	case <-time.After(3 * time.Second):
		t.Fatal("stream did not end")
	}
}

func TestStreamTeamFormation_SnapshotThenChanges(t *testing.T) {
	env := newStreamEnv(t)
	viewer := uuid.New()

	resp := env.open(t, env.session.ID(), viewer)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "text/event-stream", resp.Header.Get("Content-Type"))
	assert.Equal(t, "no-cache", resp.Header.Get("Cache-Control"))

	stream := readSSE(resp)

	first := stream.next(t)
	assert.Equal(t, "snapshot", first.event)
	assert.Equal(t, "1", first.id, "the id is the snapshot version")
	var firstData dtos.TeamFormationEventData
	require.NoError(t, json.Unmarshal([]byte(first.data), &firstData))
	assert.Equal(t, "null", string(firstData.Change))
	require.NotNil(t, firstData.Snapshot)
	assert.Equal(t, env.session.ID(), firstData.Snapshot.SessionID)

	// Outlive the server's 200ms WriteTimeout before the next change.
	time.Sleep(400 * time.Millisecond)

	picked := uuid.New()
	change := `{"session_id":"` + env.session.ID().String() + `","user_id":"` + picked.String() + `"}`
	env.hub.HandleEvent(context.Background(), "activity.session.draft.player_picked", []byte(change))

	ev := stream.next(t)
	assert.Equal(t, "draft.player_picked", ev.event)
	assert.Equal(t, "2", ev.id)
	var data dtos.TeamFormationEventData
	require.NoError(t, json.Unmarshal([]byte(ev.data), &data))
	assert.JSONEq(t, change, string(data.Change))
	assert.Equal(t, int64(2), data.Snapshot.Version)
}

func TestStreamTeamFormation_Heartbeats(t *testing.T) {
	env := newStreamEnv(t)
	stream := readSSE(env.open(t, env.session.ID(), uuid.New()))
	stream.next(t) // snapshot

	select {
	case <-stream.pings:
	case <-time.After(3 * time.Second):
		t.Fatal("no heartbeat")
	}
}

func TestStreamTeamFormation_RemovedViewerIsTold(t *testing.T) {
	env := newStreamEnv(t)
	viewer := uuid.New()
	stream := readSSE(env.open(t, env.session.ID(), viewer))
	stream.next(t) // snapshot

	env.sessions.hide(viewer)
	removal := `{"session_id":"` + env.session.ID().String() + `","user_id":"` + viewer.String() + `"}`
	env.hub.HandleEvent(context.Background(), "activity.session.attendee.removed", []byte(removal))

	ev := stream.next(t)
	assert.Equal(t, "disconnected", ev.event)
	assert.JSONEq(t, `{"reason":"removed"}`, ev.data)
	stream.requireEnded(t)

	// Reconnecting fails the ordinary way.
	resp := env.open(t, env.session.ID(), viewer)
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

func TestStreamTeamFormation_InvisibleSessionIsNotFound(t *testing.T) {
	env := newStreamEnv(t)

	resp := env.open(t, uuid.New(), uuid.New())

	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	assert.NotEqual(t, "text/event-stream", resp.Header.Get("Content-Type"))
}
