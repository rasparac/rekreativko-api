package domain

import (
	"time"

	"github.com/google/uuid"
	"github.com/rasparac/rekreativko-api/internal/shared/domainevent"
)

const (
	bioMaxLength = 500
)

type UserProfile struct {
	id               uuid.UUID
	bio              string
	nickname         *Nickname
	fullName         *FullName
	profilePicture   *ProfilePicture
	dateOfBirth      *DateOfBirth
	location         *Location
	activityInterest []*ActivityInterest
	settings         map[string]*Setting
	createdAt        time.Time
	updatedAt        time.Time
	deleteAt         *time.Time

	events []domainevent.Event
}

func NewUserProfile(
	id uuid.UUID,
) *UserProfile {
	now := time.Now()

	profile := &UserProfile{
		id:               id,
		createdAt:        now,
		updatedAt:        now,
		activityInterest: make([]*ActivityInterest, 0),
		events:           make([]domainevent.Event, 0),
		nickname:         &Nickname{},
		location:         &Location{},
		profilePicture:   &ProfilePicture{},
		fullName:         &FullName{},
		dateOfBirth:      &DateOfBirth{},
	}

	profile.addEvent(NewUserProfileCreatedEvent(profile))

	return profile
}

func ReconstructUserProfile(
	id uuid.UUID,
	nickname *Nickname,
	fullName *FullName,
	profilePicture *ProfilePicture,
	dateOfBirth *DateOfBirth,
	bio string,
	location *Location,
	activityInterest []*ActivityInterest,
	createdAt time.Time,
	updatedAt time.Time,
	deleteAt *time.Time,
) *UserProfile {
	return &UserProfile{
		id:               id,
		nickname:         nickname,
		fullName:         fullName,
		profilePicture:   profilePicture,
		dateOfBirth:      dateOfBirth,
		bio:              bio,
		location:         location,
		activityInterest: activityInterest,
		createdAt:        createdAt,
		updatedAt:        updatedAt,
		deleteAt:         deleteAt,
		events:           make([]domainevent.Event, 0),
	}
}

func (up *UserProfile) ID() uuid.UUID {
	return up.id
}

func (up *UserProfile) CreatedAt() time.Time {
	return up.createdAt
}

func (up *UserProfile) UpdatedAt() time.Time {
	return up.updatedAt
}

func (up *UserProfile) DeleteAt() *time.Time {
	return up.deleteAt
}

func (up *UserProfile) Nickname() *Nickname {
	return up.nickname
}

func (up *UserProfile) FullName() *FullName {
	return up.fullName
}

func (up *UserProfile) ProfilePicture() *ProfilePicture {
	return up.profilePicture
}

func (up *UserProfile) DateOfBirth() *DateOfBirth {
	return up.dateOfBirth
}

func (up *UserProfile) Bio() string {
	return up.bio
}

func (up *UserProfile) Location() *Location {
	return up.location
}

func (up *UserProfile) ActivityInterests() []*ActivityInterest {
	return up.activityInterest
}

func (up *UserProfile) IsDeleted() bool {
	return up.deleteAt != nil
}

func (up *UserProfile) UpdateProfile(
	fullName *FullName,
	dateOfBirth *DateOfBirth,
	bio string,
) error {
	if up.IsDeleted() {
		return ErrUserProfileDeleted
	}

	if len(bio) > bioMaxLength {
		return ErrUserProfileBioTooLong
	}

	var changed bool
	if fullName != nil && (up.fullName == nil || up.fullName.Value() != fullName.Value()) {
		up.fullName = fullName
		changed = true
	}

	if dateOfBirth != nil && (up.dateOfBirth == nil || !up.dateOfBirth.Value().Equal(dateOfBirth.Value())) {
		up.dateOfBirth = dateOfBirth
		changed = true
	}

	if bio != up.bio {
		up.bio = bio
		changed = true
	}

	if changed {
		up.updatedAt = time.Now()
	}

	return nil
}

func (up *UserProfile) SetNickname(nickname *Nickname) error {
	if up.IsDeleted() {
		return ErrUserProfileDeleted
	}

	if nickname == nil {
		return ErrInvalidNickname
	}

	if up.nickname == nil || up.nickname.Value() != nickname.Value() {
		up.nickname = nickname
		up.updatedAt = time.Now()
	}

	return nil
}

