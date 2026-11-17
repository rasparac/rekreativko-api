package domain

import (
	"time"

	"github.com/google/uuid"
)

type AccountProfileSettings struct {
	accountID uuid.UUID
	version   int
	settings  map[string]*Setting

	createdAt time.Time
	updatedAt time.Time
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
}

// UpdateSettings applies the given values to the settings it already has and
// reports whether anything actually changed. When nothing did, the aggregate is
// left untouched - same version - so the caller must not save it: the
// repository's optimistic-lock UPDATE expects a bumped version and would
// otherwise fail with a version conflict.
func (ups *AccountProfileSettings) UpdateSettings(settings map[string]*Setting) (changed bool) {
	for key, setting := range settings {
		if existingSetting, exists := ups.settings[key]; exists {
			if existingSetting.Value() == setting.Value() {
				continue
			}
			changed = true
			ups.settings[key] = setting
		}
	}

	if !changed {
		return false
	}

	ups.version++
	ups.updatedAt = time.Now()

	return true
}

func (ups *AccountProfileSettings) Version() int {
	return ups.version
}

func (ups *AccountProfileSettings) ResetToDefault() error {
	defaultSettings, err := GetDefaultSettings()
	if err != nil {
		return err
	}
	ups.settings = defaultSettings
	ups.version++
	ups.touch()

	return nil
}

func (ups *AccountProfileSettings) touch() {
	ups.updatedAt = time.Now().UTC()
}

// GetDefaultSettings is every setting of the registry at its default, so what
// an account is seeded with is exactly what GET /my/settings shows and
// PUT /my/settings accepts.
func GetDefaultSettings() (map[string]*Setting, error) {
	defaults := make(map[string]*Setting, len(settingRegistry))
	for key, def := range settingRegistry {
		setting, err := NewSetting(key, def.Default, def.Type)
		if err != nil {
			return nil, err
		}
		defaults[key] = setting
	}

	return defaults, nil
}
