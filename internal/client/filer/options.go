package filer

import "context"

type Option func(*Options)

type Options struct {
	Endpoint       string
	PublicEndpoint string
	Region         string
	Container      string
	User           string
	Secret         string
	Context        context.Context
}

func WithEndpoint(endpoint string) Option {
	return func(o *Options) {
		o.Endpoint = endpoint
	}
}

func WithPublicEndpoint(endpoint string) Option {
	return func(o *Options) {
		o.PublicEndpoint = endpoint
	}
}

func WithRegion(region string) Option {
	return func(o *Options) {
		o.Region = region
	}
}

func WithContainer(container string) Option {
	return func(o *Options) {
		o.Container = container
	}
}

func WithUser(user string) Option {
	return func(o *Options) {
		o.User = user
	}
}

func WithSecret(secret string) Option {
	return func(o *Options) {
		o.Secret = secret
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
