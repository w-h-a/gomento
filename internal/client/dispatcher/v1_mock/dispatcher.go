package v1mock

import (
	"context"
	"sync"

	v1 "github.com/w-h-a/gomento/api/domain/v1"
	"github.com/w-h-a/gomento/internal/client/dispatcher"
)

type v1MockDispatcher struct {
	options dispatcher.Options
	jobs    []*v1.Job
	mtx     sync.RWMutex
}

func (d *v1MockDispatcher) Subscribe(ctx context.Context, cb func(ctx context.Context, job *v1.Job) error, opts ...dispatcher.SubscribeOption) error {
	return nil
}

func (d *v1MockDispatcher) Publish(ctx context.Context, job *v1.Job, opts ...dispatcher.PublishOption) error {
	d.mtx.Lock()
	defer d.mtx.Unlock()
	d.jobs = append(d.jobs, job)
	return nil
}

func (d *v1MockDispatcher) CheckHealth(ctx context.Context) error {
	return nil
}

func (d *v1MockDispatcher) Close(ctx context.Context) error {
	return nil
}

func (d *v1MockDispatcher) Jobs() []*v1.Job {
	d.mtx.RLock()
	defer d.mtx.RUnlock()
	cpy := make([]*v1.Job, len(d.jobs))
	copy(cpy, d.jobs)
	return cpy
}

func NewV1Dispatcher(opts ...dispatcher.Option) *v1MockDispatcher {
	options := dispatcher.NewOptions(opts...)

	d := &v1MockDispatcher{
		options: options,
		jobs:    []*v1.Job{},
		mtx:     sync.RWMutex{},
	}

	return d
}
