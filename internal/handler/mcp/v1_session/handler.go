package v1session

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/google/uuid"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/w-h-a/gomento/internal/service"
	v1session "github.com/w-h-a/gomento/internal/service/v1_session"
	"github.com/w-h-a/gomento/internal/util"
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

func (h *v1Handler) ListTool() server.ServerTool {
	return server.ServerTool{
		Tool: mcp.Tool{
			Name:        "list_sessions",
			Description: "List all chat sessions. Can be filtered by space_id",
			InputSchema: mcp.ToolInputSchema{
				Type: "object",
				Properties: map[string]any{
					"space_id": map[string]string{"type": "string", "description": "The UUID of the space to filter by."},
				},
			},
		},
		Handler: h.list,
	}
}

func (h *v1Handler) list(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
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

	out, err := h.service.List(ctx, v1session.ListSessionsInput{
		SpaceId: sid,
	})
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	return mcp.NewToolResultJSON(out)
}

func (h *v1Handler) GetTool() server.ServerTool {
	return server.ServerTool{
		Tool: mcp.Tool{
			Name:        "get_session",
			Description: "Get a chat session by ID.",
			InputSchema: mcp.ToolInputSchema{
				Type: "object",
				Properties: map[string]any{
					"session_id": map[string]string{"type": "string", "description": "The UUID of the session to retrieve."},
				},
				Required: []string{
					"session_id",
				},
			},
		},
		Handler: h.get,
	}
}

func (h *v1Handler) get(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args, ok := req.Params.Arguments.(map[string]any)
	if !ok {
		return mcp.NewToolResultError("invalid args"), nil
	}

	idStr, ok := args["session_id"].(string)
	if !ok {
		return mcp.NewToolResultError("invalid session_id"), nil
	}

	id, err := uuid.Parse(idStr)
	if err != nil {
		return mcp.NewToolResultError("invalid session_id"), nil
	}

	sess, err := h.service.Get(ctx, id)
	if err != nil {
		if errors.Is(err, service.ErrSessionNotFound) {
			return mcp.NewToolResultError("session not found"), nil
		}
		return mcp.NewToolResultError(err.Error()), nil
	}

	return mcp.NewToolResultJSON(sess)
}

func (h *v1Handler) ConnectToSpaceTool() server.ServerTool {
	return server.ServerTool{
		Tool: mcp.Tool{
			Name:        "connect_session_to_space",
			Description: "Connect an existing chat session to a space (domain of knowledge). Useful for organizing ad-hoc sessions into a domain.",
			InputSchema: mcp.ToolInputSchema{
				Type: "object",
				Properties: map[string]any{
					"session_id": map[string]string{"type": "string", "description": "The UUID of the session."},
					"space_id":   map[string]string{"type": "string", "description": "The UUID of the space to connect to."},
				},
				Required: []string{
					"session_id",
					"space_id",
				},
			},
		},
		Handler: h.connectToSpace,
	}
}

func (h *v1Handler) connectToSpace(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args, ok := req.Params.Arguments.(map[string]any)
	if !ok {
		return mcp.NewToolResultError("invalid args"), nil
	}

	sessIdStr, ok := args["session_id"].(string)
	if !ok {
		return mcp.NewToolResultError("invalid session_id"), nil
	}

	sessId, err := uuid.Parse(sessIdStr)
	if err != nil {
		return mcp.NewToolResultError("invalid session_id"), nil
	}

	spaceIdStr, ok := args["space_id"].(string)
	if !ok {
		return mcp.NewToolResultError("invalid space_id"), nil
	}

	spaceId, err := uuid.Parse(spaceIdStr)
	if err != nil {
		return mcp.NewToolResultError("invalid space_id"), nil
	}

	if err := h.service.ConnectToSpace(ctx, sessId, spaceId); err != nil {
		if errors.Is(err, service.ErrSessionNotFound) {
			return mcp.NewToolResultError("session not found"), nil
		}
		if errors.Is(err, service.ErrSpaceNotFound) {
			return mcp.NewToolResultError("space not found"), nil
		}
		return mcp.NewToolResultError(err.Error()), nil
	}

	return mcp.NewToolResultJSON(map[string]string{"status": "connected"})
}

