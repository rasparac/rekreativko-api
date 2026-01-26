package events

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/rasparac/rekreativko-api/internal/shared/logger"
	"github.com/stretchr/testify/assert"
)

func TestInMemoryBroker(t *testing.T) {

	newBoker := NewInMemoryBroker(logger.NewDevelopment())

	ctx := t.Context()

	var wg sync.WaitGroup

	wg.Add(1)

	newBoker.Subscribe(ctx, "test", mockMessageHandler(t, &wg))

	newBoker.Publish(ctx, "test", []byte("this is message"))

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("timeout")
	}
}

func mockMessageHandler(t *testing.T, wg *sync.WaitGroup) MessageHandler {
	return func(ctx context.Context, payload []byte) error {
		defer wg.Done()

		assert.Equal(t, payload, []byte("this is message"))

		return nil
	}
}
