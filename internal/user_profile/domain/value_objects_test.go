package domain

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestNewNickname(t *testing.T) {
	testCases := []struct {
		name          string
		value         string
		expectedError error
		expectedNick  *Nickname
	}{
		{
			name:          "empty value",
			value:         "",
			expectedError: ErrInvalidNickname,
			expectedNick:  nil,
		},
		{
			name:          "value too short",
			value:         "ab",
			expectedError: ErrNicknameTooShort,
			expectedNick:  nil,
		},
		{
			name:          "value too long",
			value:         strings.Repeat("a", 51),
			expectedError: ErrNicknameTooLong,
			expectedNick:  nil,
		},
		{
			name:          "value with invalid characters",
			value:         "abc-****",
			expectedError: ErrInvalidNickname,
			expectedNick:  nil,
		},
		{
			name:          "valid value",
			value:         "abc123",
			expectedError: nil,
			expectedNick:  &Nickname{value: "abc123"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			nickname, err := NewNickname(tc.value)
			assert.Equal(t, tc.expectedError, err)
			if tc.expectedNick != nil {
				assert.Equal(t, tc.expectedNick.value, nickname.value)
			}
		})
	}
}

func TestNewDateOfBirth(t *testing.T) {
	testCases := []struct {
		name     string
		value    time.Time
		expected error
	}{
		{
			name:     "Value is zero",
			value:    time.Time{},
			expected: ErrDateOfBirthRequired,
		},
		{
			name:     "Value is in the future",
			value:    time.Now().AddDate(0, 0, 1),
			expected: ErrDateOfBirthInvalid,
		},
		{
			name:     "Value is too young",
			value:    time.Now().AddDate(-12, 0, 0),
			expected: ErrAgeTooYoung,
		},
		{
			name:     "Valid value",
			value:    time.Now().AddDate(-14, 0, 0),
			expected: nil,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			dateOfBirth, err := NewDateOfBirth(tc.value)

			if tc.expected == nil {
				assert.NoError(t, err)
				assert.NotNil(t, dateOfBirth)
			} else {
				assert.ErrorIs(t, err, tc.expected)
				assert.Nil(t, dateOfBirth)
			}

		})
	}
}

func TestNewLocation(t *testing.T) {
	tests := []struct {
		name     string
		city     string
		country  string
		lat      *float64
		lng      *float64
		expected error
	}{
		{
			name:     "valid location with coordinates",
			city:     "New York",
			country:  "USA",
			lat:      float64Ptr(40.7128),
			lng:      float64Ptr(-74.0060),
			expected: nil,
		},
		{
			name:     "valid location without coordinates",
			city:     "London",
			country:  "UK",
			lat:      nil,
			lng:      nil,
			expected: nil,
		},
		{
			name:     "invalid location with empty city",
			city:     "",
			country:  "USA",
			lat:      float64Ptr(40.7128),
			lng:      float64Ptr(-74.0060),
			expected: ErrLocationRequired,
		},
		{
			name:     "invalid location with empty country",
			city:     "New York",
			country:  "",
			lat:      float64Ptr(40.7128),
			lng:      float64Ptr(-74.0060),
			expected: ErrLocationRequired,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			lat, lng := tc.lat, tc.lng
			loc, err := NewLocation(tc.city, tc.country, lat, lng)

			if tc.expected == nil {
				assert.NoError(t, err)
				assert.NotNil(t, loc)
			} else {
				assert.ErrorIs(t, err, tc.expected)
				assert.Nil(t, loc)
			}
		})
	}
}

func TestNewActivityInterest(t *testing.T) {
	tests := []struct {
		name          string
		activityType  ActivityType
		level         ActivityLevel
		expected      *ActivityInterest
		expectedError error
	}{
		{
			name:         "valid activity type and level",
			activityType: ActivityTypeRunning,
			level:        ActivityLevelBeginner,
			expected: &ActivityInterest{
				acitivityType: ActivityTypeRunning,
				level:         ActivityLevelBeginner,
			},
			expectedError: nil,
		},
		{
			name:          "invalid activity type",
			activityType:  "invalid",
			level:         ActivityLevelBeginner,
			expected:      nil,
			expectedError: ErrInvalidActivityType,
		},
		{
			name:          "invalid level",
			activityType:  ActivityTypeRunning,
			level:         "invalid",
			expected:      nil,
			expectedError: ErrInvalidActivityLevel,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ai, err := NewActivityInterest(tc.activityType, tc.level)

			if tc.expectedError == nil {
				assert.NoError(t, err)
				assert.Equal(t, tc.expected, ai)
			} else {
				assert.ErrorIs(t, err, tc.expectedError)
				assert.Nil(t, ai)
			}
		})
	}
}

func Test_validateSettingValue(t *testing.T) {
	tests := []struct {
		name        string
		value       string
		settingType SettingType
		expected    error
	}{
		{
			name:        "valid boolean value",
			value:       "true",
			settingType: SettingTypeBool,
			expected:    nil,
		},
		{
			name:        "invalid boolean value",
			value:       "invalid",
			settingType: SettingTypeBool,
			expected:    fmt.Errorf("%w: boolean value must be 'true' or 'false'", ErrInvalidSettingsValue),
		},
		{
			name:        "valid number value",
			value:       "123.45",
			settingType: SettingTypeInt,
			expected:    nil,
		},
		{
			name:        "invalid number value",
			value:       "invalid",
			settingType: SettingTypeInt,
			expected:    fmt.Errorf("%w: invalid number format", ErrInvalidSettingsValue),
		},
		{
			name:        "valid JSON value",
			value:       `{"key": "value"}`,
			settingType: SettingTypeJSON,
			expected:    nil,
		},
		{
			name:        "invalid JSON value",
			value:       "invalid",
			settingType: SettingTypeJSON,
			expected:    fmt.Errorf("%w: invalid JSON format", ErrInvalidSettingsValue),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := validateSettingValue(tc.value, tc.settingType)

			assert.Equal(t, tc.expected, err)
		})
	}
}

func Test_isValidSettingKey(t *testing.T) {
	tests := []struct {
		name string
		key  string
		want bool
	}{
		{
			name: "valid setting key",
			key:  "setting_key",
			want: true,
		},
		{
			name: "invalid setting key with special characters",
			key:  "setting_key!",
			want: false,
		},
		{
			name: "invalid setting key with consecutive dots",
			key:  "setting..key",
			want: false,
		},
		{
			name: "invalid setting key with leading or trailing dot",
			key:  ".setting_key",
			want: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := isValidSettingKey(tc.key)

			assert.Equal(t, tc.want, got)
		})
	}
}

func float64Ptr(f float64) *float64 {
	return &f
}
