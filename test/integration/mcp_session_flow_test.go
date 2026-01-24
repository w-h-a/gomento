package integration

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	v1 "github.com/w-h-a/gomento/api/domain/v1"
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
	// Scenario 2: Message Flow, Context & Pagination
	// ==========================================
	t.Log("Step 2: Sending Messages & Verifying Deduplication via MCP")

	// Message 1: User
	msg1 := sendMessagesViaMcp(t, client, ctx, sessionId, "user", "How do I fix Nginx?", nil)
	assert.Nil(t, msg1["parent_id"], "First message must have nil parent_id")

	// Message 2: Assistant
	msg2 := sendMessagesViaMcp(t, client, ctx, sessionId, "assistant", "Check the logs.", nil)
	assert.Equal(t, msg1["id"], msg2["parent_id"], "Message 2 must point to Message 1")

	// Message 3: Upload File
	fileContent := "Critical System Log Data"
	msg3 := sendMessagesViaMcp(t, client, ctx, sessionId, "user", "Here is the log", map[string]string{
		"log.txt": fileContent,
	})

	// Extract the Asset ID from Message 3
	parts := msg3["parts"].([]any)
	filePart := parts[1].(map[string]any)
	assetId := filePart["asset_id"].(string)
	assert.NotEmpty(t, assetId)

	// Message 4: Upload Duplicate File
	msg4 := sendMessagesViaMcp(t, client, ctx, sessionId, "user", "Here is the log again", map[string]string{
		"copy_of_log.txt": fileContent,
	})

	// Extract the Asset ID from Message 4
	dupParts := msg4["parts"].([]any)
	dupFilePart := dupParts[1].(map[string]any)
	dupAssetId := dupFilePart["asset_id"].(string)
	assert.Equal(t, assetId, dupAssetId, "Uploading identical content should result in the same Asset ID")

	// Verification: Pagination
	// Fetch Page 1 (Limit 1) - Should get the latest message (msg4)
	page1 := getMessagesViaMcp(t, client, ctx, sessionId, 1, "", false)

	assert.Len(t, page1.Items, 1)
	assert.Equal(t, msg4["id"], page1.Items[0].Id.String())
	assert.True(t, page1.HasMore)
	assert.NotEmpty(t, page1.NextCursor)

	// Fetch Page 2 (Limit 10, using cursor) - Should get msg3, msg2, msg1
	page2 := getMessagesViaMcp(t, client, ctx, sessionId, 10, page1.NextCursor, false)

	assert.Len(t, page2.Items, 3)
	assert.Equal(t, msg3["id"], page2.Items[0].Id.String())
	assert.Equal(t, msg2["id"], page2.Items[1].Id.String())
	assert.Equal(t, msg1["id"], page2.Items[2].Id.String())

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

	// ==========================================
	// Scenario 4: Worker Checkpoint (Extract Tasks)
	// ==========================================
	t.Log("Step 4: Triggering Task Extraction via MCP")

	result, err = client.CallTool(ctx, mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: "extract_tasks",
			Arguments: map[string]any{
				"session_id": sessionId,
			},
		},
	})
	require.NoError(t, err)
	require.False(t, result.IsError)

	// Wait for async worker
	assert.Eventually(t, func() bool {
		var status string
		err := db.QueryRow(`
            SELECT status FROM jobs 
            WHERE payload->>'session_id' = $1 AND type = 'extract_session' 
            ORDER BY created_at DESC LIMIT 1`,
			sessionId,
		).Scan(&status)
		return err == nil && status == "success"
	}, 5*time.Second, 100*time.Millisecond, "Extraction Job should succeed")

	// Verify Tasks were extracted
	resTasks, err := client.CallTool(ctx, mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: "list_tasks",
			Arguments: map[string]any{
				"session_id": sessionId,
			},
		},
	})
	require.NoError(t, err)
	require.False(t, resTasks.IsError)

	var tasksOut v1session.ListTasksOutput
	json.Unmarshal([]byte(resTasks.Content[0].(mcp.TextContent).Text), &tasksOut)

	assert.NotEmpty(t, tasksOut.Items)

	// ==========================================
	// Scenario 5: Worker Finish (Distill Skill)
	// ==========================================
	t.Log("Step 5: Triggering Distillation via MCP")

	resFinish, err := client.CallTool(ctx, mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: "distill_skill",
			Arguments: map[string]any{
				"session_id": sessionId,
			},
		},
	})
	require.NoError(t, err)
	require.False(t, resFinish.IsError)

	// Wait for async worker
	assert.Eventually(t, func() bool {
		var status string
		err := db.QueryRow(`
            SELECT status FROM jobs 
            WHERE payload->>'session_id' = $1 AND type = 'distill_session' 
            ORDER BY created_at DESC LIMIT 1`,
			sessionId,
		).Scan(&status)
		return err == nil && status == "success"
	}, 5*time.Second, 100*time.Millisecond, "Distill Job should succeed")

	// 3. Verify Skill Created (linked to space)
	var skillCount int
	err = db.QueryRow("SELECT count(*) FROM skills WHERE space_id = $1", spaceId).Scan(&skillCount)
	assert.NoError(t, err)
	assert.Greater(t, skillCount, 0, "Skill should be saved to the Space")

	// ==========================================
	// Scenario 6: Verify Chat Attachment becomes Searchable File
	// ==========================================
	t.Log("Step 6: Verifying Chat Attachment Unification via MCP")

	// 1. Send Message with Attachment
	_, err = client.CallTool(ctx, mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: "add_message",
			Arguments: map[string]any{
				"session_id": sessionId,
				"role":       "user",
				"content":    "Here is the log file",
				"parts": []map[string]any{
					{"type": "text", "text": "Here is the log file"},
					{"type": "file", "file_field": "mcp_app.log"},
				},
				"files": map[string]any{
					"mcp_app.log": "MCP_FATAL_ERROR_UNIQUE_ID",
				},
			},
		},
	})
	require.NoError(t, err)

	// 2. Verify File Searchability
	require.Eventually(t, func() bool {
		res, err := client.CallTool(ctx, mcp.CallToolRequest{
			Params: mcp.CallToolParams{
				Name: "search_files",
				Arguments: map[string]any{
					"space_id": spaceId,
					"query":    "MCP_FATAL_ERROR",
				},
			},
		})
		if err != nil || res.IsError {
			return false
		}

		var files []v1.File
		if err := json.Unmarshal([]byte(res.Content[0].(mcp.TextContent).Text), &files); err != nil {
			return false
		}

		for _, f := range files {
			if f.Filename == "mcp_app.log" {
				return true
			}
		}
		return false
	}, 5*time.Second, 500*time.Millisecond, "Chat attachment should eventually appear in search_files tool results")

	// ==========================================
	// Scenario 7: Verify Message Text becomes Searchable
	// ==========================================
	t.Log("Step 7: Verifying Chat Message Searchability via MCP")

	// 1. Send Message
	_, err = client.CallTool(ctx, mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: "add_message",
			Arguments: map[string]any{
				"session_id": sessionId,
				"role":       "user",
				"parts": []map[string]any{
					{"type": "text", "text": "The secret project is GREEN_LANTERN"},
				},
			},
		},
	})
	require.NoError(t, err)

	// 2. Verify Message Searchability
	require.Eventually(t, func() bool {
		res, err := client.CallTool(ctx, mcp.CallToolRequest{
			Params: mcp.CallToolParams{
				Name: "search_messages",
				Arguments: map[string]any{
					"space_id": spaceId,
					"query":    "GREEN_LANTERN",
				},
			},
		})
		if err != nil || res.IsError {
			return false
		}

		var msgs []v1.Message
		if err := json.Unmarshal([]byte(res.Content[0].(mcp.TextContent).Text), &msgs); err != nil {
			return false
		}

		for _, m := range msgs {
			for _, p := range m.Parts {
				if p.Text == "The secret project is GREEN_LANTERN" {
					return true
				}
			}
		}
		return false
	}, 5*time.Second, 500*time.Millisecond, "Chat message should eventually appear in search_messages tool results")
}

