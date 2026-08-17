package http

import (
	"net/http"

	"github.com/google/uuid"
	"github.com/rasparac/rekreativko-api/activity/internal/interfaces/http/dtos"
	"github.com/rasparac/rekreativko-api/activity/internal/interfaces/http/mapper"
	"github.com/rasparac/rekreativko-api/shared/api"
	"github.com/rasparac/rekreativko-api/shared/authcontext"
)

// InviteMember handles POST /api/v1/activity-groups/{groupId}/members
//
//	@Summary		Invite a member to an activity group
//	@Description	Invites a user to join an activity group
//	@Tags			Members
//	@Accept			json
//	@Produce		json
//	@Security		GatewayKeyAuth && BearerAuth
//	@Param			groupId	path		string							true	"Activity Group ID"
//	@Param			request	body		dtos.InviteMemberRequest		true	"Invite data"
//	@Success		201		{object}	api.Response[dtos.InviteMemberResponse]	"Member invited successfully"
//	@Failure		400		{object}	api.Response[any]				"Invalid request"
//	@Failure		401		{object}	api.Response[any]				"Unauthorized"
//	@Failure		403		{object}	api.Response[any]				"Forbidden"
//	@Failure		500		{object}	api.Response[any]				"Internal server error"
//	@Router			/api/v1/activity-groups/{groupId}/members [post]
func (h *Handler) InviteMember(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	accountID := authcontext.GetAccountID(ctx)

	// Get activity group ID from path
	groupIDStr := r.PathValue("groupId")
	groupID, err := uuid.Parse(groupIDStr)
	if err != nil {
		h.logger.Error(ctx, "invalid activity group ID", "error", err)
		api.WriteValidationErrorResponse(w, err)
		return
	}

	// Decode request body
	var req dtos.InviteMemberRequest
	if err := api.DecodeJSONBody(r, &req); err != nil {
		h.logger.Error(ctx, "failed to decode request body", "error", err)
		api.WriteValidationErrorResponse(w, err)
		return
	}

	// Get requester's role from membership
	userRole, err := h.getUserRole(ctx, groupID, accountID)
	if err != nil {
		h.handleServiceError(ctx, w, err)
		return
	}

	// Convert to application params
	params := mapper.InviteMemberRequestToParams(&req, groupID, accountID, userRole)

	// Invite member
	member, err := h.memberService.InviteMember(ctx, *params)
	if err != nil {
		h.handleServiceError(ctx, w, err)
		return
	}

	h.logger.Info(ctx, "member invited",
		"member_id", member.ID(),
		"activity_group_id", groupID,
		"user_id", req.UserID,
	)

	api.WriteCreatedResponse(w, dtos.InviteMemberResponse{
		ID: member.ID(),
	}, "Member invited successfully")
}

// RemoveMember handles DELETE /api/v1/activity-groups/{groupId}/members/{userId}
//
//	@Summary		Remove a member from an activity group
//	@Description	Removes a member from an activity group
//	@Tags			Members
//	@Produce		json
//	@Security		GatewayKeyAuth && BearerAuth
//	@Param			groupId	path		string				true	"Activity Group ID"
//	@Param			userId	path		string				true	"User ID"
//	@Success		200		{object}	api.Response[any]	"Member removed successfully"
//	@Failure		400		{object}	api.Response[any]	"Invalid request"
//	@Failure		401		{object}	api.Response[any]	"Unauthorized"
//	@Failure		403		{object}	api.Response[any]	"Forbidden"
//	@Failure		404		{object}	api.Response[any]	"Not found"
//	@Failure		500		{object}	api.Response[any]	"Internal server error"
//	@Router			/api/v1/activity-groups/{groupId}/members/{userId} [delete]
func (h *Handler) RemoveMember(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	accountID := authcontext.GetAccountID(ctx)

	// Get activity group ID from path
	groupIDStr := r.PathValue("groupId")
	groupID, err := uuid.Parse(groupIDStr)
	if err != nil {
		h.logger.Error(ctx, "invalid activity group ID", "error", err)
		api.WriteValidationErrorResponse(w, err)
		return
	}

	// Get user ID from path
	userIDStr := r.PathValue("userId")
	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		h.logger.Error(ctx, "invalid user ID", "error", err)
		api.WriteValidationErrorResponse(w, err)
		return
	}

	// Get requester's role from membership
	userRole, err := h.getUserRole(ctx, groupID, accountID)
	if err != nil {
		h.handleServiceError(ctx, w, err)
		return
	}

	// Convert to application params
	params := mapper.RemoveMemberRequestToParams(groupID, userID, accountID, userRole)

	// Remove member
	err = h.memberService.RemoveMember(ctx, *params)
	if err != nil {
		h.handleServiceError(ctx, w, err)
		return
	}

	h.logger.Info(ctx, "member removed",
		"activity_group_id", groupID,
		"user_id", userID,
	)

	api.WriteOkResponse(w, struct{}{}, "Member removed successfully")
}

