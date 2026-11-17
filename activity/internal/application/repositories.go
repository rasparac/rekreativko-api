package application

import (
	"context"
	"database/sql"
	"time"

	"github.com/google/uuid"
	"github.com/rasparac/rekreativko-api/activity/internal/domain"
	"github.com/rasparac/rekreativko-api/shared/store/postgres"
)

// The filters and results below are part of the repository contracts, so they
// live here with the interfaces; persistence implements against them.

type (
	ActivityGroupFilter struct {
		ID              uuid.UUID
		CreatorID       uuid.UUID
		Title           string
		Status          domain.ActivityGroupStatus
		ActivityType    domain.ActivityType
		DifficultyLevel domain.DifficultyLevel
		// MemberID filters to groups this account has a membership row in.
		// MemberStatuses restricts which membership statuses count (e.g.
		// "pending" for outstanding join requests); the application layer
		// defaults it to ["confirmed"] when not otherwise specified, so
		// MemberID alone means "groups this account is a confirmed member
		// of" (includes ones they created, since the creator is auto-added
		// as a confirmed member) - existing callers see no behavior change.
		MemberID       uuid.UUID
		MemberStatuses []domain.MemberStatus
		// RequesterID scopes results to what this caller may actually see:
		// public groups, plus any private group they created or are a
		// confirmed member of. A private group they have no relationship to
		// is excluded entirely, not just access-denied on direct fetch. Note
		// this means a private group can still surface via someone else's
		// MemberID/CreatorID if the requester happens to independently be a
		// member of that same group too - Visibility below is the harder cap
		// the application layer uses to rule that out for profile-view.
		RequesterID uuid.UUID
		// Visibility, when set, restricts to that visibility only - used by
		// the application layer to force "public only" when MemberID or
		// CreatorID points at someone other than the requester, the same cap
		// SessionFilter applies for a session's AttendeeID/CreatedByID.
		Visibility domain.ActivityGroupVisibility
		Limit      int
		PageToken  string
	}

	DiscoveryFilter struct {
		City      string
		Country   string
		Interests []postgres.InterestPair
		// RequesterID, when set, hides groups created by accounts the
		// requester blocked (unless the requester is a member).
		RequesterID uuid.UUID
		Limit       int
		PageToken   string
	}

	// MemberFilter defines query filters for listing members
	MemberFilter struct {
		ActivityGroupID *uuid.UUID
		UserID          *uuid.UUID
		Status          *domain.MemberStatus
		Role            *domain.MemberRole
		Limit           int
		PageToken       string
	}

	// ListPendingInvitesFilter contains parameters for listing a user's pending invites
	ListPendingInvitesFilter struct {
		InvitedUserID uuid.UUID
		Limit         int
		PageToken     string
	}

	// SessionTimeframe restricts a session listing by whether the sessions have
	// ended (domain.SessionSchedule.EffectiveEndTime).
	SessionTimeframe string

	// SessionFilter defines query filters for listing sessions
	SessionFilter struct {
		Timeframe         SessionTimeframe
		ActivityGroupID   *uuid.UUID
		SessionTemplateID *uuid.UUID
		CreatedByID       *uuid.UUID
		Status            *domain.SessionStatus
		Visibility        *domain.SessionVisibility
		ActivityType      *domain.ActivityType
		DifficultyLevel   *domain.DifficultyLevel
		IsRecurring       *bool
		StartTimeFrom     *sql.NullTime
		StartTimeTo       *sql.NullTime
		// AttendeeID filters to sessions this account has RSVP'd to. AttendeeStatuses,
		// when non-empty, further restricts to those RSVP statuses (e.g. "going"/
		// "promoted" for "currently holds a spot"); empty means any status.
		AttendeeID       *uuid.UUID
		AttendeeStatuses []domain.AttendeeStatus
		// RequesterID scopes results to what this caller may actually see: public
		// sessions, plus any private session they created, are an attendee of,
		// hold a pending unexpired invite to, or belong to a group they're a
		// confirmed member of. Mirrors
		// ActivityGroupFilter.RequesterID - applied unconditionally, regardless of
		// which other filters (ActivityGroupID, CreatedByID, AttendeeID, ...) are
		// also set, so a private session an unrelated caller has no relationship
		// to is excluded from every listing shape, not just direct fetch.
		RequesterID uuid.UUID
		Limit       int
		PageToken   string
	}

	// DiscoverSessionsFilter defines query filters for finding public sessions near a location
	DiscoverSessionsFilter struct {
		Lat       float64
		Lng       float64
		RadiusKM  float64
		Interests []postgres.InterestPair
		// RequesterID, when set, hides sessions created by accounts the
		// requester blocked (unless the requester already takes part).
		RequesterID uuid.UUID
		Limit       int
		PageToken   string
	}

	// SessionWithDistance pairs a session with its distance from the search location
	SessionWithDistance struct {
		Session    *domain.Session
		DistanceKM float64
	}

	SessionTemplateFilter struct {
		ActivityGroupID *uuid.UUID
		CreatedByID     *uuid.UUID
		Status          *domain.SessionTemplateStatus
		IsRecurring     *bool // filter recurring vs non-recurring templates

		// For cron job: find templates that need session generation
		NeedsGeneration *bool          // templates where generated_up_to < NOW() + lookahead
		LookaheadWindow *time.Duration // how far ahead to generate (e.g., 7 days)

		// Pagination
		Limit     int
		PageToken string
	}

	// AttendeeFilter defines query filters for listing attendees
	AttendeeFilter struct {
		SessionID       *uuid.UUID
		ActivityGroupID *uuid.UUID
		UserID          *uuid.UUID
		Status          *domain.AttendeeStatus
		// Statuses limits the list to these statuses (any of them); combine with
		// Status and nothing matches unless it is among them.
		Statuses  []domain.AttendeeStatus
		Source    *domain.AttendeeSource
		Limit     int
		PageToken string
	}
)

