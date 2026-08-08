package domain

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func Test_NewVerificationCode(t *testing.T) {
	var (
		code      = "123456"
		expiresAt = time.Now().Add(15 * time.Minute)
	)

	account, err := newMockAccount("email@net.hr", "")
	assert.NoError(t, err)

	vc, err := NewVerificationCode(
		account,
		code,
		CodeTypeEmail,
		expiresAt,
	)
	assert.NoError(t, err)

	assert.Equal(t, account.ID(), vc.AccountID())
	assert.Equal(t, code, vc.Code())
	assert.Equal(t, expiresAt, vc.ExpiresAt())

	assert.True(t, vc.IsValid())
	assert.False(t, vc.IsExpired())

}

func Test_VerificationCode_IsValid(t *testing.T) {
	testCase := []struct {
		name      string
		expiresAt time.Time
		used      bool
		want      bool
	}{
		{
			name:      "valid code",
			expiresAt: time.Now().Add(10 * time.Minute),
			want:      true,
		},
		{
			name:      "expired code",
			expiresAt: time.Now().Add(-10 * time.Minute),
			want:      false,
		},
		{
			name:      "used code",
			expiresAt: time.Now().Add(10 * time.Minute),
			used:      true,
			want:      false,
		},
	}

	for _, tc := range testCase {
		t.Run(tc.name, func(t *testing.T) {

			account, err := newMockAccount("email@net.hr", "")
			assert.NoError(t, err)

			vc, err := NewVerificationCode(
				account,
				"123456",
				CodeTypeEmail,
				tc.expiresAt,
			)
			assert.NoError(t, err)

			if tc.used {
				err := vc.Use()
				assert.NoError(t, err)
			}

			assert.Equal(t, tc.want, vc.IsValid())
		})
	}
}

func Test_VerificationCode_Verify(t *testing.T) {
	account, err := newMockAccount("email@net.hr", "")
	assert.NoError(t, err)

	vc, err := NewVerificationCode(
		account,
		"123456",
		CodeTypeEmail,
		time.Now().Add(10*time.Minute),
	)
	assert.NoError(t, err)

	err = vc.Verify("123456")
	assert.NoError(t, err)

	err = vc.Verify("654321")
	assert.Error(t, err)
}

func Test_VerificationCode_Use(t *testing.T) {

	account, err := newMockAccount("", "+1234567890")
	assert.NoError(t, err)

	vc, err := NewVerificationCode(
		account,
		"123456",
		CodeTypeEmail,
		time.Now().Add(10*time.Minute),
	)
	assert.NoError(t, err)

	assert.False(t, vc.IsUsed())

	err = vc.Use()
	assert.NoError(t, err)

	assert.True(t, vc.IsUsed())

	err = vc.Use()
	assert.Error(t, err)
	assert.EqualError(t, ErrVerificationCodeUsed, err.Error())
}

func newMockAccount(emailValue string, phoneNumber string) (*Account, error) {
	password, err := NewPassword("StrongP@ssw0rd1234")
	if err != nil {
		return nil, err
	}

	var (
		email *Email
		phone *PhoneNumber
	)

	if emailValue != "" {
		email, err = NewEmail(emailValue)
		if err != nil {
			return nil, err
		}
	}

	if phoneNumber != "" {
		phone, err = NewPhoneNumber(phoneNumber)
		if err != nil {
			return nil, err
		}
	}

	return NewAccount(email, phone, password)
}
