package domain

import (
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type Nickname struct {
	value string
}

var nicknameRegex = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

func NewNickname(value string) (*Nickname, error) {
	value = strings.TrimSpace(value)

	if value == "" {
		return nil, ErrInvalidNickname
	}

	if len(value) < 3 {
		return nil, ErrNicknameTooShort
	}

	if len(value) > 50 {
		return nil, ErrNicknameTooLong
	}

	if !nicknameRegex.MatchString(value) {
		return nil, ErrInvalidNickname
	}

	return &Nickname{value: value}, nil
}

func (n *Nickname) Value() string {
	return n.value
}

type FullName struct {
	value string
}

func NewFullName(value string) (*FullName, error) {
	value = strings.TrimSpace(value)

	if value == "" {
		return &FullName{}, nil
	}

	return &FullName{value: value}, nil
}

func (f *FullName) Value() string {
	return f.value
}

type DateOfBirth struct {
	value time.Time
}

func NewDateOfBirth(value time.Time) (*DateOfBirth, error) {
	if value.IsZero() {
		return nil, ErrDateOfBirthRequired
	}

	if value.After(time.Now()) {
		return nil, ErrDateOfBirthInvalid
	}

	minDate := time.Now().AddDate(-13, 0, 0)
	if value.After(minDate) {
		return nil, ErrAgeTooYoung
	}

	return &DateOfBirth{value: value}, nil
}

func (d *DateOfBirth) Value() time.Time {
	return d.value
}

func (d *DateOfBirth) Age() int {
	now := time.Now()
	age := now.Year() - d.value.Year()

	if now.Month() < d.value.Month() || (now.Month() == d.value.Month() && now.Day() < d.value.Day()) {
		age--
	}

	return age
}

type (
	Location struct {
		city        string
		country     string
		coordinates *Cooridantes
	}

	Cooridantes struct {
		latitude  float64
		longitude float64
	}
)

func NewLocation(city, country string, lat, lng *float64) (*Location, error) {
	city = strings.TrimSpace(city)
	country = strings.TrimSpace(country)

	if city == "" || country == "" {
		return nil, ErrLocationRequired
	}

	loc := &Location{
		city:    city,
		country: country,
	}

	if lat != nil && lng != nil {
		loc.coordinates = &Cooridantes{
			latitude:  *lat,
			longitude: *lng,
		}
	}

	return loc, nil
}

func (l *Location) City() string {
	return l.city
}

func (l *Location) Country() string {
	return l.country
}

func (l *Location) Coordinates() *Cooridantes {
	return l.coordinates
}

func (l *Location) HasCoordinates() bool {
	return l.coordinates != nil
}

func (c *Cooridantes) Longitude() float64 {
	return c.longitude
}

func (c *Cooridantes) Latitude() float64 {
	return c.latitude
}

const (
	maxProfilePictureURLLength = 2048
)

type ProfilePicture struct {
	url        string
	uploadedAt time.Time
}

func NewProfilePicture(pictureURL string, uploadedAt time.Time) (*ProfilePicture, error) {
	pictureURL = strings.TrimSpace(pictureURL)

	if len(pictureURL) > maxProfilePictureURLLength {
		return nil, ErrProfilePictureURLTooLong
	}

	if pictureURL == "" {
		return nil, ErrInvalidProfilePictureURL
	}

	parsedURL, err := url.Parse(pictureURL)
	if err != nil {
		return nil, ErrInvalidProfilePictureURL
	}

	if parsedURL.Scheme != "http" && parsedURL.Scheme != "https" {
		return nil, ErrInvalidProfilePictureURL
	}

	return &ProfilePicture{
		url:        pictureURL,
		uploadedAt: uploadedAt,
	}, nil
}

func (p *ProfilePicture) UploadedAt() time.Time {
	return p.uploadedAt
}

func (p *ProfilePicture) URL() string {
	return p.url
}

type (
	ActivityType  string
	ActivityLevel string

	ActivityInterest struct {
		acitivityType ActivityType
		level         ActivityLevel
	}
)

const (
	ActivityTypeRunning       ActivityType = "running"
	ActivityTypeWalking       ActivityType = "walking"
	ActivityTypeJogging       ActivityType = "jogging"
	ActivityTypeBasketball    ActivityType = "basketball"
	ActivityTypeFootball      ActivityType = "football"
	ActivityTypeTennis        ActivityType = "tennis"
	ActivityTypeGym           ActivityType = "gym"
	ActivityTypeDancing       ActivityType = "dancing"
	ActivityTypeSkiing        ActivityType = "skiing"
	ActivityTypeClimbing      ActivityType = "climbing"
	ActivityTypeCycling       ActivityType = "cycling"
	ActivityTypeSwimming      ActivityType = "swimming"
	ActivityTypeHiking        ActivityType = "hiking"
	ActivityTypeYoga          ActivityType = "yoga"
	ActivityTypeWeightlifting ActivityType = "weightlifting"
)

var validActivityTypes = map[ActivityType]struct{}{
	ActivityTypeRunning:       {},
	ActivityTypeWalking:       {},
	ActivityTypeJogging:       {},
	ActivityTypeBasketball:    {},
	ActivityTypeFootball:      {},
	ActivityTypeTennis:        {},
	ActivityTypeGym:           {},
	ActivityTypeDancing:       {},
	ActivityTypeSkiing:        {},
	ActivityTypeClimbing:      {},
	ActivityTypeCycling:       {},
	ActivityTypeSwimming:      {},
	ActivityTypeHiking:        {},
	ActivityTypeYoga:          {},
	ActivityTypeWeightlifting: {},
}

const (
	ActivityLevelBeginner     ActivityLevel = "beginner"
	ActivityLevelIntermediate ActivityLevel = "intermediate"
	ActivityLevelAdvanced     ActivityLevel = "advanced"
)

var validLevels = map[ActivityLevel]struct{}{
	ActivityLevelBeginner:     {},
	ActivityLevelIntermediate: {},
	ActivityLevelAdvanced:     {},
}

func NewActivityInterest(activityType ActivityType, level ActivityLevel) (*ActivityInterest, error) {
	if _, ok := validActivityTypes[activityType]; !ok {
		return nil, ErrInvalidActivityType
	}

	if _, ok := validLevels[level]; !ok {
		return nil, ErrInvalidActivityLevel
	}

	return &ActivityInterest{
		acitivityType: activityType,
		level:         level,
	}, nil
}

func (a *ActivityInterest) ActivityType() ActivityType {
	return a.acitivityType
}

func (a *ActivityInterest) Level() ActivityLevel {
	return a.level
}

type (
	SettingType string

	Setting struct {
		key       string
		value     string
		valueType SettingType
	}
)

const (
	SettingTypeString SettingType = "string"
	SettingTypeBool   SettingType = "bool"
	SettingTypeInt    SettingType = "number"
	SettingTypeJSON   SettingType = "json"

	// Notification settings
	SettingNotificationEmailEnabled = "notification.email.enabled"
	SettingNotificationSMSenabled   = "notification.sms.enabled"
	SettingNotificationPushEnabled  = "notification.push.enabled"

	// Privacy settings
	SettingPrivacyProfilePublic   = "privacy.profile.public"
	SeetingPrivacyShowLocation    = "privacy.show_location"
	SettingPrivacyShowActivities  = "privacy.show_activities"
	SettingsPrivacyShowStatistics = "privacy.show_statistics"

	// Prefrerence settings
	SettingLanguage = "preference.language"
	SettingTheme    = "preference.theme"
	SettingTimezone = "preference.timezone"

	// Activity preferences
	SettingActivitySearchRadius = "activity.search_radius"
	SettingActivityAutoJoin     = "activity.auto_join"
)

var validSettingTypes = map[SettingType]struct{}{
	SettingTypeString: {},
	SettingTypeBool:   {},
	SettingTypeInt:    {},
	SettingTypeJSON:   {},
}

func NewSetting(
	key string,
	value string,
	settingType SettingType,
) (*Setting, error) {
	key = strings.TrimSpace(key)
	value = strings.TrimSpace(value)

	if key == "" {
		return nil, ErrInvalidSettingsKey
	}

	if _, ok := validSettingTypes[settingType]; !ok {
		return nil, ErrInvalidSettingsValue
	}

	if !isValidSettingKey(key) {
		return nil, ErrInvalidSettingsKey
	}

	if err := validateSettingValue(value, settingType); err != nil {
		return nil, err
	}

	return &Setting{
		key:   key,
		value: value,
	}, nil
}

func (s *Setting) Key() string {
	return s.key
}

func (s *Setting) Value() string {
	return s.value
}

func (s *Setting) Type() SettingType {
	return s.valueType
}

func (s *Setting) BoolValue() (bool, error) {
	if s.valueType != SettingTypeBool {
		return false, ErrInvalidSettingsValue
	}

	return strconv.ParseBool(s.value)
}

func (s *Setting) StringValue() (string, error) {
	if s.valueType != SettingTypeString {
		return "", ErrInvalidSettingsValue
	}

	return s.value, nil
}

func (s *Setting) JSONValue(v any) error {
	if s.valueType != SettingTypeJSON {
		return ErrInvalidSettingsValue
	}

	return json.Unmarshal([]byte(s.value), v)
}

func (s *Setting) NumberValue() (float64, error) {
	if s.valueType != SettingTypeInt {
		return 0, ErrInvalidSettingsValue
	}

	return strconv.ParseFloat(s.value, 64)
}

func validateSettingValue(value string, settingType SettingType) error {
	switch settingType {
	case SettingTypeBool:
		if value != "true" && value != "false" {
			return fmt.Errorf("%w: boolean value must be 'true' or 'false'", ErrInvalidSettingsValue)
		}
	case SettingTypeInt:
		if _, err := strconv.ParseFloat(value, 64); err != nil {
			return fmt.Errorf("%w: invalid number format", ErrInvalidSettingsValue)
		}
	case SettingTypeString:

	case SettingTypeJSON:
		var js json.RawMessage
		if err := json.Unmarshal([]byte(value), &js); err != nil {
			return fmt.Errorf("%w: invalid JSON format", ErrInvalidSettingsValue)
		}
	default:
		return ErrInvalidSettingsValue
	}
	return nil
}

func isValidSettingKey(key string) bool {
	if key == "" {
		return false
	}

	for _, r := range key {
		if !(r >= 'a' && r <= 'z') && !(r >= '0' && r <= '9') && r != '.' && r != '_' {
			return false
		}
	}

	if strings.HasPrefix(key, ".") || strings.HasSuffix(key, ".") {
		return false
	}

	if strings.Contains(key, "..") {
		return false
	}

	return true
}
