package http

import (
	"net/http"

	"github.com/google/uuid"
	"github.com/rasparac/rekreativko-api/activity/internal/interfaces/http/dtos"
	"github.com/rasparac/rekreativko-api/activity/internal/interfaces/http/mapper"
	"github.com/rasparac/rekreativko-api/shared/api"
	"github.com/rasparac/rekreativko-api/shared/authcontext"
)

// CreateRSVP handles POST /api/v1/sessions/{sessionId}/rsvp
//
//	@Summary		Create an RSVP for a session
//	@Description	Creates a new RSVP for the authenticated user for a specific session
//	@Tags			RSVPs
//	@Accept			json
//	@Produce		json
//	@Security		GatewayKeyAuth && BearerAuth
//	@Param			sessionId	path		string								true	"Session ID"
//	@Param			request		body		dtos.CreateRSVPRequest				true	"RSVP data"
//	@Success		201			{object}	api.Response[dtos.CreateRSVPResponse]	"RSVP created successfully"
//	@Failure		400			{object}	api.Response[any]						"Invalid request"
//	@Failure		401			{object}	api.Response[any]						"Unauthorized"
//	@Failure		409			{object}	api.Response[any]						"Already RSVPed or session full"
//	@Failure		500			{object}	api.Response[any]						"Internal server error"
//	@Router			/api/v1/sessions/{sessionId}/rsvp [post]
func (h *Handler) CreateRSVP(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := authcontext.GetAccountID(ctx)

	// Parse session ID from path
	sessionIDStr := r.PathValue("sessionId")
	sessionID, err := uuid.Parse(sessionIDStr)
	if err != nil {
		h.logger.Error(ctx, "invalid session ID", "error", err)
		api.WriteBadRequestResponse(w, "invalid_session_id", "Invalid session ID")
		return
	}

	// Decode request body
	var req dtos.CreateRSVPRequest
	if err := api.DecodeJSONBody(r, &req); err != nil {
		h.logger.Error(ctx, "failed to decode request body", "error", err)
		api.WriteValidationErrorResponse(w, err)
		return
	}

	// Convert to application params
	params := mapper.CreateRSVPRequestToParams(&req, sessionID, userID)

	// Create RSVP
	attendee, err := h.attendeeService.CreateRSVP(ctx, *params)
	if err != nil {
		h.handleServiceError(ctx, w, err)
		return
	}

	api.WriteCreatedResponse(w, &dtos.CreateRSVPResponse{ID: attendee.ID()}, "")
}

// UpdateRSVP handles PUT /api/v1/sessions/{sessionId}/rsvp
//
//	@Summary		Update an RSVP
//	@Description	Updates the authenticated user's RSVP for a specific session
//	@Tags			RSVPs
//	@Accept			json
//	@Produce		json
//	@Security		GatewayKeyAuth && BearerAuth
//	@Param			sessionId	path		string								true	"Session ID"
//	@Param			request		body		dtos.UpdateRSVPRequest				true	"Updated RSVP data"
//	@Success		200			{object}	api.Response[dtos.AttendeeResponse]	"RSVP updated successfully"
//	@Failure		400			{object}	api.Response[any]					"Invalid request"
//	@Failure		401			{object}	api.Response[any]					"Unauthorized"
//	@Failure		404			{object}	api.Response[any]					"RSVP not found"
//	@Failure		500			{object}	api.Response[any]					"Internal server error"
//	@Router			/api/v1/sessions/{sessionId}/rsvp [put]
func (h *Handler) UpdateRSVP(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := authcontext.GetAccountID(ctx)

	// Parse session ID from path
	sessionIDStr := r.PathValue("sessionId")
	sessionID, err := uuid.Parse(sessionIDStr)
	if err != nil {
		h.logger.Error(ctx, "invalid session ID", "error", err)
		api.WriteBadRequestResponse(w, "invalid_session_id", "Invalid session ID")
		return
	}

	// Decode request body
	var req dtos.UpdateRSVPRequest
	if err := api.DecodeJSONBody(r, &req); err != nil {
		h.logger.Error(ctx, "failed to decode request body", "error", err)
		api.WriteValidationErrorResponse(w, err)
		return
	}

	// Convert to application params
	params := mapper.UpdateRSVPRequestToParams(&req, sessionID, userID)

	// Update RSVP
	attendee, err := h.attendeeService.UpdateRSVP(ctx, *params)
	if err != nil {
		h.handleServiceError(ctx, w, err)
		return
	}

	api.WriteOkResponse(w, mapper.AttendeeToResponse(attendee), "")
}

