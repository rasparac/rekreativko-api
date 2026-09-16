package http

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/google/uuid"

	"github.com/rasparac/rekreativko-api/account-profile/internal/application"
	"github.com/rasparac/rekreativko-api/account-profile/internal/domain"
	"github.com/rasparac/rekreativko-api/account-profile/internal/interfaces/http/dtos"
	"github.com/rasparac/rekreativko-api/account-profile/internal/interfaces/http/mapper"
	"github.com/rasparac/rekreativko-api/shared/api"
	"github.com/rasparac/rekreativko-api/shared/authcontext"
	"github.com/rasparac/rekreativko-api/shared/domainerror"
	"github.com/rasparac/rekreativko-api/shared/logger"
	"github.com/rasparac/rekreativko-api/shared/middleware"
)

type (
	accountProfiler interface {
		GetProfile(ctx context.Context, filter application.ProfileFilter) (*domain.AccountProfile, error)
		UpdateProfile(ctx context.Context, accountID uuid.UUID, toUpdateProfile application.UpdateProfileParams) error
		GetProfiles(ctx context.Context, filter application.ProfilesFilter) ([]*domain.AccountProfile, string, error)
	}

	accountSettingsService interface {
		GetSettings(ctx context.Context, accountID uuid.UUID) (*domain.AccountProfileSettings, error)
		UpdateSettings(ctx context.Context, accountID uuid.UUID, settings application.UpdateAccountSettingsParams) error
	}

	accountPorfileHandler struct {
		accountProfileService  accountProfiler
		accountSettingsService accountSettingsService
		logger                 *logger.Logger
	}
)

func NewHandler(
	accountProfileService accountProfiler,
	accountSettingsService accountSettingsService,
	log *logger.Logger,
) *accountPorfileHandler {
	return &accountPorfileHandler{
		accountProfileService:  accountProfileService,
		accountSettingsService: accountSettingsService,
		logger:                 log.WithName("account-profile.http.handler"),
	}
}

func (h *accountPorfileHandler) RegisterRoutes(
	mux *http.ServeMux,
	middlewares *middleware.Chain,
) {
	mux.Handle(
		"GET /api/v1/profiles",
		middlewares.ThenFunc(h.GetProfiles),
	)

	mux.Handle(
		"GET /api/v1/profiles/{id}",
		middlewares.ThenFunc(h.GetProfileByID),
	)

	mux.Handle(
		"GET /api/v1/my/profile",
		middlewares.ThenFunc(h.GetProfile),
	)
	mux.Handle(
		"PUT /api/v1/my/profile",
		middlewares.ThenFunc(h.UpdateProfile),
	)

	mux.Handle(
		"GET /api/v1/my/settings",
		middlewares.ThenFunc(h.GetAccountSettings),
	)
	mux.Handle(
		"PUT /api/v1/my/settings",
		middlewares.ThenFunc(h.UpdateAccountSettings),
	)
}

// GetProfile
//
//	@Summary		Returns account profile
//	@Description	Returns account profile
//	@Tags			account Profile
//	@Produce		json
//	@Security		GatewayKeyAuth && BearerAuth
//	@Param			id	path		string				true	"Account ID"
//
//	@Success		200	{object}	api.Response[any]	"Account returned successfully"
//	@Failure		400	{object}	api.Response[any]	"Invalid request"
//	@Failure		500	{object}	api.Response[any]	"Internal server error"
//	@Router			/api/v1/my/profile [get]
func (h *accountPorfileHandler) GetProfile(
	w http.ResponseWriter,
	r *http.Request,
) {
	var (
		ctx       = r.Context()
		accountID = authcontext.GetAccountID(ctx)
	)

	profile, err := h.accountProfileService.GetProfile(r.Context(), application.ProfileFilter{
		AccountID: &accountID,
	})
	if err != nil {
		h.handleServiceError(ctx, w, err)
		return
	}

	api.WriteOkResponse(
		w,
		mapper.DomainProfileToResponse(profile),
		"",
	)
}

