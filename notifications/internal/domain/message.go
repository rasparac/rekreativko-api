package domain

type (
	// Channel is how a Message reaches the user outside the app.
	Channel string

	// MessageKind names what a Message says; it is also the template's name
	// and the "type" label of the send metrics.
	MessageKind string

	// Message is one email or SMS to send: what to say (Kind, Data), to whom
	// (Recipient, an address or phone number), how (Channel) and in which
	// language (Locale, empty for the default).
	Message struct {
		Kind      MessageKind
		Channel   Channel
		Recipient string
		Locale    string
		Data      map[string]string
	}

	// RenderedMessage is a Message turned into text. Subject is empty for
	// channels without one (SMS).
	RenderedMessage struct {
		Subject string
		Body    string
	}
)

const (
	ChannelEmail Channel = "email"
	ChannelSMS   Channel = "sms"
)

const (
	MessageVerificationCode MessageKind = "verification"
	MessageWelcome          MessageKind = "welcome_message"
	MessagePasswordChanged  MessageKind = "password_changed"
)

// SettingsPolicy says whether a message respects the recipient's
// notification.<channel>.enabled setting (account-profile).
type SettingsPolicy int

const (
	// AlwaysSent: security and account mail the user can't opt out of -
	// verification codes, password changes, anything they asked for or must
	// know about their account.
	AlwaysSent SettingsPolicy = iota + 1
	// RespectsSettings: anything else (activity updates, reminders,
	// announcements) - skipped when the user turned the channel off.
	RespectsSettings
)

// Every kind must be listed (the templates test checks each template's kind
// is). Nothing is RespectsSettings yet, so the settings aren't consulted; the
// first such kind brings their propagation to this service (rekreativko-api-79p.3).
var settingsPolicies = map[MessageKind]SettingsPolicy{
	MessageVerificationCode: AlwaysSent,
	MessagePasswordChanged:  AlwaysSent,
	// sent once, the moment the address is verified - before the user could
	// have turned email off - and it confirms the account exists
	MessageWelcome: AlwaysSent,
}

// SettingsPolicy returns how this kind treats the recipient's notification
// settings; ok is false for a kind nobody classified.
func (k MessageKind) SettingsPolicy() (policy SettingsPolicy, ok bool) {
	policy, ok = settingsPolicies[k]
	return policy, ok
}
