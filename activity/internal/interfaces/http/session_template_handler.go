package http

import (
	"net/http"

	"github.com/google/uuid"
	"github.com/rasparac/rekreativko-api/activity/internal/interfaces/http/dtos"
	"github.com/rasparac/rekreativko-api/activity/internal/interfaces/http/mapper"
	"github.com/rasparac/rekreativko-api/shared/api"
	"github.com/rasparac/rekreativko-api/shared/authcontext"
)

// CreateSessionTemplate handles POST /api/v1/activity-groups/{groupId}/templates
//
//	@Summary		Create a session template
//	@Description	Creates a new session template for an activity group
//	@Tags			Session Templates
//	@Accept			json
//	@Produce		json
//	@Security		GatewayKeyAuth && BearerAuth
//	@Param			groupId	path		string											true	"Activity Group ID"
//	@Param			request	body		dtos.CreateSessionTemplateRequest				true	"Session template data"
//	@Success		201		{object}	api.Response[dtos.CreateSessionTemplateResponse]	"Template created successfully"
//	@Failure		400		{object}	api.Response[any]									"Invalid request"
//	@Failure		401		{object}	api.Response[any]									"Unauthorized"
//	@Failure		500		{object}	api.Response[any]									"Internal server error"
//	@Router			/api/v1/activity-groups/{groupId}/templates [post]
func (h *Handler) CreateSessionTemplate(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	accountID := authcontext.GetAccountID(ctx)

	// Parse activity group ID from path
	groupIDStr := r.PathValue("groupId")
	groupID, err := uuid.Parse(groupIDStr)
	if err != nil {
		h.logger.Error(ctx, "invalid activity group ID", "error", err)
		api.WriteBadRequestResponse(w, "invalid_group_id", "Invalid activity group ID")
		return
	}

	// Decode request body
	var req dtos.CreateSessionTemplateRequest
	if err := api.DecodeJSONBody(r, &req); err != nil {
		h.logger.Error(ctx, "failed to decode request body", "error", err)
		api.WriteValidationErrorResponse(w, err)
		return
	}

	// Convert to application params
	params, err := mapper.CreateRequestToParams(&req, groupID, accountID)
	if err != nil {
		h.logger.Error(ctx, "failed to convert request to params", "error", err)
		api.WriteBadRequestResponse(w, "invalid_params", "Invalid request parameters")
		return
	}

	// Create template
	template, err := h.sessionTemplateService.CreateSessionTemplate(ctx, *params)
	if err != nil {
		h.handleServiceError(ctx, w, err)
		return
	}

	h.logger.Info(ctx, "session template created",
		"template_id", template.ID(),
		"group_id", groupID,
	)

	api.WriteCreatedResponse(w, dtos.CreateSessionTemplateResponse{
		ID: template.ID(),
	}, "Session template created successfully")
}

// GetSessionTemplate handles GET /api/v1/templates/{id}
//
//	@Summary		Get a session template
//	@Description	Retrieves a session template by ID
//	@Tags			Session Templates
//	@Produce		json
//	@Security		GatewayKeyAuth && BearerAuth
//	@Param			id	path		string									true	"Template ID"
//	@Success		200	{object}	api.Response[dtos.SessionTemplateResponse]	"Template retrieved successfully"
//	@Failure		400	{object}	api.Response[any]							"Invalid request"
//	@Failure		404	{object}	api.Response[any]							"Template not found"
//	@Failure		500	{object}	api.Response[any]							"Internal server error"
//	@Router			/api/v1/templates/{id} [get]
func (h *Handler) GetSessionTemplate(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Parse template ID from path
	templateIDStr := r.PathValue("id")
	templateID, err := uuid.Parse(templateIDStr)
	if err != nil {
		h.logger.Error(ctx, "invalid template ID", "error", err)
		api.WriteBadRequestResponse(w, "invalid_template_id", "Invalid template ID")
		return
	}

	// Get template
	template, err := h.sessionTemplateService.GetSessionTemplate(ctx, templateID)
	if err != nil {
		h.handleServiceError(ctx, w, err)
		return
	}

	api.WriteOkResponse(w, mapper.DomainToResponse(template), "")
}

