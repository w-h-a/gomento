package mcp

import (
	"context"

	mcpserver "github.com/mark3labs/mcp-go/server"
	"github.com/w-h-a/gomento/internal/server"
)

type toolMiddlewareKey struct{}

func WithToolMiddleware(ms ...func(mcpserver.ToolHandlerFunc) mcpserver.ToolHandlerFunc) server.Option {
	return func(o *server.Options) {
		o.Context = context.WithValue(o.Context, toolMiddlewareKey{}, ms)
	}
}

func ToolMiddlewareFrom(ctx context.Context) ([]func(mcpserver.ToolHandlerFunc) mcpserver.ToolHandlerFunc, bool) {
	ms, ok := ctx.Value(toolMiddlewareKey{}).([]func(mcpserver.ToolHandlerFunc) mcpserver.ToolHandlerFunc)
	return ms, ok
}

type resourceMiddlewareKey struct{}

func WithResourceMiddleware(ms ...func(mcpserver.ResourceHandlerFunc) mcpserver.ResourceHandlerFunc) server.Option {
	return func(o *server.Options) {
		o.Context = context.WithValue(o.Context, resourceMiddlewareKey{}, ms)
	}
}

func ResourceMiddlewareFrom(ctx context.Context) ([]func(mcpserver.ResourceHandlerFunc) mcpserver.ResourceHandlerFunc, bool) {
	ms, ok := ctx.Value(resourceMiddlewareKey{}).([]func(mcpserver.ResourceHandlerFunc) mcpserver.ResourceHandlerFunc)
	return ms, ok
}