// UpdateMemberRole handles PATCH /api/v1/activity-groups/{groupId}/members/{userId}/role
//
//	@Summary		Update a member's role
//	@Description	Updates a member's role (promote to admin or demote to member)
//	@Tags			Members
//	@Accept			json
//	@Produce		json
//	@Security		GatewayKeyAuth && BearerAuth
//	@Param			groupId	path		string							true	"Activity Group ID"
//	@Param			userId	path		string							true	"User ID"
//	@Param			request	body		dtos.UpdateMemberRoleRequest	true	"Role data"
//	@Success		200		{object}	api.Response[any]				"Role updated successfully"
//	@Failure		400		{object}	api.Response[any]				"Invalid request"
//	@Failure		401		{object}	api.Response[any]				"Unauthorized"
//	@Failure		403		{object}	api.Response[any]				"Forbidden"
//	@Failure		404		{object}	api.Response[any]				"Not found"
//	@Failure		500		{object}	api.Response[any]				"Internal server error"
//	@Router			/api/v1/activity-groups/{groupId}/members/{userId}/role [patch]
func (h *Handler) UpdateMemberRole(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	accountID := authcontext.GetAccountID(ctx)

	// Get activity group ID from path
	groupIDStr := r.PathValue("groupId")
	groupID, err := uuid.Parse(groupIDStr)
	if err != nil {
		h.logger.Error(ctx, "invalid activity group ID", "error", err)
		api.WriteValidationErrorResponse(w, err)
		return
	}

	// Get user ID from path
	userIDStr := r.PathValue("userId")
	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		h.logger.Error(ctx, "invalid user ID", "error", err)
		api.WriteValidationErrorResponse(w, err)
		return
	}

	// Decode request body
	var req dtos.UpdateMemberRoleRequest
	if err := api.DecodeJSONBody(r, &req); err != nil {
		h.logger.Error(ctx, "failed to decode request body", "error", err)
		api.WriteValidationErrorResponse(w, err)
		return
	}

	// TODO: Get requester's role from membership query
	// For now, default to "creator" - the domain will validate permissions
	userRole := "creator"

	// Convert to application params
	params := mapper.UpdateMemberRoleRequestToParams(&req, groupID, userID, accountID, userRole)

	// Promote or demote based on new role
	if req.Role == "admin" {
		err = h.memberService.PromoteMember(ctx, *params)
	} else {
		err = h.memberService.DemoteMember(ctx, *params)
	}

	if err != nil {
		h.handleServiceError(ctx, w, err)
		return
	}

	h.logger.Info(ctx, "member role updated",
		"activity_group_id", groupID,
		"user_id", userID,
		"new_role", req.Role,
	)

	api.WriteOkResponse(w, struct{}{}, "Member role updated successfully")
}

