package http

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/google/uuid"

	"github.com/rasparac/rekreativko-api/internal/shared/api"
	"github.com/rasparac/rekreativko-api/internal/shared/domainerror"
	"github.com/rasparac/rekreativko-api/internal/shared/middleware"
	"github.com/rasparac/rekreativko-api/internal/user_profile/application"
	"github.com/rasparac/rekreativko-api/internal/user_profile/domain"
	"github.com/rasparac/rekreativko-api/internal/user_profile/interfaces/http/dtos"
	"github.com/rasparac/rekreativko-api/internal/user_profile/interfaces/http/mapper"
)

type (
	userProfiler interface {
		GetProfile(ctx context.Context, filter application.ProfileFilter) (*domain.UserProfile, error)
		UpdateProfile(ctx context.Context, accountID uuid.UUID, toUpdateProfile application.UpdateProfileParams) error
		GetProfiles(ctx context.Context, filter application.ProfilesFilter) ([]*domain.UserProfile, error)
	}

	userPorfileHandler struct {
		userProfileService userProfiler
	}
)

func NewHandler(userProfileService userProfiler) *userPorfileHandler {
	return &userPorfileHandler{
		userProfileService: userProfileService,
	}
}

func (h *userPorfileHandler) RegisterRoutes(
	mux *http.ServeMux,
	protectedChain *middleware.Chain,
) {
	mux.Handle(
		"GET /api/v1/profiles/{id}",
		protectedChain.ThenFunc(h.GetProfile),
	)
	mux.Handle(
		"GET /api/v1/profiles",
		protectedChain.ThenFunc(h.GetProfiles),
	)
	mux.Handle(
		"PUT /api/v1/profiles/{id}",
		protectedChain.ThenFunc(h.UpdateProfile),
	)
}

// GetProfile
//
//	@Summary		Returns user profile
//	@Description	Returns user profile by id
//	@Tags			User Profile
//	@Accept			json
//	@Produce		json
//	@Success		200 	{object}	api.Response[any]	"Account returned successfully"
//	@Failure		400		{object}	api.Response[any]						"Invalid request"
//	@Failure		500		{object}	api.Response[any]						"Internal server error"
//	@Router			/api/v1/profiles/{id} [get]
func (h *userPorfileHandler) GetProfile(
	w http.ResponseWriter,
	r *http.Request,
) {
	var (
		accountID     = middleware.GetAccountID(r)
		pathAccountID = r.PathValue("id")
	)

	parsedPathAccoutID, err := uuid.Parse(pathAccountID)
	if err != nil {
		api.WriteBadRequestResponse(w, "bad_request", "Invalid request body")
		return
	}

	if accountID != parsedPathAccoutID {
		api.WriteForbiddenResponse(w, "forbidden", "Forbidden")
		return
	}

	profile, err := h.userProfileService.GetProfile(r.Context(), application.ProfileFilter{
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
//	@Summary		Returns user profiles
//	@Description	Returns user profiles filtered by query parameters
//	@Tags			User Profile
//	@Produce		json
//
//	@Param			account_id		query		[]string	false	"Filter by account IDs (can be repeated)"
//	@Param			nickname		query		[]string	false	"Filter by nicknames (can be repeated)"
//
//	@Param			dob_gt			query		string		false	"Date of birth after (YYYY-MM-DD)"
//	@Param			dob_lt			query		string		false	"Date of birth before (YYYY-MM-DD)"
//
//	@Param			include_deleted	query		boolean	false	"Include deleted profiles"
//
//	@Param			sort_by			query		string		false	"Sort field (e.g. created_at, nickname)"
//	@Param			sort_order		query		string		false	"Sort order (asc, desc)"
//
//	@Param			country			query		string		false	"Filter by location country"
//	@Param			city			query		string		false	"Filter by location city"
//
//	@Param			limit			query		int			false	"Limit number of results (default 20)"
//	@Param			offset			query		int			false	"Offset for pagination (default 0)"
//
//	@Success		200	{object}	api.Response[any]
//	@Failure		400	{object}	api.Response[any]	"Invalid request"
//	@Failure		500	{object}	api.Response[any]	"Internal server error"
//
//	@Router			/api/v1/profiles [get]
func (h *userPorfileHandler) GetProfiles(
	w http.ResponseWriter,
	r *http.Request,
) {
	filter, err := mapper.QueryToProfilesFilter(r.URL.Query())
	if err != nil {
		api.WriteBadRequestResponse(w, "bad_request", err.Error())
		return
	}

	profiles, err := h.userProfileService.GetProfiles(r.Context(), filter)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	response := make([]dtos.UserProfileResponse, 0, len(profiles))
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
//	@Summary		Update user profile
//	@Description	Update user profile by id and data
//	@Tags			User Profile
//	@Accept			json
//	@Produce		json
//	@Param			request	body		any					true	"Account Registration Data"
//	@Success		200		{object}	api.Response[any]	"Account created successfully"
//	@Failure		400		{object}	api.Response[any]						"Invalid request"
//	@Failure		500		{object}	api.Response[any]						"Internal server error"
//	@Router			/api/v1/profiles/{id} [put]
func (h *userPorfileHandler) UpdateProfile(
	w http.ResponseWriter,
	r *http.Request,
) {
	var (
		accountID     = middleware.GetAccountID(r)
		pathAccountID = r.PathValue("id")
	)

	parsedPathAccoutID, err := uuid.Parse(pathAccountID)
	if err != nil {
		api.WriteBadRequestResponse(w, "bad_request", "Invalid request body")
		return
	}

	if accountID != parsedPathAccoutID {
		api.WriteForbiddenResponse(w, "forbidden", "Forbidden")
		return
	}

	req := &dtos.UpdateProfileRequest{}
	err = json.NewDecoder(r.Body).Decode(req)
	if err != nil {
		api.WriteBadRequestResponse(w, "bad_request", "Invalid request body")
		return
	}

	params := mapper.UpdateProfileRequestToParams(req)
	err = h.userProfileService.UpdateProfile(r.Context(), accountID, params)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	api.WriteOkResponse(w, "ok", "Profile updated successfully")
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
