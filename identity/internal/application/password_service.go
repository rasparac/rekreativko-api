package application

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/rasparac/rekreativko-api/identity/internal/domain"
	"github.com/rasparac/rekreativko-api/shared/domainevent"
	"github.com/rasparac/rekreativko-api/shared/logger"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
)

type (
	passwordResetStore interface {
		// GetForUpdate returns the account's outstanding code, row-locked, or nil.
		GetForUpdate(ctx context.Context, accountID uuid.UUID) (*domain.PasswordReset, error)
		Save(ctx context.Context, reset *domain.PasswordReset) error
		DeleteAllForAccount(ctx context.Context, accountID uuid.UUID) error
	}

	// PasswordService holds the password use cases: forgot/reset for a user who
	// can't log in, change for one who can. It shares Service's repositories,
	// lockout and token issuing.
	PasswordService struct {
		*Service
		resets passwordResetStore
		logger *logger.Logger
	}
)

func NewPasswordService(svc *Service, resets passwordResetStore) *PasswordService {
	return &PasswordService{
		Service: svc,
		resets:  resets,
		logger:  svc.logger.WithName("password"),
	}
}

// ForgotPassword sends a reset code to the address, if it belongs to an active
// account. It answers the same either way, so it can't be used to find out
// which addresses have an account, and sends at most one code per
// domain.PasswordResetResendAfter.
func (p *PasswordService) ForgotPassword(ctx context.Context, req ForgotPasswordParams) error {
	ctx, span := p.tracer.Start(ctx, "identity.service.ForgotPassword")
	defer span.End()

	log := p.logger.WithValues("method", "ForgotPassword", "type", req.Type)

	codeType := domain.CodeType(req.Type)
	if !codeType.IsValid() {
		return mapToAppErr(domain.ErrInvalidVerificationCode)
	}

	var outcome string
	err := p.txManager.WithTransaction(ctx, func(tCtx context.Context) error {
		filter := identityFilter(codeType, req.Identity)
		filter.ForUpdate = true // one request per account at a time

		account, err := p.accountRepository.GetBy(tCtx, filter)
		if errors.Is(err, domain.ErrAccountNotFound) {
			outcome = "no account"
			return nil
		}
		if err != nil {
			return fmt.Errorf("get account: %w", err)
		}

		log = log.WithValues("account_id", account.ID())

		if !account.IsActive() {
			outcome = "account not active"
			return nil
		}

		previous, err := p.resets.GetForUpdate(tCtx, account.ID())
		if err != nil {
			return fmt.Errorf("get password reset: %w", err)
		}
		if previous != nil && previous.RecentlySent(time.Now().UTC()) {
			outcome = "code sent recently"
			return nil
		}

		code, err := p.codeGenerator.Generate()
		if err != nil {
			return fmt.Errorf("generate code: %w", err)
		}

		reset, err := domain.NewPasswordReset(account, code, codeType)
		if err != nil {
			return err
		}

		if err := p.resets.Save(tCtx, reset); err != nil {
			return fmt.Errorf("save password reset: %w", err)
		}

		if err := p.domainWriter.InsertEvents(tCtx, reset.Events()...); err != nil {
			return fmt.Errorf("insert events: %w", err)
		}
		reset.ClearEvents()

		outcome = "code sent"
		return nil
	})
	if err != nil {
		span.SetStatus(codes.Error, err.Error())
		log.Error(ctx, "failed to handle forgot password", "error", err)
		return mapToAppErr(err)
	}

	span.SetAttributes(attribute.String("outcome", outcome))
	span.SetStatus(codes.Ok, "forgot password handled")
	log.Info(ctx, "forgot password handled", "outcome", outcome)

	return nil
}

