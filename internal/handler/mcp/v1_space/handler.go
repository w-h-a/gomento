package v1space

import (
	"context"

	"github.com/google/uuid"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/w-h-a/gomento/internal/service"
	v1space "github.com/w-h-a/gomento/internal/service/v1_space"
)

type v1Handler struct {
	service *v1space.V1Service
}

func (h *v1Handler) CreateTool() server.ServerTool {
	return server.ServerTool{
		Tool: mcp.Tool{
			Name:        "create_space",
			Description: "Create a new space (domain of knowledge).",
			InputSchema: mcp.ToolInputSchema{
				Type: "object",
				Properties: map[string]any{
					"name": map[string]string{"type": "string", "description": "Name of the space."},
				},
				Required: []string{"name"},
			},
		},
		Handler: h.create,
	}
}

func (h *v1Handler) create(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	// TODO: traces?

	args, ok := req.Params.Arguments.(map[string]any)
	if !ok {
		return mcp.NewToolResultError("invalid arguments"), nil
	}

	name, ok := args["name"].(string)
	if !ok {
		return mcp.NewToolResultError("missing name"), nil
	}

	s, err := h.service.Create(ctx, name)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	return mcp.NewToolResultJSON(s)
}

func (h *v1Handler) ListSpacesTool() server.ServerTool {
	return server.ServerTool{
		Tool: mcp.Tool{
			Name:        "list_spaces",
			Description: "List all spaces (domains of knowledge).",
			InputSchema: mcp.ToolInputSchema{
				Type: "object",
			},
		},
		Handler: h.listSpaces,
	}
}

func (h *v1Handler) listSpaces(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	// TODO: tracing?

	out, err := h.service.List(ctx)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	return mcp.NewToolResultJSON(out)
}

func (h *v1Handler) GetSpaceTool() server.ServerTool {
	return server.ServerTool{
		Tool: mcp.Tool{
			Name:        "get_space",
			Description: "Get details about a specific space.",
			InputSchema: mcp.ToolInputSchema{
				Type: "object",
				Properties: map[string]any{
					"space_id": map[string]string{"type": "string", "description": "The UUID of the space."},
				},
				Required: []string{"space_id"},
			},
		},
		Handler: h.getSpace,
	}
}

func (h *v1Handler) getSpace(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	// TODO: tracing?

	args, ok := req.Params.Arguments.(map[string]any)
	if !ok {
		return mcp.NewToolResultError("invalid arguments"), nil
	}

	spaceId, ok := args["space_id"].(string)
	if !ok {
		return mcp.NewToolResultError("missing space_id"), nil
	}

	id, err := uuid.Parse(spaceId)
	if err != nil {
		return mcp.NewToolResultError("invalid space_id format"), nil
	}

	space, err := h.service.Get(ctx, id)
	if err != nil {
		if err == service.ErrSpaceNotFound {
			return mcp.NewToolResultError("space not found"), nil
		}
		return mcp.NewToolResultError(err.Error()), nil
	}

	return mcp.NewToolResultJSON(space)
}

func (h *v1Handler) SearchSkillsTool() server.ServerTool {
	return server.ServerTool{
		Tool: mcp.Tool{
			Name:        "search_skills",
			Description: "Semantically search for skills within a space (domain of knowledge).",
			InputSchema: mcp.ToolInputSchema{
				Type: "object",
				Properties: map[string]any{
					"space_id": map[string]string{"type": "string", "description": "The UUID of the space."},
					"query":    map[string]string{"type": "string", "description": "The natural language search query."},
				},
				Required: []string{"space_id", "query"},
			},
		},
		Handler: h.searchSkills,
	}
}

func (h *v1Handler) searchSkills(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	// TODO: tracing?

	args, ok := req.Params.Arguments.(map[string]any)
	if !ok {
		return mcp.NewToolResultError("invalid arguments"), nil
	}

	spaceId, ok := args["space_id"].(string)
	if !ok {
		return mcp.NewToolResultError("missing space_id"), nil
	}

	id, err := uuid.Parse(spaceId)
	if err != nil {
		return mcp.NewToolResultError("invalid space_id format"), nil
	}

	query, ok := args["query"].(string)
	if !ok {
		return mcp.NewToolResultError("missing query"), nil
	}

	skills, err := h.service.SearchSkills(ctx, id, query)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	return mcp.NewToolResultJSON(skills)
}

func (h *v1Handler) SearchMessagesTool() server.ServerTool {
	return server.ServerTool{
		Tool: mcp.Tool{
			Name:        "search_messages",
			Description: "Semantically search for messages within a space (domain of knoweldge).",
			InputSchema: mcp.ToolInputSchema{
				Type: "object",
				Properties: map[string]any{
					"space_id": map[string]string{"type": "string", "description": "The UUID of the space."},
					"query":    map[string]string{"type": "string", "description": "The natural language search query."},
				},
				Required: []string{"space_id", "query"},
			},
		},
		Handler: h.searchMessages,
	}
}

func (h *v1Handler) searchMessages(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	// TODO: tracing?

	args, ok := req.Params.Arguments.(map[string]any)
	if !ok {
		return mcp.NewToolResultError("invalid arguments"), nil
	}

	spaceId, ok := args["space_id"].(string)
	if !ok {
		return mcp.NewToolResultError("missing space_id"), nil
	}

	id, err := uuid.Parse(spaceId)
	if err != nil {
		return mcp.NewToolResultError("invalid space_id format"), nil
	}

	query, ok := args["query"].(string)
	if !ok {
		return mcp.NewToolResultError("missing query"), nil
	}

	msgs, err := h.service.SearchMessages(ctx, id, query)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	return mcp.NewToolResultJSON(msgs)
}

func NewV1Handler(s *v1space.V1Service) *v1Handler {
	return &v1Handler{
		service: s,
	}
}
