package events

import "context"

type MessageBroker interface {
	Publish(ctx context.Context, topic string, payload []byte) error

	Subscribe(ctx context.Context, topic string, handler MessageHandler) error

	Close(ctx context.Context) error
}

type MessageHandler func(ctx context.Context, payload []byte) error
