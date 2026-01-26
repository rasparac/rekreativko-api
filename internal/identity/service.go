package identity

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/rasparac/rekreativko-api/internal/identity/domain"
	"github.com/rasparac/rekreativko-api/internal/identity/token"
	"github.com/rasparac/rekreativko-api/internal/shared/api"
	"github.com/rasparac/rekreativko-api/internal/shared/domainerror"
	"github.com/rasparac/rekreativko-api/internal/shared/domainevent"
	"github.com/rasparac/rekreativko-api/internal/shared/logger"
	"github.com/rasparac/rekreativko-api/internal/shared/metrics"
	"github.com/rasparac/rekreativko-api/internal/shared/store/postgres"
	"github.com/rasparac/rekreativko-api/internal/shared/telemetry"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

const schema = "identity"

type Service struct {
	accountRepository          AccountReaderWriter
	verificationCodeRepository VerificationCodeReaderWriter
	txManager                  *postgres.TransactionManager
	refreshTokenRepository     RefreshTokenReaderWriter
	domainWriter               domainevent.EventWriter
	tokenGenerator             *token.Generator
	logger                     *logger.Logger
	codeGenerator              *token.VerificationCodeGenerator
	passwordHasher             *token.PasswordHasher

	tracer  trace.Tracer
	metrics *metrics.Metrics
}

func NewService(
	accountRepository AccountReaderWriter,
	verificationCodeRepository VerificationCodeReaderWriter,
	refreshTokenRepository RefreshTokenReaderWriter,
	txManager *postgres.TransactionManager,
	domainWriter domainevent.EventWriter,
	tokenGenerator *token.Generator,
	logger *logger.Logger,
	codeGenerator *token.VerificationCodeGenerator,
	passwordHasher *token.PasswordHasher,
	metrics *metrics.Metrics,
) *Service {
	return &Service{
		accountRepository:          accountRepository,
		verificationCodeRepository: verificationCodeRepository,
		txManager:                  txManager,
		refreshTokenRepository:     refreshTokenRepository,
		tokenGenerator:             tokenGenerator,
		domainWriter:               domainWriter,
		logger:                     logger.WithName("service.identity"),
		codeGenerator:              codeGenerator,
		passwordHasher:             passwordHasher,
		tracer:                     telemetry.Tracer(telemetry.TracerIdentityService),
		metrics:                    metrics,
	}
}

func (s *Service) GetAccount(ctx context.Context, accountID uuid.UUID) (*domain.Account, error) {
	ctx, span := s.tracer.Start(
		ctx,
		"GetAccount",
	)
	defer span.End()

	log := s.logger.WithValues(
		"method", "GetAccount",
		"account_id", accountID,
	)

	span.SetAttributes(attribute.String(
		"account_id", accountID.String(),
	))

	account, err := s.accountRepository.GetBy(ctx, AccountFilter{
		UUID: &accountID,
	})
	if err != nil {
		span.SetStatus(codes.Error, err.Error())
		log.Error(ctx, "failed to create a new account", "error", err)
		return nil, mapToAppErr(err)
	}

	span.SetStatus(codes.Ok, "account found")

	log.Info(ctx, "account found")

	return account, nil
}