// GetProfileByID
//
//	@Summary		Returns account profile by ID
//	@Description	Returns a single account profile by account ID
//	@Tags			account Profile
//	@Produce		json
//	@Security		GatewayKeyAuth && BearerAuth
//	@Param			id	path		string				true	"Account ID"
//	@Success		200	{object}	api.Response[any]	"Account profile returned successfully"
//	@Failure		400	{object}	api.Response[any]	"Invalid request"
//	@Failure		404	{object}	api.Response[any]	"Profile not found"
//	@Failure		500	{object}	api.Response[any]	"Internal server error"
//	@Router			/api/v1/profiles/{id} [get]
func (h *accountPorfileHandler) GetProfileByID(
	w http.ResponseWriter,
	r *http.Request,
) {
	ctx := r.Context()

	accountID, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		api.WriteBadRequestResponse(w, "invalid_account_id", "Invalid account ID")
		return
	}

	profile, err := h.accountProfileService.GetProfile(ctx, application.ProfileFilter{
		AccountID: &accountID,
	})
	if err != nil {
		h.handleServiceError(ctx, w, err)
		return
	}

	api.WriteOkResponse(
		w,
		mapper.DomainProfileToResponse(profile),
		"",
	)
}

// GetProfiles
//
//	@Summary		Returns account profiles
//	@Description	Returns account profiles filtered by query parameters
//	@Tags			account Profile
//	@Accept			json
//	@Produce		json
//	@Security		GatewayKeyAuth && BearerAuth
//
//	@Param			account_id		query		[]string	false	"Filter by account IDs (can be repeated)"
//	@Param			nickname		query		[]string	false	"Filter by nicknames (can be repeated)"
//	@Param			dob_gt			query		string		false	"Date of birth after (YYYY-MM-DD)"
//	@Param			dob_lt			query		string		false	"Date of birth before (YYYY-MM-DD)"
//	@Param			include_deleted	query		boolean		false	"Include deleted profiles"
//	@Param			sort_by			query		string		false	"Sort field (e.g. created_at, nickname)"
//	@Param			sort_order		query		string		false	"Sort order (asc, desc)"
//	@Param			country			query		string		false	"Filter by location country"
//	@Param			limit			query		int			false	"Limit number of results (default 20)"
//	@Param			page_token		query		string		false	"Token from the previous response's next_page_token, to fetch the next page"
//
//	@Success		200				{object}	api.Response[api.Page[dtos.AccountProfileResponse]]
//	@Failure		400				{object}	api.Response[any]	"Invalid request"
//	@Failure		500				{object}	api.Response[any]	"Internal server error"
//	@Router			/api/v1/profiles [get]
func (h *accountPorfileHandler) GetProfiles(
	w http.ResponseWriter,
	r *http.Request,
) {
	ctx := r.Context()

	filter, err := mapper.QueryToProfilesFilter(r.URL.Query())
	if err != nil {
		api.WriteBadRequestResponse(w, "bad_request", err.Error())
		return
	}

	profiles, nextPageToken, err := h.accountProfileService.GetProfiles(r.Context(), filter)
	if err != nil {
		h.handleServiceError(ctx, w, err)
		return
	}

	api.WriteOkResponse(
		w,
		api.NewPage(profiles, filter.Limit, nextPageToken, mapper.DomainProfileToResponse),
		"",
	)
}