const (
	// SessionTimeframeAll applies no time restriction; sorted soonest first.
	SessionTimeframeAll SessionTimeframe = ""
	// SessionTimeframeUpcoming is sessions that have not ended yet (running ones
	// included), sorted soonest first.
	SessionTimeframeUpcoming SessionTimeframe = "upcoming"
	// SessionTimeframePast is sessions that have ended, most recent first.
	SessionTimeframePast SessionTimeframe = "past"
)

// ActivityGroupRepository defines the interface for activity group persistence
type ActivityGroupRepository interface {
	CreateActivityGroup(ctx context.Context, group *domain.ActivityGroup) error
	UpdateActivityGroup(ctx context.Context, group *domain.ActivityGroup) error
	CancelActivityGroup(ctx context.Context, group *domain.ActivityGroup) error
	DeleteActivityGroup(ctx context.Context, group *domain.ActivityGroup) error
	GetActivityGroupByID(ctx context.Context, id uuid.UUID) (*domain.ActivityGroup, error)
	ListActivityGroups(ctx context.Context, filter ActivityGroupFilter) ([]*domain.ActivityGroup, string, error)
	DiscoverGroups(ctx context.Context, filter DiscoveryFilter) ([]*domain.ActivityGroup, string, error)
}

// MemberRepository defines the interface for member persistence
type MemberRepository interface {
	CreateMember(ctx context.Context, member *domain.Member) error
	UpdateMember(ctx context.Context, member *domain.Member) error
	GetMemberByID(ctx context.Context, id uuid.UUID) (*domain.Member, error)
	GetMemberByGroupAndUser(ctx context.Context, activityGroupID, userID uuid.UUID) (*domain.Member, error)
	ListMembers(ctx context.Context, filter MemberFilter) ([]*domain.Member, string, error)
	DeleteMember(ctx context.Context, id uuid.UUID) error
	CountConfirmedMembers(ctx context.Context, activityGroupID uuid.UUID) (int, error)
}

