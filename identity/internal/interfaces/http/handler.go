package identity

import (
	"context"
	"net/http"

	"github.com/google/uuid"
	"github.com/rasparac/rekreativko-api/identity/internal/application"
	"github.com/rasparac/rekreativko-api/identity/internal/domain"
	"github.com/rasparac/rekreativko-api/identity/internal/interfaces/http/dtos"
	"github.com/rasparac/rekreativko-api/shared/api"
	"github.com/rasparac/rekreativko-api/shared/authcontext"
	"github.com/rasparac/rekreativko-api/shared/domainerror"
	"github.com/rasparac/rekreativko-api/shared/logger"
	"github.com/rasparac/rekreativko-api/shared/middleware"
)

type service interface {
	Register(ctx context.Context, req application.RegistrationParams) (uuid.UUID, error)
	Login(ctx context.Context, req application.LoginParams) (*domain.RefreshToken, error)
	Logout(ctx context.Context, req application.LogoutParams) (*application.EmptyResponse, error)

	ResendVerificationCode(ctx context.Context, req application.ResendVerificationCodeParams) (*application.EmptyResponse, error)
	RefreshToken(ctx context.Context, req application.RefreshTokenParams) (*domain.RefreshToken, error)

	GetAccount(ctx context.Context, accountID uuid.UUID) (*domain.Account, error)
	DeleteAccount(ctx context.Context, accountID uuid.UUID) error
	VerifyAccount(ctx context.Context, req application.VerifyAccountParams) (uuid.UUID, error)
}

type passwordService interface {
	ForgotPassword(ctx context.Context, req application.ForgotPasswordParams) error
	ResetPassword(ctx context.Context, req application.ResetPasswordParams) error
	ChangePassword(ctx context.Context, req application.ChangePasswordParams) (*domain.RefreshToken, error)
}

// requestDecoder reads a request body into a DTO and checks its validate tags (api.Decoder).
type requestDecoder interface {
	Decode(r *http.Request, body any) error
}

type Handler struct {
	service   service
	passwords passwordService
	decoder   requestDecoder
	logger    *logger.Logger
}

func NewHandler(
	service service,
	passwords passwordService,
	decoder requestDecoder,
	logger *logger.Logger,
) *Handler {
	return &Handler{
		service:   service,
		passwords: passwords,
		decoder:   decoder,
		logger:    logger,
	}
}

func (h *Handler) RegisterRoutes(
	mux *http.ServeMux,
	middlewares *middleware.Chain,
) {
	mux.Handle(
		"POST /api/v1/register",
		middlewares.ThenFunc(h.RegisterAccountHandler),
	)
	mux.Handle(
		"POST /api/v1/login",
		middlewares.ThenFunc(h.LoginHandler),
	)
	mux.Handle(
		"POST /api/v1/verify-account",
		middlewares.ThenFunc(h.VerifyAccountHandler),
	)
	mux.Handle(
		"POST /api/v1/resend-verification-code",
		middlewares.ThenFunc(h.ResendVerificationCodeHandler),
	)

	mux.Handle(
		"POST /api/v1/refresh-token",
		middlewares.ThenFunc(h.RefreshTokenHandler),
	)

	mux.Handle(
		"GET /api/v1/me",
		middlewares.ThenFunc(h.GetCurrentAccount),
	)
	mux.Handle(
		"DELETE /api/v1/me",
		middlewares.ThenFunc(h.DeleteCurrentAccount),
	)
	mux.Handle(
		"POST /api/v1/logout",
		middlewares.ThenFunc(h.LogoutHandler),
	)

	mux.Handle(
		"POST /api/v1/forgot-password",
		middlewares.ThenFunc(h.ForgotPasswordHandler),
	)
	mux.Handle(
		"POST /api/v1/reset-password",
		middlewares.ThenFunc(h.ResetPasswordHandler),
	)
	mux.Handle(
		"POST /api/v1/change-password",
		middlewares.ThenFunc(h.ChangePasswordHandler),
	)
}