// CancelRSVP handles DELETE /api/v1/sessions/{sessionId}/rsvp
//
//	@Summary		Cancel an RSVP
//	@Description	Cancels the authenticated user's RSVP for a specific session
//	@Tags			RSVPs
//	@Produce		json
//	@Security		GatewayKeyAuth && BearerAuth
//	@Param			sessionId	path		string						true	"Session ID"
//	@Success		200			{object}	api.Response[any]			"RSVP cancelled successfully"
//	@Failure		400			{object}	api.Response[any]			"Invalid request"
//	@Failure		401			{object}	api.Response[any]			"Unauthorized"
//	@Failure		404			{object}	api.Response[any]			"RSVP not found"
//	@Failure		500			{object}	api.Response[any]			"Internal server error"
//	@Router			/api/v1/sessions/{sessionId}/rsvp [delete]
func (h *Handler) CancelRSVP(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := authcontext.GetAccountID(ctx)

	// Parse session ID from path
	sessionIDStr := r.PathValue("sessionId")
	sessionID, err := uuid.Parse(sessionIDStr)
	if err != nil {
		h.logger.Error(ctx, "invalid session ID", "error", err)
		api.WriteBadRequestResponse(w, "invalid_session_id", "Invalid session ID")
		return
	}

	// Cancel RSVP
	err = h.attendeeService.CancelRSVP(ctx, sessionID, userID)
	if err != nil {
		h.handleServiceError(ctx, w, err)
		return
	}

	api.WriteOkResponse(w, struct{}{}, "")
}

// GetRSVP handles GET /api/v1/sessions/{sessionId}/rsvp
//
//	@Summary		Get user's RSVP
//	@Description	Retrieves the authenticated user's RSVP for a specific session
//	@Tags			RSVPs
//	@Produce		json
//	@Security		GatewayKeyAuth && BearerAuth
//	@Param			sessionId	path		string								true	"Session ID"
//	@Success		200			{object}	api.Response[dtos.AttendeeResponse]	"RSVP retrieved successfully"
//	@Failure		400			{object}	api.Response[any]					"Invalid request"
//	@Failure		401			{object}	api.Response[any]					"Unauthorized"
//	@Failure		404			{object}	api.Response[any]					"RSVP not found"
//	@Failure		500			{object}	api.Response[any]					"Internal server error"
//	@Router			/api/v1/sessions/{sessionId}/rsvp [get]
func (h *Handler) GetRSVP(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := authcontext.GetAccountID(ctx)

	// Parse session ID from path
	sessionIDStr := r.PathValue("sessionId")
	sessionID, err := uuid.Parse(sessionIDStr)
	if err != nil {
		h.logger.Error(ctx, "invalid session ID", "error", err)
		api.WriteBadRequestResponse(w, "invalid_session_id", "Invalid session ID")
		return
	}

	// Get RSVP
	attendee, err := h.attendeeService.GetRSVP(ctx, sessionID, userID)
	if err != nil {
		h.handleServiceError(ctx, w, err)
		return
	}

	api.WriteOkResponse(w, mapper.AttendeeToResponse(attendee), "")
}

