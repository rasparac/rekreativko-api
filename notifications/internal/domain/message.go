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
