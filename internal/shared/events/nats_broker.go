package events

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/rasparac/rekreativko-api/internal/shared/logger"
)

type (
	natsBroker struct {
		conn      *nats.Conn
		js        jetstream.JetStream
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
		jetstream.WithExpectStream("EVENTS"),
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

	stream, err := b.js.Stream(ctx, "EVENTS")
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
			MaxDeliver:    3,
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
			Name:        "EVENTS",
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

	return nil
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

		if metadata.NumDelivered >= 3 {
			log.Error(
				ctx,
				"max delivers reached, terminating message",
				"stream", metadata.Stream,
				"num_delivered", metadata.NumDelivered,
			)
			msg.Term()
		} else {
			msg.Nak()
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

func sanitizeDurableName(topic string) string {
	name := strings.ReplaceAll(topic, ".", "-")
	name = strings.ReplaceAll(name, "*", "all")
	name = strings.ReplaceAll(name, ">", "any")
	return name
}