func sendMessagesViaMcp(
	t *testing.T,
	client *client.Client,
	ctx context.Context,
	sessId string,
	role string,
	text string,
	files map[string]string,
) map[string]any {
	t.Helper()

	parts := []map[string]string{
		{"type": "text", "text": text},
	}

	for fname := range files {
		parts = append(parts, map[string]string{
			"type":       "file",
			"file_field": fname,
		})
	}

	result, err := client.CallTool(ctx, mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: "add_message",
			Arguments: map[string]any{
				"session_id": sessId,
				"role":       role,
				"parts":      parts,
				"files":      files,
			},
		},
	})
	require.NoError(t, err)
	require.False(t, result.IsError)
	require.NotEmpty(t, result.Content)

	textContent, ok := result.Content[0].(mcp.TextContent)
	require.True(t, ok)

	var msg map[string]any
	json.Unmarshal([]byte(textContent.Text), &msg)

	return msg
}

func getMessagesViaMcp(
	t *testing.T,
	client *client.Client,
	ctx context.Context,
	sessionId string,
	limit int,
	cursor string,
	withUrl bool,
) v1session.ListMessagesOutput {
	t.Helper()

	result, err := client.CallTool(ctx, mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: "list_messages",
			Arguments: map[string]any{
				"session_id":            sessionId,
				"limit":                 limit,
				"cursor":                cursor,
				"with_asset_public_url": withUrl,
			},
		},
	})
	require.NoError(t, err)
	require.False(t, result.IsError)
	require.NotEmpty(t, result.Content)

	textContent, ok := result.Content[0].(mcp.TextContent)
	require.True(t, ok)

	var out v1session.ListMessagesOutput
	json.Unmarshal([]byte(textContent.Text), &out)

	return out
}
