package domain

import (
	"time"

	"github.com/google/uuid"
	"github.com/rasparac/rekreativko-api/internal/shared/domainevent"
)

const (
	EventUserProfileCreated           = "user_profile.created"
	EventUserProfileDeleted           = "user_profile.deleted"
	EventUserProfileAnonymized        = "user_profile.anonymized"
	EventUserProfilePictureChanged    = "user_profile.profile_picture.changed"
	EventUserProfileLocationChanged   = "user_profile.location.changed"
	EventNicknameChanged              = "user_profile.nickname.changed"
	EventProfilePictureChanged        = "user_profile.profile_picture.changed"
	EventActivityInterestAdded        = "user_profile.activity_interest.added"
	EventActivityInterestRemoved      = "user_profile.activity_interest.removed"
	EventActivityInterestLevelChanged = "user_profile.activity_interest.level.changed"
)

type UserProfileCreatedEvent struct {
	domainevent.BaseEvent
	AccountID uuid.UUID `json:"account_id"`
}

func NewUserProfileCreatedEvent(userProfile *UserProfile) *UserProfileCreatedEvent {
	return &UserProfileCreatedEvent{
		BaseEvent: domainevent.BaseEvent{
			EventID:     uuid.New(),
			EventType:   EventUserProfileCreated,
			OccurredAt:  time.Now().UTC(),
			AggregateID: userProfile.ID(),
		},
		AccountID: userProfile.ID(),
	}
}

type UserProfileLocationChangedEvent struct {
	domainevent.BaseEvent
	AccountID      uuid.UUID `json:"account_id"`
	City           string    `json:"city"`
	Country        string    `json:"country"`
	HasCoordinates bool      `json:"has_coordinates"`
	Latitude       float64   `json:"latitude"`
	Longitude      float64   `json:"longitude"`
}

func NewUserProfileLocationChangedEvent(userProfile *UserProfile) *UserProfileLocationChangedEvent {
	event := &UserProfileLocationChangedEvent{
		BaseEvent: domainevent.BaseEvent{
			EventID:     uuid.New(),
			EventType:   EventUserProfileLocationChanged,
			OccurredAt:  time.Now().UTC(),
			AggregateID: userProfile.ID(),
		},
		AccountID:      userProfile.ID(),
		City:           userProfile.location.City(),
		Country:        userProfile.location.Country(),
		HasCoordinates: userProfile.location.HasCoordinates(),
	}

	if userProfile.location.HasCoordinates() {
		event.Latitude = userProfile.location.Coordinates().Latitude()
		event.Longitude = userProfile.location.Coordinates().Longitude()
	}

	return event
}

type UserProfileDeletedEvent struct {
	domainevent.BaseEvent
	AccountID uuid.UUID `json:"account_id"`
}

func NewUserProfileDeletedEvent(userProfile *UserProfile) *UserProfileDeletedEvent {
	return &UserProfileDeletedEvent{
		BaseEvent: domainevent.BaseEvent{
			EventID:     uuid.New(),
			EventType:   EventUserProfileDeleted,
			OccurredAt:  time.Now().UTC(),
			AggregateID: userProfile.ID(),
		},
		AccountID: userProfile.ID(),
	}
}

type UserProfileAnonymizedEvent struct {
	domainevent.BaseEvent
	AccountID uuid.UUID `json:"account_id"`
}

func NewUserProfileAnonymizedEvent(userProfile *UserProfile) *UserProfileAnonymizedEvent {
	return &UserProfileAnonymizedEvent{
		BaseEvent: domainevent.BaseEvent{
			EventID:     uuid.New(),
			EventType:   EventUserProfileAnonymized,
			OccurredAt:  time.Now().UTC(),
			AggregateID: userProfile.ID(),
		},
		AccountID: userProfile.ID(),
	}
}

type UserProfileNicknameChangedEvent struct {
	domainevent.BaseEvent
	AccountID   uuid.UUID `json:"account_id"`
	NewNickname string    `json:"nickname"`
	OldNickName string    `json:"old_nickname"`
}

func NewUserProfileNicknameChangedEvent(
	profileID uuid.UUID,
	newNickname string,
	oldNickname string,
) *UserProfileNicknameChangedEvent {
	return &UserProfileNicknameChangedEvent{
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

func NewUserProfilePictureChangedEvent(
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
			EventID:    uuid.New(),
			EventType:  EventActivityInterestAdded,
			OccurredAt: time.Now().UTC(),
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
			EventID:    uuid.New(),
			EventType:  EventActivityInterestRemoved,
			OccurredAt: time.Now().UTC(),
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
			EventID:    uuid.New(),
			EventType:  EventActivityInterestLevelChanged,
			OccurredAt: time.Now().UTC(),
		},
		AccountID:    profileID,
		ActivityType: string(activityType),
		OldLevel:     string(oldLevel),
		NewLevel:     string(newLevel),
	}
}
