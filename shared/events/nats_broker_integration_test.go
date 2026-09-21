//go:build integration

package events

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/rasparac/rekreativko-api/shared/logger"
)

func startNATS(t *testing.T) string {
	t.Helper()

	ctx := context.Background()
	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image:        "nats:2-alpine",
			Cmd:          []string{"-js"},
			ExposedPorts: []string{"4222/tcp"},
			WaitingFor:   wait.ForLog("Server is ready").WithStartupTimeout(60 * time.Second),
		},
		Started: true,
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = container.Terminate(context.Background()) })

	host, err := container.Host(ctx)
	require.NoError(t, err)
	port, err := container.MappedPort(ctx, "4222/tcp")
	require.NoError(t, err)

	return "nats://" + host + ":" + port.Port()
}

// readDLQ waits for one message on the DLQ stream for the given subject.
func readDLQ(t *testing.T, b *natsBroker, subject string) jetstream.Msg {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	consumer, err := b.js.CreateOrUpdateConsumer(ctx, dlqStream, jetstream.ConsumerConfig{
		FilterSubject: subject,
		AckPolicy:     jetstream.AckExplicitPolicy,
	})
	require.NoError(t, err)

	batch, err := consumer.Fetch(1, jetstream.FetchMaxWait(30*time.Second))
	require.NoError(t, err)

	for msg := range batch.Messages() {
		return msg
	}
	t.Fatal("no message arrived on the DLQ stream")
	return nil
}

func TestNatsBroker_DeadLettersAfterMaxDeliveries(t *testing.T) {
	// Keep the test fast: retry almost immediately.
	orig := retryDelays
	retryDelays = []time.Duration{10 * time.Millisecond}
	t.Cleanup(func() { retryDelays = orig })

	url := startNATS(t)
	b, err := NewNatsBroker(url, "test-svc", logger.New("error", "json"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = b.Close(context.Background()) })

	deliveries := make(chan struct{}, 10)
	err = b.Subscribe(context.Background(), "test.failing.event", func(context.Context, []byte) error {
		deliveries <- struct{}{}
		return errors.New("always fails")
	})
	require.NoError(t, err)

	require.NoError(t, b.Publish(context.Background(), "test.failing.event", []byte(`{"hello":"world"}`)))

	msg := readDLQ(t, b, "dlq.test.failing.event")

	assert.Equal(t, []byte(`{"hello":"world"}`), msg.Data(), "original payload is preserved")
	assert.Equal(t, "events.test.failing.event", msg.Headers().Get(headerDLQOriginalSubject))
	assert.Equal(t, "test-svc", msg.Headers().Get(headerDLQService))
	assert.Equal(t, dlqReasonMaxDeliveries, msg.Headers().Get(headerDLQReason))
	assert.Equal(t, "3", msg.Headers().Get(headerDLQNumDelivered))
	assert.Equal(t, "always fails", msg.Headers().Get(headerDLQError))
	assert.Len(t, deliveries, maxDeliver, "handler ran exactly maxDeliver times")
}

func TestNatsBroker_PermanentErrorDeadLettersImmediately(t *testing.T) {
	url := startNATS(t)
	b, err := NewNatsBroker(url, "test-svc", logger.New("error", "json"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = b.Close(context.Background()) })

	deliveries := make(chan struct{}, 10)
	err = b.Subscribe(context.Background(), "test.bad.payload", func(context.Context, []byte) error {
		deliveries <- struct{}{}
		return Permanent(errors.New("decode payload"))
	})
	require.NoError(t, err)

	require.NoError(t, b.Publish(context.Background(), "test.bad.payload", []byte(`not json`)))

	msg := readDLQ(t, b, "dlq.test.bad.payload")

	assert.Equal(t, dlqReasonPermanent, msg.Headers().Get(headerDLQReason))
	assert.Equal(t, "1", msg.Headers().Get(headerDLQNumDelivered))
	assert.Len(t, deliveries, 1, "a permanent error is not retried")
}
