package events

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"runtime/debug"
	"strconv"
	"strings"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/rasparac/rekreativko-api/shared/api"
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
	// the consumer gave up without the handler ever returning (see watchMaxDeliveries)
	dlqReasonMaxDeliveriesAdvisory = "max_deliveries_advisory"

	// advisoryTimeout bounds handling one max deliveries advisory.
	advisoryTimeout = 10 * time.Second

	maxDLQErrorLen = 1024
)

// retryDelays is how long a failed message waits before its next delivery,
// indexed by attempts so far; the last value is reused if there are more
// attempts than entries. Without a delay a brief outage would burn every
// attempt within milliseconds.
var retryDelays = []time.Duration{time.Second, 5 * time.Second}

// dlqPublishDelays is how long deadLetter waits before each retry of a failed
// dead-letter publish - len+1 attempts in all - to ride out a brief NATS flap.
// Kept short: the consumer loop waits for it.
var dlqPublishDelays = []time.Duration{100 * time.Millisecond, 500 * time.Millisecond}

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

type (
	// BrokerOption configures NewNatsBroker.
	BrokerOption func(*brokerOptions)

	brokerOptions struct {
		user, password string
		manageStreams  bool
	}
)

// WithCredentials logs in to NATS as user. An empty user connects without
// credentials (a NATS server with no authorization configured).
func WithCredentials(user, password string) BrokerOption {
	return func(o *brokerOptions) { o.user, o.password = user, password }
}

// WithStreamManagement makes this broker create or update the EVENTS and DLQ
// streams. Only one component does it (the outbox-publisher, and tests), so
// the services' NATS users need no rights to manage streams; every other
// broker waits for the streams to exist (streamWaitTimeout).
func WithStreamManagement() BrokerOption {
	return func(o *brokerOptions) { o.manageStreams = true }
}

// streamWaitTimeout is how long a broker without WithStreamManagement waits
// for the outbox-publisher to create the streams before it gives up.
var streamWaitTimeout = 2 * time.Minute

func NewNatsBroker(
	url string,
	serviceName string,
	logger *logger.Logger,
	opts ...BrokerOption,
) (*natsBroker, error) {
	var options brokerOptions
	for _, opt := range opts {
		opt(&options)
	}

	natsOpts := []nats.Option{
		nats.Name(serviceName),
		nats.MaxReconnects(-1),
		nats.ReconnectWait(1 * time.Second),
		nats.DisconnectErrHandler(func(c *nats.Conn, err error) {
			logger.Error(context.Background(), "nats connection error", "error", err)
		}),
		nats.ReconnectHandler(func(c *nats.Conn) {
			logger.Info(context.Background(), "nats reconnected", "url", url)
		}),
		// a permissions violation arrives asynchronously, not as a call's error
		nats.ErrorHandler(func(_ *nats.Conn, _ *nats.Subscription, err error) {
			logger.Error(context.Background(), "nats error", "error", err)
		}),
	}
	if options.user != "" {
		natsOpts = append(natsOpts, nats.UserInfo(options.user, options.password))
	}

	conn, err := nats.Connect(url, natsOpts...)
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

	if options.manageStreams {
		err = nb.createStreams(context.Background())
	} else {
		err = nb.waitForStreams(context.Background())
	}
	if err != nil {
		conn.Close()
		return nil, err
	}

	return nb, nil
}

// waitForStreams returns once the EVENTS and DLQ streams exist: a service may
// start before the outbox-publisher has created them.
func (b *natsBroker) waitForStreams(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, streamWaitTimeout)
	defer cancel()

	for _, name := range []string{eventsStream, dlqStream} {
		for {
			_, err := b.js.Stream(ctx, name)
			if err == nil {
				break
			}
			if !errors.Is(err, jetstream.ErrStreamNotFound) {
				return fmt.Errorf("look up stream %s: %w", name, err)
			}

			b.logger.Info(ctx, "waiting for the outbox-publisher to create a stream", "stream", name)
			select {
			case <-ctx.Done():
				return fmt.Errorf("stream %s was not created within %s: %w", name, streamWaitTimeout, ctx.Err())
			case <-time.After(2 * time.Second):
			}
		}
	}

	return nil
}

