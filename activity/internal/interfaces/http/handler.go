package http

import (
	"context"
	"net/http"

	"github.com/google/uuid"
	"github.com/rasparac/rekreativko-api/activity/internal/application"
	"github.com/rasparac/rekreativko-api/activity/internal/domain"
	"github.com/rasparac/rekreativko-api/shared/logger"
	"github.com/rasparac/rekreativko-api/shared/middleware"
)

type (
	// sessionTemplateService defines the interface for session template operations
	sessionTemplateService interface {
		CreateSessionTemplate(ctx context.Context, params application.CreateSessionTemplateParams) (*domain.SessionTemplate, error)
		GetSessionTemplate(ctx context.Context, templateID uuid.UUID) (*domain.SessionTemplate, error)
		UpdateSessionTemplate(ctx context.Context, templateID uuid.UUID, params application.UpdateSessionTemplateParams) error
		ActivateSessionTemplate(ctx context.Context, templateID uuid.UUID) error
		DeactivateSessionTemplate(ctx context.Context, templateID uuid.UUID) error
		DeleteSessionTemplate(ctx context.Context, templateID uuid.UUID) error
		ListSessionTemplates(ctx context.Context, params application.ListSessionTemplatesParams) ([]*domain.SessionTemplate, error)
	}

	// activityGroupService defines the interface for activity group operations
	activityGroupService interface {
		CreateActivityGroup(ctx context.Context, params application.CreateActivityGroupParams) (*domain.ActivityGroup, error)
		GetActivityGroup(ctx context.Context, groupID uuid.UUID) (*domain.ActivityGroup, error)
		UpdateActivityGroup(ctx context.Context, groupID uuid.UUID, params application.UpdateActivityGroupParams) error
		ActivateActivityGroup(ctx context.Context, groupID uuid.UUID, requesterID uuid.UUID) error
		CancelActivityGroup(ctx context.Context, groupID uuid.UUID, requesterID uuid.UUID, reason string) error
		ListActivityGroups(ctx context.Context, params application.ListActivityGroupsParams) ([]*domain.ActivityGroup, error)
		DiscoverActivityGroups(ctx context.Context, params application.DiscoverActivityGroupsParams) ([]*domain.ActivityGroup, error)
	}

	// sessionService defines the interface for session operations
	sessionService interface {
		CreateSession(ctx context.Context, params application.CreateSessionParams) (*domain.Session, error)
		GetSession(ctx context.Context, sessionID uuid.UUID) (*domain.Session, error)
		UpdateSession(ctx context.Context, sessionID uuid.UUID, params application.UpdateSessionParams) error
		StartSession(ctx context.Context, sessionID uuid.UUID, requesterID uuid.UUID, requesterRole string) error
		CompleteSession(ctx context.Context, sessionID uuid.UUID, requesterID uuid.UUID, requesterRole string) error
		CancelSession(ctx context.Context, sessionID uuid.UUID, requesterID uuid.UUID, requesterRole string, reason string) error
		ListSessions(ctx context.Context, params application.ListSessionsParams) ([]*domain.Session, error)
	}

	// memberService defines the interface for member operations
	memberService interface {
		InviteMember(ctx context.Context, params application.InviteMemberParams) (*domain.Member, error)
		RemoveMember(ctx context.Context, params application.RemoveMemberParams) error
		PromoteMember(ctx context.Context, params application.UpdateMemberRoleParams) error
		DemoteMember(ctx context.Context, params application.UpdateMemberRoleParams) error
		ApproveMember(ctx context.Context, params application.ApproveMemberParams) error
		RejectMember(ctx context.Context, params application.RejectMemberParams) error
		LeaveMember(ctx context.Context, activityGroupID uuid.UUID, userID uuid.UUID) error
		GetMember(ctx context.Context, activityGroupID uuid.UUID, userID uuid.UUID) (*domain.Member, error)
		ListMembers(ctx context.Context, params application.ListMembersParams) ([]*domain.Member, error)
	}

	// attendeeService defines the interface for attendee/RSVP operations
	attendeeService interface {
		CreateRSVP(ctx context.Context, params application.CreateRSVPParams) (*domain.Attendee, error)
		UpdateRSVP(ctx context.Context, params application.UpdateRSVPParams) (*domain.Attendee, error)
		CancelRSVP(ctx context.Context, sessionID uuid.UUID, userID uuid.UUID) error
		GetRSVP(ctx context.Context, sessionID uuid.UUID, userID uuid.UUID) (*domain.Attendee, error)
		ListRSVPs(ctx context.Context, params application.ListRSVPsParams) ([]*domain.Attendee, error)
	}

	// Handler handles HTTP requests for the activity module
	Handler struct {
		sessionTemplateService sessionTemplateService
		activityGroupService   activityGroupService
		sessionService         sessionService
		memberService          memberService
		attendeeService        attendeeService
		logger                 *logger.Logger
	}
)

// NewHandler creates a new activity HTTP handler
func NewHandler(
	sessionTemplateService sessionTemplateService,
	activityGroupService activityGroupService,
	sessionService sessionService,
	memberService memberService,
	attendeeService attendeeService,
	logger *logger.Logger,
) *Handler {
	return &Handler{
		sessionTemplateService: sessionTemplateService,
		activityGroupService:   activityGroupService,
		sessionService:         sessionService,
		memberService:          memberService,
		attendeeService:        attendeeService,
		logger:                 logger.WithName("activity.http.handler"),
	}
}

