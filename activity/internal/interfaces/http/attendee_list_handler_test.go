package http

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/rasparac/rekreativko-api/activity/internal/application"
	"github.com/rasparac/rekreativko-api/activity/internal/domain"
	"github.com/rasparac/rekreativko-api/shared/authcontext"
	"github.com/rasparac/rekreativko-api/shared/domainerror"
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

	result []*domain.Attendee
	err    error
}

func (l *listingAttendees) ListRSVPs(_ context.Context, params application.ListRSVPsParams) ([]*domain.Attendee, error) {
	l.calls++
	l.got = &params
	return l.result, l.err
}

func listAttendeesRequest(t *testing.T, sessions *streamSessions, attendees *listingAttendees, sessionID, viewer uuid.UUID) *httptest.ResponseRecorder {
	t.Helper()
	return listAttendeesRequestWithQuery(t, sessions, attendees, sessionID, viewer, "")
}

func listAttendeesRequestWithQuery(t *testing.T, sessions *streamSessions, attendees *listingAttendees, sessionID, viewer uuid.UUID, query string) *httptest.ResponseRecorder {
	t.Helper()

	h := &Handler{sessionService: sessions, attendeeService: attendees, logger: logger.New("error", "json")}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/sessions/{sessionId}/attendees", func(w http.ResponseWriter, r *http.Request) {
		h.ListAttendees(w, r.WithContext(authcontext.WithAccountID(r.Context(), viewer)))
	})

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/sessions/"+sessionID.String()+"/attendees"+query, nil))
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

func TestListAttendees_SeveralStatusesInOneRequest(t *testing.T) {
	session := newStreamTestSession(t)
	sessions := &streamSessions{session: session, hidden: map[uuid.UUID]struct{}{}}
	attendees := &listingAttendees{}

	rec := listAttendeesRequestWithQuery(t, sessions, attendees, session.ID(), uuid.New(), "?status=going&status=promoted&status=pending")

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, []string{"going", "promoted", "pending"}, attendees.got.Statuses)
}

func TestListAttendees_IsOneListWithoutPagination(t *testing.T) {
	session := newStreamTestSession(t)
	sessions := &streamSessions{session: session, hidden: map[uuid.UUID]struct{}{}}
	attendee, err := domain.NewRSVPManualAttendee(session, session.ActivityGroupID(), uuid.New(), domain.AttendeeStatusGoing, 0)
	require.NoError(t, err)
	attendees := &listingAttendees{result: []*domain.Attendee{attendee}}

	// limit and page_token from an old client are ignored, not an error
	rec := listAttendeesRequestWithQuery(t, sessions, attendees, session.ID(), uuid.New(), "?limit=1&page_token=abc")

	require.Equal(t, http.StatusOK, rec.Code)

	var body struct {
		Data map[string]json.RawMessage `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Contains(t, body.Data, "items")
	assert.NotContains(t, body.Data, "next_page_token")
	assert.NotContains(t, body.Data, "limit")
}

func TestListAttendees_UnknownStatusIsUnprocessable(t *testing.T) {
	session := newStreamTestSession(t)
	sessions := &streamSessions{session: session, hidden: map[uuid.UUID]struct{}{}}
	attendees := &listingAttendees{err: domainerror.ValidationError("invalid_attendee_status", "Invalid attendee status", domain.ErrInvalidAttendeeStatus)}

	rec := listAttendeesRequestWithQuery(t, sessions, attendees, session.ID(), uuid.New(), "?status=attending")

	assert.Equal(t, http.StatusUnprocessableEntity, rec.Code)
	assert.Contains(t, rec.Body.String(), "invalid_attendee_status")
}
