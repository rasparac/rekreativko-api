package domain

import (
	"time"

	"github.com/google/uuid"
	"github.com/rasparac/rekreativko-api/internal/shared/domainevent"
)

const (
	bioMaxLength = 500
)

type AccountProfile struct {
	id               uuid.UUID
	bio              string
	nickname         *Nickname
	fullName         *FullName
	profilePicture   *ProfilePicture
	dateOfBirth      *DateOfBirth
	location         *Location
	activityInterest []*ActivityInterest
	createdAt        time.Time
	updatedAt        time.Time
	deletedAt        *time.Time

	events []domainevent.Event
}

func NewAccountProfile(
	id uuid.UUID,
) *AccountProfile {
	now := time.Now()

	profile := &AccountProfile{
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

	profile.addEvent(NewAccountProfileCreatedEvent(profile))

	return profile
}

func ReconstructAccountProfile(
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
	deletedAt *time.Time,
) *AccountProfile {
	return &AccountProfile{
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
		deletedAt:        deletedAt,
		events:           make([]domainevent.Event, 0),
	}
}

func (ap *AccountProfile) ID() uuid.UUID {
	return ap.id
}

func (ap *AccountProfile) CreatedAt() time.Time {
	return ap.createdAt
}

func (ap *AccountProfile) UpdatedAt() time.Time {
	return ap.updatedAt
}

func (ap *AccountProfile) DeletedAt() *time.Time {
	return ap.deletedAt
}

func (ap *AccountProfile) Nickname() *Nickname {
	return ap.nickname
}

func (ap *AccountProfile) FullName() *FullName {
	return ap.fullName
}

func (ap *AccountProfile) ProfilePicture() *ProfilePicture {
	return ap.profilePicture
}

func (ap *AccountProfile) DateOfBirth() *DateOfBirth {
	return ap.dateOfBirth
}

func (ap *AccountProfile) Bio() string {
	return ap.bio
}

func (ap *AccountProfile) Location() *Location {
	return ap.location
}

func (ap *AccountProfile) ActivityInterests() []*ActivityInterest {
	return ap.activityInterest
}

func (ap *AccountProfile) IsDeleted() bool {
	return ap.deletedAt != nil
}

func (ap *AccountProfile) UpdateProfile(
	fullName *FullName,
	dateOfBirth *DateOfBirth,
	bio string,
) error {
	if ap.IsDeleted() {
		return ErrAccountProfileDeleted
	}

	if len(bio) > bioMaxLength {
		return ErrAccountProfileBioTooLong
	}

	var changed bool
	if fullName != nil && (ap.fullName == nil || ap.fullName.Value() != fullName.Value()) {
		ap.fullName = fullName
		changed = true
	}

	if dateOfBirth != nil && (ap.dateOfBirth == nil || !ap.dateOfBirth.Value().Equal(dateOfBirth.Value())) {
		ap.dateOfBirth = dateOfBirth
		changed = true
	}

	if bio != ap.bio {
		ap.bio = bio
		changed = true
	}

	if changed {
		ap.updatedAt = time.Now()
	}

	return nil
}

func (ap *AccountProfile) SetNickname(nickname *Nickname) error {
	if ap.IsDeleted() {
		return ErrAccountProfileDeleted
	}

	if nickname == nil {
		return ErrInvalidNickname
	}

	if ap.nickname == nil || ap.nickname.Value() != nickname.Value() {
		ap.nickname = nickname
		ap.updatedAt = time.Now()
	}

	return nil
}

func (ap *AccountProfile) SetLocation(location *Location) error {
	if ap.IsDeleted() {
		return ErrAccountProfileDeleted
	}

	if location == nil {
		if ap.location != nil {
			ap.location = nil
			ap.updatedAt = time.Now()
			ap.addEvent(NewAccountProfileLocationChangedEvent(ap))
		}
		return nil
	}

	locationChanged := ap.location == nil ||
		ap.location.City() != location.City() ||
		ap.location.Country() != location.Country() ||
		(ap.location.HasCoordinates() != location.HasCoordinates())

	if locationChanged {
		ap.location = location
		ap.updatedAt = time.Now()

		ap.addEvent(NewAccountProfileLocationChangedEvent(ap))
	}

	return nil
}

func (ap *AccountProfile) SetProfilePicture(profilePicture *ProfilePicture) error {
	if ap.IsDeleted() {
		return ErrAccountProfileDeleted
	}

	var oldURL string
	if ap.profilePicture != nil {
		oldURL = ap.profilePicture.URL()
	}

	if profilePicture == nil {
		if ap.profilePicture != nil {
			ap.profilePicture = nil
			ap.updatedAt = time.Now().UTC()
			ap.addEvent(NewAccountProfilePictureChangedEvent(ap.ID(), "", oldURL))
		}
		return nil
	}

	if ap.profilePicture != nil && oldURL == profilePicture.URL() {
		return nil
	}

	ap.profilePicture = profilePicture
	ap.updatedAt = time.Now().UTC()
	ap.addEvent(NewAccountProfilePictureChangedEvent(ap.ID(), profilePicture.URL(), oldURL))

	return nil
}

func (ap *AccountProfile) AddactivityInterest(interest *ActivityInterest) error {
	if ap.IsDeleted() {
		return ErrAccountProfileDeleted
	}

	for _, existingInterest := range ap.activityInterest {
		if existingInterest.ActivityType() == interest.ActivityType() {
			return ErrDuplicateInterests
		}
	}

	ap.activityInterest = append(ap.activityInterest, interest)
	ap.updatedAt = time.Now()
	ap.addEvent(NewActivityInterestAddedEvent(ap.ID(), interest))

	return nil
}

func (ap *AccountProfile) RemoveActivityInterest(activityType ActivityType) error {
	if ap.IsDeleted() {
		return ErrAccountProfileDeleted
	}

	for i, existingInterest := range ap.activityInterest {
		if existingInterest.ActivityType() == activityType {
			ap.activityInterest = append(ap.activityInterest[:i], ap.activityInterest[i+1:]...)
			ap.updatedAt = time.Now()
			ap.addEvent(NewActivityInterestRemovedEvent(ap.ID(), existingInterest))
			return nil
		}
	}

	return ErrActivityInterestsNotFound
}

func (ap *AccountProfile) UpdateAcitivityInterestLevel(activityType ActivityType, level ActivityLevel) error {
	if ap.IsDeleted() {
		return ErrAccountProfileDeleted
	}

	for i, existingInterest := range ap.activityInterest {
		if existingInterest.ActivityType() == activityType {
			if existingInterest.Level() == level {
				return nil
			}

			ap.activityInterest[i].level = level
			ap.updatedAt = time.Now()

			ap.addEvent(NewActivityInterestLevelChangedEvent(
				ap.ID(),
				activityType,
				existingInterest.Level(),
				level,
			))
			return nil
		}
	}

	return ErrActivityInterestsNotFound
}

func (ap *AccountProfile) UpdateActivityInterests(
	newInterests []*ActivityInterest,
) error {
	if ap.IsDeleted() {
		return ErrAccountProfileDeleted
	}

	existingByType := make(map[ActivityType]*ActivityInterest)
	for _, interest := range ap.activityInterest {
		existingByType[interest.ActivityType()] = interest
	}

	newByType := make(map[ActivityType]*ActivityInterest)
	for _, interest := range newInterests {
		newByType[interest.ActivityType()] = interest
	}

	for activityType := range existingByType {
		if _, exists := newByType[activityType]; !exists {
			if err := ap.RemoveActivityInterest(activityType); err != nil {
				return err
			}
		}
	}

	for activityType, newInterest := range newByType {
		existing, exists := existingByType[activityType]
		if !exists {
			if err := ap.AddactivityInterest(newInterest); err != nil {
				return err
			}
			continue
		}

		if existing.Level() != newInterest.Level() {
			if err := ap.UpdateAcitivityInterestLevel(
				activityType,
				newInterest.Level(),
			); err != nil {
				return err
			}
		}
	}

	return nil
}

func (ap *AccountProfile) HasActivityInterest(activityType ActivityType) bool {
	for _, existingInterest := range ap.activityInterest {
		if existingInterest.ActivityType() == activityType {
			return true
		}
	}

	return false
}

func (ap *AccountProfile) GetActivityInterest(activityType ActivityType) (*ActivityInterest, error) {
	for _, existingInterest := range ap.activityInterest {
		if existingInterest.ActivityType() == activityType {
			return existingInterest, nil
		}
	}

	return nil, ErrActivityInterestsNotFound
}

func (ap *AccountProfile) Delete() error {
	if ap.IsDeleted() {
		return ErrAccountProfileDeleted
	}

	now := time.Now()
	ap.deletedAt = &now
	ap.updatedAt = now

	ap.addEvent(NewAccountProfileDeletedEvent(ap))

	return nil
}

func (ap *AccountProfile) Anonymize() error {
	if !ap.IsDeleted() {
		return ErrAccountProfileDeleted
	}

	anymizedNickname, err := NewNickname("deleted_account_" + ap.id.String()[:8])
	if err != nil {
		return err
	}

	ap.nickname = anymizedNickname
	ap.fullName = nil
	ap.profilePicture = nil
	ap.dateOfBirth = nil
	ap.bio = ""
	ap.location = nil
	ap.activityInterest = make([]*ActivityInterest, 0)
	ap.updatedAt = time.Now()

	ap.addEvent(NewAccountProfileAnonymizedEvent(ap))

	return nil
}

func (ap *AccountProfile) ClearEvents() {
	ap.events = make([]domainevent.Event, 0)
}

func (ap *AccountProfile) Events() []domainevent.Event {
	return ap.events
}

func (ap *AccountProfile) addEvent(event domainevent.Event) {
	ap.events = append(ap.events, event)
}
