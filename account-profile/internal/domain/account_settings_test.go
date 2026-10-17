package domain

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func settingsInput(t *testing.T, values map[string]any) map[string]*Setting {
	t.Helper()

	settings, err := ToDomainSettingsFromRegistry(values)
	require.NoError(t, err)
	return settings
}

func TestAccountProfileSettings_UpdateSettings_ReportsWhetherAnythingChanged(t *testing.T) {
	settings, err := NewAccountProfileSettings(uuid.New())
	require.NoError(t, err)

	// notification.email.enabled defaults to true.
	same := settingsInput(t, map[string]any{"notification.email.enabled": true})

	assert.False(t, settings.UpdateSettings(same))
	assert.Equal(t, 1, settings.Version(), "nothing changed: the version must stay put")

	assert.False(t, settings.UpdateSettings(map[string]*Setting{}))
	assert.Equal(t, 1, settings.Version())

	different := settingsInput(t, map[string]any{"notification.email.enabled": false})

	assert.True(t, settings.UpdateSettings(different))
	assert.Equal(t, 2, settings.Version())

	// The same values again are now the current ones.
	assert.False(t, settings.UpdateSettings(different))
	assert.Equal(t, 2, settings.Version())
}