// ResetPassword sets a new password with the code ForgotPassword sent. An
// unknown address and a wrong code are the same error. Every session is
// revoked, so whoever knew the old password is logged out, and a login lock
// is lifted.
func (p *PasswordService) ResetPassword(ctx context.Context, req ResetPasswordParams) error {
	ctx, span := p.tracer.Start(ctx, "identity.service.ResetPassword")
	defer span.End()

	log := p.logger.WithValues("method", "ResetPassword", "type", req.Type)

	codeType := domain.CodeType(req.Type)
	if !codeType.IsValid() {
		return mapToAppErr(domain.ErrInvalidVerificationCode)
	}

	if err := domain.ValidatePassword(req.NewPassword); err != nil {
		return mapToAppErr(err)
	}

	// A rejected code is returned after the transaction commits, so the
	// failed attempt it counted is kept.
	var rejected error
	err := p.txManager.WithTransaction(ctx, func(tCtx context.Context) error {
		filter := identityFilter(codeType, req.Identity)
		filter.ForUpdate = true

		account, err := p.accountRepository.GetBy(tCtx, filter)
		if errors.Is(err, domain.ErrAccountNotFound) {
			rejected = domain.ErrInvalidVerificationCode
			return nil
		}
		if err != nil {
			return fmt.Errorf("get account: %w", err)
		}

		log = log.WithValues("account_id", account.ID())

		reset, err := p.resets.GetForUpdate(tCtx, account.ID())
		if err != nil {
			return fmt.Errorf("get password reset: %w", err)
		}
		if reset == nil || !account.IsActive() {
			rejected = domain.ErrInvalidVerificationCode
			return nil
		}

		if checkErr := reset.Check(req.Code, time.Now().UTC()); checkErr != nil {
			rejected = checkErr
			if reset.Burned() {
				return p.resets.DeleteAllForAccount(tCtx, account.ID())
			}
			return p.resets.Save(tCtx, reset)
		}

		hash, err := p.passwordHasher.Hash(req.NewPassword)
		if err != nil {
			return fmt.Errorf("hash password: %w", err)
		}

		if err := account.ResetPassword(domain.NewPasswordFromHash(hash)); err != nil {
			return err
		}

		if err := p.accountRepository.UpdateAccount(tCtx, account); err != nil {
			return fmt.Errorf("update account: %w", err)
		}

		if err := p.resets.DeleteAllForAccount(tCtx, account.ID()); err != nil {
			return fmt.Errorf("delete password reset: %w", err)
		}

		if err := p.refreshTokenRepository.RevokeAll(tCtx, account.ID()); err != nil {
			return fmt.Errorf("revoke refresh tokens: %w", err)
		}

		if err := p.domainWriter.InsertEvents(tCtx, account.Events()...); err != nil {
			return fmt.Errorf("insert events: %w", err)
		}
		account.ClearEvents()

		return nil
	})
	if err == nil {
		err = rejected
	}
	if err != nil {
		span.SetStatus(codes.Error, err.Error())
		log.Warn(ctx, "password not reset", "error", err)
		return mapToAppErr(err)
	}

	span.SetStatus(codes.Ok, "password reset")
	log.Info(ctx, "password reset")

	return nil
}

// ChangePassword replaces a logged-in account's password. A wrong current
// password counts towards the login lockout, since it is a password guess
// too. Every session is revoked and a new token pair returned, so this device
// stays logged in and every other one has to log in again.
func (p *PasswordService) ChangePassword(ctx context.Context, req ChangePasswordParams) (*domain.RefreshToken, error) {
	ctx, span := p.tracer.Start(ctx, "identity.service.ChangePassword")
	defer span.End()

	log := p.logger.WithValues("method", "ChangePassword", "account_id", req.AccountID)
	span.SetAttributes(attribute.String("account_id", req.AccountID.String()))

	account, err := p.accountRepository.GetBy(ctx, AccountFilter{UUID: &req.AccountID})
	if err != nil {
		return nil, mapToAppErr(err)
	}

	if err := account.CanLogin(); err != nil {
		return nil, mapToAppErr(err)
	}

	// bcrypt is slow: compared outside the transaction, like Login does.
	if err := p.passwordHasher.ComparePasswordAndHash(req.OldPassword, account.Password().String()); err != nil {
		if recordErr := p.recordFailedLogin(ctx, account.ID()); recordErr != nil {
			log.Error(ctx, "failed to record failed password check", "error", recordErr)
			return nil, mapToAppErr(recordErr)
		}
		span.SetStatus(codes.Error, "wrong current password")
		return nil, mapToAppErr(domain.ErrWrongPassword)
	}

	if req.NewPassword == req.OldPassword {
		return nil, mapToAppErr(domain.ErrPasswordUnchanged)
	}

	if err := domain.ValidatePassword(req.NewPassword); err != nil {
		return nil, mapToAppErr(err)
	}

	hash, err := p.passwordHasher.Hash(req.NewPassword)
	if err != nil {
		return nil, mapToAppErr(fmt.Errorf("hash password: %w", err))
	}

	var token *domain.RefreshToken
	err = p.txManager.WithTransaction(ctx, func(tCtx context.Context) error {
		locked, err := p.accountRepository.GetBy(tCtx, AccountFilter{UUID: &req.AccountID, ForUpdate: true})
		if err != nil {
			return fmt.Errorf("lock account: %w", err)
		}

		if err := locked.ChangePassword(domain.NewPasswordFromHash(hash)); err != nil {
			return err
		}

		if err := p.accountRepository.UpdateAccount(tCtx, locked); err != nil {
			return fmt.Errorf("update account: %w", err)
		}

		if err := p.refreshTokenRepository.RevokeAll(tCtx, locked.ID()); err != nil {
			return fmt.Errorf("revoke refresh tokens: %w", err)
		}

		token, err = p.generateToken(tCtx, locked.ID())
		if err != nil {
			return fmt.Errorf("generate token: %w", err)
		}

		events := append([]domainevent.Event(nil), locked.Events()...)
		events = append(events, token.Events()...)
		if err := p.domainWriter.InsertEvents(tCtx, events...); err != nil {
			return fmt.Errorf("insert events: %w", err)
		}
		locked.ClearEvents()
		token.ClearEvents()

		return nil
	})
	if err != nil {
		span.SetStatus(codes.Error, err.Error())
		log.Error(ctx, "failed to change password", "error", err)
		return nil, mapToAppErr(err)
	}

	span.SetStatus(codes.Ok, "password changed")
	log.Info(ctx, "password changed")

	return token, nil
}

// identityFilter finds an account by the address the user typed.
func identityFilter(codeType domain.CodeType, identity string) AccountFilter {
	if codeType == domain.CodeTypePhone {
		return AccountFilter{PhoneNumber: &identity}
	}

	return AccountFilter{Email: &identity}
}
