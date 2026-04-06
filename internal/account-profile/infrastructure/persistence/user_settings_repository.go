package persistence

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/rasparac/rekreativko-api/internal/account-profile/domain"
	"github.com/rasparac/rekreativko-api/internal/shared/store/postgres"
)

type (
	accountSettingModel struct {
		AccountID uuid.UUID
		Key       string
		Value     string
		Type      string
		UpdatedAt time.Time
	}

	accountSettigMetadataModel struct {
		AccountID uuid.UUID
		Version   int
		CreatedAt time.Time
		UpdatedAt time.Time
	}

	AccountProfileSettingsManager struct {
		txManager *postgres.TransactionManager
	}

	AccountSettingsReaderWriter interface {
		GetSettings(ctx context.Context, accountID uuid.UUID) (*domain.AccountProfileSettings, error)
		CreateSettings(ctx context.Context, settings *domain.AccountProfileSettings) error
		UpdateSettings(ctx context.Context, settings *domain.AccountProfileSettings) error
	}
)

func NewAccountProfileSettingsManager(
	txManager *postgres.TransactionManager,
) *AccountProfileSettingsManager {
	return &AccountProfileSettingsManager{
		txManager: txManager,
	}
}

func (upsm *AccountProfileSettingsManager) GetSettings(
	ctx context.Context,
	accountID uuid.UUID,
) (*domain.AccountProfileSettings, error) {

	metadataQuery := `SELECT account_id, version, created_at, updated_at
	FROM account_profile.account_settings_meta
	WHERE account_id = $1`

	var metadata accountSettigMetadataModel
	err := upsm.txManager.Querier(ctx).QueryRow(ctx, metadataQuery, accountID).Scan(
		&metadata.AccountID,
		&metadata.Version,
		&metadata.CreatedAt,
		&metadata.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}

	settingsQuery := `SELECT account_id, key, value, type, updated_at
	FROM account_profile.settings
	WHERE account_id = $1 ORDER BY key ASC`

	rows, err := upsm.txManager.Querier(ctx).Query(ctx, settingsQuery, accountID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var settings []*accountSettingModel
	for rows.Next() {
		var setting accountSettingModel
		if err := rows.Scan(
			&setting.AccountID,
			&setting.Key,
			&setting.Value,
			&setting.Type,
			&setting.UpdatedAt,
		); err != nil {
			return nil, err
		}
		settings = append(settings, &setting)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	domainSettings, err := toDomainAccountProfileSettings(settings, &metadata)
	if err != nil {
		return nil, err
	}

	return domainSettings, nil
}

func (upsm *AccountProfileSettingsManager) CreateSettings(
	ctx context.Context,
	settings *domain.AccountProfileSettings,
) error {
	settingsModels, metadata := toaccountSettingModel(settings)

	if err := upsm.insertSettings(ctx, settingsModels); err != nil {
		return err
	}

	if err := upsm.insertMetadata(ctx, metadata); err != nil {
		return err
	}

	return nil
}

func (upsm *AccountProfileSettingsManager) UpdateSettings(
	ctx context.Context,
	settings *domain.AccountProfileSettings,
) error {
	settingsModels, metadata := toaccountSettingModel(settings)

	updateQuery := `UPDATE account_profile.account_settings_meta
	SET version = $1, updated_at = $2
	WHERE account_id = $3 AND version = $4`

	cmdTag, err := upsm.txManager.Querier(ctx).Exec(ctx, updateQuery,
		metadata.Version,
		metadata.UpdatedAt,
		metadata.AccountID,
		metadata.Version-1,
	)
	if err != nil {
		return err
	}

	if cmdTag.RowsAffected() == 0 {
		return fmt.Errorf("version conflict: expected version %d, but was %d", metadata.Version-1, metadata.Version)
	}

	if len(settingsModels) > 0 {
		deleteQuery := `DELETE FROM account_profile.settings WHERE account_id = $1`
		_, err := upsm.txManager.Querier(ctx).Exec(ctx, deleteQuery, metadata.AccountID)
		if err != nil {
			return err
		}

		if err := upsm.insertSettings(ctx, settingsModels); err != nil {
			return err
		}
	}

	return nil
}

func (r *AccountProfileSettingsManager) insertMetadata(ctx context.Context, metadata *accountSettigMetadataModel) error {
	const query = `INSERT INTO
		account_profile.account_settings_meta
		(account_id, version, created_at, updated_at)
	VALUES ($1, $2, $3, $4)`

	_, err := r.txManager.Querier(ctx).Exec(ctx, query,
		metadata.AccountID,
		metadata.Version,
		metadata.CreatedAt,
		metadata.UpdatedAt,
	)
	if err != nil {
		return err
	}

	return nil
}

func (r *AccountProfileSettingsManager) insertSettings(ctx context.Context, settings []*accountSettingModel) error {
	var (
		settingValuesValues = make([]any, 0, len(settings)*5)
		placeHolders        = make([]string, 0)
		query               = `INSERT INTO
		account_profile.settings
			(account_id, key, value, type, updated_at)
		VALUES %s`
	)
	for i, setting := range settings {
		settingValuesValues = append(settingValuesValues,
			setting.AccountID,
			setting.Key,
			setting.Value,
			setting.Type,
			setting.UpdatedAt,
		)
		placeHolders = append(placeHolders,
			fmt.Sprintf("($%d, $%d, $%d, $%d, $%d)", i*5+1, i*5+2, i*5+3, i*5+4, i*5+5),
		)
	}

	insertSettingsQuery := fmt.Sprintf(
		query,
		strings.Join(placeHolders, ","),
	)

	_, err := r.txManager.Querier(ctx).Exec(ctx, insertSettingsQuery, settingValuesValues...)
	if err != nil {
		return err
	}

	return nil
}

func toaccountSettingModel(settings *domain.AccountProfileSettings) ([]*accountSettingModel, *accountSettigMetadataModel) {
	settingModels := make([]*accountSettingModel, 0, len(settings.Settings()))

	for key, setting := range settings.Settings() {
		settingModels = append(settingModels, &accountSettingModel{
			AccountID: settings.AccountID(),
			Key:       key,
			Value:     setting.Value(),
			Type:      setting.Type().String(),
			UpdatedAt: settings.UpdatedAt(),
		})
	}

	metadataModel := &accountSettigMetadataModel{
		AccountID: settings.AccountID(),
		Version:   settings.Version(),
		CreatedAt: settings.CreatedAt(),
		UpdatedAt: settings.UpdatedAt(),
	}

	return settingModels, metadataModel
}

func toDomainAccountProfileSettings(
	settings []*accountSettingModel,
	metadata *accountSettigMetadataModel,
) (*domain.AccountProfileSettings, error) {
	accountSettings := make(map[string]*domain.Setting)

	for _, setting := range settings {
		domainSetting, err := domain.NewSetting(
			setting.Key,
			setting.Value,
			domain.SettingType(setting.Type),
		)
		if err != nil {
			return nil, err
		}
		accountSettings[setting.Key] = domainSetting
	}

	return domain.ReconstructAccountProfileSettings(
		metadata.AccountID,
		metadata.Version,
		accountSettings,
		metadata.CreatedAt,
		metadata.UpdatedAt,
	), nil
}
