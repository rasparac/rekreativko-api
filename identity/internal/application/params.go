package application

import "github.com/google/uuid"

type (
	RegistrationParams struct {
		Email       string
		PhoneNumber string
		Password    string
	}

	LoginParams struct {
		Email       string
		PhoneNumber string
		Password    string
	}

	LogoutParams struct {
		AccountID    uuid.UUID
		RefreshToken string
	}

	// Move this to domain and add refresh token
	TokenPairResponse struct {
		AccessToken  string
		RefreshToken string
		ExpiresIn    int64
		TokenType    string
	}

	RefreshTokenParams struct {
		RefreshToken string
	}

	ResendVerificationCodeParams struct {
		Type     string
		Identity string
	}

	VerifyAccountParams struct {
		Code string
	}

	// ForgotPasswordParams: Type is "email" or "phone", Identity the address.
	ForgotPasswordParams struct {
		Type     string
		Identity string
	}

	ResetPasswordParams struct {
		Type        string
		Identity    string
		Code        string
		NewPassword string
	}

	ChangePasswordParams struct {
		AccountID   uuid.UUID
		OldPassword string
		NewPassword string
	}

	VerifyAccount struct {
		Code string
	}

	VerifyAccountResponse struct {
		AccountID uuid.UUID
	}

	RefreshToken struct {
		RefreshToken string
	}

	EmptyResponse struct{}
)
