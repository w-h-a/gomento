package server

import "context"

type Option func(*Options)

type Options struct {
	Address string
	Name    string
	Version string
	Context context.Context
}

func WithAddress(addr string) Option {
	return func(o *Options) {
		o.Address = addr
	}
}

func WithName(name string) Option {
	return func(o *Options) {
		o.Name = name
	}
}

func WithVersion(version string) Option {
	return func(o *Options) {
		o.Version = version
	}
}

func NewOptions(opts ...Option) Options {
	options := Options{
		Address: ":0",
		Context: context.Background(),
	}

	for _, fn := range opts {
		fn(&options)
	}

	return options
}
