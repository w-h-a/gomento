package dispatcher

import "context"

type Option func(*Options)

type Options struct {
	Location string
	Context  context.Context
}

func WithLocation(loc string) Option {
	return func(o *Options) {
		o.Location = loc
	}
}

func NewOptions(opts ...Option) Options {
	options := Options{
		Context: context.Background(),
	}

	for _, fn := range opts {
		fn(&options)
	}

	return options
}

type SubscribeOption func(o *SubscribeOptions)

type SubscribeOptions struct {
	Queue   string
	Context context.Context
}

func SubscribeWithQueue(queue string) SubscribeOption {
	return func(o *SubscribeOptions) {
		o.Queue = queue
	}
}

func NewSubscribeOptions(opts ...SubscribeOption) SubscribeOptions {
	options := SubscribeOptions{
		Context: context.Background(),
	}

	for _, fn := range opts {
		fn(&options)
	}

	return options
}

type PublishOption func(o *PublishOptions)

type PublishOptions struct {
	Queue   string
	Context context.Context
}

func PublishWithQueue(queue string) PublishOption {
	return func(o *PublishOptions) {
		o.Queue = queue
	}
}

func NewPublishOptions(opts ...PublishOption) PublishOptions {
	options := PublishOptions{
		Context: context.Background(),
	}

	for _, fn := range opts {
		fn(&options)
	}

	return options
}
