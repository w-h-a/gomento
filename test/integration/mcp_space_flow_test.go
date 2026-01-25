package integration

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/pgvector/pgvector-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	v1 "github.com/w-h-a/gomento/api/domain/v1"
	v1space "github.com/w-h-a/gomento/internal/service/v1_space"
)

func TestAPI_Mcp_Space_Flow(t *testing.T) {
	if len(os.Getenv("INTEGRATION")) == 0 {
		t.Log("SKIPPING INTEGRATION TEST")
		return
	}

	client, db, _ := setupMcpServer(t)
	ctx := context.Background()

	// ==========================================
	// Scenario 1: Create Space & Session
	// ==========================================
	t.Log("Step 1: Creating Space & Session via MCP")

	result, err := client.CallTool(ctx, mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: "create_space",
			Arguments: map[string]any{
				"name": "Frontend Space",
			},
		},
	})
	require.NoError(t, err)
	require.False(t, result.IsError)
	require.NotEmpty(t, result.Content)

	textContent, ok := result.Content[0].(mcp.TextContent)
	require.True(t, ok)

	var spaceRsp map[string]any
	json.Unmarshal([]byte(textContent.Text), &spaceRsp)
	spaceId := spaceRsp["id"].(string)
	require.NotEmpty(t, spaceId)
	assert.Equal(t, "Frontend Space", spaceRsp["name"])

	sessId := uuid.New()
	_, err = db.Exec("INSERT INTO sessions (id, space_id, created_at) VALUES ($1, $2, NOW())", sessId.String(), spaceId)
	require.NoError(t, err)

	// ==========================================
	// Scenario 2: Get Space
	// ==========================================
	t.Log("Step 2: Fetching Space Details via MCP")

	result, err = client.CallTool(ctx, mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: "get_space",
			Arguments: map[string]any{
				"space_id": spaceId,
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
	assert.Equal(t, spaceId, getRes["id"])

	// ==========================================
	// Scenario 3: List Spaces
	// ==========================================
	t.Log("Step 3: Listing Spaces via MCP")

	// Create another space to ensure list works
	client.CallTool(ctx, mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name:      "create_space",
			Arguments: map[string]any{"name": "Backend Space"},
		},
	})

	result, err = client.CallTool(ctx, mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name:      "list_spaces",
			Arguments: map[string]any{},
		},
	})
	require.NoError(t, err)
	require.False(t, result.IsError)
	require.NotEmpty(t, result.Content)

	textContent, ok = result.Content[0].(mcp.TextContent)
	require.True(t, ok)

	var listRsp v1space.ListSpacesOutput
	json.Unmarshal([]byte(textContent.Text), &listRsp)

	assert.GreaterOrEqual(t, len(listRsp.Items), 2)

	// ==========================================
	// Scenario 4: Semantic Search
	// ==========================================
	t.Log("Step 4: Searching Skills & Messages via MCP")

	// Inject Data with Vectors manually
	vecNorth := make([]float32, 1536)
	vecNorth[0] = 1.0
	vecSouth := make([]float32, 1536)
	vecSouth[0] = -1.0
	vecEast := make([]float32, 1536)
	vecEast[0] = 0.5
	vecWest := make([]float32, 1536)
	vecWest[0] = -0.5

	// Inject Skill v
	_, err = db.Exec(`
		INSERT INTO skills (id, space_id, trigger, sop, embedding, created_at)
		VALUES ($1, $2, 'Trigger North', 'SOP Content', $3, NOW())
	`, uuid.New(), spaceId, pgvector.NewVector(vecNorth))
	require.NoError(t, err)

	// Inject Message
	_, err = db.Exec(`
		INSERT INTO messages (id, session_id, role, parts, embedding, created_at)
		VALUES ($1, $2, 'user', '[{"type":"text","text":"Go South"}]', $3, NOW())
	`, uuid.New(), sessId, pgvector.NewVector(vecSouth))
	require.NoError(t, err)

	// Inject Asset & File
	assetId := uuid.New()
	_, err = db.Exec(`
		INSERT INTO assets (id, container, path, etag, sha256, mime, size_bytes, created_at)
		VALUES ($1, 'test-bucket', 'docs/east.txt', 'etag', 'sha', 'text/plain', 100, NOW())
	`, assetId)
	require.NoError(t, err)

	fileId := uuid.New()

	_, err = db.Exec(`
		INSERT INTO files (id, space_id, asset_id, path, filename, embedding, created_at, updated_at)
		VALUES ($1, $2, $3, 'docs/east.txt', 'east.txt', $4, NOW(), NOW())
	`, fileId, spaceId, assetId, pgvector.NewVector(vecEast))
	require.NoError(t, err)

	_, err = db.Exec(`
		INSERT INTO file_chunks (id, file_id, chunk_index, content, embedding, created_at)
		VALUES ($1, $2, 0, 'Go West Life is Peaceful There', $3, NOW())
	`, uuid.New(), fileId, pgvector.NewVector(vecWest))
	require.NoError(t, err)

	// Search Skills
	resSkill, err := client.CallTool(ctx, mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: "search_skills",
			Arguments: map[string]any{
				"space_id": spaceId,
				"query":    "North",
			},
		},
	})
	require.NoError(t, err)
	require.False(t, resSkill.IsError)
	require.NotEmpty(t, resSkill.Content)

	textContent, ok = resSkill.Content[0].(mcp.TextContent)
	require.True(t, ok)

	var skillResults []v1.Skill
	json.Unmarshal([]byte(textContent.Text), &skillResults)
	assert.GreaterOrEqual(t, len(skillResults), 1)

	// Search Messages
	resMsg, err := client.CallTool(ctx, mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: "search_messages",
			Arguments: map[string]any{
				"space_id": spaceId,
				"query":    "South",
			},
		},
	})
	require.NoError(t, err)
	require.False(t, resMsg.IsError)
	require.NotEmpty(t, resMsg.Content)

	textContent, ok = resMsg.Content[0].(mcp.TextContent)
	require.True(t, ok)

	var msgResults []v1.Message
	json.Unmarshal([]byte(textContent.Text), &msgResults)
	assert.GreaterOrEqual(t, len(msgResults), 1)

	// Search Files
	resFile, err := client.CallTool(ctx, mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: "search_files",
			Arguments: map[string]any{
				"space_id": spaceId,
				"query":    "East",
			},
		},
	})
	require.NoError(t, err)
	require.False(t, resFile.IsError)
	require.NotEmpty(t, resFile.Content)

	textContent, ok = resFile.Content[0].(mcp.TextContent)
	require.True(t, ok)

	var fileResults []v1.File
	json.Unmarshal([]byte(textContent.Text), &fileResults)
	assert.GreaterOrEqual(t, len(fileResults), 1)
	assert.Equal(t, "east.txt", fileResults[0].Filename)

	// Search Chunks
	resChunk, err := client.CallTool(ctx, mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: "search_chunks",
			Arguments: map[string]any{
				"space_id": spaceId,
				"query":    "West",
			},
		},
	})
	require.NoError(t, err)
	require.False(t, resChunk.IsError)
	require.NotEmpty(t, resChunk.Content)

	textContent, ok = resChunk.Content[0].(mcp.TextContent)
	require.True(t, ok)

	var chunkResults []v1.MatchingChunk
	json.Unmarshal([]byte(textContent.Text), &chunkResults)
	assert.GreaterOrEqual(t, len(chunkResults), 1)
	assert.Contains(t, chunkResults[0].Chunk.Content, "Go West")
}
