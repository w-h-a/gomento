package trace

import (
	"net/http"

	"github.com/w-h-a/gomento/internal/util"
	"go.opentelemetry.io/otel/trace"
)

func Middleware(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		span := trace.SpanFromContext(ctx)

		if span.SpanContext().IsValid() {
			ctx = util.WithTraceId(ctx, span.SpanContext().TraceID().String())
		}

		h.ServeHTTP(w, r.WithContext(ctx))
	})
}
