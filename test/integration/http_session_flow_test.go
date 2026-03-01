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
	v1 "github.com/w-h-a/gomento/api/domain/v1"
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
	crtRsp, err := client.Do(req)
	require.NoError(t, err)
	defer crtRsp.Body.Close()
	require.Equal(t, http.StatusCreated, crtRsp.StatusCode)

	var out map[string]any
	json.NewDecoder(crtRsp.Body).Decode(&out)

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

	// Wait for ingestion loop to persist messages
	assert.Eventually(t, func() bool {
		var count int
		db.QueryRow("SELECT count(*) FROM messages WHERE session_id = $1", sessionId).Scan(&count)
		return count >= 4
	}, 5*time.Second, 100*time.Millisecond, "Messages should be persisted by ingestion loop")

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
	cnnRsp, err := client.Do(req)
	require.NoError(t, err)
	defer cnnRsp.Body.Close()
	assert.Equal(t, http.StatusOK, cnnRsp.StatusCode)

	// Verify Session State
	getSessRsp, err := client.Get(fmt.Sprintf("%s/api/v1/sessions/%s", baseURL, sessionId))
	require.NoError(t, err)
	defer getSessRsp.Body.Close()
	var sessDetails map[string]any
	json.NewDecoder(getSessRsp.Body).Decode(&sessDetails)
	assert.Equal(t, spaceId.String(), sessDetails["space_id"])

	// ==========================================
	// Scenario 4: Verify Auto-Extracted Tasks
	// ==========================================
	t.Log("Step 4: Verify Auto-Extracted Tasks via HTTP")

	assert.Eventually(t, func() bool {
		req, _ := http.NewRequest("GET", fmt.Sprintf("%s/api/v1/sessions/%s/tasks", baseURL, sessionId), nil)
		rsp, err := client.Do(req)
		if err != nil {
			return false
		}
		defer rsp.Body.Close()
		var tasksRsp v1session.ListTasksOutput
		json.NewDecoder(rsp.Body).Decode(&tasksRsp)
		return len(tasksRsp.Items) > 0
	}, 10*time.Second, 500*time.Millisecond, "Tasks should be auto-extracted by ingestion loop")

	// ==========================================
	// Scenario 5: Worker Trigger (Distill Skill)
	// ==========================================
	t.Log("Step 5: Triggering Distillation via HTTP")

	req, _ = http.NewRequest("POST", fmt.Sprintf("%s/api/v1/sessions/%s/distill", baseURL, sessionId), nil)
	dstRsp, err := client.Do(req)
	require.NoError(t, err)
	defer dstRsp.Body.Close()
	assert.Equal(t, http.StatusAccepted, dstRsp.StatusCode)

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

	// ==========================================
	// Scenario 6: Verify Chat Attachment becomes Searchable File
	// ==========================================
	t.Log("Step 6: Verifying Chat Attachment Unification via HTTP")

	// 1. Send Message with Attachment
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	_ = writer.WriteField("role", "user")

	textAndFileParts := []map[string]string{
		{"type": "text", "text": "Here is the error log"},
		{"type": "file", "file_field": "critical_log"},
	}
	partsBytes, _ := json.Marshal(textAndFileParts)
	_ = writer.WriteField("parts", string(partsBytes))

	part, _ := writer.CreateFormFile("critical_log", "critical.log")
	uniqueContent := fmt.Sprintf("FATAL_CRASH_%d", time.Now().UnixNano())
	part.Write([]byte(uniqueContent))
	_ = writer.Close()

	uploadUrl := fmt.Sprintf("%s/api/v1/sessions/%s/messages", baseURL, sessionId)
	req, _ = http.NewRequest("POST", uploadUrl, body)
	req.Header.Set("Content-Type", writer.FormDataContentType())

	uploadRsp, err := client.Do(req)
	require.NoError(t, err)
	defer uploadRsp.Body.Close()
	assert.Equal(t, http.StatusOK, uploadRsp.StatusCode)

	// 2. Verify File Searchability
	fileSearchUrl := fmt.Sprintf("%s/api/v1/spaces/%s/files?q=FATAL_CRASH", baseURL, spaceId)

	require.Eventually(t, func() bool {
		req, _ := http.NewRequest("GET", fileSearchUrl, nil)
		rsp, err := client.Do(req)
		if err != nil {
			return false
		}
		defer rsp.Body.Close()

		var files []v1.File
		if err := json.NewDecoder(rsp.Body).Decode(&files); err != nil {
			return false
		}

		for _, f := range files {
			if f.Filename == "critical.log" {
				return true
			}
		}

		return false
	}, 5*time.Second, 500*time.Millisecond, "Chat attachment should eventually appear in file search results")

	// ==========================================
	// Scenario 7: Verify Chat Attachment becomes Searchable File
	// ==========================================
	t.Log("Step 7: Verifying Chat Attachment Unification & Chunking via HTTP")

	// 1. Send Message with Large Content
	body = &bytes.Buffer{}
	writer = multipart.NewWriter(body)
	_ = writer.WriteField("role", "user")

	textAndFileParts = []map[string]string{
		{"type": "text", "text": "Here is the error log"},
		{"type": "file", "file_field": "critical_log"},
	}
	partsBytes, _ = json.Marshal(textAndFileParts)
	_ = writer.WriteField("parts", string(partsBytes))

	largeLogContent := "START_LOG\n" + strings.Repeat("l", 5000) + "\n\nBREAK\n\n" + strings.Repeat("o", 5000) + "\nEND_LOG"

	part, _ = writer.CreateFormFile("critical_log", "critical_large.log")
	part.Write([]byte(largeLogContent))
	_ = writer.Close()

	uploadUrl = fmt.Sprintf("%s/api/v1/sessions/%s/messages", baseURL, sessionId)
	req, _ = http.NewRequest("POST", uploadUrl, body)
	req.Header.Set("Content-Type", writer.FormDataContentType())

	uploadRsp, err = client.Do(req)
	require.NoError(t, err)
	defer uploadRsp.Body.Close()
	assert.Equal(t, http.StatusOK, uploadRsp.StatusCode)

	// 2. Verify File Search
	fileSearchUrl = fmt.Sprintf("%s/api/v1/spaces/%s/files?q=START_LOG", baseURL, spaceId)

	var attachmentFileId string

	require.Eventually(t, func() bool {
		req, _ := http.NewRequest("GET", fileSearchUrl, nil)
		rsp, err := client.Do(req)
		if err != nil {
			return false
		}
		defer rsp.Body.Close()

		var files []v1.File
		if err := json.NewDecoder(rsp.Body).Decode(&files); err != nil {
			return false
		}

		for _, f := range files {
			if f.Filename == "critical_large.log" {
				attachmentFileId = f.Id.String()
				return true
			}
		}

		return false
	}, 5*time.Second, 500*time.Millisecond, "Chat attachment should appear in file search")

	// 3. Verify Database Chunking
	require.NotEmpty(t, attachmentFileId)

	require.Eventually(t, func() bool {
		var count int
		err := db.QueryRow("SELECT count(*) FROM file_chunks WHERE file_id = $1", attachmentFileId).Scan(&count)
		return err == nil && count > 1
	}, 5*time.Second, 500*time.Millisecond, "Chat attachment should be split into multiple chunks in DB")

	// ==========================================
	// Scenario 8: Verify Message Text becomes Searchable
	// ==========================================
	t.Log("Step 8: Verifying Chat Message Searchability via HTTP")

	// 1. Send Message
	msgBody := &bytes.Buffer{}
	msgWriter := multipart.NewWriter(msgBody)
	_ = msgWriter.WriteField("role", "user")

	textParts := []map[string]string{
		{"type": "text", "text": "The secret code is BLUE_ORION"},
	}
	partsBytes, _ = json.Marshal(textParts)
	_ = msgWriter.WriteField("parts", string(partsBytes))
	_ = msgWriter.Close()

	req, _ = http.NewRequest("POST", fmt.Sprintf("%s/api/v1/sessions/%s/messages", baseURL, sessionId), msgBody)
	req.Header.Set("Content-Type", msgWriter.FormDataContentType())

	msgRsp, err := client.Do(req)
	require.NoError(t, err)
	defer msgRsp.Body.Close()
	assert.Equal(t, http.StatusOK, msgRsp.StatusCode)

	// 2. Verify Message Searchability
	msgSearchUrl := fmt.Sprintf("%s/api/v1/spaces/%s/messages?q=BLUE_ORION", baseURL, spaceId)

	require.Eventually(t, func() bool {
		req, _ := http.NewRequest("GET", msgSearchUrl, nil)
		rsp, err := client.Do(req)
		if err != nil {
			return false
		}
		defer rsp.Body.Close()

		var msgs []v1.Message
		if err := json.NewDecoder(rsp.Body).Decode(&msgs); err != nil {
			return false
		}
		for _, m := range msgs {
			if len(m.Parts) > 0 && m.Parts[0].Text == "The secret code is BLUE_ORION" {
				return true
			}
		}
		return false
	}, 5*time.Second, 500*time.Millisecond, "Chat message should eventually appear in message search results")
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