// ListAttendees handles GET /api/v1/sessions/{sessionId}/attendees
//
//	@Summary		List attendees for a session
//	@Description	Lists all attendees/RSVPs for a specific session with optional filters
//	@Tags			RSVPs
//	@Produce		json
//	@Security		GatewayKeyAuth && BearerAuth
//	@Param			sessionId	path		string									true	"Session ID"
//	@Param			status		query		string									false	"Filter by status (going, pending, not_going, maybe, promoted)"
//	@Param			limit		query		int										false	"Limit number of results (default 20)"
//	@Param			page_token	query		string									false	"Token from the previous response's next_page_token, to fetch the next page"
//	@Success		200			{object}	api.Response[api.Page[dtos.AttendeeResponse]]	"Attendees retrieved successfully"
//	@Failure		400			{object}	api.Response[any]						"Invalid request"
//	@Failure		500			{object}	api.Response[any]						"Internal server error"
//	@Router			/api/v1/sessions/{sessionId}/attendees [get]
func (h *Handler) ListAttendees(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Parse session ID from path
	sessionIDStr := r.PathValue("sessionId")
	sessionID, err := uuid.Parse(sessionIDStr)
	if err != nil {
		h.logger.Error(ctx, "invalid session ID", "error", err)
		api.WriteBadRequestResponse(w, "invalid_session_id", "Invalid session ID")
		return
	}

	// Parse query parameters
	params, err := mapper.QueryToListRSVPsParams(r.URL.Query())
	if err != nil {
		h.logger.Error(ctx, "failed to parse query parameters", "error", err)
		api.WriteBadRequestResponse(w, "invalid_params", "Invalid query parameters")
		return
	}

	// Set session ID from path
	params.SessionID = &sessionID

	// List attendees
	attendees, nextPageToken, err := h.attendeeService.ListRSVPs(ctx, *params)
	if err != nil {
		h.handleServiceError(ctx, w, err)
		return
	}

	api.WriteOkResponse(
		w,
		api.NewPage(attendees, params.Limit, nextPageToken, mapper.AttendeeToResponse),
		"",
	)
}

// ApproveAttendee handles POST /api/v1/sessions/{sessionId}/rsvp/{userId}/approve
//
//	@Summary		Approve a pending join request
//	@Description	Approves a pending join request on a session that requires creator/admin approval, moving the attendee to "going"
//	@Tags			RSVPs
//	@Produce		json
//	@Security		GatewayKeyAuth && BearerAuth
//	@Param			sessionId	path		string				true	"Session ID"
//	@Param			userId		path		string				true	"User ID of the requester to approve"
//	@Success		200			{object}	api.Response[any]	"Join request approved"
//	@Failure		400			{object}	api.Response[any]	"Invalid request"
//	@Failure		401			{object}	api.Response[any]	"Unauthorized"
//	@Failure		403			{object}	api.Response[any]	"Not authorized to manage this session"
//	@Failure		404			{object}	api.Response[any]	"Session or attendee not found"
//	@Failure		409			{object}	api.Response[any]	"Session full or attendee not awaiting approval"
//	@Failure		500			{object}	api.Response[any]	"Internal server error"
//	@Router			/api/v1/sessions/{sessionId}/rsvp/{userId}/approve [post]
func (h *Handler) ApproveAttendee(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	accountID := authcontext.GetAccountID(ctx)

	sessionID, err := uuid.Parse(r.PathValue("sessionId"))
	if err != nil {
		h.logger.Error(ctx, "invalid session ID", "error", err)
		api.WriteBadRequestResponse(w, "invalid_session_id", "Invalid session ID")
		return
	}

	userID, err := uuid.Parse(r.PathValue("userId"))
	if err != nil {
		h.logger.Error(ctx, "invalid user ID", "error", err)
		api.WriteBadRequestResponse(w, "invalid_user_id", "Invalid user ID")
		return
	}

	// Get the session so we know which group (if any) to check the requester's role against
	session, err := h.sessionService.GetSession(ctx, sessionID, accountID)
	if err != nil {
		h.handleServiceError(ctx, w, err)
		return
	}

	requesterRole, err := h.getUserRole(ctx, session.ActivityGroupID(), accountID)
	if err != nil {
		h.handleServiceError(ctx, w, err)
		return
	}

	params := mapper.ApproveAttendeeRequestToParams(sessionID, userID, accountID, requesterRole)

	if err := h.attendeeService.ApproveAttendee(ctx, *params); err != nil {
		h.handleServiceError(ctx, w, err)
		return
	}

	h.logger.Info(ctx, "attendee approved", "session_id", sessionID, "user_id", userID)

	api.WriteOkResponse(w, struct{}{}, "Join request approved")
}