// ListSessionTemplates handles GET /api/v1/activity-groups/{groupId}/templates
//
//	@Summary		List session templates
//	@Description	Lists all session templates for an activity group with optional filters
//	@Tags			Session Templates
//	@Produce		json
//	@Security		GatewayKeyAuth && BearerAuth
//	@Param			groupId		path		string										true	"Activity Group ID"
//	@Param			status		query		string										false	"Filter by status (active, inactive)"
//	@Param			is_recurring	query		boolean										false	"Filter by recurring status"
//	@Param			limit		query		int											false	"Limit number of results (default 20)"
//	@Param			offset		query		int											false	"Offset for pagination (default 0)"
//	@Success		200			{object}	api.Response[dtos.SessionTemplateListResponse]	"Templates retrieved successfully"
//	@Failure		400			{object}	api.Response[any]									"Invalid request"
//	@Failure		500			{object}	api.Response[any]									"Internal server error"
//	@Router			/api/v1/activity-groups/{groupId}/templates [get]
func (h *Handler) ListSessionTemplates(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Parse activity group ID from path
	groupIDStr := r.PathValue("groupId")
	groupID, err := uuid.Parse(groupIDStr)
	if err != nil {
		h.logger.Error(ctx, "invalid activity group ID", "error", err)
		api.WriteBadRequestResponse(w, "invalid_group_id", "Invalid activity group ID")
		return
	}

	// Parse query parameters
	params, err := mapper.QueryToListParams(r.URL.Query())
	if err != nil {
		h.logger.Error(ctx, "failed to parse query parameters", "error", err)
		api.WriteBadRequestResponse(w, "invalid_params", "Invalid query parameters")
		return
	}

	// Set activity group ID from path
	params.ActivityGroupID = &groupID

	// List templates
	templates, err := h.sessionTemplateService.ListSessionTemplates(ctx, *params)
	if err != nil {
		h.handleServiceError(ctx, w, err)
		return
	}

	api.WriteOkResponse(w,
		mapper.DomainListToResponse(templates, params.Limit, params.Offset),
		"")
}

// UpdateSessionTemplate handles PUT /api/v1/templates/{id}
//
//	@Summary		Update a session template
//	@Description	Updates an existing session template
//	@Tags			Session Templates
//	@Accept			json
//	@Produce		json
//	@Security		GatewayKeyAuth && BearerAuth
//	@Param			id		path		string									true	"Template ID"
//	@Param			request	body		dtos.UpdateSessionTemplateRequest		true	"Update data"
//	@Success		200		{object}	api.Response[dtos.EmptyResponse]		"Template updated successfully"
//	@Failure		400		{object}	api.Response[any]						"Invalid request"
//	@Failure		404		{object}	api.Response[any]						"Template not found"
//	@Failure		500		{object}	api.Response[any]						"Internal server error"
//	@Router			/api/v1/templates/{id} [put]
func (h *Handler) UpdateSessionTemplate(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Parse template ID from path
	templateIDStr := r.PathValue("id")
	templateID, err := uuid.Parse(templateIDStr)
	if err != nil {
		h.logger.Error(ctx, "invalid template ID", "error", err)
		api.WriteBadRequestResponse(w, "invalid_template_id", "Invalid template ID")
		return
	}

	// Decode request body
	var req dtos.UpdateSessionTemplateRequest
	if err := api.DecodeJSONBody(r, &req); err != nil {
		h.logger.Error(ctx, "failed to decode request body", "error", err)
		api.WriteValidationErrorResponse(w, err)
		return
	}

	// Convert to application params
	params, err := mapper.UpdateRequestToParams(&req)
	if err != nil {
		h.logger.Error(ctx, "failed to convert request to params", "error", err)
		api.WriteBadRequestResponse(w, "invalid_params", "Invalid request parameters")
		return
	}

	// Update template
	if err := h.sessionTemplateService.UpdateSessionTemplate(ctx, templateID, *params); err != nil {
		h.handleServiceError(ctx, w, err)
		return
	}

	h.logger.Info(ctx, "session template updated", "template_id", templateID)

	api.WriteOkResponse(w, dtos.EmptyResponse{}, "Session template updated successfully")
}