func (h *v1Handler) AddMessageTool() server.ServerTool {
	return server.ServerTool{
		Tool: mcp.Tool{
			Name:        "add_message",
			Description: "Add a new message to a chat session. Supports text and multiple file attachments.",
			InputSchema: mcp.ToolInputSchema{
				Type: "object",
				Properties: map[string]any{
					"session_id": map[string]string{"type": "string", "description": "The UUID of the session to add the message to."},
					"role":       map[string]string{"type": "string", "description": "'user' or 'assistant'."},
					"parts": map[string]any{
						"type":        "array",
						"description": "List of message parts.",
						"items": map[string]any{
							"type": "object",
							"properties": map[string]any{
								"type":       map[string]string{"type": "string", "description": "'text' or 'file'."},
								"text":       map[string]string{"type": "string", "description": "Content for text parts."},
								"file_field": map[string]string{"type": "string", "description": "Key in the 'files' map for file parts."},
							},
						},
					},
					"files": map[string]any{
						"type":        "object",
						"description": "Map of filename -> content strings. Used if parts contain file references.",
						"additionalProperties": map[string]string{
							"type": "string",
						},
					},
				},
				Required: []string{
					"session_id",
					"role",
					"parts",
				},
			},
		},
		Handler: h.addMessage,
	}
}

func (h *v1Handler) addMessage(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args, ok := req.Params.Arguments.(map[string]any)
	if !ok {
		return mcp.NewToolResultError("invalid args"), nil
	}

	sessIdStr, ok := args["session_id"].(string)
	if !ok {
		return mcp.NewToolResultError("invalid session_id"), nil
	}

	sessId, err := uuid.Parse(sessIdStr)
	if err != nil {
		return mcp.NewToolResultError("invalid session_id"), nil
	}

	role, ok := args["role"].(string)
	if !ok {
		return mcp.NewToolResultError("invalid role"), nil
	}

	partsRaw, ok := args["parts"].([]any)
	if !ok {
		return mcp.NewToolResultError("missing parts"), nil
	}

	partsBytes, err := json.Marshal(partsRaw)
	if err != nil {
		return mcp.NewToolResultError("invalid parts"), nil
	}

	var parts []v1session.PartInput
	if err := json.Unmarshal(partsBytes, &parts); err != nil {
		return mcp.NewToolResultError("invalid parts structure"), nil
	}

	inputFiles := make(map[string]v1session.InputFile)

	if filesRaw, ok := args["files"].(map[string]any); ok && len(filesRaw) > 0 {
		for name, file := range filesRaw {
			content, ok := file.(string)
			if !ok {
				continue
			}

			reader := strings.NewReader(content)

			inputFiles[name] = v1session.InputFile{
				Filename:    name,
				ContentType: "text/plain",
				Size:        int64(len(content)),
				Reader:      reader,
			}
		}
	}

	input := v1session.SendMessageInput{
		SessionId: sessId,
		Role:      role,
		Parts:     parts,
		Files:     inputFiles,
	}

	msg, err := h.service.AddMessage(ctx, input)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	return mcp.NewToolResultJSON(msg)
}

func (h *v1Handler) ListMessagesTool() server.ServerTool {
	return server.ServerTool{
		Tool: mcp.Tool{
			Name:        "list_messages",
			Description: "List messages in a session with pagination.",
			InputSchema: mcp.ToolInputSchema{
				Type: "object",
				Properties: map[string]any{
					"session_id":            map[string]string{"type": "string", "description": "The UUID of the session to list messages for."},
					"limit":                 map[string]string{"type": "integer", "description": "Maximum number of messages to return (default is 20)."},
					"cursor":                map[string]string{"type": "string", "description": "Cursor for pagination."},
					"with_asset_public_url": map[string]string{"type": "boolean", "description": "Include presigned URLs for assets."},
				},
				Required: []string{"session_id"},
			},
		},
		Handler: h.listMessages,
	}
}

func (h *v1Handler) listMessages(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args, ok := req.Params.Arguments.(map[string]any)
	if !ok {
		return mcp.NewToolResultError("invalid args"), nil
	}

	sessId, ok := args["session_id"].(string)
	if !ok {
		return mcp.NewToolResultError("missing session_id"), nil
	}

	id, err := uuid.Parse(sessId)
	if err != nil {
		return mcp.NewToolResultError("invalid session_id"), nil
	}

	var limit int
	if l, ok := args["limit"].(float64); ok && l > 0 {
		limit = int(l)
	}

	cursor, ok := args["cursor"].(string)
	if !ok {
		cursor = ""
	}

	withPublicUrl, ok := args["with_asset_public_url"].(bool)
	if !ok {
		withPublicUrl = false
	}

	out, err := h.service.ListMessages(ctx, v1session.ListMessagesInput{
		SessionId:          id,
		Limit:              limit,
		Cursor:             cursor,
		WithAssetPublicUrl: withPublicUrl,
	})
	if err != nil {
		if errors.Is(err, util.ErrInvalidCursor) {
			return mcp.NewToolResultError("invalid cursor format"), nil
		}
		return mcp.NewToolResultError(err.Error()), nil
	}

	return mcp.NewToolResultJSON(out)
}

