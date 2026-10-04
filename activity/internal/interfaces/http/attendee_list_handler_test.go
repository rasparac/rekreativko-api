package http

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/rasparac/rekreativko-api/activity/internal/application"
	"github.com/rasparac/rekreativko-api/activity/internal/domain"
	"github.com/rasparac/rekreativko-api/shared/authcontext"
	"github.com/rasparac/rekreativko-api/shared/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// listingAttendees is the part of attendeeService the list uses; any other
// method panics (the embedded interface is nil).
type listingAttendees struct {
	attendeeService

	got   *application.ListRSVPsParams
	calls int
}

func (l *listingAttendees) ListRSVPs(_ context.Context, params application.ListRSVPsParams) ([]*domain.Attendee, string, error) {
	l.calls++
	l.got = &params
	return nil, "", nil
}

func listAttendeesRequest(t *testing.T, sessions *streamSessions, attendees *listingAttendees, sessionID, viewer uuid.UUID) *httptest.ResponseRecorder {
	t.Helper()

	h := &Handler{sessionService: sessions, attendeeService: attendees, logger: logger.New("error", "json")}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/sessions/{sessionId}/attendees", func(w http.ResponseWriter, r *http.Request) {
		h.ListAttendees(w, r.WithContext(authcontext.WithAccountID(r.Context(), viewer)))
	})

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/sessions/"+sessionID.String()+"/attendees", nil))
	return rec
}

func TestListAttendees_PassesTheRequesterToTheService(t *testing.T) {
	session := newStreamTestSession(t)
	sessions := &streamSessions{session: session, hidden: map[uuid.UUID]struct{}{}}
	attendees := &listingAttendees{}
	viewer := uuid.New()

	rec := listAttendeesRequest(t, sessions, attendees, session.ID(), viewer)

	require.Equal(t, http.StatusOK, rec.Code)
	require.NotNil(t, attendees.got)
	assert.Equal(t, viewer, attendees.got.RequesterID, "the service decides what this person may see")
	assert.Equal(t, session.ID(), *attendees.got.SessionID)
}

func TestListAttendees_InvisibleSessionIsNotFoundAndNeverListed(t *testing.T) {
	session := newStreamTestSession(t)
	sessions := &streamSessions{session: session, hidden: map[uuid.UUID]struct{}{}}
	attendees := &listingAttendees{}
	stranger := uuid.New()
	sessions.hide(stranger)

	rec := listAttendeesRequest(t, sessions, attendees, session.ID(), stranger)

	assert.Equal(t, http.StatusNotFound, rec.Code)
	assert.Zero(t, attendees.calls, "nothing is read for someone who cannot see the session")
}
