package application

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/rasparac/rekreativko-api/activity/internal/domain"
	"github.com/rasparac/rekreativko-api/activity/internal/infrastructure/persistence"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// snapshotFakes lets a test pause the first GetSessionByID call, to make one
// Snapshot start before another and finish after it.
type snapshotFakes struct {
	SessionRepository
	AttendeeRepository

	session *domain.Session

	mu        sync.Mutex
	attendees []*domain.Attendee
	calls     int

	entered chan struct{} // closed when the first GetSessionByID call is waiting
	release chan struct{} // closing it lets that call go on
}

func (f *snapshotFakes) GetSessionByID(context.Context, uuid.UUID) (*domain.Session, error) {
	f.mu.Lock()
	f.calls++
	first := f.calls == 1
	f.mu.Unlock()

	if first {
		close(f.entered)
		<-f.release
	}

	return f.session, nil
}

func (f *snapshotFakes) ListAttendees(context.Context, persistence.AttendeeFilter) ([]*domain.Attendee, string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	return append([]*domain.Attendee(nil), f.attendees...), "", nil
}

func (f *snapshotFakes) addGoing(userID uuid.UUID) {
	f.mu.Lock()
	defer f.mu.Unlock()

	now := time.Now().UTC()
	f.attendees = append(f.attendees, domain.ReconstructAttendee(
		uuid.New(), f.session.ID(), nil, userID,
		domain.AttendeeStatusGoing, domain.AttendeeSourceRSVPManual, nil, now, now,
	))
}

type noDraft struct{}

func (noDraft) GetDraft(context.Context, uuid.UUID) (*TeamDraftState, error) {
	return nil, domain.ErrDraftNotFound
}

type noVoting struct{}

func (noVoting) GetVoting(context.Context, uuid.UUID) (*TeamVotingState, error) {
	return &TeamVotingState{}, nil
}

func newStandaloneSession(t *testing.T) *domain.Session {
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

// Clients drop a snapshot whose version is not above the one they have, so a
// snapshot that reflects more recent state must never carry a lower version.
func TestTeamFormationQuery_ASnapshotOfNewerStateHasAHigherVersion(t *testing.T) {
	fakes := &snapshotFakes{
		session: newStandaloneSession(t),
		entered: make(chan struct{}),
		release: make(chan struct{}),
	}
	query := NewTeamFormationQuery(fakes, nil, fakes, noDraft{}, noVoting{})
	sessionID := fakes.session.ID()

	// Snapshot A starts first but is held up before it reads anything.
	var (
		older    *TeamFormationSnapshot
		olderErr error
		wg       sync.WaitGroup
	)
	wg.Go(func() {
		older, olderErr = query.Snapshot(context.Background(), sessionID)
	})
	<-fakes.entered

	// Snapshot B starts later and reads the state as it is: nobody is going yet.
	time.Sleep(5 * time.Millisecond)
	earlierState, err := query.Snapshot(context.Background(), sessionID)
	require.NoError(t, err)
	require.Empty(t, earlierState.GoingUserIDs)

	// Someone joins, then A goes on and reads the state with them in it.
	fakes.addGoing(uuid.New())
	close(fakes.release)
	wg.Wait()
	require.NoError(t, olderErr)
	require.Len(t, older.GoingUserIDs, 1, "A read after the join, so it holds the newer state")

	assert.Greater(t, older.Version, earlierState.Version,
		"the snapshot with the newer state (someone joined) has a lower version, so a client that already has the stale one ignores it")
}
