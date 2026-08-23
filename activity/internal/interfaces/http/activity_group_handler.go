package http

import (
	"net/http"

	"github.com/google/uuid"
	"github.com/rasparac/rekreativko-api/activity/internal/interfaces/http/dtos"
	"github.com/rasparac/rekreativko-api/activity/internal/interfaces/http/mapper"
	"github.com/rasparac/rekreativko-api/shared/api"
	"github.com/rasparac/rekreativko-api/shared/authcontext"
)

// CreateActivityGroup handles POST /api/v1/activity-groups
//
//	@Summary		Create an activity group
//	@Description	Creates a new activity group
//	@Tags			Activity Groups
//	@Accept			json
//	@Produce		json
//	@Security		GatewayKeyAuth && BearerAuth
//	@Param			request	body		dtos.CreateActivityGroupRequest				true	"Activity group data"
//	@Success		201		{object}	api.Response[dtos.CreateActivityGroupResponse]	"Group created successfully"
//	@Failure		400		{object}	api.Response[any]								"Invalid request"
//	@Failure		401		{object}	api.Response[any]								"Unauthorized"
//	@Failure		500		{object}	api.Response[any]								"Internal server error"
//	@Router			/api/v1/activity-groups [post]
func (h *Handler) CreateActivityGroup(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	accountID := authcontext.GetAccountID(ctx)

	// Decode request body
	var req dtos.CreateActivityGroupRequest
	if err := api.DecodeJSONBody(r, &req); err != nil {
		h.logger.Error(ctx, "failed to decode request body", "error", err)
		api.WriteValidationErrorResponse(w, err)
		return
	}

	// Convert to application params
	params := mapper.CreateActivityGroupRequestToParams(&req, accountID)

	// Create group
	group, err := h.activityGroupService.CreateActivityGroup(ctx, *params)
	if err != nil {
		h.handleServiceError(ctx, w, err)
		return
	}

	h.logger.Info(ctx, "activity group created",
		"group_id", group.ID(),
		"creator_id", accountID,
	)

	api.WriteCreatedResponse(w, dtos.CreateActivityGroupResponse{
		ID: group.ID(),
	}, "Activity group created successfully")
}

// GetActivityGroup handles GET /api/v1/activity-groups/{id}
//
//	@Summary		Get an activity group
//	@Description	Retrieves an activity group by ID
//	@Tags			Activity Groups
//	@Produce		json
//	@Security		GatewayKeyAuth && BearerAuth
//	@Param			id	path		string									true	"Activity Group ID"
//	@Success		200	{object}	api.Response[dtos.ActivityGroupResponse]	"Group retrieved successfully"
//	@Failure		400	{object}	api.Response[any]							"Invalid request"
//	@Failure		404	{object}	api.Response[any]							"Group not found"
//	@Failure		500	{object}	api.Response[any]							"Internal server error"
//	@Router			/api/v1/activity-groups/{id} [get]
func (h *Handler) GetActivityGroup(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Parse group ID from path
	groupIDStr := r.PathValue("id")
	groupID, err := uuid.Parse(groupIDStr)
	if err != nil {
		h.logger.Error(ctx, "invalid activity group ID", "error", err)
		api.WriteBadRequestResponse(w, "invalid_group_id", "Invalid activity group ID")
		return
	}

	// Get group
	group, err := h.activityGroupService.GetActivityGroup(ctx, groupID)
	if err != nil {
		h.handleServiceError(ctx, w, err)
		return
	}

	api.WriteOkResponse(w, mapper.ActivityGroupToResponse(group), "")
}

