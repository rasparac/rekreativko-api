package identity

import (
	"net/http"

	"github.com/rasparac/rekreativko-api/internal/shared/api"
	"github.com/rasparac/rekreativko-api/internal/shared/domainerror"
	"github.com/rasparac/rekreativko-api/internal/shared/logger"
	"github.com/rasparac/rekreativko-api/internal/shared/middleware"
)

type Handler struct {
	service *Service
	logger  *logger.Logger
}

func NewHandler(
	service *Service,
	logger *logger.Logger,
) *Handler {
	return &Handler{
		service: service,
		logger:  logger,
	}
}

func (h *Handler) RegisterRoutes(
	mux *http.ServeMux,
	publicChain *middleware.Chain,
	protectedChain *middleware.Chain,
) {
	mux.Handle(
		"POST /api/v1/register",
		publicChain.ThenFunc(h.RegisterAccountHandler),
	)
	mux.Handle(
		"POST /api/v1/login",
		publicChain.ThenFunc(h.LoginHandler),
	)
	mux.Handle(
		"POST /api/v1/auth/verify-account",
		publicChain.ThenFunc(h.VerifyAccountHandler),
	)
	mux.Handle(
		"POST /api/v1/auth/resend-verification-code",
		publicChain.ThenFunc(h.ResendVerificationCodeHandler),
	)

	mux.Handle(
		"POST /api/v1/auth/refresh-token",
		publicChain.ThenFunc(h.RefreshTokenHandler),
	)

	mux.Handle(
		"GET /api/v1/auth/me",
		protectedChain.ThenFunc(h.GetCurrentAccount),
	)
	mux.Handle(
		"POST /api/v1/auth/logout",
		protectedChain.ThenFunc(h.LogoutHandler),
	)
}

// RegisterAccountHandler
//
//	@Summary		Register a new account
//	@Description	Creates new account with email or phone number
//	@Tags			Authentication
//	@Accept			json
//	@Produce		json
//	@Param			request	body		RegisterAccountRequest					true	"Account Registration Data"
//	@Success		201		{object}	api.Response[RegisterAccountResponse]	"Account created successfully"
//	@Failure		400		{object}	api.Response[any]						"Invalid request"
//	@Failure		500		{object}	api.Response[any]						"Internal server error"
//	@Router			/api/v1/register [post]
func (h *Handler) RegisterAccountHandler(
	w http.ResponseWriter,
	r *http.Request,
) {
	var req RegisterAccountRequest

	err := api.DecodeJSONBody(r, &req)
	if err != nil {
		h.logger.Error(r.Context(), "failed to decode request body", "error", err)
		api.WriteBadRequestResponse(w, "bad_request", "Invalid request body")
	}

	resp, err := h.service.Register(r.Context(), req)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	api.WriteCreatedResponse(
		w,
		resp,
		"Susccessfully created account",
	)

}

// LoginHandler
//
//	@Summary		Login
//	@Description	Login with email or phone number
//	@Tags			Authentication
//	@Accept			json
//	@Produce		json
//	@Param			request	body		LoginRequest					true	"Account Login Data"
//	@Success		200		{object}	api.Response[TokenPairResponse]	"Account logged in successfully"
//	@Failure		400		{object}	api.Response[any]				"Invalid request"
//	@Failure		500		{object}	api.Response[any]				"Internal server error"
//	@Router			/api/v1/login [post]
func (h *Handler) LoginHandler(
	w http.ResponseWriter,
	r *http.Request,
) {
	var req LoginRequest
	err := api.DecodeJSONBody(r, &req)
	if err != nil {
		h.logger.Error(r.Context(), "failed to decode request body", "error", err)
		api.WriteBadRequestResponse(w, "bad_request", "Invalid request body")
	}

	tokens, err := h.service.Login(r.Context(), req)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	api.WriteOkResponse(
		w,
		tokens,
		"Susccessfully logged in",
	)

}

