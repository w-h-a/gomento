package v1memory

import (
	"context"
	"log/slog"
	"sync"

	v1 "github.com/w-h-a/gomento/api/domain/v1"
	"github.com/w-h-a/gomento/internal/client/dispatcher"
)

type v1MemDispatcher struct {
	options dispatcher.Options
	queues  map[string]chan *v1.Task
	mtx     sync.RWMutex
	exit    chan struct{}
	wg      sync.WaitGroup
	once    sync.Once
}

func (b *v1MemDispatcher) Subscribe(ctx context.Context, cb func(ctx context.Context, task *v1.Task) error, opts ...dispatcher.SubscribeOption) error {
	options := dispatcher.NewSubscribeOptions(opts...)

	// span
	slog.InfoContext(ctx, "subscribing to queue", "queue", options.Queue)

	b.mtx.Lock()
	q, ok := b.queues[options.Queue]
	if !ok {
		q = make(chan *v1.Task, 100)
		b.queues[options.Queue] = q
	}
	b.mtx.Unlock()

	b.wg.Add(1)
	go func() {
		defer b.wg.Done()

		for {
			select {
			case <-b.exit:
				return
			case data := <-q:
				if err := cb(context.Background(), data); err != nil {
					// span
					slog.ErrorContext(context.Background(), "failed to process incoming data", "data", data, "error", err)
				}
			}
		}
	}()

	return nil
}

func (b *v1MemDispatcher) Publish(ctx context.Context, task *v1.Task, opts ...dispatcher.PublishOption) error {
	options := dispatcher.NewPublishOptions(opts...)

	// span
	slog.InfoContext(ctx, "publishing to queue", "data", task, "queue", options.Queue)

	b.mtx.Lock()
	q, ok := b.queues[options.Queue]
	if !ok {
		q = make(chan *v1.Task, 100)
		b.queues[options.Queue] = q
	}
	b.mtx.Unlock()

	select {
	case q <- task:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (b *v1MemDispatcher) CheckHealth(ctx context.Context) error {
	return nil
}

func (b *v1MemDispatcher) Close(ctx context.Context) error {
	done := make(chan struct{})

	b.once.Do(func() {
		close(b.exit)

		go func() {
			b.wg.Wait()
			close(done)
		}()
	})

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-done:
		return nil
	}
}

func (b *v1MemDispatcher) Queue(name string) <-chan *v1.Task {
	b.mtx.Lock()
	defer b.mtx.Unlock()

	q, ok := b.queues[name]
	if !ok {
		q = make(chan *v1.Task, 100)
		b.queues[name] = q
	}

	return q
}

func NewDispatcher(opts ...dispatcher.Option) *v1MemDispatcher {
	options := dispatcher.NewOptions(opts...)

	b := &v1MemDispatcher{
		options: options,
		queues:  map[string]chan *v1.Task{},
		mtx:     sync.RWMutex{},
		exit:    make(chan struct{}),
		wg:      sync.WaitGroup{},
		once:    sync.Once{},
	}

	return b
}