// RejectAttendee handles POST /api/v1/sessions/{sessionId}/rsvp/{userId}/reject
//
//	@Summary		Reject a pending join request
//	@Description	Rejects a pending join request on a session that requires creator/admin approval, moving the attendee to "not_going"
//	@Tags			RSVPs
//	@Produce		json
//	@Security		GatewayKeyAuth && BearerAuth
//	@Param			sessionId	path		string				true	"Session ID"
//	@Param			userId		path		string				true	"User ID of the requester to reject"
//	@Success		200			{object}	api.Response[any]	"Join request rejected"
//	@Failure		400			{object}	api.Response[any]	"Invalid request"
//	@Failure		401			{object}	api.Response[any]	"Unauthorized"
//	@Failure		403			{object}	api.Response[any]	"Not authorized to manage this session"
//	@Failure		404			{object}	api.Response[any]	"Session or attendee not found"
//	@Failure		409			{object}	api.Response[any]	"Attendee not awaiting approval"
//	@Failure		500			{object}	api.Response[any]	"Internal server error"
//	@Router			/api/v1/sessions/{sessionId}/rsvp/{userId}/reject [post]
func (h *Handler) RejectAttendee(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	accountID := authcontext.GetAccountID(ctx)

	sessionID, err := uuid.Parse(r.PathValue("sessionId"))
	if err != nil {
		h.logger.Error(ctx, "invalid session ID", "error", err)
		api.WriteBadRequestResponse(w, "invalid_session_id", "Invalid session ID")
		return
	}

	userID, err := uuid.Parse(r.PathValue("userId"))
	if err != nil {
		h.logger.Error(ctx, "invalid user ID", "error", err)
		api.WriteBadRequestResponse(w, "invalid_user_id", "Invalid user ID")
		return
	}

	session, err := h.sessionService.GetSession(ctx, sessionID, accountID)
	if err != nil {
		h.handleServiceError(ctx, w, err)
		return
	}

	requesterRole, err := h.getUserRole(ctx, session.ActivityGroupID(), accountID)
	if err != nil {
		h.handleServiceError(ctx, w, err)
		return
	}

	params := mapper.RejectAttendeeRequestToParams(sessionID, userID, accountID, requesterRole)

	if err := h.attendeeService.RejectAttendee(ctx, *params); err != nil {
		h.handleServiceError(ctx, w, err)
		return
	}

	h.logger.Info(ctx, "attendee rejected", "session_id", sessionID, "user_id", userID)

	api.WriteOkResponse(w, struct{}{}, "Join request rejected")
}