// ApproveMember handles POST /api/v1/activity-groups/{groupId}/members/{userId}/approve
//
//	@Summary		Approve a join request
//	@Description	Approves a pending join request
//	@Tags			Members
//	@Produce		json
//	@Security		GatewayKeyAuth && BearerAuth
//	@Param			groupId	path		string				true	"Activity Group ID"
//	@Param			userId	path		string				true	"User ID"
//	@Success		200		{object}	api.Response[any]	"Join request approved"
//	@Failure		400		{object}	api.Response[any]	"Invalid request"
//	@Failure		401		{object}	api.Response[any]	"Unauthorized"
//	@Failure		403		{object}	api.Response[any]	"Forbidden"
//	@Failure		404		{object}	api.Response[any]	"Not found"
//	@Failure		500		{object}	api.Response[any]	"Internal server error"
//	@Router			/api/v1/activity-groups/{groupId}/members/{userId}/approve [post]
func (h *Handler) ApproveMember(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	accountID := authcontext.GetAccountID(ctx)

	// Get activity group ID from path
	groupIDStr := r.PathValue("groupId")
	groupID, err := uuid.Parse(groupIDStr)
	if err != nil {
		h.logger.Error(ctx, "invalid activity group ID", "error", err)
		api.WriteValidationErrorResponse(w, err)
		return
	}

	// Get user ID from path
	userIDStr := r.PathValue("userId")
	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		h.logger.Error(ctx, "invalid user ID", "error", err)
		api.WriteValidationErrorResponse(w, err)
		return
	}

	// Get requester's role from membership
	userRole, err := h.getUserRole(ctx, groupID, accountID)
	if err != nil {
		h.handleServiceError(ctx, w, err)
		return
	}

	// Convert to application params
	params := mapper.ApproveMemberRequestToParams(groupID, userID, accountID, userRole)

	// Approve member
	err = h.memberService.ApproveMember(ctx, *params)
	if err != nil {
		h.handleServiceError(ctx, w, err)
		return
	}

	h.logger.Info(ctx, "member approved",
		"activity_group_id", groupID,
		"user_id", userID,
	)

	api.WriteOkResponse(w, struct{}{}, "Join request approved")
}

// RejectMember handles POST /api/v1/activity-groups/{groupId}/members/{userId}/reject
//
//	@Summary		Reject a join request
//	@Description	Rejects a pending join request
//	@Tags			Members
//	@Produce		json
//	@Security		GatewayKeyAuth && BearerAuth
//	@Param			groupId	path		string				true	"Activity Group ID"
//	@Param			userId	path		string				true	"User ID"
//	@Success		200		{object}	api.Response[any]	"Join request rejected"
//	@Failure		400		{object}	api.Response[any]	"Invalid request"
//	@Failure		401		{object}	api.Response[any]	"Unauthorized"
//	@Failure		403		{object}	api.Response[any]	"Forbidden"
//	@Failure		404		{object}	api.Response[any]	"Not found"
//	@Failure		500		{object}	api.Response[any]	"Internal server error"
//	@Router			/api/v1/activity-groups/{groupId}/members/{userId}/reject [post]
func (h *Handler) RejectMember(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	accountID := authcontext.GetAccountID(ctx)

	// Get activity group ID from path
	groupIDStr := r.PathValue("groupId")
	groupID, err := uuid.Parse(groupIDStr)
	if err != nil {
		h.logger.Error(ctx, "invalid activity group ID", "error", err)
		api.WriteValidationErrorResponse(w, err)
		return
	}

	// Get user ID from path
	userIDStr := r.PathValue("userId")
	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		h.logger.Error(ctx, "invalid user ID", "error", err)
		api.WriteValidationErrorResponse(w, err)
		return
	}

	// Get requester's role from membership
	userRole, err := h.getUserRole(ctx, groupID, accountID)
	if err != nil {
		h.handleServiceError(ctx, w, err)
		return
	}

	// Convert to application params
	params := mapper.RejectMemberRequestToParams(groupID, userID, accountID, userRole)

	// Reject member
	err = h.memberService.RejectMember(ctx, *params)
	if err != nil {
		h.handleServiceError(ctx, w, err)
		return
	}

	h.logger.Info(ctx, "member rejected",
		"activity_group_id", groupID,
		"user_id", userID,
	)

	api.WriteOkResponse(w, struct{}{}, "Join request rejected")
}

// LeaveMember handles POST /api/v1/activity-groups/{groupId}/leave
//
//	@Summary		Leave an activity group
//	@Description	Allows the authenticated user to leave an activity group
//	@Tags			Members
//	@Produce		json
//	@Security		GatewayKeyAuth && BearerAuth
//	@Param			groupId	path		string				true	"Activity Group ID"
//	@Success		200		{object}	api.Response[any]	"Successfully left the group"
//	@Failure		400		{object}	api.Response[any]	"Invalid request"
//	@Failure		401		{object}	api.Response[any]	"Unauthorized"
//	@Failure		403		{object}	api.Response[any]	"Forbidden"
//	@Failure		404		{object}	api.Response[any]	"Not found"
//	@Failure		500		{object}	api.Response[any]	"Internal server error"
//	@Router			/api/v1/activity-groups/{groupId}/leave [post]
func (h *Handler) LeaveMember(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	accountID := authcontext.GetAccountID(ctx)

	// Get activity group ID from path
	groupIDStr := r.PathValue("groupId")
	groupID, err := uuid.Parse(groupIDStr)
	if err != nil {
		h.logger.Error(ctx, "invalid activity group ID", "error", err)
		api.WriteValidationErrorResponse(w, err)
		return
	}

	// Leave group
	err = h.memberService.LeaveMember(ctx, groupID, accountID)
	if err != nil {
		h.handleServiceError(ctx, w, err)
		return
	}

	h.logger.Info(ctx, "member left group",
		"activity_group_id", groupID,
		"user_id", accountID,
	)

	api.WriteOkResponse(w, struct{}{}, "Successfully left the group")
}