// RegisterRoutes registers all HTTP routes for the activity module
func (h *Handler) RegisterRoutes(
	mux *http.ServeMux,
	middlewares *middleware.Chain,
) {
	// Activity Group routes (ordered from most specific to least specific)
	mux.Handle(
		"POST /api/v1/activity-groups",
		middlewares.ThenFunc(h.CreateActivityGroup),
	)
	mux.Handle(
		"GET /api/v1/activity-groups/discover",
		middlewares.ThenFunc(h.DiscoverActivityGroups),
	)
	mux.Handle(
		"GET /api/v1/activity-groups",
		middlewares.ThenFunc(h.ListActivityGroups),
	)
	mux.Handle(
		"GET /api/v1/activity-groups/{id}",
		middlewares.ThenFunc(h.GetActivityGroup),
	)
	mux.Handle(
		"PUT /api/v1/activity-groups/{id}",
		middlewares.ThenFunc(h.UpdateActivityGroup),
	)
	mux.Handle(
		"POST /api/v1/activity-groups/{id}/activate",
		middlewares.ThenFunc(h.ActivateActivityGroup),
	)
	mux.Handle(
		"DELETE /api/v1/activity-groups/{id}",
		middlewares.ThenFunc(h.CancelActivityGroup),
	)

	// Session Template routes
	mux.Handle(
		"POST /api/v1/activity-groups/{groupId}/templates",
		middlewares.ThenFunc(h.CreateSessionTemplate),
	)
	mux.Handle(
		"GET /api/v1/activity-groups/{groupId}/templates",
		middlewares.ThenFunc(h.ListSessionTemplates),
	)
	mux.Handle(
		"GET /api/v1/templates/{id}",
		middlewares.ThenFunc(h.GetSessionTemplate),
	)
	mux.Handle(
		"PUT /api/v1/templates/{id}",
		middlewares.ThenFunc(h.UpdateSessionTemplate),
	)
	mux.Handle(
		"DELETE /api/v1/templates/{id}",
		middlewares.ThenFunc(h.DeleteSessionTemplate),
	)
	mux.Handle(
		"POST /api/v1/templates/{id}/activate",
		middlewares.ThenFunc(h.ActivateSessionTemplate),
	)
	mux.Handle(
		"POST /api/v1/templates/{id}/deactivate",
		middlewares.ThenFunc(h.DeactivateSessionTemplate),
	)

	// Session routes (ordered from most specific to least specific)
	mux.Handle(
		"POST /api/v1/sessions",
		middlewares.ThenFunc(h.CreateSession),
	)
	mux.Handle(
		"GET /api/v1/sessions",
		middlewares.ThenFunc(h.ListSessions),
	)
	mux.Handle(
		"GET /api/v1/sessions/{id}",
		middlewares.ThenFunc(h.GetSession),
	)
	mux.Handle(
		"PUT /api/v1/sessions/{id}",
		middlewares.ThenFunc(h.UpdateSession),
	)
	mux.Handle(
		"POST /api/v1/sessions/{id}/start",
		middlewares.ThenFunc(h.StartSession),
	)
	mux.Handle(
		"POST /api/v1/sessions/{id}/complete",
		middlewares.ThenFunc(h.CompleteSession),
	)
	mux.Handle(
		"DELETE /api/v1/sessions/{id}",
		middlewares.ThenFunc(h.CancelSession),
	)

	// Member routes (ordered from most specific to least specific)
	mux.Handle(
		"POST /api/v1/activity-groups/{groupId}/members/{userId}/approve",
		middlewares.ThenFunc(h.ApproveMember),
	)
	mux.Handle(
		"POST /api/v1/activity-groups/{groupId}/members/{userId}/reject",
		middlewares.ThenFunc(h.RejectMember),
	)
	mux.Handle(
		"PATCH /api/v1/activity-groups/{groupId}/members/{userId}/role",
		middlewares.ThenFunc(h.UpdateMemberRole),
	)
	mux.Handle(
		"DELETE /api/v1/activity-groups/{groupId}/members/{userId}",
		middlewares.ThenFunc(h.RemoveMember),
	)
	mux.Handle(
		"GET /api/v1/activity-groups/{groupId}/members/{userId}",
		middlewares.ThenFunc(h.GetMember),
	)
	mux.Handle(
		"POST /api/v1/activity-groups/{groupId}/members",
		middlewares.ThenFunc(h.InviteMember),
	)
	mux.Handle(
		"GET /api/v1/activity-groups/{groupId}/members",
		middlewares.ThenFunc(h.ListMembers),
	)
	mux.Handle(
		"POST /api/v1/activity-groups/{groupId}/leave",
		middlewares.ThenFunc(h.LeaveMember),
	)

	// RSVP/Attendee routes
	mux.Handle(
		"POST /api/v1/sessions/{sessionId}/rsvp",
		middlewares.ThenFunc(h.CreateRSVP),
	)
	mux.Handle(
		"PUT /api/v1/sessions/{sessionId}/rsvp",
		middlewares.ThenFunc(h.UpdateRSVP),
	)
	mux.Handle(
		"DELETE /api/v1/sessions/{sessionId}/rsvp",
		middlewares.ThenFunc(h.CancelRSVP),
	)
	mux.Handle(
		"GET /api/v1/sessions/{sessionId}/rsvp",
		middlewares.ThenFunc(h.GetRSVP),
	)
	mux.Handle(
		"GET /api/v1/sessions/{sessionId}/attendees",
		middlewares.ThenFunc(h.ListAttendees),
	)
}
