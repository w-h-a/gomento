package dispatcher

import (
	"context"

	v1 "github.com/w-h-a/gomento/api/domain/v1"
)

type V1Dispatcher interface {
	Subscribe(ctx context.Context, cb func(ctx context.Context, task *v1.Task) error, opts ...SubscribeOption) error
	Publish(ctx context.Context, task *v1.Task, opts ...PublishOption) error
}