// GetMember handles GET /api/v1/activity-groups/{groupId}/members/{userId}
//
//	@Summary		Get a member
//	@Description	Retrieves a member by activity group and user ID
//	@Tags			Members
//	@Produce		json
//	@Security		GatewayKeyAuth && BearerAuth
//	@Param			groupId	path		string							true	"Activity Group ID"
//	@Param			userId	path		string							true	"User ID"
//	@Success		200		{object}	api.Response[dtos.MemberResponse]	"Member data"
//	@Failure		400		{object}	api.Response[any]				"Invalid request"
//	@Failure		401		{object}	api.Response[any]				"Unauthorized"
//	@Failure		404		{object}	api.Response[any]				"Not found"
//	@Failure		500		{object}	api.Response[any]				"Internal server error"
//	@Router			/api/v1/activity-groups/{groupId}/members/{userId} [get]
func (h *Handler) GetMember(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Get activity group ID from path
	groupIDStr := r.PathValue("groupId")
	groupID, err := uuid.Parse(groupIDStr)
	if err != nil {
		h.logger.Error(ctx, "invalid activity group ID", "error", err)
		api.WriteValidationErrorResponse(w, err)
		return
	}

	// Get user ID from path
	userIDStr := r.PathValue("userId")
	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		h.logger.Error(ctx, "invalid user ID", "error", err)
		api.WriteValidationErrorResponse(w, err)
		return
	}

	// Get member
	member, err := h.memberService.GetMember(ctx, groupID, userID)
	if err != nil {
		h.handleServiceError(ctx, w, err)
		return
	}

	api.WriteOkResponse(w, mapper.MemberToResponse(member), "")
}

// ListMembers handles GET /api/v1/activity-groups/{groupId}/members
//
//	@Summary		List members
//	@Description	Lists members of an activity group with optional filters
//	@Tags			Members
//	@Produce		json
//	@Security		GatewayKeyAuth && BearerAuth
//	@Param			groupId	path		string								true	"Activity Group ID"
//	@Param			status	query		string								false	"Filter by status (pending, confirmed, rejected, left, removed)"
//	@Param			role	query		string								false	"Filter by role (creator, admin, member)"
//	@Param			limit	query		int									false	"Limit results"	default(20)
//	@Param			offset	query		int									false	"Offset results"	default(0)
//	@Success		200		{object}	api.Response[dtos.MemberListResponse]	"List of members"
//	@Failure		400		{object}	api.Response[any]					"Invalid request"
//	@Failure		401		{object}	api.Response[any]					"Unauthorized"
//	@Failure		500		{object}	api.Response[any]					"Internal server error"
//	@Router			/api/v1/activity-groups/{groupId}/members [get]
func (h *Handler) ListMembers(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Get activity group ID from path
	groupIDStr := r.PathValue("groupId")
	groupID, err := uuid.Parse(groupIDStr)
	if err != nil {
		h.logger.Error(ctx, "invalid activity group ID", "error", err)
		api.WriteValidationErrorResponse(w, err)
		return
	}

	// Parse query parameters
	params, err := mapper.QueryToListMembersParams(r.URL.Query())
	if err != nil {
		h.logger.Error(ctx, "failed to parse query parameters", "error", err)
		api.WriteValidationErrorResponse(w, err)
		return
	}

	// Set activity group ID from path
	params.ActivityGroupID = &groupID

	// List members
	members, err := h.memberService.ListMembers(ctx, *params)
	if err != nil {
		h.handleServiceError(ctx, w, err)
		return
	}

	api.WriteOkResponse(
		w,
		mapper.MemberListToResponse(members, params.Limit, params.Offset),
		"",
	)
}
