package http

import (
	"context"
	"net/http"

	"github.com/google/uuid"
	"github.com/rasparac/rekreativko-api/activity/internal/application"
	"github.com/rasparac/rekreativko-api/activity/internal/domain"
	"github.com/rasparac/rekreativko-api/activity/internal/interfaces/http/dtos"
	"github.com/rasparac/rekreativko-api/shared/api"
	"github.com/rasparac/rekreativko-api/shared/authcontext"
	"github.com/rasparac/rekreativko-api/shared/domainerror"
	"github.com/rasparac/rekreativko-api/shared/logger"
	"github.com/rasparac/rekreativko-api/shared/middleware"
)

type moderationService interface {
	Report(ctx context.Context, params application.ReportParams) error
	Block(ctx context.Context, blockerID, blockedID uuid.UUID) error
	Unblock(ctx context.Context, blockerID, blockedID uuid.UUID) error
	ListBlocks(ctx context.Context, blockerID uuid.UUID) ([]*domain.AccountBlock, error)
}

// ModerationHandler serves reports and blocks (rekreativko-api-ep8.8).
type ModerationHandler struct {
	moderation moderationService
	decoder    requestDecoder
	logger     *logger.Logger
}

func NewModerationHandler(moderation moderationService, decoder requestDecoder, logger *logger.Logger) *ModerationHandler {
	return &ModerationHandler{moderation: moderation, decoder: decoder, logger: logger.WithName("activity.http.moderation")}
}

func (h *ModerationHandler) RegisterRoutes(mux *http.ServeMux, middlewares *middleware.Chain) {
	mux.Handle("POST /api/v1/reports", middlewares.ThenFunc(h.Report))
	mux.Handle("GET /api/v1/blocks", middlewares.ThenFunc(h.ListBlocks))
	mux.Handle("POST /api/v1/blocks", middlewares.ThenFunc(h.Block))
	mux.Handle("DELETE /api/v1/blocks/{accountId}", middlewares.ThenFunc(h.Unblock))
}

// Report handles POST /api/v1/reports
//
//	@Summary		Report a user, group or session
//	@Description	Reports content to the operators. reason is one of spam, harassment, hate, violence, inappropriate, other; note is optional (max 1000 characters). A group or session the caller can't see answers 404 as if it didn't exist. Reporting the same target again while the first report is open is accepted and changes nothing.
//	@Tags			Moderation
//	@Accept			json
//	@Produce		json
//	@Security		GatewayKeyAuth && BearerAuth
//	@Param			request	body		dtos.ReportRequest					true	"What is reported and why"
//	@Success		201		{object}	api.Response[dtos.EmptyResponse]	"Report received"
//	@Failure		400		{object}	api.Response[any]					"validation_error (missing or unknown field value)"
//	@Failure		401		{object}	api.Response[any]					"Unauthorized"
//	@Failure		404		{object}	api.Response[any]					"activity_group_not_found or session_not_found"
//	@Failure		422		{object}	api.Response[any]					"cannot_report_self"
//	@Failure		500		{object}	api.Response[any]					"Internal server error"
//	@Router			/api/v1/reports [post]
func (h *ModerationHandler) Report(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var req dtos.ReportRequest
	if err := h.decoder.Decode(r, &req); err != nil {
		api.WriteValidationErrorResponse(w, err)
		return
	}

	err := h.moderation.Report(ctx, application.ReportParams{
		ReporterID: authcontext.GetAccountID(ctx),
		TargetType: req.TargetType,
		TargetID:   req.TargetID,
		Reason:     req.Reason,
		Note:       req.Note,
	})
	if err != nil {
		h.writeError(w, err)
		return
	}

	api.WriteCreatedResponse(w, dtos.EmptyResponse{}, "Report received")
}

