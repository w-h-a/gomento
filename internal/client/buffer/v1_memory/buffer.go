package v1memory

import (
	"context"
	"sync"
	"time"

	"github.com/google/uuid"
	v1 "github.com/w-h-a/gomento/api/domain/v1"
	"github.com/w-h-a/gomento/internal/client/buffer"
)

type v1MemoryBuffer struct {
	options buffer.Options
	recent  map[uuid.UUID][]v1.Message
	queue   []buffer.BufferedMessage
	mtx     sync.RWMutex
}

func (b *v1MemoryBuffer) Add(ctx context.Context, msg *v1.Message, assets map[int]*v1.Asset) error {
	b.mtx.Lock()
	defer b.mtx.Unlock()

	cpy := *msg

	window := b.recent[cpy.SessionId]
	window = append(window, cpy)
	if len(window) > b.options.MaxHist {
		window = window[len(window)-b.options.MaxHist:]
	}
	b.recent[cpy.SessionId] = window

	b.queue = append(b.queue, buffer.BufferedMessage{
		Message:  cpy,
		Assets:   assets,
		QueuedAt: time.Now(),
	})

	return nil
}

func (b *v1MemoryBuffer) GetRecent(ctx context.Context, sessionId uuid.UUID) ([]v1.Message, error) {
	b.mtx.RLock()
	defer b.mtx.RUnlock()

	window := b.recent[sessionId]
	out := make([]v1.Message, len(window))
	copy(out, window)

	return out, nil
}

func (b *v1MemoryBuffer) PopBatch(ctx context.Context, limit int) ([]buffer.BufferedMessage, int, error) {
	b.mtx.Lock()
	defer b.mtx.Unlock()

	if limit > len(b.queue) {
		limit = len(b.queue)
	}

	batch := make([]buffer.BufferedMessage, limit)
	copy(batch, b.queue[:limit])

	for i := 0; i < limit; i++ {
		b.queue[i] = buffer.BufferedMessage{}
	}
	b.queue = b.queue[limit:]

	return batch, len(b.queue), nil
}

func (b *v1MemoryBuffer) Count(ctx context.Context) int {
	b.mtx.RLock()
	defer b.mtx.RUnlock()

	return len(b.queue)
}

func NewV1Buffer(opts ...buffer.Option) buffer.V1Buffer {
	options := buffer.NewOptions(opts...)

	b := &v1MemoryBuffer{
		options: options,
		recent:  make(map[uuid.UUID][]v1.Message),
		queue:   make([]buffer.BufferedMessage, 0),
		mtx:     sync.RWMutex{},
	}

	return b
}
