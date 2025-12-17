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

func (d *v1MemDispatcher) Subscribe(ctx context.Context, cb func(ctx context.Context, task *v1.Task) error, opts ...dispatcher.SubscribeOption) error {
	options := dispatcher.NewSubscribeOptions(opts...)

	// span
	slog.InfoContext(ctx, "subscribing to queue", "queue", options.Queue)

	d.mtx.Lock()
	q, ok := d.queues[options.Queue]
	if !ok {
		q = make(chan *v1.Task, 100)
		d.queues[options.Queue] = q
	}
	d.mtx.Unlock()

	d.wg.Add(1)
	go func() {
		defer d.wg.Done()

		for {
			select {
			case <-d.exit:
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

func (d *v1MemDispatcher) Publish(ctx context.Context, task *v1.Task, opts ...dispatcher.PublishOption) error {
	options := dispatcher.NewPublishOptions(opts...)

	// span
	slog.InfoContext(ctx, "publishing to queue", "data", task, "queue", options.Queue)

	d.mtx.Lock()
	q, ok := d.queues[options.Queue]
	if !ok {
		q = make(chan *v1.Task, 100)
		d.queues[options.Queue] = q
	}
	d.mtx.Unlock()

	select {
	case q <- task:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (d *v1MemDispatcher) CheckHealth(ctx context.Context) error {
	return nil
}

func (d *v1MemDispatcher) Close(ctx context.Context) error {
	done := make(chan struct{})

	d.once.Do(func() {
		close(d.exit)

		go func() {
			d.wg.Wait()
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

func (d *v1MemDispatcher) Queue(name string) <-chan *v1.Task {
	d.mtx.Lock()
	defer d.mtx.Unlock()

	q, ok := d.queues[name]
	if !ok {
		q = make(chan *v1.Task, 100)
		d.queues[name] = q
	}

	return q
}

func NewDispatcher(opts ...dispatcher.Option) *v1MemDispatcher {
	options := dispatcher.NewOptions(opts...)

	d := &v1MemDispatcher{
		options: options,
		queues:  map[string]chan *v1.Task{},
		mtx:     sync.RWMutex{},
		exit:    make(chan struct{}),
		wg:      sync.WaitGroup{},
		once:    sync.Once{},
	}

	return d
}
