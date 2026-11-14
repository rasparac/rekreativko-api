package domain

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMessageKind_SecurityMessagesIgnoreTheUsersSettings(t *testing.T) {
	// turning email or SMS off must never stop a verification code or a
	// password-changed warning
	for _, kind := range []MessageKind{MessageVerificationCode, MessagePasswordChanged} {
		policy, ok := kind.SettingsPolicy()
		assert.True(t, ok, kind)
		assert.Equal(t, AlwaysSent, policy, kind)
	}
}

func TestMessageKind_AnUnclassifiedKindIsReported(t *testing.T) {
	_, ok := MessageKind("not_a_kind").SettingsPolicy()
	assert.False(t, ok)
}