// GroupInviteRepository defines the interface for group invite persistence
type GroupInviteRepository interface {
	CreateInvite(ctx context.Context, invite *domain.GroupInvite) error
	UpdateInvite(ctx context.Context, invite *domain.GroupInvite) error
	GetInviteByID(ctx context.Context, id uuid.UUID) (*domain.GroupInvite, error)
	GetPendingInviteByGroupAndUser(ctx context.Context, activityGroupID, invitedUserID uuid.UUID) (*domain.GroupInvite, error)
	ListPendingInvitesForUser(ctx context.Context, filter ListPendingInvitesFilter) ([]*domain.GroupInvite, string, error)
	FindExpiredPendingInvites(ctx context.Context) ([]*domain.GroupInvite, error)
}

// SessionInviteRepository defines the interface for session invite persistence
type SessionInviteRepository interface {
	CreateInvite(ctx context.Context, invite *domain.SessionInvite) error
	UpdateInvite(ctx context.Context, invite *domain.SessionInvite) error
	GetInviteByID(ctx context.Context, id uuid.UUID) (*domain.SessionInvite, error)
	GetPendingInviteBySessionAndUser(ctx context.Context, sessionID, invitedUserID uuid.UUID) (*domain.SessionInvite, error)
	ListPendingInvitesForUser(ctx context.Context, filter ListPendingInvitesFilter) ([]*domain.SessionInvite, string, error)
	FindExpiredPendingInvites(ctx context.Context) ([]*domain.SessionInvite, error)
}

// SessionRepository defines the interface for session persistence
type SessionRepository interface {
	CreateSession(ctx context.Context, session *domain.Session) error
	UpdateSession(ctx context.Context, session *domain.Session) error
	GetSessionByID(ctx context.Context, id uuid.UUID) (*domain.Session, error)
	GetFormationVersion(ctx context.Context, sessionID uuid.UUID) (int64, error)
	IsSessionVisibleTo(ctx context.Context, sessionID, userID uuid.UUID) (bool, error)
	ListSessions(ctx context.Context, filter SessionFilter) ([]*domain.Session, string, error)
	DiscoverSessions(ctx context.Context, filter DiscoverSessionsFilter) ([]SessionWithDistance, string, error)
	DeleteSession(ctx context.Context, id uuid.UUID) error
	FindSessionsPastEndTime(ctx context.Context) ([]*domain.Session, error)
	ListActiveSessionIDsByGroup(ctx context.Context, activityGroupID uuid.UUID) ([]uuid.UUID, error)
	SoftDeleteGroupSessions(ctx context.Context, activityGroupID uuid.UUID) (int64, error)
	ReplaceTeams(ctx context.Context, session *domain.Session) error
}

// SessionTemplateRepository defines the interface for session template persistence
type SessionTemplateRepository interface {
	CreateSessionTemplate(ctx context.Context, template *domain.SessionTemplate) error
	UpdateSessionTemplate(ctx context.Context, template *domain.SessionTemplate) error
	UpdateGeneratedUpTo(ctx context.Context, templateID uuid.UUID, generatedUpTo time.Time) error
	DeleteSessionTemplate(ctx context.Context, templateID uuid.UUID) error
	GetSessionTemplateByID(ctx context.Context, templateID uuid.UUID) (*domain.SessionTemplate, error)
	ListSessionTemplates(ctx context.Context, filter SessionTemplateFilter) ([]*domain.SessionTemplate, string, error)
	FindRecurringTemplatesToGenerate(ctx context.Context, lookaheadWindow time.Duration) ([]*domain.SessionTemplate, error)
}