// GetCurrentAccount
//
//	@Summary		Get current account
//	@Description	Get currently logged in account
//	@Tags			Authentication
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Success		200	{object}	api.Response[AccountResponse]	"Account retrieved successfully"
//	@Failure		400	{object}	api.Response[any]				"Invalid request"
//	@Failure		401	{object}	api.Response[any]				"Unauthorized"
//	@Failure		500	{object}	api.Response[any]				"Internal server error"
//	@Router			/api/v1/auth/me [get]
func (h *Handler) GetCurrentAccount(
	w http.ResponseWriter,
	r *http.Request,
) {
	accountID := middleware.GetAccountID(r)

	account, err := h.service.GetAccount(r.Context(), accountID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	api.WriteOkResponse(
		w,
		domainToAccountResponse(account),
		"Account retrieved successfully",
	)
}

// LogoutHandler godoc
//
//	@Summary		Logout
//	@Description	Logs out the user and invalidates all refresh tokens
//	@Tags			Authentication
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			request	body		LogoutRequest				true	"Logout request"
//	@Success		200	{object}	api.Response[EmptyResponse]	"Successfully logged out"
//	@Failure		400	{object}	api.Response[any]			"Invalid request"
//	@Failure		401	{object}	api.Response[any]			"Unauthorized"
//	@Failure		500	{object}	api.Response[any]			"Internal server error"
//	@Router			/api/v1/auth/logout [post]
func (h *Handler) LogoutHandler(
	w http.ResponseWriter,
	r *http.Request,
) {
	accountID := middleware.GetAccountID(r)

	var req LogoutRequest
	err := api.DecodeJSONBody(r, &req)
	if err != nil {
		h.logger.Error(r.Context(), "failed to decode request body", "error", err)
		api.WriteBadRequestResponse(w, "bad_request", "Invalid request body")
		return
	}

	data, err := h.service.Logout(r.Context(), LogoutRequest{
		AccountID:    accountID,
		RefreshToken: req.RefreshToken,
	})
	if err != nil {
		handleServiceError(w, err)
		return
	}

	api.WriteOkResponse(w, data, "Successfully logged out")
}

// RefreshTokenHandler godoc
//
//	@Summary		Refresh token
//	@Description	Refreshes a token pair based on the given refresh token
//	@Tags			Authentication
//	@Accept			json
//	@Produce		json
//	@Param			request	body		RefreshTokenRequest				true	"Refresh token request"
//	@Success		200		{object}	api.Response[TokenPairResponse]	"Successfully refreshed token"
//	@Failure		400		{object}	api.Response[any]				"Invalid request"
//	@Failure		401		{object}	api.Response[any]				"Unauthorized"
//	@Failure		500		{object}	api.Response[any]				"Internal server error"
//	@Router			/api/v1/auth/refresh-token [post]
func (h *Handler) RefreshTokenHandler(
	w http.ResponseWriter,
	r *http.Request,
) {
	var req RefreshTokenRequest
	err := api.DecodeJSONBody(r, &req)
	if err != nil {
		h.logger.Error(r.Context(), "failed to decode request body", "error", err)
		api.WriteBadRequestResponse(w, "bad_request", "Invalid request body")
	}

	tokens, err := h.service.RefreshToken(r.Context(), req)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	api.WriteOkResponse(
		w,
		tokens,
		"Susccessfully refreshed token",
	)
}

// VerifyAccountHandler godoc
//
//	@Summary		Verify account
//	@Description	Verifies an account by sending a verification code to the given phone number
//	@Tags			Authentication
//	@Accept			json
//	@Produce		json
//	@Param			request	body		VerifyAccountRequest				true	"Verify account request"
//	@Success		200		{object}	api.Response[VerifyAccountResponse]	"Account verified"
//	@Failure		400		{object}	api.Response[any]					"Invalid request"
//	@Failure		401		{object}	api.Response[any]					"Unauthorized"
//	@Failure		500		{object}	api.Response[any]					"Internal server error"
//	@Router			/api/v1/auth/verify-account [post]
func (h *Handler) VerifyAccountHandler(
	w http.ResponseWriter,
	r *http.Request,
) {
	var req VerifyAccountRequest
	err := api.DecodeJSONBody(r, &req)
	if err != nil {
		h.logger.Error(r.Context(), "failed to decode request body", "error", err)
		api.WriteBadRequestResponse(w, "bad_request", "Invalid request body")
	}

	data, err := h.service.VerifyAccount(r.Context(), req)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	api.WriteOkResponse(w, data, "Account verified")
}

// ResendVerificationCodeHandler godoc
//
//	@Summary		Resend verification code
//	@Description	Resends a verification code to the given phone number
//	@Tags			Authentication
//	@Accept			json
//	@Produce		json
//	@Param			request	body		ResendVerificationCodeRequest	true	"Resend verification code request"
//	@Success		200		{object}	api.Response[EmptyResponse]		"Verification code resent"
//	@Failure		400		{object}	api.Response[any]				"Invalid request"
//	@Failure		401		{object}	api.Response[any]				"Unauthorized"
//	@Failure		500		{object}	api.Response[any]				"Internal server error"
//	@Router			/api/v1/auth/resend-verification-code [post]
func (h *Handler) ResendVerificationCodeHandler(
	w http.ResponseWriter,
	r *http.Request,
) {
	var req ResendVerificationCodeRequest
	err := api.DecodeJSONBody(r, &req)
	if err != nil {
		h.logger.Error(r.Context(), "failed to decode request body", "error", err)
		api.WriteBadRequestResponse(w, "bad_request", "Invalid request body")
	}

	data, err := h.service.ResendVerificationCode(r.Context(), req)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	api.WriteOkResponse(w, data, "Verification code resent")
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
