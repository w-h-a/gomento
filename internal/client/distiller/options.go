package distiller

import "context"

type Option func(*Options)

type Options struct {
	ApiKey  string
	Model   string
	Context context.Context
}

func WithApiKey(key string) Option {
	return func(o *Options) {
		o.ApiKey = key
	}
}

func WithModel(model string) Option {
	return func(o *Options) {
		o.Model = model
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
