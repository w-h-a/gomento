package buffer

import "context"

type Option func(*Options)

type Options struct {
	Location string
	MaxHist  int
	Context  context.Context
}

func WithLocation(location string) Option {
	return func(o *Options) {
		o.Location = location
	}
}

func WithMaxHist(maxHist int) Option {
	return func(o *Options) {
		o.MaxHist = maxHist
	}
}

func NewOptions(opts ...Option) Options {
	options := Options{
		MaxHist: 50,
		Context: context.Background(),
	}
	for _, fn := range opts {
		fn(&options)
	}

	return options
}
