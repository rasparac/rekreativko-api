package templates

import (
	"testing"

	"github.com/rasparac/rekreativko-api/notifications/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRenderer_Render(t *testing.T) {
	r, err := NewRenderer()
	require.NoError(t, err)

	t.Run("verification email carries the code", func(t *testing.T) {
		got, err := r.Render(domain.Message{
			Kind:    domain.MessageVerificationCode,
			Channel: domain.ChannelEmail,
			Data:    map[string]string{"code": "482913"},
		})
		require.NoError(t, err)
		assert.Equal(t, "Rekreativko verification code", got.Subject)
		assert.Contains(t, got.Body, "Your verification code is: 482913")
	})

	t.Run("verification sms has no subject", func(t *testing.T) {
		got, err := r.Render(domain.Message{
			Kind:    domain.MessageVerificationCode,
			Channel: domain.ChannelSMS,
			Data:    map[string]string{"code": "482913"},
		})
		require.NoError(t, err)
		assert.Empty(t, got.Subject)
		assert.Equal(t, "Your Rekreativko verification code is: 482913", got.Body)
	})

	t.Run("every kind and channel the service sends has a template", func(t *testing.T) {
		for _, tc := range []struct {
			kind    domain.MessageKind
			channel domain.Channel
		}{
			{domain.MessageVerificationCode, domain.ChannelEmail},
			{domain.MessageVerificationCode, domain.ChannelSMS},
			{domain.MessageWelcome, domain.ChannelEmail},
			{domain.MessagePasswordChanged, domain.ChannelEmail},
			{domain.MessagePasswordChanged, domain.ChannelSMS},
		} {
			got, err := r.Render(domain.Message{
				Kind: tc.kind, Channel: tc.channel,
				Data: map[string]string{"code": "1"},
			})
			require.NoError(t, err, "%s/%s", tc.kind, tc.channel)
			assert.NotEmpty(t, got.Body)
			if tc.channel == domain.ChannelEmail {
				assert.NotEmpty(t, got.Subject)
			}
		}
	})

	t.Run("an unknown locale falls back to the default", func(t *testing.T) {
		got, err := r.Render(domain.Message{
			Kind: domain.MessageWelcome, Channel: domain.ChannelEmail, Locale: "xx",
		})
		require.NoError(t, err)
		assert.Equal(t, "Welcome to Rekreativko!", got.Subject)
	})

	t.Run("a missing template is an error", func(t *testing.T) {
		_, err := r.Render(domain.Message{Kind: domain.MessageWelcome, Channel: domain.ChannelSMS})
		assert.Error(t, err)
	})

	t.Run("a missing value is an error, not an empty string", func(t *testing.T) {
		_, err := r.Render(domain.Message{Kind: domain.MessageVerificationCode, Channel: domain.ChannelSMS})
		assert.Error(t, err)
	})
}
