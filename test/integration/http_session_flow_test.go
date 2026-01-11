package integration

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	v1session "github.com/w-h-a/gomento/internal/service/v1_session"
)

func TestAPI_Http_Session_Flow(t *testing.T) {
	if len(os.Getenv("INTEGRATION")) == 0 {
		t.Log("SKIPPING INTEGRATION TEST")
		return
	}

	client, baseURL, db, _ := setupHttpServer(t)

	// ==========================================
	// Scenario 1: Create & List Orphan Session
	// ==========================================
	t.Log("Step 1: Creating & Listing Orphan Session via HTTP")

	createReq, _ := json.Marshal(map[string]any{
		"space_id": nil,
	})
	req, _ := http.NewRequest("POST", baseURL+"/api/v1/sessions", bytes.NewBuffer(createReq))
	rsp, err := client.Do(req)
	require.NoError(t, err)
	defer rsp.Body.Close()
	require.Equal(t, http.StatusCreated, rsp.StatusCode)

	var out map[string]any
	json.NewDecoder(rsp.Body).Decode(&out)

	sessionId := out["id"].(string)
	require.NotEmpty(t, sessionId)
	require.Nil(t, out["space_id"])

	listSessRsp, err := client.Get(fmt.Sprintf("%s/api/v1/sessions", baseURL))
	require.NoError(t, err)
	defer listSessRsp.Body.Close()
	var sessListRsp v1session.ListSessionsOutput
	json.NewDecoder(listSessRsp.Body).Decode(&sessListRsp)
	assert.Equal(t, sessionId, sessListRsp.Items[0].Id.String())

	// ==========================================
	// Scenario 2: Message Flow, Context & Pagination
	// ==========================================
	t.Log("Step 2: Sending Messages & Verifying Deduplication via HTTP")

	// Message 1: User
	msg1 := sendMessagesViaHttp(t, client, baseURL, sessionId, "user", "How do I fix Nginx?", nil)
	assert.Nil(t, msg1["parent_id"], "First message must have nil parent_id")

	// Message 2: Assistant
	msg2 := sendMessagesViaHttp(t, client, baseURL, sessionId, "assistant", "Check the logs.", nil)
	assert.Equal(t, msg1["id"], msg2["parent_id"], "Message 2 must point to Message 1")

	// Message 3: Upload File
	fileContent := "Critical System Log Data"
	msg3 := sendMessagesViaHttp(t, client, baseURL, sessionId, "user", "Here is the log", map[string]string{
		"log.txt": fileContent,
	})

	// Extract the Asset ID from Message 3
	parts := msg3["parts"].([]any)
	filePart := parts[1].(map[string]any)
	assetId := filePart["asset_id"].(string)
	assert.NotEmpty(t, assetId)

	// Message 4: Upload Duplicate File
	msg4 := sendMessagesViaHttp(t, client, baseURL, sessionId, "user", "Here is the log again", map[string]string{
		"copy_of_log.txt": fileContent,
	})

	// Extract the Asset ID from Message 4
	dupParts := msg4["parts"].([]any)
	dupFilePart := dupParts[1].(map[string]any)
	dupAssetId := dupFilePart["asset_id"].(string)
	assert.Equal(t, assetId, dupAssetId, "Uploading identical content should result in the same Asset ID")

	// Verification: Pagination
	// Fetch Page 1 (Limit 1) - Should get the latest message (msg4)
	page1 := getMessagesViaHttp(t, client, baseURL, sessionId, 1, "", false)

	assert.Len(t, page1.Items, 1)
	assert.Equal(t, msg4["id"], page1.Items[0].Id.String())
	assert.True(t, page1.HasMore)
	assert.NotEmpty(t, page1.NextCursor)

	// Fetch Page 2 (Limit 10, using cursor) - Should get msg3, msg2, msg1
	page2 := getMessagesViaHttp(t, client, baseURL, sessionId, 10, page1.NextCursor, false)

	assert.Len(t, page2.Items, 3)
	assert.False(t, page2.HasMore)
	assert.Equal(t, msg3["id"], page2.Items[0].Id.String())
	assert.Equal(t, msg2["id"], page2.Items[1].Id.String())
	assert.Equal(t, msg1["id"], page2.Items[2].Id.String())

	// ==========================================
	// Scenario 3: Ad-Hoc Space Connection
	// ==========================================
	t.Log("Step 3: Connecting to Space via HTTP")

	spaceId := uuid.New()
	_, err = db.Exec("INSERT INTO spaces (id, name, created_at) VALUES ($1, $2, NOW())", spaceId, "Support Space")
	require.NoError(t, err)

	connectReq, _ := json.Marshal(map[string]string{"space_id": spaceId.String()})
	req, _ = http.NewRequest("POST", fmt.Sprintf("%s/api/v1/sessions/%s/connect_to_space", baseURL, sessionId), bytes.NewBuffer(connectReq))
	rsp, err = client.Do(req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rsp.StatusCode)

	// Verify Session State
	getSessRsp, err := client.Get(fmt.Sprintf("%s/api/v1/sessions/%s", baseURL, sessionId))
	require.NoError(t, err)
	defer getSessRsp.Body.Close()
	var sessDetails map[string]any
	json.NewDecoder(getSessRsp.Body).Decode(&sessDetails)
	assert.Equal(t, spaceId.String(), sessDetails["space_id"])

	// ==========================================
	// Scenario 4: Worker Trigger (Extract Tasks)
	// ==========================================
	t.Log("Step 4: Triggering Task Extraction via HTTP")

	req, _ = http.NewRequest("POST", fmt.Sprintf("%s/api/v1/sessions/%s/extract", baseURL, sessionId), nil)
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
	var tasksRsp v1session.ListTasksOutput
	json.NewDecoder(rsp.Body).Decode(&tasksRsp)

	assert.NotEmpty(t, tasksRsp.Items)

	// ==========================================
	// Scenario 5: Worker Trigger (Distill Skill)
	// ==========================================
	t.Log("Step 5: Triggering Distillation via HTTP")

	req, _ = http.NewRequest("POST", fmt.Sprintf("%s/api/v1/sessions/%s/distill", baseURL, sessionId), nil)
	rsp, err = client.Do(req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusAccepted, rsp.StatusCode)

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

	// Verify Skill Created (linked to space)
	var skillCount int
	err = db.QueryRow("SELECT count(*) FROM skills WHERE space_id = $1", spaceId.String()).Scan(&skillCount)
	assert.NoError(t, err)
	assert.Greater(t, skillCount, 0, "Skill should be saved to the Space")
}