// UpdateActivityGroup handles PUT /api/v1/activity-groups/{id}
//
//	@Summary		Update an activity group
//	@Description	Updates an existing activity group
//	@Tags			Activity Groups
//	@Accept			json
//	@Produce		json
//	@Security		GatewayKeyAuth && BearerAuth
//	@Param			id		path		string								true	"Activity Group ID"
//	@Param			request	body		dtos.UpdateActivityGroupRequest		true	"Update data"
//	@Success		200		{object}	api.Response[dtos.EmptyResponse]	"Group updated successfully"
//	@Failure		400		{object}	api.Response[any]					"Invalid request"
//	@Failure		404		{object}	api.Response[any]					"Group not found"
//	@Failure		500		{object}	api.Response[any]					"Internal server error"
//	@Router			/api/v1/activity-groups/{id} [put]
func (h *Handler) UpdateActivityGroup(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	accountID := authcontext.GetAccountID(ctx)

	// Parse group ID from path
	groupIDStr := r.PathValue("id")
	groupID, err := uuid.Parse(groupIDStr)
	if err != nil {
		h.logger.Error(ctx, "invalid activity group ID", "error", err)
		api.WriteBadRequestResponse(w, "invalid_group_id", "Invalid activity group ID")
		return
	}

	// Decode request body
	var req dtos.UpdateActivityGroupRequest
	if err := api.DecodeJSONBody(r, &req); err != nil {
		h.logger.Error(ctx, "failed to decode request body", "error", err)
		api.WriteValidationErrorResponse(w, err)
		return
	}

	// Convert to application params
	params := mapper.UpdateActivityGroupRequestToParams(&req, accountID)

	// Update group
	if err := h.activityGroupService.UpdateActivityGroup(ctx, groupID, *params); err != nil {
		h.handleServiceError(ctx, w, err)
		return
	}

	h.logger.Info(ctx, "activity group updated", "group_id", groupID)

	api.WriteOkResponse(w, dtos.EmptyResponse{}, "Activity group updated successfully")
}

// ActivateActivityGroup handles POST /api/v1/activity-groups/{id}/activate
//
//	@Summary		Activate an activity group
//	@Description	Activates a draft activity group
//	@Tags			Activity Groups
//	@Produce		json
//	@Security		GatewayKeyAuth && BearerAuth
//	@Param			id	path		string							true	"Activity Group ID"
//	@Success		200	{object}	api.Response[dtos.EmptyResponse]	"Group activated successfully"
//	@Failure		400	{object}	api.Response[any]				"Invalid request"
//	@Failure		404	{object}	api.Response[any]				"Group not found"
//	@Failure		500	{object}	api.Response[any]				"Internal server error"
//	@Router			/api/v1/activity-groups/{id}/activate [post]
func (h *Handler) ActivateActivityGroup(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	accountID := authcontext.GetAccountID(ctx)

	// Parse group ID from path
	groupIDStr := r.PathValue("id")
	groupID, err := uuid.Parse(groupIDStr)
	if err != nil {
		h.logger.Error(ctx, "invalid activity group ID", "error", err)
		api.WriteBadRequestResponse(w, "invalid_group_id", "Invalid activity group ID")
		return
	}

	// Activate group
	if err := h.activityGroupService.ActivateActivityGroup(ctx, groupID, accountID); err != nil {
		h.handleServiceError(ctx, w, err)
		return
	}

	h.logger.Info(ctx, "activity group activated", "group_id", groupID)

	api.WriteOkResponse(w, dtos.EmptyResponse{}, "Activity group activated successfully")
}

// DeleteActivityGroup handles DELETE /api/v1/activity-groups/{id}
//
//	@Summary		Delete an activity group
//	@Description	Soft-deletes an activity group
//	@Tags			Activity Groups
//	@Accept			json
//	@Produce		json
//	@Security		GatewayKeyAuth && BearerAuth
//	@Param			id	path		string							true	"Activity Group ID"
//	@Success		200	{object}	api.Response[dtos.EmptyResponse]	"Group deleted successfully"
//	@Failure		400	{object}	api.Response[any]				"Invalid request"
//	@Failure		404	{object}	api.Response[any]				"Group not found"
//	@Failure		500	{object}	api.Response[any]				"Internal server error"
//	@Router			/api/v1/activity-groups/{id} [delete]
func (h *Handler) DeleteActivityGroup(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	accountID := authcontext.GetAccountID(ctx)

	// Parse group ID from path
	groupIDStr := r.PathValue("id")
	groupID, err := uuid.Parse(groupIDStr)
	if err != nil {
		h.logger.Error(ctx, "invalid activity group ID", "error", err)
		api.WriteBadRequestResponse(w, "invalid_group_id", "Invalid activity group ID")
		return
	}

	if err := h.activityGroupService.DeleteActivityGroup(ctx, groupID, accountID); err != nil {
		h.handleServiceError(ctx, w, err)
		return
	}

	h.logger.Info(ctx, "activity group deleted", "group_id", groupID)

	api.WriteOkResponse(w, dtos.EmptyResponse{}, "Activity group deleted successfully")
}

