package http

import (
	"net/http"

	"github.com/google/uuid"
	"github.com/rasparac/rekreativko-api/activity/internal/interfaces/http/dtos"
	"github.com/rasparac/rekreativko-api/activity/internal/interfaces/http/mapper"
	"github.com/rasparac/rekreativko-api/shared/api"
	"github.com/rasparac/rekreativko-api/shared/authcontext"
)

// CreateSession handles POST /api/v1/sessions
//
//	@Summary		Create a session
//	@Description	Creates a new session for an activity group
//	@Tags			Sessions
//	@Accept			json
//	@Produce		json
//	@Security		GatewayKeyAuth && BearerAuth
//	@Param			request	body		dtos.CreateSessionRequest			true	"Session data"
//	@Success		201		{object}	api.Response[dtos.CreateSessionResponse]	"Session created successfully"
//	@Failure		400		{object}	api.Response[any]					"Invalid request"
//	@Failure		401		{object}	api.Response[any]					"Unauthorized"
//	@Failure		500		{object}	api.Response[any]					"Internal server error"
//	@Router			/api/v1/sessions [post]
func (h *Handler) CreateSession(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	accountID := authcontext.GetAccountID(ctx)

	// Decode request body
	var req dtos.CreateSessionRequest
	if err := api.DecodeJSONBody(r, &req); err != nil {
		h.logger.Error(ctx, "failed to decode request body", "error", err)
		api.WriteValidationErrorResponse(w, err)
		return
	}

	// Convert to application params
	params := mapper.CreateSessionRequestToParams(&req, accountID)

	// Create session
	session, err := h.sessionService.CreateSession(ctx, *params)
	if err != nil {
		h.handleServiceError(ctx, w, err)
		return
	}

	h.logger.Info(ctx, "session created",
		"session_id", session.ID(),
		"activity_group_id", params.ActivityGroupID,
	)

	api.WriteCreatedResponse(w, dtos.CreateSessionResponse{
		ID: session.ID(),
	}, "Session created successfully")
}

// GetSession handles GET /api/v1/sessions/{id}
//
//	@Summary		Get a session
//	@Description	Retrieves a session by ID
//	@Tags			Sessions
//	@Produce		json
//	@Security		GatewayKeyAuth && BearerAuth
//	@Param			id	path		string							true	"Session ID"
//	@Success		200	{object}	api.Response[dtos.SessionResponse]	"Session retrieved successfully"
//	@Failure		400	{object}	api.Response[any]				"Invalid request"
//	@Failure		404	{object}	api.Response[any]				"Session not found"
//	@Failure		500	{object}	api.Response[any]				"Internal server error"
//	@Router			/api/v1/sessions/{id} [get]
func (h *Handler) GetSession(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Parse session ID from path
	sessionIDStr := r.PathValue("id")
	sessionID, err := uuid.Parse(sessionIDStr)
	if err != nil {
		h.logger.Error(ctx, "invalid session ID", "error", err)
		api.WriteBadRequestResponse(w, "invalid_session_id", "Invalid session ID")
		return
	}

	// Get session
	session, err := h.sessionService.GetSession(ctx, sessionID)
	if err != nil {
		h.handleServiceError(ctx, w, err)
		return
	}

	api.WriteOkResponse(w, mapper.SessionToResponse(session), "")
}

