package domain

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_NewEmail(t *testing.T) {
	testCase := []struct {
		name    string
		email   string
		wantErr bool
	}{
		{
			name:  "valid email",
			email: "user@example.com",
		},
		{
			name:    "invalid email format",
			email:   "invalid-email",
			wantErr: true,
		},
		{
			name:  "empty email",
			email: "",
		},
		{
			name:    "invalid email no domain",
			email:   "user@.com",
			wantErr: true,
		},
	}

	for _, tc := range testCase {
		t.Run(tc.name, func(t *testing.T) {
			email, err := NewEmail(tc.email)
			if tc.wantErr {
				assert.Error(t, err)
				return
			}

			assert.NoError(t, err)
			assert.Equal(t, tc.email, email.String())
		})
	}
}

func Test_NewPhoneNumber(t *testing.T) {
	testCase := []struct {
		name        string
		phoneNumber string
		wantErr     bool
	}{
		{
			name:        "valid phone number",
			phoneNumber: "+1234567890",
		},
		{
			name:        "invalid phone number format",
			phoneNumber: "123-abc-7890",
			wantErr:     true,
		},
		{
			name:        "empty phone number",
			phoneNumber: "",
		},
		{
			name:        "invalid phone number special chars",
			phoneNumber: "+12(345)67890",
			wantErr:     true,
		},
	}

	for _, tc := range testCase {
		t.Run(tc.name, func(t *testing.T) {
			phone, err := NewPhoneNumber(tc.phoneNumber)
			if tc.wantErr {
				assert.Error(t, err)
				return
			}

			assert.NoError(t, err)
			assert.Equal(t, tc.phoneNumber, phone.String())
		})
	}
}

func Test_ValidatePassword(t *testing.T) {
	testCase := []struct {
		name     string
		password string
		wantErr  bool
	}{
		{
			name:     "valid password",
			password: "StrongP@ssw0rd!",
		},
		{
			name:     "too short password",
			password: "Short1!",
			wantErr:  true,
		},
		{
			name:     "missing uppercase letter",
			password: "weakp@ssw0rd!",
			wantErr:  true,
		},
		{
			name:     "missing lowercase letter",
			password: "WEAKP@SSW0RD!",
			wantErr:  true,
		},
		{
			name:     "missing digit",
			password: "NoDigitPass!",
			wantErr:  true,
		},
		{
			name:     "missing special character",
			password: "NoSpecialChar1",
			wantErr:  true,
		},
	}

	for _, tc := range testCase {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidatePassword(tc.password)
			if tc.wantErr {
				assert.Error(t, err)
				return
			}

			assert.NoError(t, err)
		})
	}
}
