package application

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/rasparac/rekreativko-api/internal/account-profile/domain"
	"github.com/rasparac/rekreativko-api/internal/account-profile/metrics"
	"github.com/rasparac/rekreativko-api/internal/shared/domainevent"
	"github.com/rasparac/rekreativko-api/internal/shared/logger"
	"github.com/rasparac/rekreativko-api/internal/shared/store/postgres"
	"github.com/rasparac/rekreativko-api/internal/shared/telemetry"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

type (
	accountSettingsReaderWriter interface {
		GetSettings(ctx context.Context, accountID uuid.UUID) (*domain.AccountProfileSettings, error)
		CreateSettings(ctx context.Context, settings *domain.AccountProfileSettings) error
		UpdateSettings(ctx context.Context, settings *domain.AccountProfileSettings) error
	}

	accountSettingsService struct {
		accountSettingsManager accountSettingsReaderWriter
		logger                 *logger.Logger
		txManager              *postgres.TransactionManager
		tracer                 trace.Tracer
		metrics                *metrics.Metrics
		eventWriter            domainevent.EventWriter
	}
)

func NewAccountSettingsService(
	accountSettingsManager accountSettingsReaderWriter,
	txManager *postgres.TransactionManager,
	eventWriter domainevent.EventWriter,
	logger *logger.Logger,
	metrics *metrics.Metrics,
) *accountSettingsService {
	return &accountSettingsService{
		accountSettingsManager: accountSettingsManager,
		txManager:              txManager,
		logger:                 logger.WithName("account_settings.service"),
		tracer:                 telemetry.Tracer(telemetry.TracerAccountSettingsService),
		metrics:                metrics,
		eventWriter:            eventWriter,
	}
}

func (uss *accountSettingsService) GetSettings(ctx context.Context, accountID uuid.UUID) (*domain.AccountProfileSettings, error) {
	ctx, span := uss.tracer.Start(
		ctx,
		"account_settings.service.GetSettings",
	)
	defer span.End()

	log := uss.logger.WithValues(
		"method", "GetSettings",
		"account_id", accountID,
	)

	span.SetAttributes(attribute.String(
		"account_id", accountID.String(),
	))

	settings, err := uss.accountSettingsManager.GetSettings(ctx, accountID)
	if err != nil {
		span.SetStatus(codes.Error, err.Error())
		log.Error(ctx, "get an account", "error", err)
		return nil, mapToAppErr(err)
	}

	span.SetStatus(codes.Ok, "settings found")

	log.Info(ctx, "settings found")

	return settings, nil
}

func (uss *accountSettingsService) UpdateSettings(
	ctx context.Context,
	accountID uuid.UUID,
	toUpdateSettings UpdateAccountSettingsParams,
) error {
	ctx, span := uss.tracer.Start(
		ctx,
		"account_settings.service.UpdateSettings",
	)
	defer span.End()

	log := uss.logger.WithValues(
		"method", "UpdateSettings",
		"account_id", accountID,
	)

	err := uss.txManager.WithTransaction(ctx, func(tCtx context.Context) error {
		currentSettings, err := uss.accountSettingsManager.GetSettings(tCtx, accountID)
		if err != nil {
			return err
		}

		domainSettings, err := domain.ToDomainSettingsFromRegistry(toUpdateSettings.Settings)
		if err != nil {
			return err
		}

		currentSettings.UpdateSettings(domainSettings)

		err = uss.accountSettingsManager.UpdateSettings(
			ctx,
			currentSettings,
		)
		if err != nil {
			return err
		}

		return nil
	})
	if err != nil {
		span.SetStatus(codes.Error, err.Error())
		log.Error(ctx, "update settings", "error", err)
		return fmt.Errorf("update settings: %w", err)
	}

	return nil
}

func (uss *accountSettingsService) CreateSettings(ctx context.Context, params CreateAccountSettingsParams) error {
	ctx, span := uss.tracer.Start(
		ctx,
		"account_settings.service.CreateSettings",
	)
	defer span.End()

	log := uss.logger.WithValues(
		"method", "CreateSettings",
		"account_id", params.AccountID,
	)

	span.SetAttributes(attribute.String(
		"account_id", params.AccountID.String(),
	))

	err := uss.txManager.WithTransaction(ctx, func(tCtx context.Context) error {
		settings, err := domain.NewAccountProfileSettings(params.AccountID)
		if err != nil {
			return fmt.Errorf("new default settings: %w", err)
		}

		err = uss.accountSettingsManager.CreateSettings(tCtx, settings)
		if err != nil {
			return err
		}

		return nil
	})
	if err != nil {
		log.Error(ctx, "create settings", "error", err)
		span.SetStatus(codes.Error, err.Error())
		return fmt.Errorf("create settings: %w", err)
	}

	span.SetStatus(codes.Ok, "settings created")

	log.Info(ctx, "settings created")

	return nil
}
