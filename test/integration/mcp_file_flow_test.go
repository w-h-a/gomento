package integration

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/google/uuid"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	v1file "github.com/w-h-a/gomento/internal/service/v1_file"
)

func TestAPI_Mcp_File_Flow(t *testing.T) {
	if len(os.Getenv("INTEGRATION")) == 0 {
		t.Log("SKIPPING INTEGRATION TEST")
		return
	}

	client, db, s3Client := setupMcpServer(t)
	ctx := context.Background()

	// ==========================================
	// Scenario 1: Global File Lifecycle
	// ==========================================
	t.Log("Step 1: Uploading a Global File via MCP")

	toolName := "upload_file"

	args := map[string]any{
		"filename": "config.yaml",
		"content":  "env: global",
		"path":     "/etc",
	}

	result, err := client.CallTool(ctx, mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name:      toolName,
			Arguments: args,
		},
	})
	require.NoError(t, err)
	require.False(t, result.IsError)
	require.NotEmpty(t, result.Content)

	textContent, ok := result.Content[0].(mcp.TextContent)
	require.True(t, ok)

	var globalFile map[string]any
	err = json.Unmarshal([]byte(textContent.Text), &globalFile)
	require.NoError(t, err)

	globalId := globalFile["id"].(string)
	assert.Nil(t, globalFile["space_id"], "Global file should not have a space_id")
	assert.Equal(t, "config.yaml", globalFile["filename"])

	// Verification: DB Persistence
	var count int
	err = db.QueryRow("SELECT count(*) FROM files WHERE id = $1 AND space_id IS NULL", globalId).Scan(&count)
	assert.NoError(t, err)
	assert.Equal(t, 1, count, "Global file must exist in DB")

	// Verification: S3 Persistence
	var assetPath string
	err = db.QueryRow("SELECT path FROM assets WHERE id = $1", globalFile["asset_id"]).Scan(&assetPath)
	assert.NoError(t, err)
	_, err = s3Client.HeadObject(context.Background(), &s3.HeadObjectInput{
		Bucket: aws.String(TEST_BUCKET),
		Key:    aws.String(assetPath),
	})
	assert.NoError(t, err, "File must exist in S3 bucket")

	// ==========================================
	// Scenario 2: Space File Lifecycle & Isolation
	// ==========================================
	t.Log("Step 2: Uploading a Space-Scoped File via MCP")

	// Create Space
	spaceId := uuid.New()
	_, err = db.Exec("INSERT INTO spaces (id, name, created_at) VALUES ($1, $2, NOW())", spaceId, "DevOps Space")
	require.NoError(t, err)

	// Upload File to Space
	args = map[string]any{
		"filename": "config.yaml",
		"content":  "env: local",
		"path":     "/etc",
		"space_id": spaceId.String(),
	}

	result, err = client.CallTool(ctx, mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name:      toolName,
			Arguments: args,
		},
	})
	require.NoError(t, err)
	require.False(t, result.IsError)
	require.NotEmpty(t, result.Content)

	textContent, ok = result.Content[0].(mcp.TextContent)
	require.True(t, ok)

	var spaceFile map[string]any
	err = json.Unmarshal([]byte(textContent.Text), &spaceFile)
	require.NoError(t, err)

	assert.Equal(t, spaceId.String(), spaceFile["space_id"])
	assert.NotEqual(t, globalId, spaceFile["id"], "Space file should have distinct ID from global file")

	err = db.QueryRow("SELECT count(*) FROM files WHERE id = $1 AND space_id = $2", spaceFile["id"], spaceId).Scan(&count)
	assert.NoError(t, err)
	assert.Equal(t, 1, count, "Space file must exist in DB and be linked to space")

	// ==========================================
	// Scenario 3: Listing & Filtering
	// ==========================================
	t.Log("Step 3: Verifying List Isolation via MCP")

	toolName = "list_files"

	// List Global Only
	result, err = client.CallTool(ctx, mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name:      toolName,
			Arguments: map[string]any{},
		},
	})
	require.NoError(t, err)
	require.False(t, result.IsError)
	require.NotEmpty(t, result.Content)

	textContent, ok = result.Content[0].(mcp.TextContent)
	require.True(t, ok)

	var globalList v1file.ListFilesOutput
	err = json.Unmarshal([]byte(textContent.Text), &globalList)
	require.NoError(t, err)

	assert.Len(t, globalList.Items, 1)
	assert.Equal(t, globalId, globalList.Items[0].Id.String())

	// List Space Only
	result, err = client.CallTool(ctx, mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: toolName,
			Arguments: map[string]any{
				"space_id": spaceId.String(),
			},
		},
	})
	require.NoError(t, err)
	require.False(t, result.IsError)
	require.NotEmpty(t, result.Content)

	textContent, ok = result.Content[0].(mcp.TextContent)
	require.True(t, ok)

	var spaceList v1file.ListFilesOutput
	err = json.Unmarshal([]byte(textContent.Text), &spaceList)
	require.NoError(t, err)

	assert.Len(t, spaceList.Items, 1)
	assert.Equal(t, spaceFile["id"], spaceList.Items[0].Id.String())

	// ==========================================
	// Scenario 4: Get By ID & Presigned URL
	// ==========================================
	t.Log("Step 4: Fetching File with Presigned URL via MCP")

	toolName = "get_file"

	result, err = client.CallTool(ctx, mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: toolName,
			Arguments: map[string]any{
				"file_id":  globalId,
				"with_url": true,
			},
		},
	})
	require.NoError(t, err)
	require.False(t, result.IsError)
	require.NotEmpty(t, result.Content)

	textContent, ok = result.Content[0].(mcp.TextContent)
	require.True(t, ok)

	var getRes map[string]any
	json.Unmarshal([]byte(textContent.Text), &getRes)

	f := getRes["file"].(map[string]any)
	assert.Equal(t, globalId, f["id"])
	assert.NotEmpty(t, getRes["public_url"])
	assert.Contains(t, getRes["public_url"], TEST_BUCKET)

	// ==========================================
	// Scenario 5: Ad-Hoc Connection (Moving Global to Space)
	// ==========================================
	t.Log("Step 5: Connecting Global File to Space via MCP")

	// 1. Upload new orphan
	toolName = "upload_file"

	result, err = client.CallTool(ctx, mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: toolName,
			Arguments: map[string]any{
				"filename": "orphan.txt",
				"content":  "orphan data",
				"path":     "/",
			},
		},
	})
	require.NoError(t, err)
	require.False(t, result.IsError)
	require.NotEmpty(t, result.Content)

	textContent, ok = result.Content[0].(mcp.TextContent)
	require.True(t, ok)

	var orphanFile map[string]any
	json.Unmarshal([]byte(textContent.Text), &orphanFile)
	orphanId := orphanFile["id"].(string)

	// 2. Connect
	toolName = "connect_file_to_space"

	result, err = client.CallTool(ctx, mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: toolName,
			Arguments: map[string]any{
				"file_id":  orphanId,
				"space_id": spaceId.String(),
			},
		},
	})
	require.NoError(t, err)
	require.False(t, result.IsError)

	// 3. Verify it moved
	// Check Global List
	toolName = "list_files"

	result, err = client.CallTool(ctx, mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name:      toolName,
			Arguments: map[string]any{},
		},
	})
	require.NoError(t, err)
	require.False(t, result.IsError)
	require.NotEmpty(t, result.Content)

	textContent, ok = result.Content[0].(mcp.TextContent)
	require.True(t, ok)

	json.Unmarshal([]byte(textContent.Text), &globalList)
	foundOrphanInGlobal := false
	for _, item := range globalList.Items {
		if item.Id.String() == orphanId {
			foundOrphanInGlobal = true
		}
	}
	assert.False(t, foundOrphanInGlobal, "Orphan file should no longer appear in global list")

	// Check Space List
	result, err = client.CallTool(ctx, mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: toolName,
			Arguments: map[string]any{
				"space_id": spaceId.String(),
			},
		},
	})
	require.NoError(t, err)
	require.False(t, result.IsError)
	require.NotEmpty(t, result.Content)

	textContent, ok = result.Content[0].(mcp.TextContent)
	require.True(t, ok)

	json.Unmarshal([]byte(textContent.Text), &spaceList)

	foundOrphanInSpace := false
	for _, item := range spaceList.Items {
		if item.Id.String() == orphanId {
			foundOrphanInSpace = true
		}
	}
	assert.True(t, foundOrphanInSpace, "Orphan file should now appear in space list")

	// ==========================================
	// Scenario 6: Filter by Path Prefix
	// ==========================================
	t.Log("Step 6: Filtering Files by Path via MCP")

	// 1. Upload file to a specific directory in the space
	toolName = "upload_file"

	result, err = client.CallTool(ctx, mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: toolName,
			Arguments: map[string]any{
				"filename": "main.go",
				"content":  "package main",
				"path":     "src/cmd",
				"space_id": spaceId.String(),
			},
		},
	})
	require.NoError(t, err)
	require.False(t, result.IsError)

	// 2. Filter for that directory
	toolName = "list_files"

	result, err = client.CallTool(ctx, mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: toolName,
			Arguments: map[string]any{
				"space_id":    spaceId.String(),
				"path_prefix": "src",
			},
		},
	})
	require.NoError(t, err)

	var filteredList v1file.ListFilesOutput
	json.Unmarshal([]byte(result.Content[0].(mcp.TextContent).Text), &filteredList)

	assert.Len(t, filteredList.Items, 1)
	assert.Equal(t, "main.go", filteredList.Items[0].Filename)
}
