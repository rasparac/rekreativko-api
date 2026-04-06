package domain

import (
	"time"

	"github.com/google/uuid"
	"github.com/rasparac/rekreativko-api/internal/shared/domainevent"
)

type AccountProfileSettings struct {
	accountID uuid.UUID
	version   int
	settings  map[string]*Setting

	createdAt time.Time
	updatedAt time.Time
	events    []domainevent.Event
}

func NewAccountProfileSettings(accountID uuid.UUID) (*AccountProfileSettings, error) {
	defaultSettings, err := GetDefaultSettings()
	if err != nil {
		return nil, err
	}

	return &AccountProfileSettings{
		accountID: accountID,
		settings:  defaultSettings,
		version:   1,
		updatedAt: time.Now().UTC(),
		createdAt: time.Now(),
	}, nil
}

func ReconstructAccountProfileSettings(
	accountID uuid.UUID,
	version int,
	settings map[string]*Setting,
	createdAt time.Time,
	updatedAt time.Time,
) *AccountProfileSettings {
	return &AccountProfileSettings{
		accountID: accountID,
		version:   version,
		settings:  settings,
		createdAt: createdAt,
		updatedAt: updatedAt,
	}
}

func (ups *AccountProfileSettings) AccountID() uuid.UUID {
	return ups.accountID
}

func (ups *AccountProfileSettings) Settings() map[string]*Setting {
	return ups.settings
}

func (ups *AccountProfileSettings) CreatedAt() time.Time {
	return ups.updatedAt
}

func (ups *AccountProfileSettings) UpdatedAt() time.Time {
	return ups.updatedAt
}

func (ups *AccountProfileSettings) GetSetting(key string) (*Setting, bool) {
	s, exists := ups.settings[key]
	return s, exists
}

func (ups *AccountProfileSettings) UpdateSetting(setting *Setting) {
	ups.settings[setting.Key()] = setting
	ups.version++
	ups.updatedAt = time.Now()

	ups.addEvent(NewAccountSettingUpdatedEvent(
		ups.accountID,
		setting.Key(),
		setting.Value(),
	))
}

func (ups *AccountProfileSettings) UpdateSettings(settings map[string]*Setting) {
	changedSettings := make(map[string]*Setting, 0)
	for key, setting := range settings {
		if existingSetting, exists := ups.settings[key]; exists {
			if existingSetting.Value() == setting.Value() {
				continue
			}
			changedSettings[key] = setting
			ups.settings[key] = setting
		}
	}

	if len(changedSettings) == 0 {
		return
	}

	ups.version++
	ups.updatedAt = time.Now()

	ups.addEvent(NewAccountSettingsBulkUpdatedEvent(
		ups.accountID,
		changedSettings,
	))
}

func (ups *AccountProfileSettings) Version() int {
	return ups.version
}

func (ups *AccountProfileSettings) Events() []domainevent.Event {
	return ups.events
}

func (ups *AccountProfileSettings) ClearEvents() {
	ups.events = make([]domainevent.Event, 0)
}

func (ups *AccountProfileSettings) ResetToDefault() error {
	defaultSettings, err := GetDefaultSettings()
	if err != nil {
		return err
	}
	ups.settings = defaultSettings
	ups.version++
	ups.touch()

	ups.addEvent(NewAccountSettingsResetEvent(
		ups.accountID,
	))

	return nil
}

func (ups *AccountProfileSettings) touch() {
	ups.updatedAt = time.Now().UTC()
}

func (ups *AccountProfileSettings) addEvent(event domainevent.Event) {
	ups.events = append(ups.events, event)
}

func GetDefaultSettings() (map[string]*Setting, error) {
	var (
		defaults = make(map[string]*Setting)
		err      error
	)

	defaults["notification.email.enabled"], err = NewSetting(
		"notification.email.enabled",
		"true",
		SettingTypeBool,
	)
	if err != nil {
		return nil, err
	}

	defaults["notification.sms.enabled"], err = NewSetting(
		"notification.sms.enabled",
		"false",
		SettingTypeBool,
	)
	if err != nil {
		return nil, err
	}

	defaults["notification.push.enabled"], err = NewSetting(
		"notification.push.enabled",
		"false",
		SettingTypeBool,
	)
	if err != nil {
		return nil, err
	}

	// privacy
	defaults["privacy.location.enabled"], err = NewSetting(
		"privacy.location.enabled",
		"false",
		SettingTypeBool,
	)
	if err != nil {
		return nil, err
	}

	defaults["privacy.profile.public"], err = NewSetting(
		"privacy.profile.public",
		"false",
		SettingTypeBool,
	)
	if err != nil {
		return nil, err
	}

	defaults["privacy.activity.public"], err = NewSetting(
		"privacy.activity.public",
		"false",
		SettingTypeBool,
	)
	if err != nil {
		return nil, err
	}

	// prefrences
	defaults["preference.language"], err = NewSetting(
		"preference.language",
		"en",
		SettingTypeString,
	)
	if err != nil {
		return nil, err
	}

	defaults["preference.timezone"], err = NewSetting(
		"preference.timezone",
		"UTC",
		SettingTypeString,
	)
	if err != nil {
		return nil, err
	}

	defaults["preference.theme"], err = NewSetting(
		"preference.theme",
		"light",
		SettingTypeString,
	)
	if err != nil {
		return nil, err
	}

	defaults["activity.search_unit"], err = NewSetting(
		"activity.search_unit",
		"km",
		SettingTypeString,
	)
	if err != nil {
		return nil, err
	}

	defaults["activity.search_radius"], err = NewSetting(
		"activity.search_radius",
		"10.00",
		SettingTypeFloat,
	)
	if err != nil {
		return nil, err
	}

	return defaults, nil
}
