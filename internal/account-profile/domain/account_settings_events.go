package domain

import (
	"time"

	"github.com/google/uuid"
	"github.com/rasparac/rekreativko-api/internal/shared/domainevent"
)

const (
	EventAccountSettingUpdated      = "account_profile.account_settings_updated"
	EventAccountSettingsBulkUpdated = "account_profile.account_settings_bulk_updated"
	EventAccountSettingsReset       = "account_profile.account_settings_reset"
)

type accountSettingsUpdated struct {
	domainevent.BaseEvent
	Key       string      `json:"key"`
	Value     string      `json:"value"`
	Type      SettingType `json:"type"`
	AccountID uuid.UUID   `json:"account_id"`
}

func NewAccountSettingUpdatedEvent(
	accountID uuid.UUID,
	key string,
	value string,
) *accountSettingsUpdated {
	return &accountSettingsUpdated{
		BaseEvent: domainevent.BaseEvent{
			EventID:     uuid.New(),
			EventType:   EventAccountSettingUpdated,
			OccurredAt:  time.Now().UTC(),
			AggregateID: accountID,
		},
		Key:       key,
		Value:     value,
		AccountID: accountID,
	}
}

type accountSettingsBulkUpdated struct {
	domainevent.BaseEvent
	AccountID    uuid.UUID `json:"account_id"`
	Keys         []string  `json:"keys"`
	UpdatedCount int       `json:"updated_count"`
}

func NewAccountSettingsBulkUpdatedEvent(
	accountID uuid.UUID,
	settings map[string]*Setting,
) *accountSettingsBulkUpdated {
	var keys []string
	for key := range settings {
		keys = append(keys, key)
	}

	return &accountSettingsBulkUpdated{
		BaseEvent: domainevent.BaseEvent{
			EventID:     uuid.New(),
			EventType:   EventAccountSettingsBulkUpdated,
			OccurredAt:  time.Now().UTC(),
			AggregateID: accountID,
		},
		Keys:         keys,
		UpdatedCount: len(settings),
		AccountID:    accountID,
	}
}

type accountSettingsResetEvent struct {
	domainevent.BaseEvent
	AccountID uuid.UUID `json:"account_id"`
}

func NewAccountSettingsResetEvent(accountID uuid.UUID) *accountSettingsResetEvent {
	return &accountSettingsResetEvent{
		BaseEvent: domainevent.BaseEvent{
			EventID:     uuid.New(),
			EventType:   EventAccountSettingsReset,
			OccurredAt:  time.Now().UTC(),
			AggregateID: accountID,
		},
		AccountID: accountID,
	}
}
