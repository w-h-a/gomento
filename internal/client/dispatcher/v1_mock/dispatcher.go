package v1mock

import (
	"context"
	"sync"

	v1 "github.com/w-h-a/gomento/api/domain/v1"
	"github.com/w-h-a/gomento/internal/client/dispatcher"
)

type v1MockDispatcher struct {
	options dispatcher.Options
	tasks   []*v1.Task
	mtx     sync.RWMutex
}

func (d *v1MockDispatcher) Subscribe(ctx context.Context, cb func(ctx context.Context, task *v1.Task) error, opts ...dispatcher.SubscribeOption) error {
	return nil
}

func (d *v1MockDispatcher) Publish(ctx context.Context, task *v1.Task, opts ...dispatcher.PublishOption) error {
	d.mtx.Lock()
	defer d.mtx.Unlock()
	d.tasks = append(d.tasks, task)
	return nil
}

func (d *v1MockDispatcher) CheckHealth(ctx context.Context) error {
	return nil
}

func (d *v1MockDispatcher) Close(ctx context.Context) error {
	return nil
}

func (d *v1MockDispatcher) Tasks() []*v1.Task {
	d.mtx.RLock()
	defer d.mtx.RUnlock()
	cpy := make([]*v1.Task, len(d.tasks))
	copy(cpy, d.tasks)
	return cpy
}

func NewV1Dispatcher(opts ...dispatcher.Option) *v1MockDispatcher {
	options := dispatcher.NewOptions(opts...)

	d := &v1MockDispatcher{
		options: options,
		tasks:   []*v1.Task{},
		mtx:     sync.RWMutex{},
	}

	return d
}
