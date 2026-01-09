package v1session

import (
	"context"

	"github.com/google/uuid"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	v1session "github.com/w-h-a/gomento/internal/service/v1_session"
)

type v1Handler struct {
	service *v1session.V1Service
}

func (h *v1Handler) CreateTool() server.ServerTool {
	return server.ServerTool{
		Tool: mcp.Tool{
			Name:        "create_session",
			Description: "Create a new chat session. Can be standalone (orphan) or linked to a space (domain of knowledge).",
			InputSchema: mcp.ToolInputSchema{
				Type: "object",
				Properties: map[string]any{
					"space_id": map[string]string{"type": "string", "description": "Optional UUID of the space (domain of knowledge) to link this session to."},
				},
			},
		},
		Handler: h.create,
	}
}

func (h *v1Handler) create(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	// TODO: traces?

	args, ok := req.Params.Arguments.(map[string]any)
	if !ok {
		return mcp.NewToolResultError("invalid args"), nil
	}

	var sid *uuid.UUID
	if s, ok := args["space_id"].(string); ok && len(s) > 0 {
		id, err := uuid.Parse(s)
		if err != nil {
			return mcp.NewToolResultError("invalid space_id"), nil
		}
		sid = &id
	}

	s, err := h.service.Create(ctx, sid)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	return mcp.NewToolResultJSON(s)
}

func NewV1Handler(s *v1session.V1Service) *v1Handler {
	return &v1Handler{
		service: s,
	}
}
