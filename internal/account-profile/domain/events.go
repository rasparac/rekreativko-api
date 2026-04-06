package domain

import (
	"time"

	"github.com/google/uuid"
	"github.com/rasparac/rekreativko-api/internal/shared/domainevent"
)

const (
	EventAccountProfileCreated         = "account_profile.created"
	EventAccountProfileDeleted         = "account_profile.deleted"
	EventAccountProfileAnonymized      = "account_profile.anonymized"
	EventAccountProfilePictureChanged  = "account_profile.profile_picture.changed"
	EventAccountProfileLocationChanged = "account_profile.location.changed"
	EventNicknameChanged               = "account_profile.nickname.changed"
	EventProfilePictureChanged         = "account_profile.profile_picture.changed"
	EventActivityInterestAdded         = "account_profile.activity_interest.added"
	EventActivityInterestRemoved       = "account_profile.activity_interest.removed"
	EventActivityInterestLevelChanged  = "account_profile.activity_interest.level.changed"
)

type AccountProfileCreatedEvent struct {
	domainevent.BaseEvent
	AccountID uuid.UUID `json:"account_id"`
}

func NewAccountProfileCreatedEvent(AccountProfile *AccountProfile) *AccountProfileCreatedEvent {
	return &AccountProfileCreatedEvent{
		BaseEvent: domainevent.BaseEvent{
			EventID:     uuid.New(),
			EventType:   EventAccountProfileCreated,
			OccurredAt:  time.Now().UTC(),
			AggregateID: AccountProfile.ID(),
		},
		AccountID: AccountProfile.ID(),
	}
}

type AccountProfileLocationChangedEvent struct {
	domainevent.BaseEvent
	AccountID      uuid.UUID `json:"account_id"`
	City           string    `json:"city"`
	Country        string    `json:"country"`
	HasCoordinates bool      `json:"has_coordinates"`
	Latitude       float64   `json:"latitude"`
	Longitude      float64   `json:"longitude"`
}

func NewAccountProfileLocationChangedEvent(AccountProfile *AccountProfile) *AccountProfileLocationChangedEvent {
	event := &AccountProfileLocationChangedEvent{
		BaseEvent: domainevent.BaseEvent{
			EventID:     uuid.New(),
			EventType:   EventAccountProfileLocationChanged,
			OccurredAt:  time.Now().UTC(),
			AggregateID: AccountProfile.ID(),
		},
		AccountID: AccountProfile.ID(),
	}

	if AccountProfile.location == nil {
		event.HasCoordinates = false
		return event
	}

	event.City = AccountProfile.location.City()
	event.Country = AccountProfile.location.Country()
	event.HasCoordinates = AccountProfile.location.HasCoordinates()

	if AccountProfile.location.HasCoordinates() {
		event.Latitude = AccountProfile.location.Coordinates().Latitude()
		event.Longitude = AccountProfile.location.Coordinates().Longitude()
	}

	return event
}

type AccountProfileDeletedEvent struct {
	domainevent.BaseEvent
	AccountID uuid.UUID `json:"account_id"`
}

func NewAccountProfileDeletedEvent(accountProfile *AccountProfile) *AccountProfileDeletedEvent {
	return &AccountProfileDeletedEvent{
		BaseEvent: domainevent.BaseEvent{
			EventID:     uuid.New(),
			EventType:   EventAccountProfileDeleted,
			OccurredAt:  time.Now().UTC(),
			AggregateID: accountProfile.ID(),
		},
		AccountID: accountProfile.ID(),
	}
}

type AccountProfileAnonymizedEvent struct {
	domainevent.BaseEvent
	AccountID uuid.UUID `json:"account_id"`
}

func NewAccountProfileAnonymizedEvent(AccountProfile *AccountProfile) *AccountProfileAnonymizedEvent {
	return &AccountProfileAnonymizedEvent{
		BaseEvent: domainevent.BaseEvent{
			EventID:     uuid.New(),
			EventType:   EventAccountProfileAnonymized,
			OccurredAt:  time.Now().UTC(),
			AggregateID: AccountProfile.ID(),
		},
		AccountID: AccountProfile.ID(),
	}
}

