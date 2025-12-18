package service

import "context"

type Option func(*Options)

type Options struct {
	StartCallback func() error
	StopCallback  func(ctx context.Context) error
}

func WithStartCallback(cb func() error) Option {
	return func(o *Options) {
		o.StartCallback = cb
	}
}

func WithStopCallback(cb func(ctx context.Context) error) Option {
	return func(o *Options) {
		o.StopCallback = cb
	}
}

func NewOptions(opts ...Option) Options {
	options := Options{}

	for _, fn := range opts {
		fn(&options)
	}

	return options
}