func (h *v1Handler) ListTasksTool() server.ServerTool {
	return server.ServerTool{
		Tool: mcp.Tool{
			Name:        "list_tasks",
			Description: "List the tasks extracted from the session so far.",
			InputSchema: mcp.ToolInputSchema{
				Type: "object",
				Properties: map[string]any{
					"session_id": map[string]string{"type": "string", "description": "The UUID of the session to list tasks for."},
					"status":     map[string]string{"type": "string", "description": "Filter by status (e.g., 'pending', 'running', 'success', 'failed')."},
				},
				Required: []string{
					"session_id",
				},
			},
		},
		Handler: h.listTasks,
	}
}

func (h *v1Handler) listTasks(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args, ok := req.Params.Arguments.(map[string]any)
	if !ok {
		return mcp.NewToolResultError("invalid args"), nil
	}

	sessId, ok := args["session_id"].(string)
	if !ok {
		return mcp.NewToolResultError("missing session_id"), nil
	}

	id, err := uuid.Parse(sessId)
	if err != nil {
		return mcp.NewToolResultError("invalid session_id"), nil
	}

	var status *string
	if s, ok := args["status"].(string); ok && s != "" {
		status = &s
	}

	out, err := h.service.ListTasks(ctx, v1session.ListTasksInput{
		SessionId: id,
		Status:    status,
	})
	if err != nil {
		if errors.Is(err, service.ErrSessionNotFound) {
			return mcp.NewToolResultError("session not found"), nil
		}
		return mcp.NewToolResultError(err.Error()), nil
	}

	return mcp.NewToolResultJSON(out)
}

func (h *v1Handler) ExtractTasksTool() server.ServerTool {
	return server.ServerTool{
		Tool: mcp.Tool{
			Name:        "extract_tasks",
			Description: "Trigger a background job to analyze the session history and update the task list (Extract).",
			InputSchema: mcp.ToolInputSchema{
				Type: "object",
				Properties: map[string]any{
					"session_id": map[string]string{"type": "string", "description": "The UUID of the session to extract tasks for."},
				},
				Required: []string{
					"session_id",
				},
			},
		},
		Handler: h.extractTasks,
	}
}

func (h *v1Handler) extractTasks(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args, ok := req.Params.Arguments.(map[string]any)
	if !ok {
		return mcp.NewToolResultError("invalid args"), nil
	}

	sessId, ok := args["session_id"].(string)
	if !ok {
		return mcp.NewToolResultError("missing session_id"), nil
	}

	id, err := uuid.Parse(sessId)
	if err != nil {
		return mcp.NewToolResultError("invalid session_id"), nil
	}

	if err := h.service.ExtractTasks(ctx, id); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	return mcp.NewToolResultJSON(map[string]string{"status": "extraction_initiated"})
}

func (h *v1Handler) DistillSkillTool() server.ServerTool {
	return server.ServerTool{
		Tool: mcp.Tool{
			Name:        "distill_skill",
			Description: "Trigger a background job to distill the session into a reusable skill (Distill).",
			InputSchema: mcp.ToolInputSchema{
				Type: "object",
				Properties: map[string]any{
					"session_id": map[string]string{"type": "string", "description": "The UUID of the session to distill into a reusable skill."},
				},
				Required: []string{"session_id"},
			},
		},
		Handler: h.distillSkill,
	}
}

func (h *v1Handler) distillSkill(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args, ok := req.Params.Arguments.(map[string]any)
	if !ok {
		return mcp.NewToolResultError("invalid args"), nil
	}

	sessId, ok := args["session_id"].(string)
	if !ok {
		return mcp.NewToolResultError("missing session_id"), nil
	}

	id, err := uuid.Parse(sessId)
	if err != nil {
		return mcp.NewToolResultError("invalid session_id"), nil
	}

	if err := h.service.DistillSkill(ctx, id); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	return mcp.NewToolResultJSON(map[string]string{"status": "distillation_initiated"})
}

func NewV1Handler(s *v1session.V1Service) *v1Handler {
	return &v1Handler{
		service: s,
	}
}
