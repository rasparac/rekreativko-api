package domain

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

func Test_NewRefreshToken(t *testing.T) {

	var (
		accountID = uuid.New()
		tokenHash = "hashed_token"
		expiresAt = time.Now().Add(24 * time.Hour)
	)

	token := NewRefreshToken(accountID, tokenHash, expiresAt)

	assert.Equal(t, accountID, token.AccountID())
	assert.Equal(t, tokenHash, token.Token())
	assert.Equal(t, expiresAt, token.ExpiresAt())

	assert.True(t, token.IsValid())
	assert.False(t, token.IsExpired())
	assert.False(t, token.IsRevoked())

}

func Test_RefreshToken_IsValid(t *testing.T) {
	testCase := []struct {
		name      string
		expiresAt time.Time
		revoked   bool
		want      bool
	}{
		{
			name:      "valid token",
			expiresAt: time.Now().Add(1 * time.Hour),
			want:      true,
			revoked:   false,
		},
		{
			name:      "expired token",
			expiresAt: time.Now().Add(-1 * time.Hour),
			want:      false,
			revoked:   false,
		},
		{
			name:      "revoked token",
			expiresAt: time.Now().Add(1 * time.Hour),
			want:      false,
			revoked:   true,
		},
	}

	for _, tc := range testCase {
		t.Run(tc.name, func(t *testing.T) {
			token := NewRefreshToken(uuid.New(), "hash", tc.expiresAt)

			if tc.revoked {
				err := token.Revoke("unit test revoke")
				assert.NoError(t, err)
			}

			assert.Equal(t, tc.want, token.IsValid())
		})
	}
}

func Test_RefreshToken_Revoke(t *testing.T) {
	token := NewRefreshToken(uuid.New(), "hash", time.Now().Add(1*time.Hour))

	assert.False(t, token.IsRevoked())

	err := token.Revoke("unit test revoke")
	assert.NoError(t, err)

	assert.True(t, token.IsRevoked())

	err = token.Revoke("already revoked")
	assert.Error(t, err)
}
