package events

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rasparac/rekreativko-api/shared/logger"
)

// fakeMsg embeds the interface so only the methods processMessage uses are implemented.
type fakeMsg struct {
	jetstream.Msg
	data         []byte
	numDelivered uint64
	acked        bool
	nakDelay     *time.Duration
	termed       bool
}

func (m *fakeMsg) Data() []byte    { return m.data }
func (m *fakeMsg) Subject() string { return "events.identity.account.verified" }
func (m *fakeMsg) Metadata() (*jetstream.MsgMetadata, error) {
	md := &jetstream.MsgMetadata{Stream: "EVENTS", Consumer: "svc-events-identity-account-verified", NumDelivered: m.numDelivered}
	md.Sequence.Stream = 42
	return md, nil
}
func (m *fakeMsg) Ack() error { m.acked = true; return nil }
func (m *fakeMsg) NakWithDelay(d time.Duration) error {
	m.nakDelay = &d
	return nil
}
func (m *fakeMsg) Term() error { m.termed = true; return nil }

type fakeDLQ struct {
	published []*nats.Msg
	msgIDs    []string
	err       error
}

func (f *fakeDLQ) PublishMsg(_ context.Context, msg *nats.Msg, opts ...jetstream.PublishOpt) (*jetstream.PubAck, error) {
	if f.err != nil {
		return nil, f.err
	}
	f.published = append(f.published, msg)
	return &jetstream.PubAck{}, nil
}

func newTestBroker(dlq *fakeDLQ) *natsBroker {
	return &natsBroker{
		logger:      logger.New("error", "json"),
		serviceName: "svc",
		dlq:         dlq,
	}
}

func handlerReturning(err error) MessageHandler {
	return func(context.Context, []byte) error { return err }
}

func TestProcessMessage_SuccessAcks(t *testing.T) {
	dlq := &fakeDLQ{}
	msg := &fakeMsg{numDelivered: 1}

	newTestBroker(dlq).processMessage(context.Background(), "identity.account.verified", msg, handlerReturning(nil))

	assert.True(t, msg.acked)
	assert.Nil(t, msg.nakDelay)
	assert.False(t, msg.termed)
	assert.Empty(t, dlq.published)
}

func TestProcessMessage_TransientFailureNaksWithGrowingDelay(t *testing.T) {
	for delivered, want := range map[uint64]time.Duration{1: time.Second, 2: 5 * time.Second} {
		dlq := &fakeDLQ{}
		msg := &fakeMsg{numDelivered: delivered}

		newTestBroker(dlq).processMessage(context.Background(), "t", msg, handlerReturning(errors.New("db down")))

		require.NotNil(t, msg.nakDelay, "delivery %d", delivered)
		assert.Equal(t, want, *msg.nakDelay, "delivery %d", delivered)
		assert.False(t, msg.termed)
		assert.False(t, msg.acked)
		assert.Empty(t, dlq.published, "must not dead-letter before max deliveries")
	}
}

func TestProcessMessage_MaxDeliveriesDeadLetters(t *testing.T) {
	dlq := &fakeDLQ{}
	msg := &fakeMsg{data: []byte(`{"a":1}`), numDelivered: maxDeliver}

	newTestBroker(dlq).processMessage(context.Background(), "identity.account.verified", msg,
		handlerReturning(errors.New("db down\nsecond line")))

	require.Len(t, dlq.published, 1)
	dead := dlq.published[0]
	assert.Equal(t, "dlq.identity.account.verified", dead.Subject)
	assert.Equal(t, []byte(`{"a":1}`), dead.Data, "original payload is preserved")
	assert.Equal(t, "events.identity.account.verified", dead.Header.Get(headerDLQOriginalSubject))
	assert.Equal(t, "svc", dead.Header.Get(headerDLQService))
	assert.Equal(t, dlqReasonMaxDeliveries, dead.Header.Get(headerDLQReason))
	assert.Equal(t, "3", dead.Header.Get(headerDLQNumDelivered))
	assert.Equal(t, "42", dead.Header.Get(headerDLQStreamSeq))
	assert.Equal(t, "db down second line", dead.Header.Get(headerDLQError), "newlines are stripped from headers")
	assert.NotEmpty(t, dead.Header.Get(headerDLQFailedAt))
	assert.True(t, msg.termed)
	assert.Nil(t, msg.nakDelay)
}

func TestProcessMessage_PermanentErrorDeadLettersOnFirstDelivery(t *testing.T) {
	dlq := &fakeDLQ{}
	msg := &fakeMsg{numDelivered: 1}

	newTestBroker(dlq).processMessage(context.Background(), "t", msg,
		handlerReturning(Permanent(errors.New("decode payload"))))

	require.Len(t, dlq.published, 1)
	assert.Equal(t, dlqReasonPermanent, dlq.published[0].Header.Get(headerDLQReason))
	assert.True(t, msg.termed)
	assert.Nil(t, msg.nakDelay, "retrying a permanent error is pointless")
}

func TestProcessMessage_WrappedPermanentErrorIsDetected(t *testing.T) {
	dlq := &fakeDLQ{}
	msg := &fakeMsg{numDelivered: 1}

	wrapped := fmt.Errorf("handler: %w", Permanent(errors.New("bad payload")))
	newTestBroker(dlq).processMessage(context.Background(), "t", msg, handlerReturning(wrapped))

	require.Len(t, dlq.published, 1)
	assert.True(t, msg.termed)
}

func TestProcessMessage_DLQPublishFailureStillTerminates(t *testing.T) {
	dlq := &fakeDLQ{err: errors.New("nats down")}
	msg := &fakeMsg{numDelivered: maxDeliver}

	newTestBroker(dlq).processMessage(context.Background(), "t", msg, handlerReturning(errors.New("boom")))

	assert.True(t, msg.termed, "must terminate even if the DLQ is unreachable, to avoid looping forever")
	assert.Nil(t, msg.nakDelay)
}

func TestSanitizeHeaderValue_TruncatesAndStripsLineBreaks(t *testing.T) {
	got := sanitizeHeaderValue("a\r\nb"+strings.Repeat("x", 2000), 10)

	assert.Len(t, got, 10)
	assert.NotContains(t, got, "\n")
	assert.NotContains(t, got, "\r")
}

func TestPermanent(t *testing.T) {
	assert.NoError(t, Permanent(nil))

	inner := errors.New("inner")
	err := Permanent(inner)
	assert.ErrorIs(t, err, ErrPermanent)
	assert.ErrorIs(t, err, inner)
}

func TestRetryDelay(t *testing.T) {
	assert.Equal(t, time.Second, retryDelay(0))
	assert.Equal(t, time.Second, retryDelay(1))
	assert.Equal(t, 5*time.Second, retryDelay(2))
	assert.Equal(t, 5*time.Second, retryDelay(10), "reuses the last delay past the end")
}
