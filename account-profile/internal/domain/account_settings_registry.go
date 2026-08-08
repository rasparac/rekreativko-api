package domain

import (
	"fmt"
	"strconv"
)

type (
	SettingCategory string

	SettingDefinition struct {
		Key         string
		Type        SettingType
		Default     string
		Description string          // optional (useful for docs)
		Category    SettingCategory // optional (for grouping in UI)
		Label       string          // optional (for display in UI)
	}
)

const (
	CategoryNotification SettingCategory = "notification"
	CategoryPrivacy      SettingCategory = "privacy"
	CategoryPreference   SettingCategory = "preference"
	CategoryActivity     SettingCategory = "activity"
)

var settingRegistry = map[string]SettingDefinition{
	// Notification settings
	SettingNotificationEmailEnabled: {
		Key:         SettingNotificationEmailEnabled,
		Type:        SettingTypeBool,
		Default:     "true",
		Description: "Enable or disable email notifications. When enabled, you will receive email updates about your account activity and important announcements.",
		Category:    CategoryNotification,
		Label:       "Email notifications",
	},
	SettingNotificationSMSenabled: {
		Key:         SettingNotificationSMSenabled,
		Type:        SettingTypeBool,
		Default:     "false",
		Description: "",
		Category:    CategoryNotification,
		Label:       "SMS notifications",
	},
	SettingNotificationPushEnabled: {
		Key:         SettingNotificationPushEnabled,
		Type:        SettingTypeBool,
		Default:     "true",
		Description: "When enabled, you will receive push notifications on your mobile device for important updates and activity related to your account.",
		Category:    CategoryNotification,
		Label:       "Push notifications",
	},

	// Privacy settings
	SettingPrivacyProfilePublic: {
		Key:         SettingPrivacyProfilePublic,
		Type:        SettingTypeBool,
		Default:     "false",
		Category:    CategoryPrivacy,
		Description: "Show or hide your profile details from other users. When enabled, your profile information will be visible to everyone. When disabled, only you will be able to see your profile details.",
		Label:       "Public profile",
	},
	SeetingPrivacyShowLocation: {
		Key:         SeetingPrivacyShowLocation,
		Type:        SettingTypeBool,
		Default:     "false",
		Description: "Show or hide your location on your profile. When enabled, other users will be able to see your location information. When disabled, your location will be hidden from your profile.",
		Category:    CategoryPrivacy,
		Label:       "Show location",
	},
	SettingPrivacyShowActivities: {
		Key:         SettingPrivacyShowActivities,
		Type:        SettingTypeBool,
		Default:     "true",
		Description: "Show or hide your activities on your profile. When enabled, other users will be able to see your recent activities and posts. When disabled, your activities will be hidden from your profile.",
		Category:    CategoryPrivacy,
		Label:       "Show activities",
	},
	SettingsPrivacyShowStatistics: {
		Key:         SettingsPrivacyShowStatistics,
		Type:        SettingTypeBool,
		Default:     "true",
		Description: "Show or hide your statistics on your profile. When enabled, other users will be able to see your statistics. When disabled, your statistics will be hidden from your profile.",
		Category:    CategoryPrivacy,
		Label:       "Show statistics",
	},

	// Preferences
	SettingLanguage: {
		Key:         SettingLanguage,
		Type:        SettingTypeString,
		Default:     "en",
		Description: "Set your preferred language for the application interface.",
		Category:    CategoryPreference,
		Label:       "Language",
	},
	SettingTheme: {
		Key:         SettingTheme,
		Type:        SettingTypeString,
		Default:     "light",
		Description: "Set your preferred theme for the application interface.",
		Category:    CategoryPreference,
		Label:       "Theme",
	},
	SettingTimezone: {
		Key:         SettingTimezone,
		Type:        SettingTypeString,
		Default:     "UTC",
		Description: "Set your preferred timezone for displaying dates and times in the application.",
		Category:    CategoryPreference,
		Label:       "Timezone",
	},

	// Activity
	SettingActivitySearchRadius: {
		Key:         SettingActivitySearchRadius,
		Type:        SettingTypeInt,
		Default:     "10",
		Description: "Set the search radius for activities in kilometers. When you search for activities, the application will show results within this radius from your location.",
		Category:    CategoryActivity,
		Label:       "Activity search radius (km)",
	},
	SettingsActivitySearchUnit: {
		Key:         SettingsActivitySearchUnit,
		Type:        SettingTypeString,
		Default:     "km",
		Description: "Set the unit for activity search radius. You can choose between kilometers (km) and miles (mi). This setting works together with the Activity search radius setting to determine the distance for activity searches.",
		Category:    CategoryActivity,
		Label:       "Activity search radius unit",
	},
}

func GetSettingDefinition(key string) (SettingDefinition, bool) {
	def, ok := settingRegistry[key]
	return def, ok
}

func AllSettingDefinitions() []SettingDefinition {
	defs := make([]SettingDefinition, 0, len(settingRegistry))
	for _, def := range settingRegistry {
		defs = append(defs, def)
	}
	return defs
}

func ToDomainSettingsFromRegistry(
	input map[string]any,
) (map[string]*Setting, error) {
	settings := make(map[string]*Setting, len(input))
	for key, rawValue := range input {
		def, ok := GetSettingDefinition(key)
		if !ok {
			return nil, fmt.Errorf("unknown setting: %s", key)
		}

		stringValue, err := convertAnyToString(rawValue, def.Type)
		if err != nil {
			return nil, fmt.Errorf("invalid value for %s: %w", key, err)
		}

		setting, err := NewSetting(key, stringValue, def.Type)
		if err != nil {
			return nil, err
		}

		settings[key] = setting
	}

	return settings, nil
}

func convertAnyToString(v any, valueType SettingType) (string, error) {
	switch valueType {
	case SettingTypeBool:
		b, ok := v.(bool)
		if !ok {
			return "", fmt.Errorf("expected bool")
		}
		return strconv.FormatBool(b), nil
	case SettingTypeString:
		s, ok := v.(string)
		if !ok {
			return "", fmt.Errorf("expected string")
		}
		return s, nil
	case SettingTypeInt:
		switch n := v.(type) {
		case float64: // JSON numbers decode as float64
			return strconv.Itoa(int(n)), nil
		case int:
			return strconv.Itoa(n), nil
		default:
			return "", fmt.Errorf("expected int")
		}
	case SettingTypeFloat:
		switch n := v.(type) {
		case float64:
			return strconv.FormatFloat(n, 'f', -1, 64), nil
		default:
			return "", fmt.Errorf("expected float")
		}
	default:
		return "", fmt.Errorf("invalid setting type")
	}
}
