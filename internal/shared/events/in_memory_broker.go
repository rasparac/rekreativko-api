package events

import (
	"context"
	"sync"

	"github.com/rasparac/rekreativko-api/internal/shared/logger"
)

type inMemoryBroker struct {
	subscribers map[string][]MessageHandler

	mu sync.RWMutex

	logger *logger.Logger
}

func NewInMemoryBroker(logger *logger.Logger) MessageBroker {
	return &inMemoryBroker{
		subscribers: make(map[string][]MessageHandler),
		logger:      logger,
	}
}

func (b *inMemoryBroker) Publish(ctx context.Context, topic string, payload []byte) error {
	b.mu.RLock()
	handlers, exists := b.subscribers[topic]
	b.mu.RUnlock()

	if !exists {
		b.logger.Info(ctx, "no subscribers for topic", "topic", topic)
		return nil
	}

	b.logger.Info(ctx, "publishing message", "topic", topic, "payload", string(payload))

	for _, handler := range handlers {
		go func(handler MessageHandler) {
			err := handler(ctx, payload)
			if err != nil {
				b.logger.Error(ctx, "failed to handle message", "error", err)
			}
		}(handler)
	}

	return nil
}

func (b *inMemoryBroker) Subscribe(ctx context.Context, topic string, handler MessageHandler) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.subscribers == nil {
		b.subscribers = make(map[string][]MessageHandler)
	}

	b.subscribers[topic] = append(b.subscribers[topic], handler)

	b.logger.Info(ctx, "subscribed to topic", "topic", topic)

	return nil
}

func (b *inMemoryBroker) Start(ctx context.Context) error {
	b.logger.Info(ctx, "in memory logger started")
	return nil
}

func (b *inMemoryBroker) Close(ctx context.Context) error {
	b.logger.Info(ctx, "in memory logger closed")
	return nil
}
