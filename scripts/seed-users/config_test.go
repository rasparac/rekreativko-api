package main

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseSeedFile(t *testing.T) {
	t.Run("the example file is valid", func(t *testing.T) {
		data, err := os.ReadFile("users.example.yaml")
		require.NoError(t, err)

		file, err := ParseSeedFile(data)
		require.NoError(t, err)

		require.Len(t, file.Users, 6)
		assert.Equal(t, "Test1234!", file.Users[0].Password, "the default password fills in")
		assert.Equal(t, "1990-05-15", file.Users[0].Profile.DateOfBirth)
		assert.Equal(t, true, file.Users[0].Settings["privacy.profile.public"])
	})

	t.Run("an unquoted date and capitalised interests are accepted", func(t *testing.T) {
		file, err := ParseSeedFile([]byte(`
defaults:
  password: Default1234!
users:
  - email: a@example.com
    password: Test1234!
    profile:
      date_of_birth: 1990-05-15
      activity_interests:
        - { name: Football, level: Advanced }
`))
		require.NoError(t, err)

		assert.Equal(t, "Test1234!", file.Users[0].Password, "a user's own password wins over the default")

		p := file.Users[0].Profile
		assert.Equal(t, "1990-05-15", p.DateOfBirth)
		assert.Equal(t, Interest{Name: "football", Level: "advanced"}, p.ActivityInterests[0])
	})

	t.Run("every problem is reported at once", func(t *testing.T) {
		_, err := ParseSeedFile([]byte(`
users:
  - email: not-an-email
    password: short
  - email: b@example.com
    password: Test1234!
    profile:
      date_of_birth: 15.05.1990
      location: { country: Croatia, latitude: 45.8 }
      activity_interests:
        - { name: quidditch, level: expert }
  - email: B@example.com
    password: Test1234!
    profile:
      nickname: Same
  - email: c@example.com
    password: Test1234!
    profile:
      nickname: same
`))
		require.Error(t, err)

		for _, want := range []string{
			"users[0] (not-an-email): invalid email",
			"users[0] (not-an-email): password must be at least 8 characters",
			`date_of_birth "15.05.1990" is not YYYY-MM-DD`,
			"location.city is required",
			"latitude and location.longitude must be set together",
			`unknown activity "quidditch"`,
			`level "expert" must be`,
			"users[2] (B@example.com): duplicate email",
			`users[3] (c@example.com): duplicate nickname "same"`,
		} {
			assert.Contains(t, err.Error(), want)
		}
	})

	t.Run("a misspelt key is an error, not a silently dropped field", func(t *testing.T) {
		_, err := ParseSeedFile([]byte(`
users:
  - email: a@example.com
    profile:
      nick_name: typo
`))
		assert.ErrorContains(t, err, "field nick_name not found")
	})

	t.Run("an empty file", func(t *testing.T) {
		_, err := ParseSeedFile([]byte("users: []\n"))
		assert.ErrorContains(t, err, "no users")
	})
}

func TestProfileRequest(t *testing.T) {
	lat, lng := 45.8, 15.9
	body := profileRequest(Profile{
		Nickname:          "igorb",
		Location:          &Location{City: "Zagreb", Country: "Croatia", Latitude: &lat, Longitude: &lng},
		ActivityInterests: []Interest{{Name: "football", Level: "beginner"}},
	})

	assert.Equal(t, map[string]any{
		"nickname":           "igorb",
		"location_city":      "Zagreb",
		"location_country":   "Croatia",
		"location_latitude":  45.8,
		"location_longitude": 15.9,
		"activity_interests": []map[string]string{{"name": "football", "level": "beginner"}},
	}, body, "fields the YAML leaves out are not sent, so the service keeps them")
}