// UpdateSession handles PUT /api/v1/sessions/{id}
//
//	@Summary		Update a session
//	@Description	Updates an existing session
//	@Tags			Sessions
//	@Accept			json
//	@Produce		json
//	@Security		GatewayKeyAuth && BearerAuth
//	@Param			id		path		string							true	"Session ID"
//	@Param			request	body		dtos.UpdateSessionRequest		true	"Update data"
//	@Success		200		{object}	api.Response[dtos.EmptyResponse]	"Session updated successfully"
//	@Failure		400		{object}	api.Response[any]				"Invalid request"
//	@Failure		404		{object}	api.Response[any]				"Session not found"
//	@Failure		500		{object}	api.Response[any]				"Internal server error"
//	@Router			/api/v1/sessions/{id} [put]
func (h *Handler) UpdateSession(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	accountID := authcontext.GetAccountID(ctx)

	// Parse session ID from path
	sessionIDStr := r.PathValue("id")
	sessionID, err := uuid.Parse(sessionIDStr)
	if err != nil {
		h.logger.Error(ctx, "invalid session ID", "error", err)
		api.WriteBadRequestResponse(w, "invalid_session_id", "Invalid session ID")
		return
	}

	// Decode request body
	var req dtos.UpdateSessionRequest
	if err := api.DecodeJSONBody(r, &req); err != nil {
		h.logger.Error(ctx, "failed to decode request body", "error", err)
		api.WriteValidationErrorResponse(w, err)
		return
	}

	// Get session to determine activity group
	session, err := h.sessionService.GetSession(ctx, sessionID)
	if err != nil {
		h.handleServiceError(ctx, w, err)
		return
	}

	// Get user's role from membership
	userRole, err := h.getUserRole(ctx, session.ActivityGroupID(), accountID)
	if err != nil {
		h.handleServiceError(ctx, w, err)
		return
	}

	// Convert to application params
	params := mapper.UpdateSessionRequestToParams(&req, accountID, userRole)

	// Update session
	if err := h.sessionService.UpdateSession(ctx, sessionID, *params); err != nil {
		h.handleServiceError(ctx, w, err)
		return
	}

	h.logger.Info(ctx, "session updated", "session_id", sessionID)

	api.WriteOkResponse(w, dtos.EmptyResponse{}, "Session updated successfully")
}

// StartSession handles POST /api/v1/sessions/{id}/start
//
//	@Summary		Start a session
//	@Description	Starts a session (changes status to started)
//	@Tags			Sessions
//	@Produce		json
//	@Security		GatewayKeyAuth && BearerAuth
//	@Param			id	path		string							true	"Session ID"
//	@Success		200	{object}	api.Response[dtos.EmptyResponse]	"Session started successfully"
//	@Failure		400	{object}	api.Response[any]				"Invalid request"
//	@Failure		404	{object}	api.Response[any]				"Session not found"
//	@Failure		500	{object}	api.Response[any]				"Internal server error"
//	@Router			/api/v1/sessions/{id}/start [post]
func (h *Handler) StartSession(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	accountID := authcontext.GetAccountID(ctx)

	// Parse session ID from path
	sessionIDStr := r.PathValue("id")
	sessionID, err := uuid.Parse(sessionIDStr)
	if err != nil {
		h.logger.Error(ctx, "invalid session ID", "error", err)
		api.WriteBadRequestResponse(w, "invalid_session_id", "Invalid session ID")
		return
	}

	// Get session to determine activity group
	session, err := h.sessionService.GetSession(ctx, sessionID)
	if err != nil {
		h.handleServiceError(ctx, w, err)
		return
	}

	// Get user's role from membership
	userRole, err := h.getUserRole(ctx, session.ActivityGroupID(), accountID)
	if err != nil {
		h.handleServiceError(ctx, w, err)
		return
	}

	// Start session
	if err := h.sessionService.StartSession(ctx, sessionID, accountID, userRole); err != nil {
		h.handleServiceError(ctx, w, err)
		return
	}

	h.logger.Info(ctx, "session started", "session_id", sessionID)

	api.WriteOkResponse(w, dtos.EmptyResponse{}, "Session started successfully")
}

// CompleteSession handles POST /api/v1/sessions/{id}/complete
//
//	@Summary		Complete a session
//	@Description	Completes a session (changes status to completed)
//	@Tags			Sessions
//	@Produce		json
//	@Security		GatewayKeyAuth && BearerAuth
//	@Param			id	path		string							true	"Session ID"
//	@Success		200	{object}	api.Response[dtos.EmptyResponse]	"Session completed successfully"
//	@Failure		400	{object}	api.Response[any]				"Invalid request"
//	@Failure		404	{object}	api.Response[any]				"Session not found"
//	@Failure		500	{object}	api.Response[any]				"Internal server error"
//	@Router			/api/v1/sessions/{id}/complete [post]
func (h *Handler) CompleteSession(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	accountID := authcontext.GetAccountID(ctx)

	// Parse session ID from path
	sessionIDStr := r.PathValue("id")
	sessionID, err := uuid.Parse(sessionIDStr)
	if err != nil {
		h.logger.Error(ctx, "invalid session ID", "error", err)
		api.WriteBadRequestResponse(w, "invalid_session_id", "Invalid session ID")
		return
	}

	// Get session to determine activity group
	session, err := h.sessionService.GetSession(ctx, sessionID)
	if err != nil {
		h.handleServiceError(ctx, w, err)
		return
	}

	// Get user's role from membership
	userRole, err := h.getUserRole(ctx, session.ActivityGroupID(), accountID)
	if err != nil {
		h.handleServiceError(ctx, w, err)
		return
	}

	// Complete session
	if err := h.sessionService.CompleteSession(ctx, sessionID, accountID, userRole); err != nil {
		h.handleServiceError(ctx, w, err)
		return
	}

	h.logger.Info(ctx, "session completed", "session_id", sessionID)

	api.WriteOkResponse(w, dtos.EmptyResponse{}, "Session completed successfully")
}