// RemoveAttendee handles DELETE /api/v1/sessions/{sessionId}/rsvp/{userId}
//
//	@Summary		Remove an attendee from a session
//	@Description	Removes an already-confirmed attendee from a session (a manager kicking someone out), as opposed to DELETE /rsvp which only lets a user cancel their own RSVP
//	@Tags			RSVPs
//	@Produce		json
//	@Security		GatewayKeyAuth && BearerAuth
//	@Param			sessionId	path		string				true	"Session ID"
//	@Param			userId		path		string				true	"User ID of the attendee to remove"
//	@Success		200			{object}	api.Response[any]	"Attendee removed"
//	@Failure		400			{object}	api.Response[any]	"Invalid request"
//	@Failure		401			{object}	api.Response[any]	"Unauthorized"
//	@Failure		403			{object}	api.Response[any]	"Not authorized to manage this session"
//	@Failure		404			{object}	api.Response[any]	"Session or attendee not found"
//	@Failure		409			{object}	api.Response[any]	"Attendee is not confirmed, or is the session creator"
//	@Failure		500			{object}	api.Response[any]	"Internal server error"
//	@Router			/api/v1/sessions/{sessionId}/rsvp/{userId} [delete]
func (h *Handler) RemoveAttendee(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	accountID := authcontext.GetAccountID(ctx)

	sessionID, err := uuid.Parse(r.PathValue("sessionId"))
	if err != nil {
		h.logger.Error(ctx, "invalid session ID", "error", err)
		api.WriteBadRequestResponse(w, "invalid_session_id", "Invalid session ID")
		return
	}

	userID, err := uuid.Parse(r.PathValue("userId"))
	if err != nil {
		h.logger.Error(ctx, "invalid user ID", "error", err)
		api.WriteBadRequestResponse(w, "invalid_user_id", "Invalid user ID")
		return
	}

	session, err := h.sessionService.GetSession(ctx, sessionID, accountID)
	if err != nil {
		h.handleServiceError(ctx, w, err)
		return
	}

	requesterRole, err := h.getUserRole(ctx, session.ActivityGroupID(), accountID)
	if err != nil {
		h.handleServiceError(ctx, w, err)
		return
	}

	params := mapper.RemoveAttendeeRequestToParams(sessionID, userID, accountID, requesterRole)

	if err := h.attendeeService.RemoveAttendee(ctx, *params); err != nil {
		h.handleServiceError(ctx, w, err)
		return
	}

	h.logger.Info(ctx, "attendee removed", "session_id", sessionID, "user_id", userID)

	api.WriteOkResponse(w, struct{}{}, "Attendee removed")
}

// AssignAttendeeTeam handles PUT /api/v1/sessions/{sessionId}/rsvp/{userId}/team
//
//	@Summary		Assign an attendee to a team
//	@Description	Puts a confirmed (going/promoted) attendee on one of the session's teams, or moves them there from another team. Session creator or group admin/creator only. Rejected with 409 when the team already has players_per_team members.
//	@Tags			RSVPs
//	@Accept			json
//	@Produce		json
//	@Security		GatewayKeyAuth && BearerAuth
//	@Param			sessionId	path		string								true	"Session ID"
//	@Param			userId		path		string								true	"User ID of the attendee"
//	@Param			request		body		dtos.AssignTeamRequest				true	"Target team"
//	@Success		200			{object}	api.Response[dtos.AttendeeResponse]	"Attendee assigned to team"
//	@Failure		400			{object}	api.Response[any]					"Invalid request"
//	@Failure		401			{object}	api.Response[any]					"Unauthorized"
//	@Failure		404			{object}	api.Response[any]					"Session, attendee or team not found"
//	@Failure		409			{object}	api.Response[any]					"Session has no teams, attendee not confirmed, team full, or session canceled/completed"
//	@Failure		500			{object}	api.Response[any]					"Internal server error"
//	@Router			/api/v1/sessions/{sessionId}/rsvp/{userId}/team [put]
func (h *Handler) AssignAttendeeTeam(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	accountID := authcontext.GetAccountID(ctx)

	sessionID, err := uuid.Parse(r.PathValue("sessionId"))
	if err != nil {
		h.logger.Error(ctx, "invalid session ID", "error", err)
		api.WriteBadRequestResponse(w, "invalid_session_id", "Invalid session ID")
		return
	}

	userID, err := uuid.Parse(r.PathValue("userId"))
	if err != nil {
		h.logger.Error(ctx, "invalid user ID", "error", err)
		api.WriteBadRequestResponse(w, "invalid_user_id", "Invalid user ID")
		return
	}

	var req dtos.AssignTeamRequest
	if err := api.DecodeJSONBody(r, &req); err != nil {
		h.logger.Error(ctx, "failed to decode request body", "error", err)
		api.WriteValidationErrorResponse(w, err)
		return
	}

	session, err := h.sessionService.GetSession(ctx, sessionID, accountID)
	if err != nil {
		h.handleServiceError(ctx, w, err)
		return
	}

	requesterRole, err := h.getUserRole(ctx, session.ActivityGroupID(), accountID)
	if err != nil {
		h.handleServiceError(ctx, w, err)
		return
	}

	params := mapper.AssignTeamRequestToParams(&req, sessionID, userID, accountID, requesterRole)

	attendee, err := h.attendeeService.AssignAttendeeTeam(ctx, *params)
	if err != nil {
		h.handleServiceError(ctx, w, err)
		return
	}

	h.logger.Info(ctx, "attendee assigned to team", "session_id", sessionID, "user_id", userID, "team_id", req.TeamID)

	api.WriteOkResponse(w, mapper.AttendeeToResponse(attendee), "")
}

