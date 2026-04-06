package http

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/google/uuid"

	"github.com/rasparac/rekreativko-api/internal/account-profile/application"
	"github.com/rasparac/rekreativko-api/internal/account-profile/domain"
	"github.com/rasparac/rekreativko-api/internal/account-profile/interfaces/http/dtos"
	"github.com/rasparac/rekreativko-api/internal/account-profile/interfaces/http/mapper"
	"github.com/rasparac/rekreativko-api/internal/shared/api"
	"github.com/rasparac/rekreativko-api/internal/shared/authcontext"
	"github.com/rasparac/rekreativko-api/internal/shared/domainerror"
	"github.com/rasparac/rekreativko-api/internal/shared/middleware"
)

type (
	accountProfiler interface {
		GetProfile(ctx context.Context, filter application.ProfileFilter) (*domain.AccountProfile, error)
		UpdateProfile(ctx context.Context, accountID uuid.UUID, toUpdateProfile application.UpdateProfileParams) error
		GetProfiles(ctx context.Context, filter application.ProfilesFilter) ([]*domain.AccountProfile, error)
	}

	accountSettingsService interface {
		GetSettings(ctx context.Context, accountID uuid.UUID) (*domain.AccountProfileSettings, error)
		UpdateSettings(ctx context.Context, accountID uuid.UUID, settings application.UpdateAccountSettingsParams) error
	}

	accountPorfileHandler struct {
		accountProfileService  accountProfiler
		accountSettingsService accountSettingsService
	}
)

func NewHandler(
	accountProfileService accountProfiler,
	accountSettingsService accountSettingsService,
) *accountPorfileHandler {
	return &accountPorfileHandler{
		accountProfileService:  accountProfileService,
		accountSettingsService: accountSettingsService,
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
		middlewares.ThenFunc(nil),
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
//	@Security		BearerAuth
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
		handleServiceError(w, err)
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
//	@Security		BearerAuth
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
//	@Param			offset			query		int			false	"Offset for pagination (default 0)"
//
//	@Success		200				{object}	api.Response[any]
//	@Failure		400				{object}	api.Response[any]	"Invalid request"
//	@Failure		500				{object}	api.Response[any]	"Internal server error"
//	@Router			/api/v1/profiles [get]
func (h *accountPorfileHandler) GetProfiles(
	w http.ResponseWriter,
	r *http.Request,
) {
	filter, err := mapper.QueryToProfilesFilter(r.URL.Query())
	if err != nil {
		api.WriteBadRequestResponse(w, "bad_request", err.Error())
		return
	}

	profiles, err := h.accountProfileService.GetProfiles(r.Context(), filter)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	response := make([]dtos.AccountProfileResponse, 0, len(profiles))
	for _, profile := range profiles {
		response = append(response, mapper.DomainProfileToResponse(profile))
	}

	api.WriteOkResponse(
		w,
		response,
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
//	@Security		BearerAuth
//	@Param			id		path		string						true	"Profile account ID"
//	@Param			request	body		dtos.UpdateProfileRequest	true	"Account Registration Data"
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

	params := mapper.UpdateProfileRequestToParams(req)
	err = h.accountProfileService.UpdateProfile(r.Context(), accountID, params)
	if err != nil {
		handleServiceError(w, err)
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
//	@Security		BearerAuth
//	@Param			request	body		dtos.UpdateAccountSettingsRequest	true	"Account Settings Data"
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
		handleServiceError(w, err)
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
//	@Security		BearerAuth
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
		handleServiceError(w, err)
		return
	}

	respSetings := mapper.ToAccountSettingsResponse(accountSettings.Settings())

	api.WriteOkResponse(
		w,
		respSetings,
		"",
	)
}

func handleServiceError(w http.ResponseWriter, err error) {
	appErr := domainerror.GetAppError(err)

	api.WriteError(
		w,
		appErr.StatusCode,
		appErr.Code,
		appErr.Message,
		nil,
	)
}
