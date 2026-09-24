package application

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/rasparac/rekreativko-api/activity/internal/domain"
	"github.com/rasparac/rekreativko-api/activity/internal/infrastructure/persistence"
)

const activitySchema = "activity"

// ActivityGroupRepository defines the interface for activity group persistence
type ActivityGroupRepository interface {
	CreateActivityGroup(ctx context.Context, group *domain.ActivityGroup) error
	UpdateActivityGroup(ctx context.Context, group *domain.ActivityGroup) error
	CancelActivityGroup(ctx context.Context, group *domain.ActivityGroup) error
	DeleteActivityGroup(ctx context.Context, group *domain.ActivityGroup) error
	GetActivityGroupByID(ctx context.Context, id uuid.UUID) (*domain.ActivityGroup, error)
	ListActivityGroups(ctx context.Context, filter persistence.ActivityGroupFilter) ([]*domain.ActivityGroup, string, error)
	DiscoverGroups(ctx context.Context, filter persistence.DiscoveryFilter) ([]*domain.ActivityGroup, string, error)
}

// MemberRepository defines the interface for member persistence
type MemberRepository interface {
	CreateMember(ctx context.Context, member *domain.Member) error
	UpdateMember(ctx context.Context, member *domain.Member) error
	GetMemberByID(ctx context.Context, id uuid.UUID) (*domain.Member, error)
	GetMemberByGroupAndUser(ctx context.Context, activityGroupID, userID uuid.UUID) (*domain.Member, error)
	ListMembers(ctx context.Context, filter persistence.MemberFilter) ([]*domain.Member, string, error)
	DeleteMember(ctx context.Context, id uuid.UUID) error
	CountConfirmedMembers(ctx context.Context, activityGroupID uuid.UUID) (int, error)
}

// GroupInviteRepository defines the interface for group invite persistence
type GroupInviteRepository interface {
	CreateInvite(ctx context.Context, invite *domain.GroupInvite) error
	UpdateInvite(ctx context.Context, invite *domain.GroupInvite) error
	GetInviteByID(ctx context.Context, id uuid.UUID) (*domain.GroupInvite, error)
	GetPendingInviteByGroupAndUser(ctx context.Context, activityGroupID, invitedUserID uuid.UUID) (*domain.GroupInvite, error)
	ListPendingInvitesForUser(ctx context.Context, filter persistence.ListPendingInvitesFilter) ([]*domain.GroupInvite, string, error)
	FindExpiredPendingInvites(ctx context.Context) ([]*domain.GroupInvite, error)
}

// SessionInviteRepository defines the interface for session invite persistence
type SessionInviteRepository interface {
	CreateInvite(ctx context.Context, invite *domain.SessionInvite) error
	UpdateInvite(ctx context.Context, invite *domain.SessionInvite) error
	GetInviteByID(ctx context.Context, id uuid.UUID) (*domain.SessionInvite, error)
	GetPendingInviteBySessionAndUser(ctx context.Context, sessionID, invitedUserID uuid.UUID) (*domain.SessionInvite, error)
	ListPendingInvitesForUser(ctx context.Context, filter persistence.ListPendingInvitesFilter) ([]*domain.SessionInvite, string, error)
	FindExpiredPendingInvites(ctx context.Context) ([]*domain.SessionInvite, error)
}

// SessionRepository defines the interface for session persistence
type SessionRepository interface {
	CreateSession(ctx context.Context, session *domain.Session) error
	UpdateSession(ctx context.Context, session *domain.Session) error
	GetSessionByID(ctx context.Context, id uuid.UUID) (*domain.Session, error)
	ListSessions(ctx context.Context, filter persistence.SessionFilter) ([]*domain.Session, string, error)
	DiscoverSessions(ctx context.Context, filter persistence.DiscoverSessionsFilter) ([]persistence.SessionWithDistance, string, error)
	DeleteSession(ctx context.Context, id uuid.UUID) error
	FindSessionsPastEndTime(ctx context.Context) ([]*domain.Session, error)
}

// SessionTemplateRepository defines the interface for session template persistence
type SessionTemplateRepository interface {
	CreateSessionTemplate(ctx context.Context, template *domain.SessionTemplate) error
	UpdateSessionTemplate(ctx context.Context, template *domain.SessionTemplate) error
	UpdateGeneratedUpTo(ctx context.Context, templateID uuid.UUID, generatedUpTo time.Time) error
	DeleteSessionTemplate(ctx context.Context, templateID uuid.UUID) error
	GetSessionTemplateByID(ctx context.Context, templateID uuid.UUID) (*domain.SessionTemplate, error)
	ListSessionTemplates(ctx context.Context, filter persistence.SessionTemplateFilter) ([]*domain.SessionTemplate, string, error)
	FindRecurringTemplatesToGenerate(ctx context.Context, lookaheadWindow time.Duration) ([]*domain.SessionTemplate, error)
}

// AttendeeRepository defines the interface for attendee persistence
type AttendeeRepository interface {
	CreateAttendee(ctx context.Context, attendee *domain.Attendee) error
	UpdateAttendee(ctx context.Context, attendee *domain.Attendee) error
	GetAttendeeByID(ctx context.Context, id uuid.UUID) (*domain.Attendee, error)
	GetAttendeeBySessionAndUser(ctx context.Context, sessionID, userID uuid.UUID) (*domain.Attendee, error)
	ListAttendees(ctx context.Context, filter persistence.AttendeeFilter) ([]*domain.Attendee, string, error)
	DeleteAttendee(ctx context.Context, id uuid.UUID) error
	GetFirstPendingAttendee(ctx context.Context, sessionID uuid.UUID) (*domain.Attendee, error)
	CountConfirmedAttendees(ctx context.Context, sessionID uuid.UUID) (int, error)
	GetAttendeeStatusesForUser(ctx context.Context, userID uuid.UUID, sessionIDs []uuid.UUID) (map[uuid.UUID]domain.AttendeeStatus, error)
}