// UpdateProfile
//
//	@Summary		Update account profile
//	@Description	Update account profile by id and data
//	@Tags			account Profile
//	@Accept			json
//	@Produce		json
//
//	@Security		GatewayKeyAuth && BearerAuth
//	@Param			id		path		string						true	"Profile account ID"
//
// TODO: Fix swagger parsing for UpdateProfileRequest
//
//	Param			request	body		dtos.UpdateProfileRequest	true	"Account Registration Data"
//	@Success		200		{object}	api.Response[any]			"Account created successfully"
//	@Failure		400		{object}	api.Response[any]			"Invalid request"
//	@Failure		500		{object}	api.Response[any]			"Internal server error"
//	@Router			/api/v1/my/profile [put]
func (h *accountPorfileHandler) UpdateProfile(
	w http.ResponseWriter,
	r *http.Request,
) {
	var (
		ctx       = r.Context()
		accountID = authcontext.GetAccountID(ctx)
		req       = &dtos.UpdateProfileRequest{}
	)

	err := json.NewDecoder(r.Body).Decode(req)
	if err != nil {
		api.WriteBadRequestResponse(w, "bad_request", "Invalid request body")
		return
	}

	params, err := mapper.UpdateProfileRequestToParams(req)
	if err != nil {
		api.WriteBadRequestResponse(w, "invalid_date_of_birth", "date_of_birth must be in YYYY-MM-DD format")
		return
	}

	err = h.accountProfileService.UpdateProfile(r.Context(), accountID, params)
	if err != nil {
		h.handleServiceError(ctx, w, err)
		return
	}

	api.WriteOkResponse(w, "ok", "Profile updated successfully")
}

// UpdateAccountSettings
//
//	@Summary		Update account settings
//	@Description	Update account settings by id and data
//	@Tags			account Profile
//	@Accept			json
//	@Produce		json
//
//	@Security		GatewayKeyAuth && BearerAuth
//	@Param	request	body	dtos.UpdateAccountSettingsRequest	true	"Account Settings Data"
//	@Success		200		{object}	api.Response[any]			"Account settings updated successfully"
//	@Failure		400		{object}	api.Response[any]			"Invalid request"
//	@Failure		500		{object}	api.Response[any]			"Internal server error"
//	@Router			/api/v1/my/settings [put]
func (h *accountPorfileHandler) UpdateAccountSettings(
	w http.ResponseWriter,
	r *http.Request,
) {
	var (
		ctx       = r.Context()
		accountID = authcontext.GetAccountID(ctx)
		req       dtos.UpdateAccountSettingsRequest
	)

	err := json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		api.WriteBadRequestResponse(w, "bad_request", "Invalid request body")
		return
	}

	params := mapper.UpdateAccountSettingsRequestToParams(req.Settings)
	err = h.accountSettingsService.UpdateSettings(ctx, accountID, params)
	if err != nil {
		h.handleServiceError(ctx, w, err)
		return
	}

	api.WriteOkResponse(w, "ok", "Account settings updated successfully")
}

// GetAccountSettings
//
//	@Summary		Returns account account settings
//	@Description	Returns account account settings for logged in account
//	@Tags			account Profile
//	@Produce		json
//	@Security		GatewayKeyAuth && BearerAuth
//
//	@Success		200	{object}	api.Response[dtos.Setting]	"Account settings returned successfully"
//	@Failure		400	{object}	api.Response[any]	"Invalid request"
//	@Failure		500	{object}	api.Response[any]	"Internal server error"
//	@Router			/api/v1/my/settings [get]
func (h *accountPorfileHandler) GetAccountSettings(
	w http.ResponseWriter,
	r *http.Request,
) {
	var (
		ctx       = r.Context()
		accountID = authcontext.GetAccountID(ctx)
	)
	accountSettings, err := h.accountSettingsService.GetSettings(r.Context(), accountID)
	if err != nil {
		h.handleServiceError(ctx, w, err)
		return
	}

	respSetings := mapper.ToAccountSettingsResponse(accountSettings.Settings())

	api.WriteOkResponse(
		w,
		respSetings,
		"",
	)
}

// handleServiceError converts service errors to HTTP responses
// Logging is handled by middleware based on HTTP status code
func (h *accountPorfileHandler) handleServiceError(ctx context.Context, w http.ResponseWriter, err error) {
	appErr := domainerror.GetAppError(err)

	api.WriteError(
		w,
		appErr.StatusCode,
		appErr.Code,
		appErr.Message,
		nil,
	)
}
