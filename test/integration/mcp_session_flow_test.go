package integration

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/require"
)

func TestAPI_Mcp_Session_Flow(t *testing.T) {
	if len(os.Getenv("INTEGRATION")) == 0 {
		t.Log("SKIPPING INTEGRATION TEST")
		return
	}

	client, _, _ := setupMcpServer(t)
	ctx := context.Background()

	// ==========================================
	// Scenario 1: Create Orphan Session
	// ==========================================
	t.Log("Step 1: Creating Orphan Session via MCP")

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
}
