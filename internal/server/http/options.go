package http

import (
	"context"
	"net/http"

	"github.com/w-h-a/gomento/internal/server"
)

type middlewareKey struct{}

func WithMiddleware(ms ...func(h http.Handler) http.Handler) server.Option {
	return func(o *server.Options) {
		o.Context = context.WithValue(o.Context, middlewareKey{}, ms)
	}
}

func MiddlewareFrom(ctx context.Context) ([]func(h http.Handler) http.Handler, bool) {
	ms, ok := ctx.Value(middlewareKey{}).([]func(h http.Handler) http.Handler)
	return ms, ok
}