// TeamDraftRepository defines the interface for captain draft persistence
type TeamDraftRepository interface {
	CreateDraft(ctx context.Context, draft *domain.TeamDraft) error
	UpdateDraft(ctx context.Context, draft *domain.TeamDraft) error
	GetActiveDraft(ctx context.Context, sessionID uuid.UUID) (*domain.TeamDraft, error)
	GetLatestDraft(ctx context.Context, sessionID uuid.UUID) (*domain.TeamDraft, error)
	// DeleteSessionDrafts removes every draft of the session, picks included,
	// so none is reported as its latest draft (a team formation reset).
	DeleteSessionDrafts(ctx context.Context, sessionID uuid.UUID) error
}

// TeamVotingRepository defines the interface for team voting persistence
type TeamVotingRepository interface {
	CreateRound(ctx context.Context, round *domain.TeamVotingRound) error
	UpdateRound(ctx context.Context, round *domain.TeamVotingRound) error
	// AddProposal saves the round's state and the one proposal added to it.
	AddProposal(ctx context.Context, round *domain.TeamVotingRound, proposal *domain.TeamProposal) error
	// SaveVote saves the round's state and voterID's vote, removing the vote
	// when the round no longer has one from them.
	SaveVote(ctx context.Context, round *domain.TeamVotingRound, voterID uuid.UUID) error
	GetOpenRound(ctx context.Context, sessionID uuid.UUID) (*domain.TeamVotingRound, error)
	GetLatestRound(ctx context.Context, sessionID uuid.UUID) (*domain.TeamVotingRound, error)
	// MarkTieNotified records the tie (see domain.TieError.Key) the managers
	// were told about. It reports false when the round already has that key.
	MarkTieNotified(ctx context.Context, roundID uuid.UUID, tieKey string) (bool, error)
	// DeleteSessionRounds removes every round of the session with whatever it
	// still holds, so none is reported as its latest round (a team formation
	// reset).
	DeleteSessionRounds(ctx context.Context, sessionID uuid.UUID) error
}

// AttendeeRepository defines the interface for attendee persistence
type AttendeeRepository interface {
	CreateAttendee(ctx context.Context, attendee *domain.Attendee) error
	UpdateAttendee(ctx context.Context, attendee *domain.Attendee) error
	GetAttendeeByID(ctx context.Context, id uuid.UUID) (*domain.Attendee, error)
	GetAttendeeBySessionAndUser(ctx context.Context, sessionID, userID uuid.UUID) (*domain.Attendee, error)
	ListAttendees(ctx context.Context, filter AttendeeFilter) ([]*domain.Attendee, string, error)
	DeleteAttendee(ctx context.Context, id uuid.UUID) error
	GetFirstPendingAttendee(ctx context.Context, sessionID uuid.UUID) (*domain.Attendee, error)
	CountConfirmedAttendees(ctx context.Context, sessionID uuid.UUID) (int, error)
	GetAttendeeStatusesForUser(ctx context.Context, userID uuid.UUID, sessionIDs []uuid.UUID) (map[uuid.UUID]domain.AttendeeStatus, error)
	ListTeamMembers(ctx context.Context, sessionID uuid.UUID) (map[uuid.UUID][]uuid.UUID, error)
}

// ModerationRepository stores reports and blocks.
type ModerationRepository interface {
	// CreateReport returns false, storing nothing, when the reporter already
	// has an open report on that target.
	CreateReport(ctx context.Context, report *domain.Report) (bool, error)
	Block(ctx context.Context, block *domain.AccountBlock) error
	Unblock(ctx context.Context, blockerID, blockedID uuid.UUID) error
	ListBlocked(ctx context.Context, blockerID uuid.UUID, limit int) ([]*domain.AccountBlock, error)
	IsBlockedEitherWay(ctx context.Context, a, b uuid.UUID) (bool, error)
}

// blockChecker tells whether either of two accounts has blocked the other.
type blockChecker interface {
	IsBlockedEitherWay(ctx context.Context, a, b uuid.UUID) (bool, error)
}