// UnassignAttendeeTeam handles DELETE /api/v1/sessions/{sessionId}/rsvp/{userId}/team
//
//	@Summary		Remove an attendee from their team
//	@Description	Takes an attendee off their team, back to the unassigned pool (they stay attending the session). Session creator or group admin/creator only. A no-op if the attendee isn't on a team.
//	@Tags			RSVPs
//	@Produce		json
//	@Security		GatewayKeyAuth && BearerAuth
//	@Param			sessionId	path		string								true	"Session ID"
//	@Param			userId		path		string								true	"User ID of the attendee"
//	@Success		200			{object}	api.Response[dtos.AttendeeResponse]	"Attendee unassigned from team"
//	@Failure		400			{object}	api.Response[any]					"Invalid request"
//	@Failure		401			{object}	api.Response[any]					"Unauthorized"
//	@Failure		404			{object}	api.Response[any]					"Session or attendee not found"
//	@Failure		409			{object}	api.Response[any]					"Session has no teams, or session canceled/completed"
//	@Failure		500			{object}	api.Response[any]					"Internal server error"
//	@Router			/api/v1/sessions/{sessionId}/rsvp/{userId}/team [delete]
func (h *Handler) UnassignAttendeeTeam(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	accountID := authcontext.GetAccountID(ctx)

	sessionID, err := uuid.Parse(r.PathValue("sessionId"))
	if err != nil {
		h.logger.Error(ctx, "invalid session ID", "error", err)
		api.WriteBadRequestResponse(w, "invalid_session_id", "Invalid session ID")
		return
	}

	userID, err := uuid.Parse(r.PathValue("userId"))
	if err != nil {
		h.logger.Error(ctx, "invalid user ID", "error", err)
		api.WriteBadRequestResponse(w, "invalid_user_id", "Invalid user ID")
		return
	}

	session, err := h.sessionService.GetSession(ctx, sessionID, accountID)
	if err != nil {
		h.handleServiceError(ctx, w, err)
		return
	}

	requesterRole, err := h.getUserRole(ctx, session.ActivityGroupID(), accountID)
	if err != nil {
		h.handleServiceError(ctx, w, err)
		return
	}

	params := mapper.UnassignTeamRequestToParams(sessionID, userID, accountID, requesterRole)

	attendee, err := h.attendeeService.UnassignAttendeeTeam(ctx, *params)
	if err != nil {
		h.handleServiceError(ctx, w, err)
		return
	}

	h.logger.Info(ctx, "attendee unassigned from team", "session_id", sessionID, "user_id", userID)

	api.WriteOkResponse(w, mapper.AttendeeToResponse(attendee), "")
}