func (b *natsBroker) Publish(ctx context.Context, topic, msgID string, payload []byte) error {
	subject := "events." + topic

	opts := []jetstream.PublishOpt{jetstream.WithExpectStream(eventsStream)}
	if msgID != "" {
		opts = append(opts, jetstream.WithMsgID(msgID))
	}

	ack, err := b.js.Publish(ctx, subject, payload, opts...)
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
		// true when the stream already had this msgID: nothing new was stored
		"duplicate", ack.Duplicate,
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

	if err := b.watchMaxDeliveries(durableName, topic); err != nil {
		return err
	}

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

// SubscribeBroadcast delivers every event matching topic (a subject pattern
// without the "events." prefix, wildcards allowed, e.g. "activity.session.>")
// to this process, independently of every other instance. Subscribe instead
// shares one durable consumer between all instances of a service, so each
// event reaches only one of them. Delivery is at-most-once - a plain NATS
// subscription with no acks or redelivery, and nothing published while this
// process was down - so use it only for live, best-effort fan-out whose
// receivers can resync on their own (e.g. pushing state to connected
// clients). Messages are handled one at a time, in order. The subscription
// ends when ctx is done or the broker is closed.
func (b *natsBroker) SubscribeBroadcast(
	ctx context.Context,
	topic string,
	handler BroadcastHandler,
) error {
	subject := "events." + topic

	sub, err := b.conn.Subscribe(subject, func(msg *nats.Msg) {
		topic := strings.TrimPrefix(msg.Subject, "events.")
		ctx := withEventID(ctx, msg.Data)
		b.logger.Debug(ctx, "broadcast event received", "topic", topic)

		handler(ctx, topic, msg.Data)
	})
	if err != nil {
		return fmt.Errorf("subscribe to %s: %w", subject, err)
	}

	// Round-trip to the server so the subscription is registered before we
	// return - otherwise an event published right after could be missed.
	if err := b.conn.Flush(); err != nil {
		_ = sub.Unsubscribe()
		return fmt.Errorf("register subscription to %s: %w", subject, err)
	}

	go func() {
		<-ctx.Done()
		_ = sub.Unsubscribe()
	}()

	b.logger.Info(ctx, "created broadcast subscription", "subject", subject)

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
			// The outbox publishes with the event_id as message id, so an event
			// republished within this window (published, but marking it failed)
			// is stored once.
			Duplicates: 1 * time.Minute,
			Replicas:   1,
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
		// redelivered after AckWait even if the nak is lost
		if err := msg.Nak(); err != nil {
			b.logger.Error(ctx, "failed to nak message", "error", err)
		}
		return
	}

	log := b.logger.WithValues(
		"stream", metadata.Stream,
		"sequence", metadata.Sequence,
		"consumer", metadata.Consumer,
		"topic", topic,
	)

	// every line the handler logs about this event carries its id
	ctx = withEventID(ctx, msg.Data())
	log.Debug(ctx, "event received", "num_delivered", metadata.NumDelivered)

	err = runHandler(ctx, log, handler, msg.Data())
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

// runHandler runs handler, turning a panic into an error: the message is then
// retried and dead-lettered like any other failure, instead of the panic
// crashing the service on every redelivery until JetStream gives up on it.
func runHandler(ctx context.Context, log *logger.Logger, handler MessageHandler, payload []byte) (err error) {
	defer func() {
		if r := recover(); r != nil {
			log.Error(ctx, "handler panicked", "panic", fmt.Sprint(r), "stack", string(debug.Stack()))
			err = fmt.Errorf("handler panicked: %v", r)
		}
	}()

	return handler(ctx, payload)
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
// it failed, and then terminates it so this consumer never sees it again. A
// failed DLQ publish is retried a few times (dlqPublishDelays); if it still
// fails the message is terminated anyway (retrying for longer could stall the
// consumer on a broker problem), logged and counted with dlq_published="false",
// which the DeadLetterPublishFailed alert watches.
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

	b.storeDeadLetter(ctx, log, deadLetterInfo{
		topic:        topic,
		subject:      msg.Subject(),
		consumer:     metadata.Consumer,
		reason:       reason,
		errText:      handlerErr.Error(),
		data:         msg.Data(),
		streamSeq:    metadata.Sequence.Stream,
		numDelivered: metadata.NumDelivered,
	})

	if err := msg.Term(); err != nil {
		log.Error(ctx, "failed to terminate message", "error", err)
	}
}

// deadLetterInfo is one message to copy to the DLQ stream and why.
type deadLetterInfo struct {
	topic, subject, consumer, reason, errText string
	data                                      []byte
	streamSeq, numDelivered                   uint64
}

// storeDeadLetter publishes the message to the DLQ stream, logs the outcome and
// counts it in events_dead_lettered_total.
func (b *natsBroker) storeDeadLetter(ctx context.Context, log *logger.Logger, d deadLetterInfo) {
	dlqMsg := nats.NewMsg(dlqSubjectPrefix + d.topic)
	dlqMsg.Data = d.data
	dlqMsg.Header.Set(headerDLQOriginalSubject, d.subject)
	dlqMsg.Header.Set(headerDLQService, b.serviceName)
	dlqMsg.Header.Set(headerDLQConsumer, d.consumer)
	dlqMsg.Header.Set(headerDLQError, sanitizeHeaderValue(d.errText, maxDLQErrorLen))
	dlqMsg.Header.Set(headerDLQReason, d.reason)
	dlqMsg.Header.Set(headerDLQNumDelivered, strconv.FormatUint(d.numDelivered, 10))
	dlqMsg.Header.Set(headerDLQStreamSeq, strconv.FormatUint(d.streamSeq, 10))
	dlqMsg.Header.Set(headerDLQFailedAt, time.Now().UTC().Format(time.RFC3339))

	// Deterministic id so a retried dead-letter of the same message is
	// deduplicated. Set as the header jetstream.WithMsgID would set.
	dlqMsg.Header.Set(jetstream.MsgIDHeader, fmt.Sprintf("%s-%d", d.consumer, d.streamSeq))

	err := b.publishDeadLetter(ctx, dlqMsg)
	published := err == nil
	if err != nil {
		// No payload here: it may hold personal data, which Loki can't erase.
		// The message stays in the EVENTS stream for its retention (7 days),
		// found by stream_sequence (nats stream get EVENTS <seq>).
		log.Error(ctx, "failed to publish message to dead-letter stream, dropping it",
			"error", err,
			"attempts", len(dlqPublishDelays)+1,
			"dlq_subject", dlqMsg.Subject,
			"reason", d.reason,
			"stream_sequence", d.streamSeq,
		)
	} else {
		log.Error(ctx, "message dead-lettered",
			"dlq_subject", dlqMsg.Subject,
			"reason", d.reason,
			"num_delivered", d.numDelivered,
			"stream_sequence", d.streamSeq,
		)
	}

	deadLetteredTotal.WithLabelValues(b.serviceName, d.topic, d.reason, strconv.FormatBool(published)).Inc()
}

// maxDeliveriesAdvisory is what JetStream sends when a consumer stops
// redelivering a message that never got an ack, nak or term.
type maxDeliveriesAdvisory struct {
	Consumer   string `json:"consumer"`
	StreamSeq  uint64 `json:"stream_seq"`
	Deliveries uint64 `json:"deliveries"`
}

// watchMaxDeliveries dead-letters what the consumer gives up on without
// processMessage ever deciding: the service crashed on every delivery (an OOM
// kill, say) or the handler outlasted AckWait each time. The instances of a
// service share a queue group, so one of them handles each advisory (and the
// deterministic DLQ message id drops a second copy anyway). An advisory is a
// plain NATS message: it is lost if no instance is connected when it is sent.
func (b *natsBroker) watchMaxDeliveries(consumerName, topic string) error {
	subject := fmt.Sprintf("$JS.EVENT.ADVISORY.CONSUMER.MAX_DELIVERIES.%s.%s", eventsStream, consumerName)

	_, err := b.conn.QueueSubscribe(subject, consumerName, func(m *nats.Msg) {
		b.handleMaxDeliveries(topic, m.Data)
	})
	if err != nil {
		return fmt.Errorf("subscribe to max deliveries advisories: %w", err)
	}

	return nil
}

func (b *natsBroker) handleMaxDeliveries(topic string, payload []byte) {
	ctx, cancel := context.WithTimeout(context.Background(), advisoryTimeout)
	defer cancel()

	log := b.logger.WithValues("topic", topic, "reason", dlqReasonMaxDeliveriesAdvisory)

	var advisory maxDeliveriesAdvisory
	if err := json.Unmarshal(payload, &advisory); err != nil {
		log.Error(ctx, "failed to decode max deliveries advisory", "error", err)
		return
	}
	log = log.WithValues("consumer", advisory.Consumer, "stream_sequence", advisory.StreamSeq)

	stream, err := b.js.Stream(ctx, eventsStream)
	if err == nil {
		var raw *jetstream.RawStreamMsg
		if raw, err = stream.GetMsg(ctx, advisory.StreamSeq); err == nil {
			ctx = withEventID(ctx, raw.Data)
			b.storeDeadLetter(ctx, log, deadLetterInfo{
				topic:        topic,
				subject:      raw.Subject,
				consumer:     advisory.Consumer,
				reason:       dlqReasonMaxDeliveriesAdvisory,
				errText:      "no result from the handler after max deliveries: the service crashed or the handler outlasted AckWait",
				data:         raw.Data,
				streamSeq:    advisory.StreamSeq,
				numDelivered: advisory.Deliveries,
			})
			return
		}
	}

	log.Error(ctx, "failed to read a message the consumer gave up on, dropping it", "error", err)
	deadLetteredTotal.WithLabelValues(b.serviceName, topic, dlqReasonMaxDeliveriesAdvisory, "false").Inc()
}

// publishDeadLetter publishes msg to the DLQ stream, retrying after each of
// dlqPublishDelays. A retry is safe: the message id makes the stream drop a
// second copy of one that did arrive.
func (b *natsBroker) publishDeadLetter(ctx context.Context, msg *nats.Msg) error {
	for attempt := 0; ; attempt++ {
		_, err := b.dlq.PublishMsg(ctx, msg)
		if err == nil || attempt >= len(dlqPublishDelays) {
			return err
		}

		select {
		case <-ctx.Done():
			return errors.Join(err, ctx.Err())
		case <-time.After(dlqPublishDelays[attempt]):
		}
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

// withEventID puts the event_id of a domain event payload into ctx, so every
// line logged while handling it can be found by that id. A payload without
// one leaves ctx as it is.
func withEventID(ctx context.Context, payload []byte) context.Context {
	var event struct {
		EventID string `json:"event_id"`
	}
	if err := json.Unmarshal(payload, &event); err != nil || event.EventID == "" {
		return ctx
	}
	return api.WithEventID(ctx, event.EventID)
}
