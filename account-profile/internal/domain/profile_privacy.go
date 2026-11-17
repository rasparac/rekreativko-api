package domain

import "strconv"

// ProfilePrivacy is what an account lets other accounts see of its profile
// (rekreativko-api-ep8.7).
type ProfilePrivacy struct {
	// Public shows the full name, bio and activity interests.
	Public bool
	// ShowLocation shows the city and country.
	ShowLocation bool
}

// ProfilePrivacySettings are the setting keys ProfilePrivacyFromSettings reads.
var ProfilePrivacySettings = []string{SettingPrivacyProfilePublic, SettingPrivacyShowLocation}

// ProfilePrivacyFromSettings reads the privacy settings from their stored
// values by key; a missing or unreadable one takes the registry's default.
func ProfilePrivacyFromSettings(values map[string]string) ProfilePrivacy {
	return ProfilePrivacy{
		Public:       boolSetting(values, SettingPrivacyProfilePublic),
		ShowLocation: boolSetting(values, SettingPrivacyShowLocation),
	}
}

func boolSetting(values map[string]string, key string) bool {
	if v, err := strconv.ParseBool(values[key]); err == nil {
		return v
	}

	v, _ := strconv.ParseBool(settingRegistry[key].Default)
	return v
}

// SeenByOthers is a copy of the profile with only what other accounts may see:
// the nickname and profile picture always (groups and invites need them), the
// full name, bio and activity interests if the profile is public, the city and
// country if the location is shown. The date of birth and the coordinates are
// never shown to anyone else.
func (ap *AccountProfile) SeenByOthers(privacy ProfilePrivacy) *AccountProfile {
	seen := &AccountProfile{
		id:             ap.id,
		nickname:       ap.nickname,
		profilePicture: ap.profilePicture,
		createdAt:      ap.createdAt,
		updatedAt:      ap.updatedAt,
	}

	if privacy.Public {
		seen.fullName = ap.fullName
		seen.bio = ap.bio
		seen.activityInterest = ap.activityInterest
	}

	if privacy.ShowLocation && ap.location != nil {
		seen.location = &Location{city: ap.location.city, country: ap.location.country}
	}

	return seen
}
