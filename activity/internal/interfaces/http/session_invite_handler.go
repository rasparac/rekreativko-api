package http

import (
	"net/http"

	"github.com/google/uuid"
	"github.com/rasparac/rekreativko-api/activity/internal/interfaces/http/dtos"
	"github.com/rasparac/rekreativko-api/activity/internal/interfaces/http/mapper"
	"github.com/rasparac/rekreativko-api/shared/api"
	"github.com/rasparac/rekreativko-api/shared/authcontext"
)

// SendSessionInvite handles POST /api/v1/sessions/{sessionId}/invites
//
//	@Summary		Invite a user to a standalone session
//	@Description	Sends a pending invite that the invited user can accept or decline. Only the session creator can invite, and only to standalone sessions (sessions without an activity group)
//	@Tags			Session Invites
//	@Accept			json
//	@Produce		json
//	@Security		GatewayKeyAuth && BearerAuth
//	@Param			sessionId	path		string											true	"Session ID"
//	@Param			request		body		dtos.SendSessionInviteRequest					true	"Invite data"
//	@Success		201			{object}	api.Response[dtos.SendSessionInviteResponse]	"Invite sent successfully"
//	@Failure		400			{object}	api.Response[any]								"Invalid request"
//	@Failure		401			{object}	api.Response[any]								"Unauthorized, or caller is not the session creator"
//	@Failure		404			{object}	api.Response[any]								"Session not found"
//	@Failure		409			{object}	api.Response[any]								"User already attending or already invited, or session not scheduled"
//	@Failure		422			{object}	api.Response[any]								"Session belongs to a group, or inviting yourself"
//	@Failure		500			{object}	api.Response[any]								"Internal server error"
//	@Router			/api/v1/sessions/{sessionId}/invites [post]
func (h *Handler) SendSessionInvite(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	accountID := authcontext.GetAccountID(ctx)

	sessionID, err := uuid.Parse(r.PathValue("sessionId"))
	if err != nil {
		h.logger.Error(ctx, "invalid session ID", "error", err)
		api.WriteValidationErrorResponse(w, err)
		return
	}

	var req dtos.SendSessionInviteRequest
	if err := api.DecodeJSONBody(r, &req); err != nil {
		h.logger.Error(ctx, "failed to decode request body", "error", err)
		api.WriteValidationErrorResponse(w, err)
		return
	}

	params := mapper.SendSessionInviteRequestToParams(&req, sessionID, accountID)

	invite, err := h.sessionInviteService.SendInvite(ctx, *params)
	if err != nil {
		h.handleServiceError(ctx, w, err)
		return
	}

	h.logger.Info(ctx, "session invite sent",
		"invite_id", invite.ID(),
		"session_id", sessionID,
		"invited_user_id", req.UserID,
	)

	api.WriteCreatedResponse(w, dtos.SendSessionInviteResponse{
		ID: invite.ID(),
	}, "Invite sent successfully")
}

// ListMySessionInvites handles GET /api/v1/session-invites
//
//	@Summary		List my pending session invites
//	@Description	Lists the authenticated user's own pending, unexpired session invites
//	@Tags			Session Invites
//	@Produce		json
//	@Security		GatewayKeyAuth && BearerAuth
//	@Param			limit		query		int													false	"Limit number of results (default 20)"
//	@Param			page_token	query		string												false	"Token from the previous response's next_page_token, to fetch the next page"
//	@Success		200			{object}	api.Response[api.Page[dtos.SessionInviteResponse]]	"Invites retrieved successfully"
//	@Failure		401			{object}	api.Response[any]									"Unauthorized"
//	@Failure		500			{object}	api.Response[any]									"Internal server error"
//	@Router			/api/v1/session-invites [get]
func (h *Handler) ListMySessionInvites(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	accountID := authcontext.GetAccountID(ctx)

	params, err := mapper.QueryToListMyInvitesParams(r.URL.Query())
	if err != nil {
		h.logger.Error(ctx, "failed to parse query parameters", "error", err)
		api.WriteBadRequestResponse(w, "invalid_params", "Invalid query parameters")
		return
	}

	invites, nextPageToken, err := h.sessionInviteService.ListMyInvites(ctx, accountID, *params)
	if err != nil {
		h.handleServiceError(ctx, w, err)
		return
	}

	api.WriteOkResponse(w, api.NewPage(invites, params.Limit, nextPageToken, mapper.SessionInviteToResponse), "")
}

