package main

import (
	"bytes"
	"errors"
	"fmt"
	"net/mail"
	"os"
	"strings"
	"time"

	"github.com/rasparac/rekreativko-api/shared/activitycatalog"
	"gopkg.in/yaml.v3"
)

// minPasswordLength mirrors identity's register validation (validate:"min=8").
const minPasswordLength = 8

type (
	// SeedFile is the YAML document: one entry per account to create.
	SeedFile struct {
		Defaults Defaults `yaml:"defaults"`
		Users    []User   `yaml:"users"`
	}

	// Defaults apply to every user that leaves the field empty.
	Defaults struct {
		Password string `yaml:"password"`
	}

	User struct {
		Email    string `yaml:"email"`
		Password string `yaml:"password"`
		Profile  Profile `yaml:"profile"`
		// Settings is sent as-is to PUT /my/settings, e.g.
		// {"privacy.profile.public": true, "preference.language": "hr"}.
		Settings map[string]any `yaml:"settings"`
	}

	Profile struct {
		FullName          string     `yaml:"full_name"`
		Nickname          string     `yaml:"nickname"`
		Bio               string     `yaml:"bio"`
		DateOfBirth       string     `yaml:"date_of_birth"` // YYYY-MM-DD
		Location          *Location  `yaml:"location"`
		ProfilePictureURL string     `yaml:"profile_picture_url"`
		ActivityInterests []Interest `yaml:"activity_interests"`
	}

	Location struct {
		City      string   `yaml:"city"`
		Country   string   `yaml:"country"`
		Latitude  *float64 `yaml:"latitude"`
		Longitude *float64 `yaml:"longitude"`
	}

	Interest struct {
		Name  string `yaml:"name"`  // an activity type, e.g. football
		Level string `yaml:"level"` // beginner, intermediate or advanced
	}
)

// LoadSeedFile reads and validates the YAML file at path. Unknown keys are an
// error, so a typo doesn't silently drop a field.
func LoadSeedFile(path string) (*SeedFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}

	return ParseSeedFile(data)
}

func ParseSeedFile(data []byte) (*SeedFile, error) {
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)

	var file SeedFile
	if err := dec.Decode(&file); err != nil {
		return nil, fmt.Errorf("parse yaml: %w", err)
	}

	file.normalize()

	if err := file.Validate(); err != nil {
		return nil, err
	}

	return &file, nil
}

// normalize fills defaults and lowercases the values the services match
// exactly (activity types and levels are lowercase in the catalog).
func (f *SeedFile) normalize() {
	for i := range f.Users {
		u := &f.Users[i]
		u.Email = strings.TrimSpace(u.Email)
		if u.Password == "" {
			u.Password = f.Defaults.Password
		}
		for j := range u.Profile.ActivityInterests {
			ai := &u.Profile.ActivityInterests[j]
			ai.Name = strings.ToLower(strings.TrimSpace(ai.Name))
			ai.Level = strings.ToLower(strings.TrimSpace(ai.Level))
		}
	}
}

// Validate reports every problem in the file at once, so they can all be
// fixed in one go instead of one per run.
func (f *SeedFile) Validate() error {
	if len(f.Users) == 0 {
		return errors.New("no users in the file")
	}

	var errs []error
	seen := make(map[string]struct{}, len(f.Users))

	for i, u := range f.Users {
		at := func(format string, args ...any) {
			errs = append(errs, fmt.Errorf("users[%d] (%s): %s", i, u.Email, fmt.Sprintf(format, args...)))
		}

		if _, err := mail.ParseAddress(u.Email); err != nil || u.Email == "" {
			at("invalid email")
		}

		// emails are case-insensitive (citext in identity.accounts)
		key := strings.ToLower(u.Email)
		if _, dup := seen[key]; dup {
			at("duplicate email")
		}
		seen[key] = struct{}{}

		if len(u.Password) < minPasswordLength {
			at("password must be at least %d characters (set it on the user or in defaults.password)", minPasswordLength)
		}

		p := u.Profile
		if p.DateOfBirth != "" {
			if _, err := time.Parse(time.DateOnly, p.DateOfBirth); err != nil {
				at("date_of_birth %q is not YYYY-MM-DD", p.DateOfBirth)
			}
		}

		if loc := p.Location; loc != nil {
			if loc.City == "" {
				at("location.city is required when location is set")
			}
			if (loc.Latitude == nil) != (loc.Longitude == nil) {
				at("location.latitude and location.longitude must be set together")
			}
		}

		for _, ai := range p.ActivityInterests {
			if !activitycatalog.ActivityType(ai.Name).IsValid() {
				at("unknown activity %q", ai.Name)
			}
			if !activitycatalog.ActivityLevel(ai.Level).IsValid() {
				at("activity %q: level %q must be beginner, intermediate or advanced", ai.Name, ai.Level)
			}
		}
	}

	return errors.Join(errs...)
}