type AccountProfileNicknameChangedEvent struct {
	domainevent.BaseEvent
	AccountID   uuid.UUID `json:"account_id"`
	NewNickname string    `json:"nickname"`
	OldNickName string    `json:"old_nickname"`
}

func NewAccountProfileNicknameChangedEvent(
	profileID uuid.UUID,
	newNickname string,
	oldNickname string,
) *AccountProfileNicknameChangedEvent {
	return &AccountProfileNicknameChangedEvent{
		BaseEvent: domainevent.BaseEvent{
			EventID:     uuid.New(),
			EventType:   EventNicknameChanged,
			OccurredAt:  time.Now().UTC(),
			AggregateID: profileID,
		},
		AccountID:   profileID,
		NewNickname: newNickname,
		OldNickName: oldNickname,
	}
}

type ProfilePictureChangedEvent struct {
	domainevent.BaseEvent
	AccountID            uuid.UUID `json:"account_id"`
	NewProfilePictureURL string    `json:"new_profile_picture_url"`
	OldProfilePictureURL string    `json:"old_profile_picture_url"`
}

func NewAccountProfilePictureChangedEvent(
	profileID uuid.UUID,
	newURL string,
	oldURL string,
) *ProfilePictureChangedEvent {
	return &ProfilePictureChangedEvent{
		BaseEvent: domainevent.BaseEvent{
			EventID:     uuid.New(),
			EventType:   EventProfilePictureChanged,
			OccurredAt:  time.Now().UTC(),
			AggregateID: profileID,
		},

		NewProfilePictureURL: newURL,
		OldProfilePictureURL: oldURL,
	}
}

type ActivityInterestAddedEvent struct {
	domainevent.BaseEvent
	AccountID    uuid.UUID `json:"account_id"`
	ActivityType string    `json:"activity_type"`
	Level        string    `json:"level"`
}

func NewActivityInterestAddedEvent(
	profileID uuid.UUID,
	ai *ActivityInterest,
) *ActivityInterestAddedEvent {
	return &ActivityInterestAddedEvent{
		BaseEvent: domainevent.BaseEvent{
			EventID:     uuid.New(),
			EventType:   EventActivityInterestAdded,
			OccurredAt:  time.Now().UTC(),
			AggregateID: profileID,
		},
		AccountID:    profileID,
		ActivityType: string(ai.ActivityType()),
		Level:        string(ai.Level()),
	}
}

type ActivityInterestRemovedEvent struct {
	domainevent.BaseEvent
	AccountID    uuid.UUID `json:"account_id"`
	ActivityType string    `json:"activity_type"`
	Level        string    `json:"level"`
}

func NewActivityInterestRemovedEvent(
	profileID uuid.UUID,
	ai *ActivityInterest,
) *ActivityInterestRemovedEvent {
	return &ActivityInterestRemovedEvent{
		BaseEvent: domainevent.BaseEvent{
			EventID:     uuid.New(),
			EventType:   EventActivityInterestRemoved,
			OccurredAt:  time.Now().UTC(),
			AggregateID: profileID,
		},
		AccountID:    profileID,
		ActivityType: string(ai.ActivityType()),
		Level:        string(ai.Level()),
	}
}

type ActivityInterestLevelChangedEvent struct {
	domainevent.BaseEvent
	AccountID    uuid.UUID `json:"account_id"`
	ActivityType string    `json:"activity_type"`
	OldLevel     string    `json:"old_level"`
	NewLevel     string    `json:"new_level"`
}

func NewActivityInterestLevelChangedEvent(
	profileID uuid.UUID,
	activityType ActivityType,
	oldLevel ActivityLevel,
	newLevel ActivityLevel,
) *ActivityInterestLevelChangedEvent {
	return &ActivityInterestLevelChangedEvent{
		BaseEvent: domainevent.BaseEvent{
			EventID:     uuid.New(),
			EventType:   EventActivityInterestLevelChanged,
			OccurredAt:  time.Now().UTC(),
			AggregateID: profileID,
		},
		AccountID:    profileID,
		ActivityType: string(activityType),
		OldLevel:     string(oldLevel),
		NewLevel:     string(newLevel),
	}
}