func sendMessagesViaHttp(
	t *testing.T,
	client *http.Client,
	baseURL string,
	sessId string,
	role string,
	text string,
	files map[string]string,
) map[string]any {
	t.Helper()

	var b bytes.Buffer
	w := multipart.NewWriter(&b)

	w.WriteField("role", role)

	parts := []map[string]string{
		{"type": "text", "text": text},
	}

	for fname, content := range files {
		parts = append(parts, map[string]string{
			"type":       "file",
			"file_field": fname,
		})

		fw, _ := w.CreateFormFile(fname, fname)
		io.Copy(fw, strings.NewReader(content))
	}

	partsJson, _ := json.Marshal(parts)
	w.WriteField("parts", string(partsJson))
	w.Close()

	req, _ := http.NewRequest("POST", fmt.Sprintf("%s/api/v1/sessions/%s/messages", baseURL, sessId), &b)
	req.Header.Set("Content-Type", w.FormDataContentType())

	rsp, err := client.Do(req)
	require.NoError(t, err)
	defer rsp.Body.Close()

	require.Equal(t, http.StatusOK, rsp.StatusCode)

	var out map[string]any
	json.NewDecoder(rsp.Body).Decode(&out)

	return out
}

func getMessagesViaHttp(
	t *testing.T,
	client *http.Client,
	baseURL string,
	sessId string,
	limit int,
	cursor string,
	withUrl bool,
) v1session.ListMessagesOutput {
	t.Helper()

	url := fmt.Sprintf("%s/api/v1/sessions/%s/messages?limit=%d&cursor=%s&with_asset_public_url=%v", baseURL, sessId, limit, cursor, withUrl)
	req, _ := http.NewRequest("GET", url, nil)

	rsp, err := client.Do(req)
	require.NoError(t, err)
	defer rsp.Body.Close()

	require.Equal(t, http.StatusOK, rsp.StatusCode)

	var res v1session.ListMessagesOutput
	json.NewDecoder(rsp.Body).Decode(&res)

	return res
}
