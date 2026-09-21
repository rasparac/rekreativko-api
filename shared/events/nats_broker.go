package events

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/rasparac/rekreativko-api/shared/logger"
)

const (
	// maxDeliver is how many times a message is delivered to a consumer before
	// it is dead-lettered.
	maxDeliver = 3

	eventsStream = "EVENTS"

	// dlqStream keeps messages a consumer gave up on, so they can be inspected
	// and replayed instead of silently disappearing.
	dlqStream        = "DLQ"
	dlqSubjectPrefix = "dlq."
	dlqMaxAge        = 14 * 24 * time.Hour

	// Dead-letter message headers.
	headerDLQOriginalSubject = "Dlq-Original-Subject"
	headerDLQService         = "Dlq-Service"
	headerDLQConsumer        = "Dlq-Consumer"
	headerDLQError           = "Dlq-Error"
	headerDLQReason          = "Dlq-Reason"
	headerDLQNumDelivered    = "Dlq-Num-Delivered"
	headerDLQStreamSeq       = "Dlq-Stream-Seq"
	headerDLQFailedAt        = "Dlq-Failed-At"

	dlqReasonPermanent     = "permanent_error"
	dlqReasonMaxDeliveries = "max_deliveries"

	maxDLQErrorLen = 1024
)

// retryDelays is how long a failed message waits before its next delivery,
// indexed by attempts so far; the last value is reused if there are more
// attempts than entries. Without a delay a brief outage would burn every
// attempt within milliseconds.
var retryDelays = []time.Duration{time.Second, 5 * time.Second}

var deadLetteredTotal = promauto.NewCounterVec(
	prometheus.CounterOpts{
		Subsystem: "events",
		Name:      "dead_lettered_total",
		Help:      "Total number of consumed messages given up on and sent to the dead-letter stream",
	},
	[]string{"service", "topic", "reason", "dlq_published"},
)

type (
	// dlqPublisher publishes dead-lettered messages; jetstream.JetStream satisfies it.
	dlqPublisher interface {
		PublishMsg(ctx context.Context, msg *nats.Msg, opts ...jetstream.PublishOpt) (*jetstream.PubAck, error)
	}

	natsBroker struct {
		conn      *nats.Conn
		js        jetstream.JetStream
		dlq       dlqPublisher
		consumers map[string]jetstream.Consumer
		cancelFns map[string]context.CancelFunc

		serviceName string

		logger *logger.Logger
	}
)

func NewNatsBroker(
	url string,
	serviceName string,
	logger *logger.Logger,
) (*natsBroker, error) {
	conn, err := nats.Connect(
		url,
		nats.MaxReconnects(-1),
		nats.ReconnectWait(1*time.Second),
		nats.DisconnectErrHandler(func(c *nats.Conn, err error) {
			logger.Error(context.Background(), "nats connection error", "error", err)
		}),
		nats.ReconnectHandler(func(c *nats.Conn) {
			logger.Info(context.Background(), "nats reconnected", "url", url)
		}),
	)
	if err != nil {
		return nil, err
	}

	js, err := jetstream.New(conn)
	if err != nil {
		conn.Close()
		return nil, err
	}

	nb := &natsBroker{
		conn:        conn,
		js:          js,
		dlq:         js,
		logger:      logger.WithName(fmt.Sprintf("broker.%s", serviceName)),
		serviceName: serviceName,
		consumers:   map[string]jetstream.Consumer{},
		cancelFns:   map[string]context.CancelFunc{},
	}

	err = nb.createStreams(context.Background())
	if err != nil {
		conn.Close()
		return nil, err
	}

	return nb, nil
}

func (b *natsBroker) Publish(ctx context.Context, topic string, payload []byte) error {
	subject := "events." + topic

	ack, err := b.js.Publish(
		ctx,
		subject,
		payload,
		jetstream.WithExpectStream(eventsStream),
	)
	if err != nil {
		b.logger.Error(
			ctx,
			"failed to publish message",
			"topic", topic,
			"error", err,
		)
		return err
	}

	b.logger.Info(
		ctx,
		"published message",
		"subject", subject,
		"stream", ack.Stream,
		"sequence", ack.Sequence,
	)

	return nil
}