// CancelSession handles DELETE /api/v1/sessions/{id}
//
//	@Summary		Cancel a session
//	@Description	Cancels a session (soft delete)
//	@Tags			Sessions
//	@Produce		json
//	@Security		GatewayKeyAuth && BearerAuth
//	@Param			id	path		string							true	"Session ID"
//	@Success		200	{object}	api.Response[dtos.EmptyResponse]	"Session cancelled successfully"
//	@Failure		400	{object}	api.Response[any]				"Invalid request"
//	@Failure		404	{object}	api.Response[any]				"Session not found"
//	@Failure		500	{object}	api.Response[any]				"Internal server error"
//	@Router			/api/v1/sessions/{id} [delete]
func (h *Handler) CancelSession(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	accountID := authcontext.GetAccountID(ctx)

	// Parse session ID from path
	sessionIDStr := r.PathValue("id")
	sessionID, err := uuid.Parse(sessionIDStr)
	if err != nil {
		h.logger.Error(ctx, "invalid session ID", "error", err)
		api.WriteBadRequestResponse(w, "invalid_session_id", "Invalid session ID")
		return
	}

	// Get session to determine activity group
	session, err := h.sessionService.GetSession(ctx, sessionID)
	if err != nil {
		h.handleServiceError(ctx, w, err)
		return
	}

	// Get user's role from membership
	userRole, err := h.getUserRole(ctx, session.ActivityGroupID(), accountID)
	if err != nil {
		h.handleServiceError(ctx, w, err)
		return
	}

	// Cancel session with default reason
	reason := "Cancelled by user"
	if err := h.sessionService.CancelSession(ctx, sessionID, accountID, userRole, reason); err != nil {
		h.handleServiceError(ctx, w, err)
		return
	}

	h.logger.Info(ctx, "session cancelled", "session_id", sessionID)

	api.WriteOkResponse(w, dtos.EmptyResponse{}, "Session cancelled successfully")
}

// ListSessions handles GET /api/v1/sessions
//
//	@Summary		List sessions
//	@Description	Lists sessions with optional filters
//	@Tags			Sessions
//	@Produce		json
//	@Security		GatewayKeyAuth && BearerAuth
//	@Param			activity_group_id	query		string									false	"Filter by activity group ID"
//	@Param			session_template_id	query		string									false	"Filter by session template ID"
//	@Param			status				query		string									false	"Filter by status (scheduled, started, canceled, completed)"
//	@Param			is_recurring		query		boolean									false	"Filter by recurring status"
//	@Param			start_time_from		query		string									false	"Filter by start time from (RFC3339)"
//	@Param			start_time_to		query		string									false	"Filter by start time to (RFC3339)"
//	@Param			limit				query		int										false	"Limit number of results (default 20)"
//	@Param			offset				query		int										false	"Offset for pagination (default 0)"
//	@Success		200					{object}	api.Response[dtos.SessionListResponse]	"Sessions retrieved successfully"
//	@Failure		400					{object}	api.Response[any]							"Invalid request"
//	@Failure		500					{object}	api.Response[any]							"Internal server error"
//	@Router			/api/v1/sessions [get]
func (h *Handler) ListSessions(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Parse query parameters
	params, err := mapper.QueryToListSessionsParams(r.URL.Query())
	if err != nil {
		h.logger.Error(ctx, "failed to parse query parameters", "error", err)
		api.WriteBadRequestResponse(w, "invalid_params", "Invalid query parameters")
		return
	}

	// List sessions
	sessions, err := h.sessionService.ListSessions(ctx, *params)
	if err != nil {
		h.handleServiceError(ctx, w, err)
		return
	}

	api.WriteOkResponse(w,
		mapper.SessionListToResponse(sessions, params.Limit, params.Offset),
		"")
}
