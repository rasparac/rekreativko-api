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