func (s *Service) Register(ctx context.Context, req RegisterAccountRequest) (*RegisterAccountResponse, error) {
	ctx, span := s.tracer.Start(
		ctx,
		"Register",
	)
	defer span.End()

	log := s.logger.WithValues(
		"method", "Register",
		"email", req.Email,
		"phone_number", req.PhoneNumber,
	)

	span.AddEvent("registration started")

	if req.Email == "" && req.PhoneNumber == "" {
		log.Error(ctx, "no email or phone number provided")
		return nil, domainerror.BadRequest("email_or_phone_required", "Email or phone number is required", nil)
	}

	var (
		accountID          uuid.UUID
		registrationMethod string
	)
	err := s.txManager.WithTransaction(ctx, func(tCtx context.Context) error {
		email, err := domain.NewEmail(req.Email)
		if err != nil {
			return err
		}

		phoneNumber, err := domain.NewPhoneNumber(req.PhoneNumber)
		if err != nil {
			return err
		}

		err = domain.ValidatePassword(req.Password)
		if err != nil {
			return fmt.Errorf("validate password: %w", err)
		}

		hashedPassword, err := s.passwordHasher.Hash(req.Password)
		if err != nil {
			return fmt.Errorf("hash password: %w", err)
		}

		password := domain.NewPasswordFromHash(hashedPassword)
		account, err := domain.NewAccount(email, phoneNumber, password)
		if err != nil {
			return err
		}

		span.AddEvent("insert new account")

		err = s.accountRepository.CreateAccount(tCtx, account)
		if err != nil {
			return fmt.Errorf("persist new account: %w", err)
		}

		accountID = account.ID()

		log = log.WithValues(
			"account_id", accountID,
		)

		code, err := s.codeGenerator.Generate()
		if err != nil {
			return fmt.Errorf("generate verification code: %w", err)
		}

		log = log.WithValues(
			"code", code,
		)

		registrationMethod = "email"
		codeType := domain.CodeTypeEmail
		if req.PhoneNumber != "" {
			registrationMethod = "phone"
			codeType = domain.CodeTypePhone
		}

		log = log.WithValues(
			"code_type", codeType,
		)

		verificationCode, err := domain.NewVerificationCode(
			account,
			code,
			codeType,
			time.Now().Add(time.Hour),
		)
		if err != nil {
			return err
		}

		span.AddEvent("insert new verification code")

		err = s.verificationCodeRepository.CreateVerificationCode(tCtx, verificationCode)
		if err != nil {
			return fmt.Errorf("persist verification code: %w", err)
		}

		allEvents := make(
			[]domainevent.Event,
			0,
			len(account.Events())+len(verificationCode.Events()),
		)

		allEvents = append(allEvents, account.Events()...)
		allEvents = append(allEvents, verificationCode.Events()...)

		err = s.domainWriter.InsertEvents(
			tCtx,
			schema,
			allEvents,
		)
		if err != nil {
			return fmt.Errorf("insert domain events: %w", err)
		}

		account.ClearEvents()
		verificationCode.ClearEvents()

		return nil
	})
	if err != nil {
		span.SetStatus(codes.Error, err.Error())
		log.Error(ctx, "failed to create a new account", "error", err)
		return nil, mapToAppErr(err)
	}

	span.SetStatus(codes.Ok, "registration successful")

	s.metrics.IdentityRegistrationsTotal.WithLabelValues(registrationMethod).Inc()

	log.Info(ctx, "account created")

	return &RegisterAccountResponse{
		AccountID: accountID,
	}, nil
}

