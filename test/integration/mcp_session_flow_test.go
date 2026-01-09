package integration

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	v1session "github.com/w-h-a/gomento/internal/service/v1_session"
)

func TestAPI_Mcp_Session_Flow(t *testing.T) {
	if len(os.Getenv("INTEGRATION")) == 0 {
		t.Log("SKIPPING INTEGRATION TEST")
		return
	}

	client, db, _ := setupMcpServer(t)
	ctx := context.Background()

	// ==========================================
	// Scenario 1: Create & List Orphan Session
	// ==========================================
	t.Log("Step 1: Creating & Listing Orphan Session via MCP")

	toolName := "create_session"

	result, err := client.CallTool(ctx, mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name:      toolName,
			Arguments: map[string]any{},
		},
	})
	require.NoError(t, err)
	require.False(t, result.IsError)
	require.NotEmpty(t, result.Content)

	textContent, ok := result.Content[0].(mcp.TextContent)
	require.True(t, ok)

	var sessionData map[string]any
	json.Unmarshal([]byte(textContent.Text), &sessionData)

	sessionId := sessionData["id"].(string)
	require.NotEmpty(t, sessionId)
	require.Nil(t, sessionData["space_id"])

	toolName = "list_sessions"

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

	var sessListRsp v1session.ListSessionsOutput
	json.Unmarshal([]byte(textContent.Text), &sessListRsp)

	assert.Equal(t, sessionId, sessListRsp.Items[0].Id.String())

	// ==========================================
	// Scenario 3: Ad-Hoc Space Connection
	// ==========================================
	t.Log("Step 3: Connecting to Space via MCP")

	spaceId := uuid.New()
	_, err = db.Exec("INSERT INTO spaces (id, name, created_at) VALUES ($1, $2, NOW())", spaceId, "Support Space")
	require.NoError(t, err)

	toolName = "connect_session_to_space"

	result, err = client.CallTool(ctx, mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: toolName,
			Arguments: map[string]any{
				"session_id": sessionId,
				"space_id":   spaceId.String(),
			},
		},
	})
	require.NoError(t, err)
	require.False(t, result.IsError)
	require.NotEmpty(t, result.Content)

	// Verify Session State
	getSessRsp, err := client.CallTool(ctx, mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: "get_session",
			Arguments: map[string]any{
				"session_id": sessionId,
			},
		},
	})
	require.NoError(t, err)
	require.False(t, getSessRsp.IsError)
	require.NotEmpty(t, getSessRsp.Content)

	textContent, ok = getSessRsp.Content[0].(mcp.TextContent)
	require.True(t, ok)

	var sessDetails map[string]any
	json.Unmarshal([]byte(textContent.Text), &sessDetails)
	assert.Equal(t, spaceId.String(), sessDetails["space_id"])
}
