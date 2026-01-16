package trace

import (
	"context"

	"github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"
	"github.com/w-h-a/gomento/internal/util"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

func Middleware(h mcpserver.ToolHandlerFunc) mcpserver.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		toolName := req.Params.Name

		if args, ok := req.Params.Arguments.(map[string]any); ok {
			if meta, ok := args["_meta"].(map[string]any); ok {
				if tp, ok := meta["traceparent"].(string); ok && len(tp) > 0 {
					carrier := propagation.MapCarrier{"traceparent": tp}
					ctx = otel.GetTextMapPropagator().Extract(ctx, carrier)
				}
			}
		}

		tracer := otel.Tracer("gomento-mcp")

		ctx, span := tracer.Start(
			ctx,
			"mcp.tool."+toolName,
			trace.WithSpanKind(trace.SpanKindServer),
			trace.WithAttributes(
				attribute.String("rpc.system", "mcp"),
				attribute.String("rpc.service", "gomento"),
				attribute.String("rpc.method", toolName),
			),
		)
		defer span.End()

		if span.SpanContext().IsValid() {
			ctx = util.WithTraceId(ctx, span.SpanContext().TraceID().String())
		}

		res, err := h(ctx, req)
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
		} else if res != nil && res.IsError {
			span.SetStatus(codes.Error, "tool reported error")
			span.SetAttributes(attribute.Bool("mcp.result.is_error", true))
		}

		return res, err
	}
}
