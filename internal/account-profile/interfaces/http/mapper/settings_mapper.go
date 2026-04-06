package mapper

import (
	"maps"

	"github.com/rasparac/rekreativko-api/internal/account-profile/application"
	"github.com/rasparac/rekreativko-api/internal/account-profile/domain"
	"github.com/rasparac/rekreativko-api/internal/account-profile/interfaces/http/dtos"
)

func ToAccountSettingsResponse(settings map[string]*domain.Setting) map[string]dtos.Setting {
	resp := make(map[string]dtos.Setting, len(settings))
	for _, setting := range settings {

		value, err := mapValue(setting)
		if err != nil {
			continue
		}

		settinDefiniton, ok := domain.GetSettingDefinition(setting.Key())
		if !ok {
			continue
		}

		resp[setting.Key()] = dtos.Setting{
			Value:       value,
			Type:        setting.Type().String(),
			Label:       settinDefiniton.Label,
			Description: settinDefiniton.Description,
			Category:    string(settinDefiniton.Category),
		}
	}

	return resp
}

func UpdateAccountSettingsRequestToParams(settings map[string]any) application.UpdateAccountSettingsParams {
	appSettings := make(map[string]any, len(settings))
	maps.Copy(appSettings, settings)

	params := application.UpdateAccountSettingsParams{
		Settings: appSettings,
	}

	return params
}

func mapValue(setting *domain.Setting) (any, error) {
	switch setting.Type() {
	case domain.SettingTypeBool:
		return setting.BoolValue()
	case domain.SettingTypeFloat:
		return setting.FloatValue()
	case domain.SettingTypeInt:
		return setting.IntValue()
	case domain.SettingTypeString:
		return setting.StringValue()
	default:
		return setting.StringValue()
	}
}
