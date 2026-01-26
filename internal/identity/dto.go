package identity

import (
	"time"

	"github.com/google/uuid"
	"github.com/rasparac/rekreativko-api/internal/identity/domain"
)

type (

	// RegisterAccountRequest is a request to register a new account
	RegisterAccountRequest struct {
		Email       string `json:"email" validate:"omitempty,email" example:"user@gmail.com"`
		PhoneNumber string `json:"phone_number" validate:"omitempty,e164" example:"+1234567890"`
		Password    string `json:"password" validate:"required,min=8" example:"StrongPassw0rd!"`
	}

	// LoginRequest is a request to login
	LoginRequest struct {
		Email       string `json:"email" validate:"omitempty,email" example:"user@gmail.com"`
		PhoneNumber string `json:"phone_number" validate:"omitempty,e164" example:"+1234567890"`
		Password    string `json:"password" validate:"required" example:"StrongPassw0rd!"`
	}

	// LogoutRequest is a request to logout
	LogoutRequest struct {
		AccountID    uuid.UUID `json:"account_id" validate:"required" example:"123e4567-e89b-12d3-a456-426655440000"`
		RefreshToken string    `json:"refresh_token" validate:"required" example:"ey12344fwbfwbefiuwgfiiAA=="`
	}

	// VerifyAccountRequest is a request to verify an account
	VerifyAccountRequest struct {
		Code string `json:"code" validate:"required,min=6" example:"123456"`
	}

	// ResendVerificationCodeRequest is a request to resend a verification code
	ResendVerificationCodeRequest struct {
		Type     string `json:"type" validate:"required,oneof=email phone" example:"email"`
		Identity string `json:"identity" validate:"required" example:"+1234567890,user@gmail.com"`
	}

	// RefreshTokenRequest is a request to refresh a token
	RefreshTokenRequest struct {
		RefreshToken string `json:"refresh_token" validate:"required" example:"ey12344fwbfwbefiuwgfiiAA=="`
	}

	// ChangePasswordRequest is a request to change a password
	ChangePasswordRequest struct {
		OldPassword string `json:"old_password" validate:"required,min=8" example:"12345678"`
		NewPassword string `json:"new_password" validate:"required,min=8" example:"12345678"`
	}

	// Response

	// RegisterAccountResponse is a response to register a new account
	RegisterAccountResponse struct {
		AccountID uuid.UUID `json:"account_id" example:"123e4567-e89b-12d3-a456-426655440000"`
	}

	// TokenPairResponse is a response to login
	TokenPairResponse struct {
		AccessToken  string `json:"access_token" example:"ey12344fwbfwbefiuwgfiiAA..."`
		RefreshToken string `json:"refresh_token" example:"ey12344fwbfwbefiuwgfiiAA=="`
		ExpiresIn    int64  `json:"expires_in" example:"3600"`
		TokenType    string `json:"token_type" example:"Bearer"`
	}

	// VerifyAccountResponse is a response to verify an account
	VerifyAccountResponse struct {
		AccountID uuid.UUID `json:"account_id" example:"123e4567-e89b-12d3-a456-426655440000"`
	}

	// AccountResponse is a response to get an account
	AccountResponse struct {
		AccountID   uuid.UUID `json:"account_id" example:"123e4567-e89b-12d3-a456-426655440000"`
		Email       string    `json:"email" example:"user@example"`
		PhoneNumber string    `json:"phone_number" example:"+1234567890"`
		Status      string    `json:"status" example:"active"`
		CreatedAt   time.Time `json:"created_at" example:"2020-01-01T00:00:00Z"`
		UpdatedAt   time.Time `json:"updated_at" example:"2020-01-01T00:00:00Z"`
	}

	// EmptyResponse is a response with no data
	EmptyResponse struct{}
)

func domainToAccountResponse(account *domain.Account) *AccountResponse {
	return &AccountResponse{
		AccountID:   account.ID(),
		Email:       account.Email().String(),
		PhoneNumber: account.PhoneNumber().String(),
		Status:      string(account.Status()),
		CreatedAt:   account.CreatedAt(),
		UpdatedAt:   account.UpdatedAt(),
	}
}