// RegisterAccountHandler
//
//	@Summary		Register a new account
//	@Description	Creates new account with email or phone number
//	@Tags			Authentication
//	@Accept			json
//	@Produce		json
//	@Param			request	body		dtos.RegisterAccountRequest					true	"Account Registration Data"
//	@Success		201		{object}	api.Response[dtos.RegisterAccountResponse]	"Account created successfully"
//	@Failure		400		{object}	api.Response[any]						"Invalid request"
//	@Failure		500		{object}	api.Response[any]						"Internal server error"
//	@Router			/api/v1/register [post]
func (h *Handler) RegisterAccountHandler(
	w http.ResponseWriter,
	r *http.Request,
) {
	ctx := r.Context()

	var req dtos.RegisterAccountRequest
	err := h.decoder.Decode(r, &req)
	if err != nil {
		h.logger.Error(r.Context(), "failed to decode request body", "error", err)
		api.WriteValidationErrorResponse(w, err)
		return
	}

	resp, err := h.service.Register(r.Context(), application.RegistrationParams{
		Email:       req.Email,
		PhoneNumber: req.PhoneNumber,
		Password:    req.Password,
	})
	if err != nil {
		h.handleServiceError(ctx, w, err)
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
//	@Param			request	body		dtos.LoginRequest					true	"Account Login Data"
//	@Success		200		{object}	api.Response[dtos.TokenPairResponse]	"Account logged in successfully"
//	@Failure		400		{object}	api.Response[any]				"Invalid request"
//	@Failure		500		{object}	api.Response[any]				"Internal server error"
//	@Router			/api/v1/login [post]
func (h *Handler) LoginHandler(
	w http.ResponseWriter,
	r *http.Request,
) {
	ctx := r.Context()

	var req dtos.LoginRequest
	err := h.decoder.Decode(r, &req)
	if err != nil {
		h.logger.Error(r.Context(), "failed to decode request body", "error", err)
		api.WriteValidationErrorResponse(w, err)
		return
	}

	token, err := h.service.Login(r.Context(), application.LoginParams{
		Email:       req.Email,
		PhoneNumber: req.PhoneNumber,
		Password:    req.Password,
	})
	if err != nil {
		h.handleServiceError(ctx, w, err)
		return
	}

	api.WriteOkResponse(
		w,
		dtos.DomainToTokenPairResponse(token, 123),
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
//	@Security		GatewayKeyAuth && BearerAuth
//	@Success		200	{object}	api.Response[dtos.AccountResponse]	"Account retrieved successfully"
//	@Failure		400	{object}	api.Response[any]				"Invalid request"
//	@Failure		401	{object}	api.Response[any]				"Unauthorized"
//	@Failure		500	{object}	api.Response[any]				"Internal server error"
//	@Router			/api/v1/me [get]
func (h *Handler) GetCurrentAccount(
	w http.ResponseWriter,
	r *http.Request,
) {
	var (
		ctx       = r.Context()
		accountID = authcontext.GetAccountID(ctx)
	)

	account, err := h.service.GetAccount(r.Context(), accountID)
	if err != nil {
		h.handleServiceError(ctx, w, err)
		return
	}

	api.WriteOkResponse(
		w,
		dtos.DomainToAccountResponse(account),
		"Account retrieved successfully",
	)
}

// DeleteCurrentAccount
//
//	@Summary		Delete current account
//	@Description	Marks the currently logged in account as deleted and revokes all refresh tokens
//	@Tags			Authentication
//	@Accept			json
//	@Produce		json
//	@Security		GatewayKeyAuth && BearerAuth
//	@Success		200	{object}	api.Response[dtos.EmptyResponse]	"Account deleted successfully"
//	@Failure		401	{object}	api.Response[any]				"Unauthorized"
//	@Failure		500	{object}	api.Response[any]				"Internal server error"
//	@Router			/api/v1/me [delete]
func (h *Handler) DeleteCurrentAccount(
	w http.ResponseWriter,
	r *http.Request,
) {
	var (
		ctx       = r.Context()
		accountID = authcontext.GetAccountID(ctx)
	)

	err := h.service.DeleteAccount(ctx, accountID)
	if err != nil {
		h.handleServiceError(ctx, w, err)
		return
	}

	api.WriteOkResponse(
		w,
		dtos.EmptyResponse{},
		"Account deleted successfully",
	)
}

// LogoutHandler godoc
//
//	@Summary		Logout
//	@Description	Logs out the account and invalidates all refresh tokens
//	@Tags			Authentication
//	@Accept			json
//	@Produce		json
//	@Security		GatewayKeyAuth && BearerAuth
//	@Param			request	body		dtos.LogoutRequest				true	"Logout request"
//	@Success		200		{object}	api.Response[dtos.EmptyResponse]	"Successfully logged out"
//	@Failure		400		{object}	api.Response[any]			"Invalid request"
//	@Failure		401		{object}	api.Response[any]			"Unauthorized"
//	@Failure		500		{object}	api.Response[any]			"Internal server error"
//	@Router			/api/v1/logout [post]
func (h *Handler) LogoutHandler(
	w http.ResponseWriter,
	r *http.Request,
) {
	var (
		ctx       = r.Context()
		accountID = authcontext.GetAccountID(ctx)
		req       dtos.LogoutRequest
	)

	err := h.decoder.Decode(r, &req)
	if err != nil {
		h.logger.Error(r.Context(), "failed to decode request body", "error", err)
		api.WriteValidationErrorResponse(w, err)
		return
	}

	data, err := h.service.Logout(r.Context(), application.LogoutParams{
		AccountID:    accountID,
		RefreshToken: req.RefreshToken,
	})
	if err != nil {
		h.handleServiceError(ctx, w, err)
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
//	@Param			request	body		dtos.RefreshTokenRequest				true	"Refresh token request"
//	@Success		200		{object}	api.Response[dtos.TokenPairResponse]	"Successfully refreshed token"
//	@Failure		400		{object}	api.Response[any]				"Invalid request"
//	@Failure		401		{object}	api.Response[any]				"Unauthorized"
//	@Failure		500		{object}	api.Response[any]				"Internal server error"
//	@Router			/api/v1/refresh-token [post]
func (h *Handler) RefreshTokenHandler(
	w http.ResponseWriter,
	r *http.Request,
) {
	ctx := r.Context()

	var req dtos.RefreshTokenRequest
	err := h.decoder.Decode(r, &req)
	if err != nil {
		h.logger.Error(r.Context(), "failed to decode request body", "error", err)
		api.WriteValidationErrorResponse(w, err)
		return
	}

	token, err := h.service.RefreshToken(r.Context(), application.RefreshTokenParams{
		RefreshToken: req.RefreshToken,
	})
	if err != nil {
		h.handleServiceError(ctx, w, err)
		return
	}

	api.WriteOkResponse(
		w,
		dtos.DomainToTokenPairResponse(token, 365*24*60*60),
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
//	@Param			request	body		dtos.VerifyAccountRequest				true	"Verify account request"
//	@Success		200		{object}	api.Response[dtos.VerifyAccountResponse]	"Account verified"
//	@Failure		400		{object}	api.Response[any]					"Invalid request"
//	@Failure		401		{object}	api.Response[any]					"Unauthorized"
//	@Failure		500		{object}	api.Response[any]					"Internal server error"
//	@Router			/api/v1/verify-account [post]
func (h *Handler) VerifyAccountHandler(
	w http.ResponseWriter,
	r *http.Request,
) {
	ctx := r.Context()

	var req dtos.VerifyAccountRequest
	err := h.decoder.Decode(r, &req)
	if err != nil {
		h.logger.Error(r.Context(), "failed to decode request body", "error", err)
		api.WriteValidationErrorResponse(w, err)
		return
	}

	data, err := h.service.VerifyAccount(r.Context(), application.VerifyAccountParams{
		Code: req.Code,
	})
	if err != nil {
		h.handleServiceError(ctx, w, err)
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
//	@Param			request	body		dtos.ResendVerificationCodeRequest	true	"Resend verification code request"
//	@Success		200		{object}	api.Response[dtos.EmptyResponse]		"Verification code resent"
//	@Failure		400		{object}	api.Response[any]				"Invalid request"
//	@Failure		401		{object}	api.Response[any]				"Unauthorized"
//	@Failure		500		{object}	api.Response[any]				"Internal server error"
//	@Router			/api/v1/resend-verification-code [post]
func (h *Handler) ResendVerificationCodeHandler(
	w http.ResponseWriter,
	r *http.Request,
) {
	ctx := r.Context()

	var req dtos.ResendVerificationCodeRequest
	err := h.decoder.Decode(r, &req)
	if err != nil {
		h.logger.Error(r.Context(), "failed to decode request body", "error", err)
		api.WriteValidationErrorResponse(w, err)
		return
	}

	data, err := h.service.ResendVerificationCode(r.Context(), application.ResendVerificationCodeParams{
		Type:     req.Type,
		Identity: req.Identity,
	})
	if err != nil {
		h.handleServiceError(ctx, w, err)
		return
	}

	api.WriteOkResponse(w, data, "Verification code resent")
}

// ForgotPasswordHandler
//
//	@Summary		Send a password reset code
//	@Description	Sends a 6-digit code (valid 15 minutes) to the email or phone, if it belongs to an active account. Always answers 200 the same way, so it does not reveal whether an account exists. At most one code per minute per account; a new code replaces the previous one.
//	@Tags			Authentication
//	@Accept			json
//	@Produce		json
//	@Param			request	body		dtos.ForgotPasswordRequest		true	"Address to send the code to"
//	@Success		200		{object}	api.Response[dtos.EmptyResponse]	"Code sent if the account exists"
//	@Failure		400		{object}	api.Response[any]				"Invalid request"
//	@Failure		429		{object}	api.Response[any]				"Too many requests (gateway)"
//	@Failure		500		{object}	api.Response[any]				"Internal server error"
//	@Router			/api/v1/forgot-password [post]
func (h *Handler) ForgotPasswordHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var req dtos.ForgotPasswordRequest
	if err := h.decoder.Decode(r, &req); err != nil {
		api.WriteValidationErrorResponse(w, err)
		return
	}

	err := h.passwords.ForgotPassword(ctx, application.ForgotPasswordParams{
		Type:     req.Type,
		Identity: req.Identity,
	})
	if err != nil {
		h.handleServiceError(ctx, w, err)
		return
	}

	api.WriteOkResponse(w, dtos.EmptyResponse{}, "If the account exists, a reset code was sent")
}

// ResetPasswordHandler
//
//	@Summary		Reset a forgotten password
//	@Description	Sets a new password with the code from forgot-password. Every session is logged out; log in again with the new password. An unknown address and a wrong code both answer identity_invalid_verification_code. The 5th wrong code burns it (identity_too_many_verification_attempts): request a new one.
//	@Tags			Authentication
//	@Accept			json
//	@Produce		json
//	@Param			request	body		dtos.ResetPasswordRequest		true	"Address, code and new password"
//	@Success		200		{object}	api.Response[dtos.EmptyResponse]	"Password reset"
//	@Failure		400		{object}	api.Response[any]				"identity_invalid_verification_code, identity_verification_code_expired, identity_too_many_verification_attempts, identity_weak_password or invalid request"
//	@Failure		429		{object}	api.Response[any]				"Too many requests (gateway)"
//	@Failure		500		{object}	api.Response[any]				"Internal server error"
//	@Router			/api/v1/reset-password [post]
func (h *Handler) ResetPasswordHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var req dtos.ResetPasswordRequest
	if err := h.decoder.Decode(r, &req); err != nil {
		api.WriteValidationErrorResponse(w, err)
		return
	}

	err := h.passwords.ResetPassword(ctx, application.ResetPasswordParams{
		Type:        req.Type,
		Identity:    req.Identity,
		Code:        req.Code,
		NewPassword: req.NewPassword,
	})
	if err != nil {
		h.handleServiceError(ctx, w, err)
		return
	}

	api.WriteOkResponse(w, dtos.EmptyResponse{}, "Password reset")
}

// ChangePasswordHandler
//
//	@Summary		Change the password
//	@Description	Replaces the logged-in account's password. Logs out every other session and returns a new token pair for this one. A wrong current password answers 400 identity_wrong_password (not 401) and counts towards the login lockout.
//	@Tags			Authentication
//	@Accept			json
//	@Produce		json
//	@Security		GatewayKeyAuth && BearerAuth
//	@Param			request	body		dtos.ChangePasswordRequest			true	"Current and new password"
//	@Success		200		{object}	api.Response[dtos.TokenPairResponse]	"Password changed; use the new tokens"
//	@Failure		400		{object}	api.Response[any]					"identity_wrong_password, identity_password_unchanged, identity_weak_password or invalid request"
//	@Failure		401		{object}	api.Response[any]					"Unauthorized"
//	@Failure		403		{object}	api.Response[any]					"identity_account_locked"
//	@Failure		500		{object}	api.Response[any]					"Internal server error"
//	@Router			/api/v1/change-password [post]
func (h *Handler) ChangePasswordHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var req dtos.ChangePasswordRequest
	if err := h.decoder.Decode(r, &req); err != nil {
		api.WriteValidationErrorResponse(w, err)
		return
	}

	token, err := h.passwords.ChangePassword(ctx, application.ChangePasswordParams{
		AccountID:   authcontext.GetAccountID(ctx),
		OldPassword: req.OldPassword,
		NewPassword: req.NewPassword,
	})
	if err != nil {
		h.handleServiceError(ctx, w, err)
		return
	}

	// expires_in as LoginHandler sends it.
	api.WriteOkResponse(w, dtos.DomainToTokenPairResponse(token, 123), "Password changed")
}

// handleServiceError converts service errors to HTTP responses
// Logging is handled by middleware based on HTTP status code
func (h *Handler) handleServiceError(ctx context.Context, w http.ResponseWriter, err error) {
	appErr := domainerror.GetAppError(err)

	api.WriteError(
		w,
		appErr.StatusCode,
		appErr.Code,
		appErr.Message,
		nil,
	)
}