func (up *UserProfile) SetLocation(location *Location) error {
	if up.IsDeleted() {
		return ErrUserProfileDeleted
	}

	if location == nil {
		return ErrLocationRequired
	}

	locationChanged := up.location == nil ||
		up.location.City() != location.City() ||
		up.location.Country() != location.Country() ||
		(up.location.HasCoordinates() != location.HasCoordinates())

	if locationChanged {
		up.location = location
		up.updatedAt = time.Now()

		up.addEvent(NewUserProfileLocationChangedEvent(up))
	}

	return nil
}

func (up *UserProfile) SetProfilePicture(profilePicture *ProfilePicture) error {
	if up.IsDeleted() {
		return ErrUserProfileDeleted
	}

	var oldURL string
	if up.profilePicture != nil {
		oldURL = up.profilePicture.URL()
	}

	if profilePicture == nil {
		if up.profilePicture != nil {
			up.profilePicture = nil
			up.updatedAt = time.Now().UTC()
			up.addEvent(NewUserProfilePictureChangedEvent(up.ID(), "", oldURL))
		}
		return nil
	}

	if up.profilePicture != nil && oldURL == profilePicture.URL() {
		return nil
	}

	up.profilePicture = profilePicture
	up.updatedAt = time.Now().UTC()
	up.addEvent(NewUserProfilePictureChangedEvent(up.ID(), profilePicture.URL(), oldURL))

	return nil
}

func (up *UserProfile) AddactivityInterest(interest *ActivityInterest) error {
	if up.IsDeleted() {
		return ErrUserProfileDeleted
	}

	for _, existingInterest := range up.activityInterest {
		if existingInterest.ActivityType() == interest.ActivityType() {
			return ErrDuplicateInterests
		}
	}

	up.activityInterest = append(up.activityInterest, interest)
	up.updatedAt = time.Now()
	up.addEvent(NewActivityInterestAddedEvent(up.ID(), interest))

	return nil
}

func (up *UserProfile) RemoveActivityInterest(activityType ActivityType) error {
	if up.IsDeleted() {
		return ErrUserProfileDeleted
	}

	for i, existingInterest := range up.activityInterest {
		if existingInterest.ActivityType() == activityType {
			up.activityInterest = append(up.activityInterest[:i], up.activityInterest[i+1:]...)
			up.updatedAt = time.Now()
			up.addEvent(NewActivityInterestRemovedEvent(up.ID(), existingInterest))
			return nil
		}
	}

	return ErrActivityInterestsNotFound
}

func (up *UserProfile) UpdateAcitivityInterestLevel(activityType ActivityType, level ActivityLevel) error {
	if up.IsDeleted() {
		return ErrUserProfileDeleted
	}

	for i, existingInterest := range up.activityInterest {
		if existingInterest.ActivityType() == activityType {
			if existingInterest.Level() == level {
				return nil
			}

			up.activityInterest[i].level = level
			up.updatedAt = time.Now()

			up.addEvent(NewActivityInterestLevelChangedEvent(
				up.ID(),
				activityType,
				existingInterest.Level(),
				level,
			))
			return nil
		}
	}

	return ErrActivityInterestsNotFound
}

func (up *UserProfile) HasActivityInterest(activityType ActivityType) bool {
	for _, existingInterest := range up.activityInterest {
		if existingInterest.ActivityType() == activityType {
			return true
		}
	}

	return false
}

func (up *UserProfile) GetActivityInterest(activityType ActivityType) (*ActivityInterest, error) {
	for _, existingInterest := range up.activityInterest {
		if existingInterest.ActivityType() == activityType {
			return existingInterest, nil
		}
	}

	return nil, ErrActivityInterestsNotFound
}

func (up *UserProfile) Delete() error {
	if up.IsDeleted() {
		return ErrUserProfileDeleted
	}

	now := time.Now()
	up.deleteAt = &now
	up.updatedAt = now

	up.addEvent(NewUserProfileDeletedEvent(up))

	return nil
}

func (up *UserProfile) Anonymize() error {
	if !up.IsDeleted() {
		return ErrUserProfileDeleted
	}

	anymizedNickname, err := NewNickname("deleted_user_" + up.id.String()[:8])
	if err != nil {
		return err
	}

	up.nickname = anymizedNickname
	up.fullName = nil
	up.profilePicture = nil
	up.dateOfBirth = nil
	up.bio = ""
	up.location = nil
	up.activityInterest = make([]*ActivityInterest, 0)
	up.updatedAt = time.Now()

	up.addEvent(NewUserProfileAnonymizedEvent(up))

	return nil
}

func (up *UserProfile) ClearEvents() {
	up.events = make([]domainevent.Event, 0)
}

func (up *UserProfile) Events() []domainevent.Event {
	return up.events
}

func (up *UserProfile) addEvent(event domainevent.Event) {
	up.events = append(up.events, event)
}
