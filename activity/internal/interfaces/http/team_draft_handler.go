package http

import (
	"net/http"

	"github.com/google/uuid"
	"github.com/rasparac/rekreativko-api/activity/internal/application"
	"github.com/rasparac/rekreativko-api/activity/internal/interfaces/http/dtos"
	"github.com/rasparac/rekreativko-api/activity/internal/interfaces/http/mapper"
	"github.com/rasparac/rekreativko-api/shared/api"
	"github.com/rasparac/rekreativko-api/shared/authcontext"
)

// StartDraft handles POST /api/v1/sessions/{id}/draft
//
//	@Summary		Start a captain draft
//	@Description	Starts a captain draft for a team-sport session: two people going become captains (the first leads Team A and picks first) and pick everyone else going in turn (snake A,B,B,A by default, or alternate A,B,A,B). Needs at least 2 x min_players_per_team people going (2 without a minimum). The session's current teams are untouched until the draft completes - when everyone going has been picked - and are then replaced by the two drafted teams. People joining mid-draft enter the pool; a captain leaving pauses it (see PUT /draft/captains); fewer people than needed cancels it. While a draft runs, POST /sessions/{id}/teams and direct team assignment return 409 draft_already_active. Session creator or group admin/creator only. No turn timeout.
//	@Tags			Team draft
//	@Accept			json
//	@Produce		json
//	@Security		GatewayKeyAuth && BearerAuth
//	@Param			id		path		string								true	"Session ID"
//	@Param			request	body		dtos.StartDraftRequest				true	"Captains and draft setup"
//	@Success		201		{object}	api.Response[dtos.DraftResponse]	"Draft started (or already completed, when nobody besides the captains is attending)"
//	@Failure		400		{object}	api.Response[any]					"Invalid request"
//	@Failure		401		{object}	api.Response[any]					"Unauthorized"
//	@Failure		404		{object}	api.Response[any]					"Session not found"
//	@Failure		409		{object}	api.Response[any]					"A draft is already running, not enough people are going, or the session is canceled/completed"
//	@Failure		422		{object}	api.Response[any]					"Not a team sport, invalid captains (not going, same person twice), pick order or colors"
//	@Failure		500		{object}	api.Response[any]					"Internal server error"
//	@Router			/api/v1/sessions/{id}/draft [post]
func (h *Handler) StartDraft(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	accountID := authcontext.GetAccountID(ctx)

	sessionID, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		h.logger.Error(ctx, "invalid session ID", "error", err)
		api.WriteBadRequestResponse(w, "invalid_session_id", "Invalid session ID")
		return
	}

	var req dtos.StartDraftRequest
	if err := api.DecodeJSONBody(r, &req); err != nil {
		h.logger.Error(ctx, "failed to decode request body", "error", err)
		api.WriteValidationErrorResponse(w, err)
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

	params := mapper.StartDraftRequestToParams(&req, sessionID, accountID, requesterRole)

	state, err := h.teamDraftService.StartDraft(ctx, *params)
	if err != nil {
		h.handleServiceError(ctx, w, err)
		return
	}

	h.logger.Info(ctx, "team draft started", "session_id", sessionID, "draft_id", state.Draft.ID())

	api.WriteCreatedResponse(w, mapper.TeamDraftStateToResponse(state), "Draft started")
}

// PickDraftPlayer handles POST /api/v1/sessions/{id}/draft/picks
//
//	@Summary		Pick a player in the captain draft
//	@Description	The captain whose turn it is (the caller) picks one available player. The response carries the updated draft - whose turn is next, or the final teams when this pick completed it.
//	@Tags			Team draft
//	@Accept			json
//	@Produce		json
//	@Security		GatewayKeyAuth && BearerAuth
//	@Param			id		path		string								true	"Session ID"
//	@Param			request	body		dtos.DraftPickRequest				true	"Player to pick"
//	@Success		200		{object}	api.Response[dtos.DraftResponse]	"Player picked"
//	@Failure		400		{object}	api.Response[any]					"Invalid request"
//	@Failure		401		{object}	api.Response[any]					"Unauthorized"
//	@Failure		403		{object}	api.Response[any]					"Caller is not a draft captain"
//	@Failure		404		{object}	api.Response[any]					"Session not found"
//	@Failure		409		{object}	api.Response[any]					"No draft running, draft paused, not your turn, player not available, or session canceled/completed"
//	@Failure		500		{object}	api.Response[any]					"Internal server error"
//	@Router			/api/v1/sessions/{id}/draft/picks [post]
func (h *Handler) PickDraftPlayer(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	accountID := authcontext.GetAccountID(ctx)

	sessionID, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		h.logger.Error(ctx, "invalid session ID", "error", err)
		api.WriteBadRequestResponse(w, "invalid_session_id", "Invalid session ID")
		return
	}

	var req dtos.DraftPickRequest
	if err := api.DecodeJSONBody(r, &req); err != nil {
		h.logger.Error(ctx, "failed to decode request body", "error", err)
		api.WriteValidationErrorResponse(w, err)
		return
	}

	// Private sessions are only visible to related users - 404 for anyone else
	if _, err := h.sessionService.GetSession(ctx, sessionID, accountID); err != nil {
		h.handleServiceError(ctx, w, err)
		return
	}

	state, err := h.teamDraftService.Pick(ctx, application.DraftPickParams{
		SessionID: sessionID,
		CaptainID: accountID,
		UserID:    req.UserID,
	})
	if err != nil {
		h.handleServiceError(ctx, w, err)
		return
	}

	api.WriteOkResponse(w, mapper.TeamDraftStateToResponse(state), "")
}

