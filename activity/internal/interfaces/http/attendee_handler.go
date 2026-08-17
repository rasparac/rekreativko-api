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

	// Get activity group ID from query parameter
	activityGroupIDStr := r.URL.Query().Get("activity_group_id")
	if activityGroupIDStr == "" {
		h.logger.Error(ctx, "missing activity group ID")
		api.WriteBadRequestResponse(w, "missing_group_id", "Activity group ID is required")
		return
	}

	activityGroupID, err := uuid.Parse(activityGroupIDStr)
	if err != nil {
		h.logger.Error(ctx, "invalid activity group ID", "error", err)
		api.WriteBadRequestResponse(w, "invalid_group_id", "Invalid activity group ID")
		return
	}

	// Convert to application params
	params := mapper.CreateRSVPRequestToParams(&req, sessionID, activityGroupID, userID)

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

	// Get activity group ID from query parameter
	activityGroupIDStr := r.URL.Query().Get("activity_group_id")
	if activityGroupIDStr == "" {
		h.logger.Error(ctx, "missing activity group ID")
		api.WriteBadRequestResponse(w, "missing_group_id", "Activity group ID is required")
		return
	}

	activityGroupID, err := uuid.Parse(activityGroupIDStr)
	if err != nil {
		h.logger.Error(ctx, "invalid activity group ID", "error", err)
		api.WriteBadRequestResponse(w, "invalid_group_id", "Invalid activity group ID")
		return
	}

	// Convert to application params
	params := mapper.UpdateRSVPRequestToParams(&req, sessionID, activityGroupID, userID)

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
//	@Param			offset		query		int										false	"Offset for pagination (default 0)"
//	@Success		200			{object}	api.Response[dtos.AttendeeListResponse]	"Attendees retrieved successfully"
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
	attendees, err := h.attendeeService.ListRSVPs(ctx, *params)
	if err != nil {
		h.handleServiceError(ctx, w, err)
		return
	}

	api.WriteOkResponse(
		w,
		mapper.AttendeeListToResponse(attendees, params.Limit, params.Offset),
		"",
	)
}
