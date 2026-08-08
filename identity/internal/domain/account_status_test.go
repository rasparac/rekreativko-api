package domain

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_IsValid(t *testing.T) {
	tests := []struct {
		name string
		as   AccountStatus
		want bool
	}{
		{"valid active", AccountStatusActive, true},
		{"valid pending", AccountStatusPending, true},
		{"valid suspended", AccountStatusSuspended, true},
		{"valid deleted", AccountStatusDeleted, true},
		{"invalid status", AccountStatus("unknown"), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.as.IsValid()

			assert.Equal(t, tt.want, got)
		})
	}
}

func Test_CanLogin(t *testing.T) {
	tests := []struct {
		name string
		as   AccountStatus
		want bool
	}{
		{"can login active", AccountStatusActive, true},
		{"cannot login pending", AccountStatusPending, false},
		{"cannot login suspended", AccountStatusSuspended, false},
		{"cannot login deleted", AccountStatusDeleted, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.as.CanLogin()

			assert.Equal(t, tt.want, got)
		})
	}
}

func Test_CanBeActivated(t *testing.T) {
	tests := []struct {
		name string
		as   AccountStatus
		want bool
	}{
		{"can be activated pending", AccountStatusPending, true},
		{"cannot be activated active", AccountStatusActive, false},
		{"cannot be activated suspended", AccountStatusSuspended, false},
		{"cannot be activated deleted", AccountStatusDeleted, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.as.CanBeActivated()

			assert.Equal(t, tt.want, got)
		})
	}
}

func Test_CanBeSuspended(t *testing.T) {
	tests := []struct {
		name string
		as   AccountStatus
		want bool
	}{
		{"can be suspended active", AccountStatusActive, true},
		{"cannot be suspended pending", AccountStatusPending, false},
		{"cannot be suspended suspended", AccountStatusSuspended, false},
		{"cannot be suspended deleted", AccountStatusDeleted, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.as.CanBeSuspended()

			assert.Equal(t, tt.want, got)
		})
	}
}

func Test_CanBeDeleted(t *testing.T) {
	tests := []struct {
		name string
		as   AccountStatus
		want bool
	}{
		{"can be deleted active", AccountStatusActive, true},
		{"can be deleted suspended", AccountStatusSuspended, true},
		{"cannot be deleted pending", AccountStatusPending, false},
		{"cannot be deleted deleted", AccountStatusDeleted, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.as.CanBeDeleted()

			assert.Equal(t, tt.want, got)
		})
	}
}