// ReplaceDraftCaptain handles PUT /api/v1/sessions/{id}/draft/captains
//
//	@Summary		Replace a captain who left
//	@Description	While a draft is paused because a captain stopped going, the organizer names a new captain for that team: someone already on it, or still unpicked. The draft continues where it stopped (and completes if nobody is left to pick). Session creator or group admin/creator only.
//	@Tags			Team draft
//	@Accept			json
//	@Produce		json
//	@Security		GatewayKeyAuth && BearerAuth
//	@Param			id		path		string								true	"Session ID"
//	@Param			request	body		dtos.ReplaceDraftCaptainRequest		true	"Team position and new captain"
//	@Success		200		{object}	api.Response[dtos.DraftResponse]	"Captain replaced"
//	@Failure		400		{object}	api.Response[any]					"Invalid request"
//	@Failure		401		{object}	api.Response[any]					"Unauthorized"
//	@Failure		404		{object}	api.Response[any]					"Session not found"
//	@Failure		409		{object}	api.Response[any]					"No draft running, or the draft is not paused"
//	@Failure		422		{object}	api.Response[any]					"invalid_captains: that team has a captain, or the user is not going, is on the other team or is the other captain"
//	@Failure		500		{object}	api.Response[any]					"Internal server error"
//	@Router			/api/v1/sessions/{id}/draft/captains [put]
func (h *Handler) ReplaceDraftCaptain(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	accountID := authcontext.GetAccountID(ctx)

	sessionID, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		h.logger.Error(ctx, "invalid session ID", "error", err)
		api.WriteBadRequestResponse(w, "invalid_session_id", "Invalid session ID")
		return
	}

	var req dtos.ReplaceDraftCaptainRequest
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

	params := mapper.ReplaceDraftCaptainRequestToParams(&req, sessionID, accountID, requesterRole)

	state, err := h.teamDraftService.ReplaceCaptain(ctx, *params)
	if err != nil {
		h.handleServiceError(ctx, w, err)
		return
	}

	h.logger.Info(ctx, "draft captain replaced", "session_id", sessionID, "draft_id", state.Draft.ID())

	api.WriteOkResponse(w, mapper.TeamDraftStateToResponse(state), "Captain replaced")
}

// GetDraft handles GET /api/v1/sessions/{id}/draft
//
//	@Summary		Get the captain draft
//	@Description	Returns the session's latest captain draft (running, completed or cancelled) with both sides, all picks, whose turn it is and who can still be picked. Use it to render the draft screen and to resync after reconnecting.
//	@Tags			Team draft
//	@Produce		json
//	@Security		GatewayKeyAuth && BearerAuth
//	@Param			id	path		string								true	"Session ID"
//	@Success		200	{object}	api.Response[dtos.DraftResponse]	"Draft found"
//	@Failure		400	{object}	api.Response[any]					"Invalid request"
//	@Failure		401	{object}	api.Response[any]					"Unauthorized"
//	@Failure		404	{object}	api.Response[any]					"Session not found, or it never had a draft"
//	@Failure		500	{object}	api.Response[any]					"Internal server error"
//	@Router			/api/v1/sessions/{id}/draft [get]
func (h *Handler) GetDraft(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	accountID := authcontext.GetAccountID(ctx)

	sessionID, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		h.logger.Error(ctx, "invalid session ID", "error", err)
		api.WriteBadRequestResponse(w, "invalid_session_id", "Invalid session ID")
		return
	}

	if _, err := h.sessionService.GetSession(ctx, sessionID, accountID); err != nil {
		h.handleServiceError(ctx, w, err)
		return
	}

	state, err := h.teamDraftService.GetDraft(ctx, sessionID)
	if err != nil {
		h.handleServiceError(ctx, w, err)
		return
	}

	api.WriteOkResponse(w, mapper.TeamDraftStateToResponse(state), "")
}

// CancelDraft handles DELETE /api/v1/sessions/{id}/draft
//
//	@Summary		Cancel the captain draft
//	@Description	Stops the running draft. The session's teams stay exactly as they were before the draft. Session creator or group admin/creator only.
//	@Tags			Team draft
//	@Produce		json
//	@Security		GatewayKeyAuth && BearerAuth
//	@Param			id	path		string								true	"Session ID"
//	@Success		200	{object}	api.Response[dtos.DraftResponse]	"Draft cancelled"
//	@Failure		400	{object}	api.Response[any]					"Invalid request"
//	@Failure		401	{object}	api.Response[any]					"Unauthorized"
//	@Failure		404	{object}	api.Response[any]					"Session not found"
//	@Failure		409	{object}	api.Response[any]					"No draft running"
//	@Failure		500	{object}	api.Response[any]					"Internal server error"
//	@Router			/api/v1/sessions/{id}/draft [delete]
func (h *Handler) CancelDraft(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	accountID := authcontext.GetAccountID(ctx)

	sessionID, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		h.logger.Error(ctx, "invalid session ID", "error", err)
		api.WriteBadRequestResponse(w, "invalid_session_id", "Invalid session ID")
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

	state, err := h.teamDraftService.CancelDraft(ctx, application.CancelDraftParams{
		SessionID:     sessionID,
		RequesterID:   accountID,
		RequesterRole: requesterRole,
	})
	if err != nil {
		h.handleServiceError(ctx, w, err)
		return
	}

	h.logger.Info(ctx, "team draft cancelled", "session_id", sessionID, "draft_id", state.Draft.ID())

	api.WriteOkResponse(w, mapper.TeamDraftStateToResponse(state), "Draft cancelled")
}
