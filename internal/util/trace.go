package util

import "context"

type traceKey struct{}

func WithTraceId(ctx context.Context, traceId string) context.Context {
	return context.WithValue(ctx, traceKey{}, traceId)
}

func TraceIdFrom(ctx context.Context) (string, bool) {
	val, ok := ctx.Value(traceKey{}).(string)
	return val, ok
}
