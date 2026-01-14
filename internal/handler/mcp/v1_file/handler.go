package v1file

import (
	"context"
	"errors"
	"strings"

	"github.com/google/uuid"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/w-h-a/gomento/internal/service"
	v1file "github.com/w-h-a/gomento/internal/service/v1_file"
)

type v1Handler struct {
	service *v1file.V1Service
}

func (h *v1Handler) UploadTool() server.ServerTool {
	return server.ServerTool{
		Tool: mcp.Tool{
			Name:        "upload_file",
			Description: "Upload a text file globally or to a space (domain of knowledge). Use this to persist code, configurations, or documentation that you generate or the user provides, ensuring it is indexed in memory. This tool performs an upsert.",
			InputSchema: mcp.ToolInputSchema{
				Type: "object",
				Properties: map[string]any{
					"space_id": map[string]string{"type": "string", "description": "The UUID of the space (domain of knowledge) that this file belongs to."},
					"path":     map[string]string{"type": "string", "description": "The virtual file path (e.g., '/src/main.go'). Use strictly unix-style paths. Default path is '/'"},
					"filename": map[string]string{"type": "string", "description": "The name of the file."},
					"content":  map[string]string{"type": "string", "description": "The full text content of the file."},
				},
				Required: []string{"filename", "content"},
			},
		},
		Handler: h.upload,
	}
}

func (h *v1Handler) upload(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	// TODO: tracing?

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

	path := "/"
	if p, ok := args["path"].(string); ok {
		path = p
	}

	var filename string
	if f, ok := args["filename"].(string); ok {
		filename = f
	} else {
		return mcp.NewToolResultError("missing 'filename'"), nil
	}

	var content string
	if c, ok := args["content"].(string); ok {
		content = c
	} else {
		return mcp.NewToolResultError("missing file 'content'"), nil
	}

	f, err := h.service.Upload(ctx, v1file.CreateFileInput{
		SpaceId:  sid,
		Path:     path,
		Filename: filename,
		MimeType: "text/plain",
		Size:     int64(len(content)),
		Reader:   strings.NewReader(content),
	})
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	return mcp.NewToolResultJSON(f)
}

func (h *v1Handler) ListTool() server.ServerTool {
	return server.ServerTool{
		Tool: mcp.Tool{
			Name:        "list_files",
			Description: "List files. Can be filtered by space_id (to see files in a specific domain) or path_prefix (to see files in a specific directory).",
			InputSchema: mcp.ToolInputSchema{
				Type: "object",
				Properties: map[string]any{
					"space_id":    map[string]string{"type": "string", "description": "The UUID of the space to filter by. If omitted, lists global files."},
					"path_prefix": map[string]string{"type": "string", "description": "Filter files starting with this path (e.g., 'src/')."},
				},
			},
		},
		Handler: h.list,
	}
}

func (h *v1Handler) list(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	// TODO: tracing?

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

	var pathPrefix string
	if p, ok := args["path_prefix"].(string); ok {
		pathPrefix = p
	}

	out, err := h.service.List(ctx, v1file.ListFilesInput{
		SpaceId:    sid,
		PathPrefix: pathPrefix,
	})
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	return mcp.NewToolResultJSON(out)
}

func (h *v1Handler) GetTool() server.ServerTool {
	return server.ServerTool{
		Tool: mcp.Tool{
			Name:        "get_file",
			Description: "Get file metadata by ID. Optionally returns a short-lived public URL to download the content",
			InputSchema: mcp.ToolInputSchema{
				Type: "object",
				Properties: map[string]any{
					"file_id":  map[string]string{"type": "string", "description": "The UUID of the file."},
					"with_url": map[string]string{"type": "boolean", "description": "Whether to return a short-lived public URL to download the content."},
				},
				Required: []string{"file_id"},
			},
		},
		Handler: h.get,
	}
}

func (h *v1Handler) get(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	// TODO: tracing?

	args, ok := req.Params.Arguments.(map[string]any)
	if !ok {
		return mcp.NewToolResultError("invalid args"), nil
	}

	fileId, ok := args["file_id"].(string)
	if !ok {
		return mcp.NewToolResultError("missing 'file_id'"), nil
	}

	id, err := uuid.Parse(fileId)
	if err != nil {
		return mcp.NewToolResultError("invalid file_id"), nil
	}

	withUrl := false
	if u, ok := args["with_url"].(bool); ok {
		withUrl = u
	}

	file, url, err := h.service.Get(ctx, id, withUrl)
	if err != nil {
		if errors.Is(err, service.ErrFileNotFound) {
			return mcp.NewToolResultError("file not found"), nil
		}
		return mcp.NewToolResultError(err.Error()), nil
	}

	rsp := map[string]any{
		"file": file,
	}

	if len(url) > 0 {
		rsp["public_url"] = url
	}

	return mcp.NewToolResultJSON(rsp)
}

func (h *v1Handler) ConnectToSpaceTool() server.ServerTool {
	return server.ServerTool{
		Tool: mcp.Tool{
			Name:        "connect_file_to_space",
			Description: "Connect a file to a space (domain of knowledge). Use for for moving global/orphan files into a specific domain of knowledge.",
			InputSchema: mcp.ToolInputSchema{
				Type: "object",
				Properties: map[string]any{
					"file_id":  map[string]string{"type": "string", "description": "The UUID of the file to move."},
					"space_id": map[string]string{"type": "string", "description": "The UUID of the destination space."},
				},
				Required: []string{"file_id", "space_id"},
			},
		},
		Handler: h.connectToSpace,
	}
}

func (h *v1Handler) connectToSpace(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	// TODO: tracing?

	args, ok := req.Params.Arguments.(map[string]any)
	if !ok {
		return mcp.NewToolResultError("invalid args"), nil
	}

	fileIdStr, ok := args["file_id"].(string)
	if !ok {
		return mcp.NewToolResultError("missing 'file_id'"), nil
	}

	fileId, err := uuid.Parse(fileIdStr)
	if err != nil {
		return mcp.NewToolResultError("invalid file_id"), nil
	}

	spaceIdStr, ok := args["space_id"].(string)
	if !ok {
		return mcp.NewToolResultError("missing 'space_id'"), nil
	}

	spaceId, err := uuid.Parse(spaceIdStr)
	if err != nil {
		return mcp.NewToolResultError("invalid space_id"), nil
	}

	if err := h.service.ConnectToSpace(ctx, fileId, spaceId); err != nil {
		if errors.Is(err, service.ErrFileNotFound) {
			return mcp.NewToolResultError("file not found"), nil
		}
		if errors.Is(err, service.ErrSpaceNotFound) {
			return mcp.NewToolResultError("space not found"), nil
		}
		return mcp.NewToolResultError(err.Error()), nil
	}

	return mcp.NewToolResultJSON(map[string]string{"status": "connected"})
}

func NewV1Handler(s *v1file.V1Service) *v1Handler {
	return &v1Handler{
		service: s,
	}
}