// ListBlocks handles GET /api/v1/blocks
//
//	@Summary		List the accounts I blocked
//	@Description	The caller's blocked accounts, newest first. One list, not paginated: at most 500. Resolve names with account-profile's GET /profiles?account_id=...
//	@Tags			Moderation
//	@Produce		json
//	@Security		GatewayKeyAuth && BearerAuth
//	@Success		200	{object}	api.Response[api.List[dtos.BlockResponse]]	"Blocked accounts"
//	@Failure		401	{object}	api.Response[any]							"Unauthorized"
//	@Failure		500	{object}	api.Response[any]							"Internal server error"
//	@Router			/api/v1/blocks [get]
func (h *ModerationHandler) ListBlocks(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	blocks, err := h.moderation.ListBlocks(ctx, authcontext.GetAccountID(ctx))
	if err != nil {
		h.writeError(w, err)
		return
	}

	api.WriteOkResponse(w, api.NewList(blocks, func(b *domain.AccountBlock) dtos.BlockResponse {
		return dtos.BlockResponse{AccountID: b.BlockedID(), BlockedAt: b.CreatedAt()}
	}), "")
}

// Block handles POST /api/v1/blocks
//
//	@Summary		Block an account
//	@Description	Neither account can invite the other any more (403 account_blocked, the same both ways), invites from the blocked account disappear from the caller's invite lists, and the blocked account's groups and sessions are hidden from the caller's discover and browse lists - except ones the caller already belongs to. Blocking an account again changes nothing.
//	@Tags			Moderation
//	@Accept			json
//	@Produce		json
//	@Security		GatewayKeyAuth && BearerAuth
//	@Param			request	body		dtos.BlockRequest					true	"Account to block"
//	@Success		201		{object}	api.Response[dtos.EmptyResponse]	"Account blocked"
//	@Failure		400		{object}	api.Response[any]					"validation_error"
//	@Failure		401		{object}	api.Response[any]					"Unauthorized"
//	@Failure		422		{object}	api.Response[any]					"cannot_block_self"
//	@Failure		500		{object}	api.Response[any]					"Internal server error"
//	@Router			/api/v1/blocks [post]
func (h *ModerationHandler) Block(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var req dtos.BlockRequest
	if err := h.decoder.Decode(r, &req); err != nil {
		api.WriteValidationErrorResponse(w, err)
		return
	}

	if err := h.moderation.Block(ctx, authcontext.GetAccountID(ctx), req.AccountID); err != nil {
		h.writeError(w, err)
		return
	}

	api.WriteCreatedResponse(w, dtos.EmptyResponse{}, "Account blocked")
}

// Unblock handles DELETE /api/v1/blocks/{accountId}
//
//	@Summary		Unblock an account
//	@Description	Removes the block. Unblocking an account that isn't blocked changes nothing.
//	@Tags			Moderation
//	@Produce		json
//	@Security		GatewayKeyAuth && BearerAuth
//	@Param			accountId	path		string								true	"Blocked account ID"
//	@Success		200			{object}	api.Response[dtos.EmptyResponse]	"Account unblocked"
//	@Failure		400			{object}	api.Response[any]					"invalid_account_id"
//	@Failure		401			{object}	api.Response[any]					"Unauthorized"
//	@Failure		500			{object}	api.Response[any]					"Internal server error"
//	@Router			/api/v1/blocks/{accountId} [delete]
func (h *ModerationHandler) Unblock(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	blockedID, err := uuid.Parse(r.PathValue("accountId"))
	if err != nil {
		api.WriteBadRequestResponse(w, "invalid_account_id", "Invalid account ID")
		return
	}

	if err := h.moderation.Unblock(ctx, authcontext.GetAccountID(ctx), blockedID); err != nil {
		h.writeError(w, err)
		return
	}

	api.WriteOkResponse(w, dtos.EmptyResponse{}, "Account unblocked")
}

func (h *ModerationHandler) writeError(w http.ResponseWriter, err error) {
	appErr := domainerror.GetAppError(err)
	api.WriteError(w, appErr.StatusCode, appErr.Code, appErr.Message, appErr.Details)
}
