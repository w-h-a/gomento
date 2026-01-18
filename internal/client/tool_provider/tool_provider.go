package toolprovider

import (
	"context"

	v1 "github.com/w-h-a/gomento/api/tools/v1"
)

type ToolProvider interface {
	Start(ctx context.Context) error
	List(ctx context.Context) ([]v1.ToolDefinition, error)
	Call(ctx context.Context, name string, args map[string]any) (string, error)
}