func (s *Service) Login(ctx context.Context, req LoginRequest) (*TokenPairResponse, error) {
	ctx, span := s.tracer.Start(
		ctx,
		"Login",
	)
	defer span.End()

	log := s.logger.WithValues(
		"method", "Login",
		"email", req.Email,
		"phone_number", req.PhoneNumber,
	)

	loginMethod := "email"
	if req.PhoneNumber != "" {
		loginMethod = "phone"
	}

	s.metrics.IdentityLoginAttemptsTotal.WithLabelValues(loginMethod).Inc()

	if req.Email == "" && req.PhoneNumber == "" {
		log.Error(ctx, "no email or phone number provided")
		return nil, domainerror.BadRequest("email_or_phone_required", "Email or phone number is required", nil)
	}

	var filter AccountFilter
	if req.PhoneNumber != "" {
		filter.PhoneNumber = &req.PhoneNumber
	} else if req.Email != "" {
		filter.Email = &req.Email
	}

	account, err := s.accountRepository.GetBy(ctx, filter)
	if err != nil {
		s.logger.Error(ctx, "failed to get account", "error", err)
		return nil, mapToAppErr(err)
	}

	log = log.WithValues(
		"account_id", account.ID(),
	)

	err = account.CanLogin()
	if err != nil {
		s.logger.Error(ctx, "failed to login", "error", err)
		return nil, mapToAppErr(err)
	}

	var (
		token       *domain.RefreshToken
		accessToken string
	)
	err = s.txManager.WithTransaction(ctx, func(tCtx context.Context) error {
		err = s.passwordHasher.ComparePasswordAndHash(req.Password, account.Password().String())
		if err != nil {
			span.RecordError(err)
			s.metrics.IdentityLoginFailuresTotal.WithLabelValues("invalid_credentials").Inc()

			s.logger.Error(ctx, "failed to compare password", "error", err)

			account.RecordFailedLoginAttempt(api.IpAddressFromContext(ctx))

			err = s.accountRepository.UpdateAccount(tCtx, account)
			if err != nil {
				span.RecordError(err)
				return fmt.Errorf("update account, failed login: %w", err)
			}

			span.AddEvent("insert events, failed login")

			err = s.domainWriter.InsertEvents(tCtx, schema, account.Events())
			if err != nil {
				span.RecordError(err)
				return fmt.Errorf("insert events: %w", err)
			}

			return domain.ErrInvalidCredentials
		}

		token, accessToken, err = s.generateToken(tCtx, account.ID())
		if err != nil {
			return fmt.Errorf("generate token: %w", err)
		}

		account.RecordSuccessfulLogin(api.IpAddressFromContext(ctx))

		err = s.accountRepository.UpdateAccount(tCtx, account)
		if err != nil {
			return fmt.Errorf("update account, successful login: %w", err)
		}

		allEvents := make(
			[]domainevent.Event,
			0,
			len(account.Events())+(len(token.Events())),
		)
		allEvents = append(allEvents, account.Events()...)
		allEvents = append(allEvents, token.Events()...)

		err = s.domainWriter.InsertEvents(tCtx, schema, allEvents)
		if err != nil {
			return fmt.Errorf("insert events: %w", err)
		}

		account.ClearEvents()
		token.ClearEvents()

		return nil
	})
	if err != nil {
		span.SetStatus(codes.Error, err.Error())
		s.logger.Error(ctx, "failed to generate token", "error", err)
		return nil, mapToAppErr(err)
	}

	span.SetStatus(codes.Ok, "login successful")

	s.metrics.IdentityLoginSuccessTotal.Inc()

	log.Debug(ctx, "login successful")

	return &TokenPairResponse{
		AccessToken:  accessToken,
		RefreshToken: token.Token(),
		ExpiresIn:    int64(s.tokenGenerator.AccessTokenDuration().Seconds()),
		TokenType:    "Bearer",
	}, nil
}

func (s *Service) Logout(ctx context.Context, req LogoutRequest) (*EmptyResponse, error) {
	ctx, span := s.tracer.Start(
		ctx,
		"Logout",
	)
	defer span.End()

	log := s.logger.WithValues(
		"method", "Logout",
		"account_id", req.AccountID,
		"refresh_token", req.RefreshToken,
	)

	err := s.txManager.WithTransaction(ctx, func(txCtx context.Context) error {
		token, err := s.refreshTokenRepository.GetTokenBy(ctx, RefreshTokenFilter{
			AccountID: req.AccountID,
			Token:     req.RefreshToken,
		})
		if err != nil {
			return fmt.Errorf("get refresh token by accountID: %w", err)
		}

		err = token.Revoke("user logout")
		if err != nil {
			return err
		}

		err = s.refreshTokenRepository.Revoke(ctx, token.ID())
		if err != nil {
			return fmt.Errorf("revoke all tokens: %w", err)
		}

		events := token.Events()
		err = s.domainWriter.InsertEvents(txCtx, schema, events)
		if err != nil {
			return fmt.Errorf("insert events: %w", err)
		}

		token.ClearEvents()

		return nil
	})
	if err != nil {
		span.SetStatus(codes.Error, err.Error())
		log.Error(ctx, "failed to logout", "error", err)
		return nil, mapToAppErr(err)
	}

	span.SetStatus(codes.Ok, "logout successful")

	log.Info(ctx, "logout successful")

	return nil, nil
}

