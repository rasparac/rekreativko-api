package application

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/rasparac/rekreativko-api/activity/internal/domain"
	"github.com/rasparac/rekreativko-api/activity/internal/infrastructure/persistence"
)

// TeamFormationSnapshot is a session's whole team-formation state at one
// moment: its teams with their members, the latest captain draft and the
// latest voting round. GET /sessions/{id}/team-formation returns it, and the
// session's event stream pushes it on every change.
type TeamFormationSnapshot struct {
	Session *domain.Session
	// TeamMembers is keyed by team ID.
	TeamMembers map[uuid.UUID][]uuid.UUID
	// GoingUserIDs are the people going (going/promoted attendees), in join
	// order - with TeamMembers a client can render the teams and who is still
	// unassigned from this one document. Waitlisted and pending people are
	// deliberately not here: only managers may learn about those.
	GoingUserIDs []uuid.UUID
	// Draft is nil when the session never had a draft.
	Draft  *TeamDraftState
	Voting *TeamVotingState
	// Version orders snapshots of the same session - a higher version is newer
	// state. It is the build time in microseconds: a snapshot reflects every
	// change committed before it was built.
	Version int64
}

type (
	teamDraftReader interface {
		GetDraft(ctx context.Context, sessionID uuid.UUID) (*TeamDraftState, error)
	}

	teamVotingReader interface {
		GetVoting(ctx context.Context, sessionID uuid.UUID) (*TeamVotingState, error)
	}
)

// TeamFormationQuery builds TeamFormationSnapshots. Read-only, and it does no
// visibility check of its own - callers check the session through
// SessionService.GetSession first.
type TeamFormationQuery struct {
	sessionRepo  SessionRepository
	memberRepo   MemberRepository
	attendeeRepo AttendeeRepository
	drafts       teamDraftReader
	voting       teamVotingReader
}

func NewTeamFormationQuery(
	sessionRepo SessionRepository,
	memberRepo MemberRepository,
	attendeeRepo AttendeeRepository,
	drafts teamDraftReader,
	voting teamVotingReader,
) *TeamFormationQuery {
	return &TeamFormationQuery{
		sessionRepo:  sessionRepo,
		memberRepo:   memberRepo,
		attendeeRepo: attendeeRepo,
		drafts:       drafts,
		voting:       voting,
	}
}

// Snapshot returns the session's current team-formation state.
func (q *TeamFormationQuery) Snapshot(ctx context.Context, sessionID uuid.UUID) (*TeamFormationSnapshot, error) {
	version := time.Now().UnixMicro()

	session, err := q.sessionRepo.GetSessionByID(ctx, sessionID)
	if err != nil {
		return nil, mapToAppErr(err)
	}

	snapshot := &TeamFormationSnapshot{
		Session:     session,
		TeamMembers: map[uuid.UUID][]uuid.UUID{},
		Version:     version,
	}

	attendees, _, err := q.attendeeRepo.ListAttendees(ctx, persistence.AttendeeFilter{SessionID: &sessionID})
	if err != nil {
		return nil, mapToAppErr(fmt.Errorf("list attendees: %w", err))
	}
	snapshot.GoingUserIDs = []uuid.UUID{}
	for _, a := range attendees {
		if a.Status().IsConfirmed() {
			snapshot.GoingUserIDs = append(snapshot.GoingUserIDs, a.UserID())
		}
	}

	if session.HasTeams() {
		members, err := q.attendeeRepo.ListTeamMembers(ctx, sessionID)
		if err != nil {
			return nil, mapToAppErr(fmt.Errorf("list team members: %w", err))
		}
		snapshot.TeamMembers = members
	}

	draft, err := q.drafts.GetDraft(ctx, sessionID)
	switch {
	case err == nil:
		snapshot.Draft = draft
	case !errors.Is(err, domain.ErrDraftNotFound):
		return nil, err
	}

	snapshot.Voting, err = q.voting.GetVoting(ctx, sessionID)
	if err != nil {
		return nil, err
	}

	return snapshot, nil
}

// SessionManagers returns who can manage the session: its creator and the
// group's confirmed admins/creator. The live stream uses it to decide who may
// see manager-only events.
func (q *TeamFormationQuery) SessionManagers(ctx context.Context, sessionID uuid.UUID) ([]uuid.UUID, error) {
	session, err := q.sessionRepo.GetSessionByID(ctx, sessionID)
	if err != nil {
		return nil, mapToAppErr(err)
	}

	return sessionManagerIDs(ctx, q.memberRepo, session)
}
