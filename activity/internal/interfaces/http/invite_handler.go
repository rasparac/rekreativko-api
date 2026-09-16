package http

import (
	"net/http"

	"github.com/google/uuid"
	"github.com/rasparac/rekreativko-api/activity/internal/interfaces/http/dtos"
	"github.com/rasparac/rekreativko-api/activity/internal/interfaces/http/mapper"
	"github.com/rasparac/rekreativko-api/shared/api"
	"github.com/rasparac/rekreativko-api/shared/authcontext"
)

// SendInvite handles POST /api/v1/activity-groups/{groupId}/invites
//
//	@Summary		Invite a user to an activity group
//	@Description	Sends a pending invite that the invited user can accept or decline
//	@Tags			Invites
//	@Accept			json
//	@Produce		json
//	@Security		GatewayKeyAuth && BearerAuth
//	@Param			groupId	path		string								true	"Activity Group ID"
//	@Param			request	body		dtos.SendInviteRequest				true	"Invite data"
//	@Success		201		{object}	api.Response[dtos.SendInviteResponse]	"Invite sent successfully"
//	@Failure		400		{object}	api.Response[any]						"Invalid request"
//	@Failure		401		{object}	api.Response[any]						"Unauthorized"
//	@Failure		403		{object}	api.Response[any]						"Forbidden"
//	@Failure		409		{object}	api.Response[any]						"User already a member or already invited"
//	@Failure		500		{object}	api.Response[any]						"Internal server error"
//	@Router			/api/v1/activity-groups/{groupId}/invites [post]
func (h *Handler) SendInvite(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	accountID := authcontext.GetAccountID(ctx)

	groupIDStr := r.PathValue("groupId")
	groupID, err := uuid.Parse(groupIDStr)
	if err != nil {
		h.logger.Error(ctx, "invalid activity group ID", "error", err)
		api.WriteValidationErrorResponse(w, err)
		return
	}

	var req dtos.SendInviteRequest
	if err := api.DecodeJSONBody(r, &req); err != nil {
		h.logger.Error(ctx, "failed to decode request body", "error", err)
		api.WriteValidationErrorResponse(w, err)
		return
	}

	userRole, err := h.getUserRole(ctx, &groupID, accountID)
	if err != nil {
		h.handleServiceError(ctx, w, err)
		return
	}

	params := mapper.SendInviteRequestToParams(&req, groupID, accountID, userRole)

	invite, err := h.inviteService.SendInvite(ctx, *params)
	if err != nil {
		h.handleServiceError(ctx, w, err)
		return
	}

	h.logger.Info(ctx, "invite sent",
		"invite_id", invite.ID(),
		"activity_group_id", groupID,
		"invited_user_id", req.UserID,
	)

	api.WriteCreatedResponse(w, dtos.SendInviteResponse{
		ID: invite.ID(),
	}, "Invite sent successfully")
}

// ListMyInvites handles GET /api/v1/invites
//
//	@Summary		List my pending invites
//	@Description	Lists the authenticated user's own pending group invites
//	@Tags			Invites
//	@Produce		json
//	@Security		GatewayKeyAuth && BearerAuth
//	@Param			limit		query		int										false	"Limit number of results (default 20)"
//	@Param			page_token	query		string									false	"Token from the previous response's next_page_token, to fetch the next page"
//	@Success		200			{object}	api.Response[api.Page[dtos.InviteResponse]]	"Invites retrieved successfully"
//	@Failure		401			{object}	api.Response[any]						"Unauthorized"
//	@Failure		500			{object}	api.Response[any]						"Internal server error"
//	@Router			/api/v1/invites [get]
func (h *Handler) ListMyInvites(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	accountID := authcontext.GetAccountID(ctx)

	params, err := mapper.QueryToListMyInvitesParams(r.URL.Query())
	if err != nil {
		h.logger.Error(ctx, "failed to parse query parameters", "error", err)
		api.WriteBadRequestResponse(w, "invalid_params", "Invalid query parameters")
		return
	}

	invites, nextPageToken, err := h.inviteService.ListMyInvites(ctx, accountID, *params)
	if err != nil {
		h.handleServiceError(ctx, w, err)
		return
	}

	api.WriteOkResponse(w, api.NewPage(invites, params.Limit, nextPageToken, mapper.InviteToResponse), "")
}

// AcceptInvite handles POST /api/v1/invites/{id}/accept
//
//	@Summary		Accept an invite
//	@Description	Accepts a pending invite, creating a confirmed membership
//	@Tags			Invites
//	@Produce		json
//	@Security		GatewayKeyAuth && BearerAuth
//	@Param			id	path		string							true	"Invite ID"
//	@Success		200	{object}	api.Response[dtos.EmptyResponse]	"Invite accepted successfully"
//	@Failure		400	{object}	api.Response[any]				"Invalid request"
//	@Failure		401	{object}	api.Response[any]				"Unauthorized"
//	@Failure		404	{object}	api.Response[any]				"Invite not found"
//	@Failure		409	{object}	api.Response[any]				"Invite expired or already processed"
//	@Failure		500	{object}	api.Response[any]				"Internal server error"
//	@Router			/api/v1/invites/{id}/accept [post]
func (h *Handler) AcceptInvite(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	accountID := authcontext.GetAccountID(ctx)

	inviteIDStr := r.PathValue("id")
	inviteID, err := uuid.Parse(inviteIDStr)
	if err != nil {
		h.logger.Error(ctx, "invalid invite ID", "error", err)
		api.WriteBadRequestResponse(w, "invalid_invite_id", "Invalid invite ID")
		return
	}

	member, err := h.inviteService.AcceptInvite(ctx, inviteID, accountID)
	if err != nil {
		h.handleServiceError(ctx, w, err)
		return
	}

	h.logger.Info(ctx, "invite accepted", "invite_id", inviteID, "member_id", member.ID())

	api.WriteOkResponse(w, dtos.EmptyResponse{}, "Invite accepted successfully")
}

// DeclineInvite handles POST /api/v1/invites/{id}/decline
//
//	@Summary		Decline an invite
//	@Description	Declines a pending invite; no membership is created
//	@Tags			Invites
//	@Produce		json
//	@Security		GatewayKeyAuth && BearerAuth
//	@Param			id	path		string							true	"Invite ID"
//	@Success		200	{object}	api.Response[dtos.EmptyResponse]	"Invite declined successfully"
//	@Failure		400	{object}	api.Response[any]				"Invalid request"
//	@Failure		401	{object}	api.Response[any]				"Unauthorized"
//	@Failure		404	{object}	api.Response[any]				"Invite not found"
//	@Failure		409	{object}	api.Response[any]				"Invite expired or already processed"
//	@Failure		500	{object}	api.Response[any]				"Internal server error"
//	@Router			/api/v1/invites/{id}/decline [post]
func (h *Handler) DeclineInvite(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	accountID := authcontext.GetAccountID(ctx)

	inviteIDStr := r.PathValue("id")
	inviteID, err := uuid.Parse(inviteIDStr)
	if err != nil {
		h.logger.Error(ctx, "invalid invite ID", "error", err)
		api.WriteBadRequestResponse(w, "invalid_invite_id", "Invalid invite ID")
		return
	}

	if err := h.inviteService.DeclineInvite(ctx, inviteID, accountID); err != nil {
		h.handleServiceError(ctx, w, err)
		return
	}

	h.logger.Info(ctx, "invite declined", "invite_id", inviteID)

	api.WriteOkResponse(w, dtos.EmptyResponse{}, "Invite declined successfully")
}