func (b *natsBroker) Subscribe(
	ctx context.Context,
	topic string,
	handler MessageHandler,
) error {
	var (
		subject     = "events." + topic
		durableName = fmt.Sprintf("%s-%s", b.serviceName, sanitizeDurableName(subject))
	)

	stream, err := b.js.Stream(ctx, eventsStream)
	if err != nil {
		return err
	}

	consumer, err := stream.CreateOrUpdateConsumer(
		ctx,
		jetstream.ConsumerConfig{
			Name:          durableName,
			Durable:       durableName,
			Description:   fmt.Sprintf("Consumer for %s", topic),
			FilterSubject: subject,
			AckPolicy:     jetstream.AckExplicitPolicy,
			AckWait:       30 * time.Second,
			MaxDeliver:    maxDeliver,
			DeliverPolicy: jetstream.DeliverAllPolicy,
			ReplayPolicy:  jetstream.ReplayInstantPolicy,
		},
	)
	if err != nil {
		return err
	}

	b.consumers[topic] = consumer

	info, err := consumer.Info(ctx)
	if err != nil {
		return err
	}

	b.logger.Info(
		ctx,
		"created jetstream consumer",
		"name", info.Name,
		"topic", topic,
		"pending", info.NumPending,
		"subject", subject,
	)

	consumeCtx, cancel := context.WithCancel(ctx)

	b.cancelFns[topic] = cancel

	go b.consumeMessages(
		consumeCtx,
		consumer,
		topic,
		handler,
	)

	return nil
}

func (b *natsBroker) Close(ctx context.Context) error {
	for topic, cancel := range b.cancelFns {
		cancel()
		delete(b.cancelFns, topic)
	}

	b.conn.Close()

	return nil
}

func (b *natsBroker) createStreams(ctx context.Context) error {
	stream, err := b.js.CreateStream(
		ctx,
		jetstream.StreamConfig{
			Name:        eventsStream,
			Description: "Domain events stream",
			Subjects:    []string{"events.>"},
			Storage:     jetstream.FileStorage,
			Retention:   jetstream.LimitsPolicy,
			MaxAge:      7 * 24 * time.Hour,
			MaxBytes:    1024 * 1024 * 1024, // 1GB
			Duplicates:  1 * time.Minute,
			Replicas:    1,
		},
	)
	if err != nil {
		return err
	}

	info, err := stream.Info(ctx)
	if err != nil {
		return err
	}

	b.logger.Info(ctx, "created stream",
		"stream", info.Config.Name,
		"messages", info.State.Msgs,
		"bytes", info.State.Bytes,
		"last_seq", info.State.LastSeq,
		"first_seq", info.State.FirstSeq,
	)

	_, err = b.js.CreateOrUpdateStream(
		ctx,
		jetstream.StreamConfig{
			Name:        dlqStream,
			Description: "Dead-lettered domain events (consumer gave up after max deliveries or a permanent error)",
			Subjects:    []string{dlqSubjectPrefix + ">"},
			Storage:     jetstream.FileStorage,
			Retention:   jetstream.LimitsPolicy,
			MaxAge:      dlqMaxAge,
			MaxBytes:    256 * 1024 * 1024, // 256MB
			Duplicates:  2 * time.Minute,
			Replicas:    1,
		},
	)
	return err
}

func (b *natsBroker) consumeMessages(
	ctx context.Context,
	consumer jetstream.Consumer,
	topic string,
	handler MessageHandler,
) {
	msgs, err := consumer.Messages(
		jetstream.PullMaxMessages(10),
		jetstream.PullExpiry(5*time.Second),
	)
	if err != nil {
		b.logger.Error(ctx, "failed to pull messages", "error", err)
		return
	}
	defer msgs.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		default:
			msg, err := msgs.Next()
			if err != nil {
				b.logger.Error(ctx, "failed to get next message", "error", err)
				return
			}

			b.processMessage(ctx, topic, msg, handler)
		}
	}

}