func (s *Service) VerifyAccount(ctx context.Context, req VerifyAccountRequest) (*VerifyAccountResponse, error) {
	ctx, span := s.tracer.Start(
		ctx,
		"VerifyAccount",
	)
	defer span.End()

	log := s.logger.WithValues(
		"method", "VerifyAccount",
		"code", req.Code,
	)

	var accountID uuid.UUID
	err := s.txManager.WithTransaction(ctx, func(tCtx context.Context) error {
		verificationCodes, err := s.verificationCodeRepository.GetVerificationCodesBy(
			tCtx,
			VerificationCodeFilter{
				Code: req.Code,
			},
		)
		if err != nil {
			return fmt.Errorf("get verification code: %w", err)
		}

		if len(verificationCodes) == 0 {
			return domainerror.NotFound("verification_code_not_found", "Verification code not found", err)
		}

		verificationCode := verificationCodes[0]
		accountID = verificationCode.AccountID()

		log = log.WithValues(
			"account_id", accountID,
		)

		account, err := s.accountRepository.GetBy(tCtx, AccountFilter{
			UUID: &accountID,
		})
		if err != nil {
			return fmt.Errorf("get account by verification code: %w", err)
		}

		if account.IsActive() {
			return domain.ErrAccountAlredyVerified
		}

		err = verificationCode.Verify(req.Code)
		if err != nil {
			return err
		}

		err = verificationCode.Use()
		if err != nil {
			return err
		}

		err = s.verificationCodeRepository.MarkAsUsed(tCtx, req.Code)
		if err != nil {
			return err
		}

		err = account.Activate(verificationCode)
		if err != nil {
			return err
		}

		err = s.accountRepository.UpdateAccount(tCtx, account)
		if err != nil {
			return fmt.Errorf("update account: %w", err)
		}

		allEvents := make(
			[]domainevent.Event,
			0,
			len(account.Events())+len(verificationCode.Events()),
		)

		allEvents = append(allEvents, account.Events()...)
		allEvents = append(allEvents, verificationCode.Events()...)

		err = s.domainWriter.InsertEvents(tCtx, schema, allEvents)
		if err != nil {
			return fmt.Errorf("insert events: %w", err)
		}

		account.ClearEvents()
		verificationCode.ClearEvents()

		return nil
	})
	if err != nil {
		span.SetStatus(codes.Error, err.Error())
		s.metrics.IdentityVerificationTotal.WithLabelValues("fail").Inc()
		log.Error(ctx, "failed to verify account", "error", err)
		return nil, mapToAppErr(err)
	}

	span.SetStatus(codes.Ok, "account verified")
	s.metrics.IdentityVerificationTotal.WithLabelValues("success").Inc()

	log.Info(ctx, "account verified")

	return &VerifyAccountResponse{
		AccountID: accountID,
	}, nil
}

func (s *Service) RefreshToken(ctx context.Context, req RefreshTokenRequest) (*TokenPairResponse, error) {
	ctx, span := s.tracer.Start(
		ctx,
		"RefreshToken",
	)
	defer span.End()

	log := s.logger.WithValues(
		"method", "RefreshToken",
		"refresh_token", req.RefreshToken,
	)

	if req.RefreshToken == "" {
		return nil, domainerror.BadRequest("refresh_token_required", "Refresh token is required", nil)
	}

	var (
		newToken    *domain.RefreshToken
		accessToken string
	)
	err := s.txManager.WithTransaction(ctx, func(tCtx context.Context) error {
		oldToken, err := s.refreshTokenRepository.GetTokenBy(tCtx, RefreshTokenFilter{
			Token: req.RefreshToken,
		})
		if err != nil {
			return fmt.Errorf("get refresh token: %w", err)
		}

		err = oldToken.Validate()
		if err != nil {
			return err
		}

		err = oldToken.Revoke("token rotation")
		if err != nil {
			return err
		}

		err = s.refreshTokenRepository.Revoke(tCtx, oldToken.ID())
		if err != nil {
			return fmt.Errorf("revoke token :%w", err)
		}

		newToken, accessToken, err = s.generateToken(tCtx, oldToken.AccountID())
		if err != nil {
			return fmt.Errorf("generate new token: %w", err)
		}

		newTokenEvents := newToken.Events()
		oldTokenEvents := oldToken.Events()
		allEvents := make(
			[]domainevent.Event,
			0,
			len(newTokenEvents)+len(oldTokenEvents),
		)
		allEvents = append(allEvents, newTokenEvents...)
		allEvents = append(allEvents, oldTokenEvents...)

		err = s.domainWriter.InsertEvents(tCtx, schema, allEvents)
		if err != nil {
			return fmt.Errorf("insert events: %w", err)
		}

		newToken.ClearEvents()
		oldToken.ClearEvents()

		return nil
	})
	if err != nil {
		span.SetStatus(codes.Error, err.Error())
		log.Error(ctx, "failed to refresh token", "error", err)
		return nil, mapToAppErr(err)
	}

	span.SetStatus(codes.Ok, "token refreshed")

	log.Info(ctx, "token refreshed")

	return &TokenPairResponse{
		AccessToken:  accessToken,
		RefreshToken: newToken.Token(),
		ExpiresIn:    int64(s.tokenGenerator.AccessTokenDuration().Seconds()),
		TokenType:    "Bearer",
	}, nil
}

