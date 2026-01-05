package mock

import (
	"context"

	"github.com/w-h-a/gomento/internal/client/embedder"
)

type errorKey struct{}

func WithError(e error) embedder.Option {
	return func(o *embedder.Options) {
		o.Context = context.WithValue(o.Context, errorKey{}, e)
	}
}

func ErrorFrom(ctx context.Context) (error, bool) {
	e, ok := ctx.Value(errorKey{}).(error)
	return e, ok
}