func (b *natsBroker) processMessage(
	ctx context.Context,
	topic string,
	msg jetstream.Msg,
	handler MessageHandler,
) {
	metadata, err := msg.Metadata()
	if err != nil {
		b.logger.Error(ctx, "failed to get message metadata", "error", err)
		msg.Nak()
		return
	}

	log := b.logger.WithValues(
		"stream", metadata.Stream,
		"sequence", metadata.Sequence,
		"consumer", metadata.Consumer,
		"topic", topic,
	)

	err = handler(ctx, msg.Data())
	if err != nil {
		log.Error(ctx, "failed to handle message", "error", err)

		permanent := errors.Is(err, ErrPermanent)
		if permanent || metadata.NumDelivered >= maxDeliver {
			b.deadLetter(ctx, log, topic, msg, metadata, err, permanent)
			return
		}

		delay := retryDelay(metadata.NumDelivered)
		if nakErr := msg.NakWithDelay(delay); nakErr != nil {
			log.Error(ctx, "failed to nak message", "error", nakErr)
		}
		return
	}

	err = msg.Ack()
	if err != nil {
		log.Error(ctx, "failed to ack message", "error", err)
	} else {
		log.Info(
			ctx,
			"message acked",
		)
	}
}

func retryDelay(numDelivered uint64) time.Duration {
	idx := int(numDelivered) - 1
	if idx < 0 {
		idx = 0
	}
	if idx >= len(retryDelays) {
		idx = len(retryDelays) - 1
	}
	return retryDelays[idx]
}

// deadLetter copies the message to the DLQ stream, with headers describing why
// it failed, and then terminates it so this consumer never sees it again. If
// the DLQ publish itself fails the message is still terminated (retrying could
// loop forever on a broker problem); the failure is logged and counted with
// dlq_published="false".
func (b *natsBroker) deadLetter(
	ctx context.Context,
	log *logger.Logger,
	topic string,
	msg jetstream.Msg,
	metadata *jetstream.MsgMetadata,
	handlerErr error,
	permanent bool,
) {
	reason := dlqReasonMaxDeliveries
	if permanent {
		reason = dlqReasonPermanent
	}

	dlqMsg := nats.NewMsg(dlqSubjectPrefix + topic)
	dlqMsg.Data = msg.Data()
	dlqMsg.Header.Set(headerDLQOriginalSubject, msg.Subject())
	dlqMsg.Header.Set(headerDLQService, b.serviceName)
	dlqMsg.Header.Set(headerDLQConsumer, metadata.Consumer)
	dlqMsg.Header.Set(headerDLQError, sanitizeHeaderValue(handlerErr.Error(), maxDLQErrorLen))
	dlqMsg.Header.Set(headerDLQReason, reason)
	dlqMsg.Header.Set(headerDLQNumDelivered, strconv.FormatUint(metadata.NumDelivered, 10))
	dlqMsg.Header.Set(headerDLQStreamSeq, strconv.FormatUint(metadata.Sequence.Stream, 10))
	dlqMsg.Header.Set(headerDLQFailedAt, time.Now().UTC().Format(time.RFC3339))

	// Deterministic id so a retried dead-letter of the same message is deduplicated.
	msgID := fmt.Sprintf("%s-%d", metadata.Consumer, metadata.Sequence.Stream)

	published := true
	if _, err := b.dlq.PublishMsg(ctx, dlqMsg, jetstream.WithMsgID(msgID)); err != nil {
		published = false
		log.Error(ctx, "failed to publish message to dead-letter stream, dropping it",
			"error", err,
			"dlq_subject", dlqMsg.Subject,
			"stream_sequence", metadata.Sequence.Stream,
		)
	} else {
		log.Error(ctx, "message dead-lettered",
			"dlq_subject", dlqMsg.Subject,
			"reason", reason,
			"num_delivered", metadata.NumDelivered,
			"stream_sequence", metadata.Sequence.Stream,
		)
	}

	deadLetteredTotal.WithLabelValues(b.serviceName, topic, reason, strconv.FormatBool(published)).Inc()

	if err := msg.Term(); err != nil {
		log.Error(ctx, "failed to terminate message", "error", err)
	}
}

// sanitizeHeaderValue makes s safe as a NATS header value: no CR/LF, bounded length.
func sanitizeHeaderValue(s string, maxLen int) string {
	s = strings.NewReplacer("\r", " ", "\n", " ").Replace(s)
	if len(s) > maxLen {
		s = s[:maxLen]
	}
	return s
}

func sanitizeDurableName(topic string) string {
	name := strings.ReplaceAll(topic, ".", "-")
	name = strings.ReplaceAll(name, "*", "all")
	name = strings.ReplaceAll(name, ">", "any")
	return name
}
