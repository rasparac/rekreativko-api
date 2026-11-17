package domain

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func fullProfile(t *testing.T) *AccountProfile {
	t.Helper()

	nickname, err := NewNickname("runner_1")
	require.NoError(t, err)
	fullName, err := NewFullName("Ana Anić")
	require.NoError(t, err)
	picture, err := NewProfilePicture("https://example.com/a.jpg", time.Now())
	require.NoError(t, err)
	lat, lng := 44.81, 20.46
	location, err := NewLocation("Belgrade", "Serbia", &lat, &lng)
	require.NoError(t, err)
	interest, err := NewActivityInterest(ActivityTypeRunning, ActivityLevelBeginner)
	require.NoError(t, err)

	p := NewAccountProfile(uuid.New())
	p.nickname = nickname
	p.fullName = fullName
	p.profilePicture = picture
	p.bio = "I run"
	p.location = location
	p.activityInterest = []*ActivityInterest{interest}
	p.dateOfBirth = &DateOfBirth{}
	return p
}

func TestAccountProfile_SeenByOthers(t *testing.T) {
	p := fullProfile(t)

	t.Run("private: only the nickname and picture", func(t *testing.T) {
		seen := p.SeenByOthers(ProfilePrivacy{})

		assert.Equal(t, p.ID(), seen.ID())
		assert.Equal(t, p.Nickname(), seen.Nickname())
		assert.Equal(t, p.ProfilePicture(), seen.ProfilePicture())
		assert.Nil(t, seen.FullName())
		assert.Empty(t, seen.Bio())
		assert.Empty(t, seen.ActivityInterests())
		assert.Nil(t, seen.Location())
		assert.Nil(t, seen.DateOfBirth())
	})

	t.Run("public with location: never the date of birth or coordinates", func(t *testing.T) {
		seen := p.SeenByOthers(ProfilePrivacy{Public: true, ShowLocation: true})

		assert.Equal(t, p.FullName(), seen.FullName())
		assert.Equal(t, "I run", seen.Bio())
		assert.Len(t, seen.ActivityInterests(), 1)
		require.NotNil(t, seen.Location())
		assert.Equal(t, "Belgrade", seen.Location().City())
		assert.Equal(t, "Serbia", seen.Location().Country())
		assert.False(t, seen.Location().HasCoordinates())
		assert.Nil(t, seen.DateOfBirth())
	})

	t.Run("the owner's profile is left as it was", func(t *testing.T) {
		assert.True(t, p.Location().HasCoordinates())
		assert.NotNil(t, p.DateOfBirth())
	})
}

func TestProfilePrivacyFromSettings(t *testing.T) {
	assert.Equal(t, ProfilePrivacy{}, ProfilePrivacyFromSettings(nil), "the registry defaults: private")
	assert.Equal(t, ProfilePrivacy{Public: true, ShowLocation: true}, ProfilePrivacyFromSettings(map[string]string{
		SettingPrivacyProfilePublic: "true",
		SettingPrivacyShowLocation:  "true",
	}))
	assert.Equal(t, ProfilePrivacy{}, ProfilePrivacyFromSettings(map[string]string{SettingPrivacyProfilePublic: "garbage"}))
}

func TestGetDefaultSettings_MatchesTheRegistry(t *testing.T) {
	defaults, err := GetDefaultSettings()
	require.NoError(t, err)

	for _, def := range AllSettingDefinitions() {
		s, ok := defaults[def.Key]
		require.True(t, ok, def.Key)
		assert.Equal(t, def.Default, s.Value(), def.Key)
		assert.Equal(t, def.Type, s.Type(), def.Key)
	}
	assert.Len(t, defaults, len(AllSettingDefinitions()))
}
