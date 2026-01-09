package integration

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAPI_Http_Session_Flow(t *testing.T) {
	if len(os.Getenv("INTEGRATION")) == 0 {
		t.Log("SKIPPING INTEGRATION TEST")
		return
	}

	client, baseURL, db, _ := setupHttpServer(t)

	// ==========================================
	// Scenario 1: Create Orphan Session
	// ==========================================
	t.Log("Step 1: Creating Orphan Session via HTTP")

	sessRsp := createJson(t, client, baseURL+"/api/v1/sessions", map[string]any{
		"space_id": nil,
	})
	sessionId := sessRsp["id"].(string)
	require.NotEmpty(t, sessionId)
	require.Nil(t, sessRsp["space_id"])

	// ==========================================
	// Scenario 2: Message Flow, Context & Pagination
	// ==========================================
	t.Log("Step 2: Sending Messages & Verifying Deduplication")

	// Message 1: User
	msg1 := sendMessage(t, client, baseURL, sessionId, "user", "How do I fix Nginx?", nil)
	assert.Nil(t, msg1["parent_id"], "First message must have nil parent_id")

	// Message 2: Assistant
	msg2 := sendMessage(t, client, baseURL, sessionId, "assistant", "Check the logs.", nil)
	assert.Equal(t, msg1["id"], msg2["parent_id"], "Message 2 must point to Message 1")

	// Message 3: Upload File
	fileContent := "Critical System Log Data"
	msg3 := sendMessage(t, client, baseURL, sessionId, "user", "Here is the log", map[string]string{
		"log.txt": fileContent,
	})

	// Extract the Asset ID from Message 3
	parts := msg3["parts"].([]any)
	filePart := parts[1].(map[string]any)
	assetId := filePart["asset_id"].(string)
	assert.NotEmpty(t, assetId)

	// Message 4: Upload Duplicate File
	msg4 := sendMessage(t, client, baseURL, sessionId, "user", "Here is the log again", map[string]string{
		"copy_of_log.txt": fileContent,
	})

	// Extract the Asset ID from Message 4
	dupParts := msg4["parts"].([]any)
	dupFilePart := dupParts[1].(map[string]any)
	dupAssetId := dupFilePart["asset_id"].(string)

	// Assertion: Asset ID must match (Deduplication)
	assert.Equal(t, assetId, dupAssetId, "Uploading identical content should result in the same Asset ID")

	// Verification: Pagination
	// Fetch Page 1 (Limit 1) - Should get the latest message (msg4)
	page1 := getMessages(t, client, baseURL, sessionId, 1, "", false)
	assert.Len(t, page1.Items, 1)
	assert.Equal(t, msg4["id"], page1.Items[0].Id.String())
	assert.True(t, page1.HasMore)
	assert.NotEmpty(t, page1.NextCursor)

	// Fetch Page 2 (Limit 10, utilizing cursor) - Should get msg3, msg2, msg1
	page2 := getMessages(t, client, baseURL, sessionId, 10, page1.NextCursor, false)
	assert.Len(t, page2.Items, 3)
	assert.False(t, page2.HasMore)
	assert.Equal(t, msg3["id"], page2.Items[0].Id.String())
	assert.Equal(t, msg2["id"], page2.Items[1].Id.String())
	assert.Equal(t, msg1["id"], page2.Items[2].Id.String())

	// ==========================================
	// Scenario 3: Ad-Hoc Space Connection
	// ==========================================
	t.Log("Step 3: Connecting to Space")

	spaceRsp := createJson(t, client, baseURL+"/api/v1/spaces", map[string]any{"name": "DevOps Space"})
	spaceId := spaceRsp["id"].(string)

	connectReq, _ := json.Marshal(map[string]string{"space_id": spaceId})
	req, _ := http.NewRequest("POST", fmt.Sprintf("%s/api/v1/sessions/%s/connect_to_space", baseURL, sessionId), bytes.NewBuffer(connectReq))
	rsp, err := client.Do(req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rsp.StatusCode)

	// Verify Session State
	getSessRsp, _ := client.Get(fmt.Sprintf("%s/api/v1/sessions/%s", baseURL, sessionId))
	var sessDetails map[string]any
	json.NewDecoder(getSessRsp.Body).Decode(&sessDetails)
	assert.Equal(t, spaceId, sessDetails["space_id"])

	// ==========================================
	// Scenario 4: Worker Checkpoint (Extract Tasks)
	// ==========================================
	t.Log("Step 4: Triggering Checkpoint")

	req, _ = http.NewRequest("POST", fmt.Sprintf("%s/api/v1/sessions/%s/checkpoint", baseURL, sessionId), nil)
	rsp, err = client.Do(req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusAccepted, rsp.StatusCode)

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
	req, _ = http.NewRequest("GET", fmt.Sprintf("%s/api/v1/sessions/%s/tasks", baseURL, sessionId), nil)
	rsp, _ = client.Do(req)
	var tasksRsp struct {
		Items []map[string]any `json:"items"`
	}
	json.NewDecoder(rsp.Body).Decode(&tasksRsp)

	assert.NotEmpty(t, tasksRsp.Items)

	// ==========================================
	// Scenario 5: Worker Finish (Distill Skill)
	// ==========================================
	t.Log("Step 5: Triggering Distillation")

	req, _ = http.NewRequest("POST", fmt.Sprintf("%s/api/v1/sessions/%s/finish", baseURL, sessionId), nil)
	rsp, err = client.Do(req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusAccepted, rsp.StatusCode)

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

	// Verify Skill Created (linked to space)
	var skillCount int
	err = db.QueryRow("SELECT count(*) FROM skills WHERE space_id = $1", spaceId).Scan(&skillCount)
	assert.NoError(t, err)
	assert.Greater(t, skillCount, 0, "Skill should be saved to the Space")
}