// AcceptSessionInvite handles POST /api/v1/session-invites/{id}/accept
//
//	@Summary		Accept a session invite
//	@Description	Accepts a pending invite, making the caller an attendee. If the session is full the attendee is placed on the waitlist (status "pending")
//	@Tags			Session Invites
//	@Produce		json
//	@Security		GatewayKeyAuth && BearerAuth
//	@Param			id	path		string										true	"Invite ID"
//	@Success		200	{object}	api.Response[dtos.AttendeeResponse]			"Invite accepted successfully"
//	@Failure		400	{object}	api.Response[any]							"Invalid request"
//	@Failure		401	{object}	api.Response[any]							"Unauthorized, or the invite is not addressed to the caller"
//	@Failure		404	{object}	api.Response[any]							"Invite not found"
//	@Failure		409	{object}	api.Response[any]							"Invite expired or already processed, session not scheduled, or already attending"
//	@Failure		500	{object}	api.Response[any]							"Internal server error"
//	@Router			/api/v1/session-invites/{id}/accept [post]
func (h *Handler) AcceptSessionInvite(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	accountID := authcontext.GetAccountID(ctx)

	inviteID, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		h.logger.Error(ctx, "invalid invite ID", "error", err)
		api.WriteBadRequestResponse(w, "invalid_invite_id", "Invalid invite ID")
		return
	}

	attendee, err := h.sessionInviteService.AcceptInvite(ctx, inviteID, accountID)
	if err != nil {
		h.handleServiceError(ctx, w, err)
		return
	}

	h.logger.Info(ctx, "session invite accepted", "invite_id", inviteID, "attendee_id", attendee.ID())

	api.WriteOkResponse(w, mapper.AttendeeToResponse(attendee), "Invite accepted successfully")
}

// DeclineSessionInvite handles POST /api/v1/session-invites/{id}/decline
//
//	@Summary		Decline a session invite
//	@Description	Declines a pending invite; no attendee is created
//	@Tags			Session Invites
//	@Produce		json
//	@Security		GatewayKeyAuth && BearerAuth
//	@Param			id	path		string								true	"Invite ID"
//	@Success		200	{object}	api.Response[dtos.EmptyResponse]	"Invite declined successfully"
//	@Failure		400	{object}	api.Response[any]					"Invalid request"
//	@Failure		401	{object}	api.Response[any]					"Unauthorized, or the invite is not addressed to the caller"
//	@Failure		404	{object}	api.Response[any]					"Invite not found"
//	@Failure		409	{object}	api.Response[any]					"Invite expired or already processed"
//	@Failure		500	{object}	api.Response[any]					"Internal server error"
//	@Router			/api/v1/session-invites/{id}/decline [post]
func (h *Handler) DeclineSessionInvite(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	accountID := authcontext.GetAccountID(ctx)

	inviteID, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		h.logger.Error(ctx, "invalid invite ID", "error", err)
		api.WriteBadRequestResponse(w, "invalid_invite_id", "Invalid invite ID")
		return
	}

	if err := h.sessionInviteService.DeclineInvite(ctx, inviteID, accountID); err != nil {
		h.handleServiceError(ctx, w, err)
		return
	}

	h.logger.Info(ctx, "session invite declined", "invite_id", inviteID)

	api.WriteOkResponse(w, dtos.EmptyResponse{}, "Invite declined successfully")
}
