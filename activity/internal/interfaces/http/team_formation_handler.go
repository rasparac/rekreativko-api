package http

import (
	"net/http"

	"github.com/google/uuid"
	"github.com/rasparac/rekreativko-api/activity/internal/interfaces/http/mapper"
	"github.com/rasparac/rekreativko-api/shared/api"
	"github.com/rasparac/rekreativko-api/shared/authcontext"
)

// GetTeamFormation handles GET /api/v1/sessions/{id}/team-formation
//
//	@Summary		Get the team-formation snapshot
//	@Description	Returns the session's whole team-formation state in one document: teams with members, the latest captain draft and the latest voting round. It is the same document every event on GET /sessions/{id}/events carries - use it to render the screen before the stream connects, or poll it if streaming misbehaves.
//	@Tags			Team formation
//	@Produce		json
//	@Security		GatewayKeyAuth && BearerAuth
//	@Param			id	path		string										true	"Session ID"
//	@Success		200	{object}	api.Response[dtos.TeamFormationResponse]	"Team-formation snapshot"
//	@Failure		400	{object}	api.Response[any]							"Invalid request"
//	@Failure		401	{object}	api.Response[any]							"Unauthorized"
//	@Failure		404	{object}	api.Response[any]							"Session not found"
//	@Failure		500	{object}	api.Response[any]							"Internal server error"
//	@Router			/api/v1/sessions/{id}/team-formation [get]
func (h *Handler) GetTeamFormation(w http.ResponseWriter, r *http.Request) {
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

	snapshot, err := h.teamFormation.Snapshot(ctx, sessionID)
	if err != nil {
		h.handleServiceError(ctx, w, err)
		return
	}

	api.WriteOkResponse(w, mapper.TeamFormationSnapshotToResponse(snapshot, accountID), "")
}