func (s *Service) ResendVerificationCode(ctx context.Context, req ResendVerificationCodeRequest) (*EmptyResponse, error) {
	ctx, span := s.tracer.Start(
		ctx,
		"ResendVerificationCode",
	)
	defer span.End()

	log := s.logger.WithValues(
		"method", "ResendVerificationCode",
		"account_id", req.Identity,
		"type", req.Type,
	)

	err := s.txManager.WithTransaction(ctx, func(tCtx context.Context) error {
		filter := AccountFilter{}
		if req.Type == "email" {
			filter.Email = &req.Identity
		} else {
			filter.PhoneNumber = &req.Identity
		}

		account, err := s.accountRepository.GetBy(tCtx, filter)
		if err != nil {
			return fmt.Errorf("get account by identity: %w", err)
		}

		if account.IsActive() {
			return domainerror.BadRequest("account_already_verified", "Account is already verified", nil)
		}

		code, err := s.codeGenerator.Generate()
		if err != nil {
			return fmt.Errorf("generate verification code: %w", err)
		}

		log = log.WithValues(
			"code", code,
		)

		codeType := domain.CodeTypeEmail
		if !account.HasEmail() {
			codeType = domain.CodeTypePhone
		}

		verificationCode, err := domain.NewVerificationCode(
			account,
			code,
			codeType,
			time.Now().UTC().Add(15*time.Minute),
		)
		if err != nil {
			return err
		}

		err = s.verificationCodeRepository.CreateVerificationCode(tCtx, verificationCode)
		if err != nil {
			return fmt.Errorf("create new verification code: %w", err)
		}

		err = s.domainWriter.InsertEvents(
			tCtx,
			schema,
			verificationCode.Events(),
		)
		if err != nil {
			return fmt.Errorf("insert events: %w", err)
		}

		return nil
	})
	if err != nil {
		span.SetStatus(codes.Error, err.Error())
		log.Error(ctx, "failed to resend verification code", "error", err)
		return nil, mapToAppErr(err)
	}

	span.SetStatus(codes.Ok, "verification code resent")

	log.Info(ctx, "verification code resent")

	return &EmptyResponse{}, nil
}

func (s *Service) ValidateAccessToken(ctx context.Context, token string) (uuid.UUID, error) {
	claims, err := s.tokenGenerator.ValidateAccessToken(token)
	if err != nil {
		return uuid.Nil, err
	}

	accountUUID, err := uuid.Parse(claims.Subject)
	if err != nil {
		return uuid.Nil, domainerror.Unauthorized("invalid_token_subject", "Invalid token subject", nil)
	}

	return accountUUID, nil
}

func (s *Service) generateToken(ctx context.Context, accountID uuid.UUID) (*domain.RefreshToken, string, error) {
	refreshTokenValue, err := s.tokenGenerator.GenerateRefreshToken()
	if err != nil {
		return nil, "", err
	}

	refreshToken := domain.NewRefreshToken(
		accountID,
		refreshTokenValue,
		time.Now().UTC().Add(s.tokenGenerator.RefreshTokenDuration()),
	)

	accessToken, err := s.tokenGenerator.GenerateAccessToken(accountID)
	if err != nil {
		return nil, "", err
	}

	err = s.refreshTokenRepository.CreateRefreshToken(ctx, refreshToken)
	if err != nil {
		return nil, "", err
	}

	return refreshToken, accessToken, nil
}

func mapToAppErr(err error) *domainerror.AppError {
	pgErr := postgres.GetPgxError(err)
	if pgErr != nil {
		return MapPostgresError(pgErr)
	}

	return domain.MapErrToAppError(err)
}