// ListActivityGroups handles GET /api/v1/activity-groups
//
//	@Summary		List activity groups
//	@Description	Lists activity groups with optional filters
//	@Tags			Activity Groups
//	@Produce		json
//	@Security		GatewayKeyAuth && BearerAuth
//	@Param			creator_id	query		string										false	"Filter by creator ID"
//	@Param			status		query		string										false	"Filter by status (draft, active, cancelled)"
//	@Param			title		query		string										false	"Filter by title (partial match)"
//	@Param			limit		query		int											false	"Limit number of results (default 20)"
//	@Param			offset		query		int											false	"Offset for pagination (default 0)"
//	@Success		200			{object}	api.Response[dtos.ActivityGroupListResponse]	"Groups retrieved successfully"
//	@Failure		400			{object}	api.Response[any]									"Invalid request"
//	@Failure		500			{object}	api.Response[any]									"Internal server error"
//	@Router			/api/v1/activity-groups [get]
func (h *Handler) ListActivityGroups(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Parse query parameters
	params, err := mapper.QueryToListActivityGroupsParams(r.URL.Query())
	if err != nil {
		h.logger.Error(ctx, "failed to parse query parameters", "error", err)
		api.WriteBadRequestResponse(w, "invalid_params", "Invalid query parameters")
		return
	}

	// List groups
	groups, err := h.activityGroupService.ListActivityGroups(ctx, *params)
	if err != nil {
		h.handleServiceError(ctx, w, err)
		return
	}

	api.WriteOkResponse(
		w,
		mapper.ActivityGroupListToResponse(groups, params.Limit, params.Offset),
		"",
	)
}

// DiscoverActivityGroups handles GET /api/v1/activity-groups/discover
//
//	@Summary		Discover activity groups
//	@Description	Discovers public activity groups with optional filters
//	@Tags			Activity Groups
//	@Produce		json
//	@Security		GatewayKeyAuth && BearerAuth
//	@Param			city			query		string										false	"Filter by city"
//	@Param			country			query		string										false	"Filter by country"
//	@Param			activity_type	query		string										false	"Filter by activity type"
//	@Param			limit			query		int											false	"Limit number of results (default 20)"
//	@Param			offset			query		int											false	"Offset for pagination (default 0)"
//	@Success		200				{object}	api.Response[dtos.ActivityGroupListResponse]	"Groups retrieved successfully"
//	@Failure		400				{object}	api.Response[any]									"Invalid request"
//	@Failure		500				{object}	api.Response[any]									"Internal server error"
//	@Router			/api/v1/activity-groups/discover [get]
func (h *Handler) DiscoverActivityGroups(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Parse query parameters
	params, err := mapper.QueryToDiscoverActivityGroupsParams(r.URL.Query())
	if err != nil {
		h.logger.Error(ctx, "failed to parse query parameters", "error", err)
		api.WriteBadRequestResponse(w, "invalid_params", "Invalid query parameters")
		return
	}

	// Discover groups
	groups, err := h.activityGroupService.DiscoverActivityGroups(ctx, *params)
	if err != nil {
		h.handleServiceError(ctx, w, err)
		return
	}

	api.WriteOkResponse(w,
		mapper.ActivityGroupListToResponse(groups, params.Limit, params.Offset),
		"")
}