// DeleteSessionTemplate handles DELETE /api/v1/templates/{id}
//
//	@Summary		Delete a session template
//	@Description	Soft-deletes a session template
//	@Tags			Session Templates
//	@Produce		json
//	@Security		GatewayKeyAuth && BearerAuth
//	@Param			id	path		string							true	"Template ID"
//	@Success		200	{object}	api.Response[dtos.EmptyResponse]	"Template deleted successfully"
//	@Failure		400	{object}	api.Response[any]				"Invalid request"
//	@Failure		404	{object}	api.Response[any]				"Template not found"
//	@Failure		500	{object}	api.Response[any]				"Internal server error"
//	@Router			/api/v1/templates/{id} [delete]
func (h *Handler) DeleteSessionTemplate(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Parse template ID from path
	templateIDStr := r.PathValue("id")
	templateID, err := uuid.Parse(templateIDStr)
	if err != nil {
		h.logger.Error(ctx, "invalid template ID", "error", err)
		api.WriteBadRequestResponse(w, "invalid_template_id", "Invalid template ID")
		return
	}

	// Delete template
	if err := h.sessionTemplateService.DeleteSessionTemplate(ctx, templateID); err != nil {
		h.handleServiceError(ctx, w, err)
		return
	}

	h.logger.Info(ctx, "session template deleted", "template_id", templateID)

	api.WriteOkResponse(w, dtos.EmptyResponse{}, "Session template deleted successfully")
}

// ActivateSessionTemplate handles POST /api/v1/templates/{id}/activate
//
//	@Summary		Activate a session template
//	@Description	Activates an inactive session template
//	@Tags			Session Templates
//	@Produce		json
//	@Security		GatewayKeyAuth && BearerAuth
//	@Param			id	path		string							true	"Template ID"
//	@Success		200	{object}	api.Response[dtos.EmptyResponse]	"Template activated successfully"
//	@Failure		400	{object}	api.Response[any]				"Invalid request"
//	@Failure		404	{object}	api.Response[any]				"Template not found"
//	@Failure		500	{object}	api.Response[any]				"Internal server error"
//	@Router			/api/v1/templates/{id}/activate [post]
func (h *Handler) ActivateSessionTemplate(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Parse template ID from path
	templateIDStr := r.PathValue("id")
	templateID, err := uuid.Parse(templateIDStr)
	if err != nil {
		h.logger.Error(ctx, "invalid template ID", "error", err)
		api.WriteBadRequestResponse(w, "invalid_template_id", "Invalid template ID")
		return
	}

	// Activate template
	if err := h.sessionTemplateService.ActivateSessionTemplate(ctx, templateID); err != nil {
		h.handleServiceError(ctx, w, err)
		return
	}

	h.logger.Info(ctx, "session template activated", "template_id", templateID)

	api.WriteOkResponse(w, dtos.EmptyResponse{}, "Session template activated successfully")
}

// DeactivateSessionTemplate handles POST /api/v1/templates/{id}/deactivate
//
//	@Summary		Deactivate a session template
//	@Description	Deactivates an active session template
//	@Tags			Session Templates
//	@Produce		json
//	@Security		GatewayKeyAuth && BearerAuth
//	@Param			id	path		string							true	"Template ID"
//	@Success		200	{object}	api.Response[dtos.EmptyResponse]	"Template deactivated successfully"
//	@Failure		400	{object}	api.Response[any]				"Invalid request"
//	@Failure		404	{object}	api.Response[any]				"Template not found"
//	@Failure		500	{object}	api.Response[any]				"Internal server error"
//	@Router			/api/v1/templates/{id}/deactivate [post]
func (h *Handler) DeactivateSessionTemplate(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Parse template ID from path
	templateIDStr := r.PathValue("id")
	templateID, err := uuid.Parse(templateIDStr)
	if err != nil {
		h.logger.Error(ctx, "invalid template ID", "error", err)
		api.WriteBadRequestResponse(w, "invalid_template_id", "Invalid template ID")
		return
	}

	// Deactivate template
	if err := h.sessionTemplateService.DeactivateSessionTemplate(ctx, templateID); err != nil {
		h.handleServiceError(ctx, w, err)
		return
	}

	h.logger.Info(ctx, "session template deactivated", "template_id", templateID)

	api.WriteOkResponse(w, dtos.EmptyResponse{}, "Session template deactivated successfully")
}
