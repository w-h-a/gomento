package dispatcher

import (
	"context"

	v1 "github.com/w-h-a/gomento/api/domain/v1"
)

type V1Dispatcher interface {
	Subscribe(ctx context.Context, cb func(ctx context.Context, job *v1.Job) error, opts ...SubscribeOption) error
	Publish(ctx context.Context, job *v1.Job, opts ...PublishOption) error
	CheckHealth(ctx context.Context) error
	Close(ctx context.Context) error
}
