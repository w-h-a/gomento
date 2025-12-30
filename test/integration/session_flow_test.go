package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/google/uuid"
	_ "github.com/lib/pq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAPI_Session(t *testing.T) {
	if len(os.Getenv("INTEGRATION")) == 0 {
		t.Log("SKIPPING INTEGRATION TEST")
		return
	}

	// Arrange
	client, baseURL, db, s3Client := setupIntegrationServer(t)

	projRsp := createJson(t, client, baseURL+"/api/v1/projects", map[string]any{"name": "Integration Proj"})
	projId := projRsp["id"].(string)
	require.NotEmpty(t, projId)

	sessRsp := createJson(t, client, baseURL+"/api/v1/sessions", map[string]any{
		"project_id": projId,
		"space_id":   nil,
	})
	sessionId := sessRsp["id"].(string)
	require.NotEmpty(t, sessionId)

	spaceRsp := createJson(t, client, baseURL+"/api/v1/spaces", map[string]any{"project_id": projId, "name": "Integration Space"})
	spaceId := spaceRsp["id"].(string)
	require.NotEmpty(t, spaceId)

	connectReq, _ := json.Marshal(map[string]string{"space_id": spaceId})
	req, _ := http.NewRequest("POST", fmt.Sprintf("%s/api/v1/sessions/%s/connect_to_space", baseURL, sessionId), bytes.NewBuffer(connectReq))
	rsp, err := client.Do(req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rsp.StatusCode)

	// Act
	msg1 := sendMessage(t, client, baseURL, sessionId, "user", "Hello World", nil)
	assert.Nil(t, msg1["parent_id"], "First message must have nil parent_id")

	msg2 := sendMessage(t, client, baseURL, sessionId, "assistant", "Hi User", nil)
	assert.Equal(t, msg1["id"], msg2["parent_id"], "Message 2 must point to Message 1")

	fileContent := "Important Log Data"
	msg3 := sendMessage(t, client, baseURL, sessionId, "user", "Here is my log", map[string]string{
		"log.txt": fileContent,
	})

	// Assert
	parts := msg3["parts"].([]any)
	filePart := parts[1].(map[string]any)
	assetId := filePart["asset_id"].(string)
	assert.NotEmpty(t, assetId)

	listResp, err := s3Client.ListObjectsV2(context.Background(), &s3.ListObjectsV2Input{Bucket: aws.String(TEST_BUCKET)})
	assert.NoError(t, err)
	assert.Equal(t, 1, int(*listResp.KeyCount), "Bucket should contain 1 file")

	msg4 := sendMessage(t, client, baseURL, sessionId, "user", "Duplicate log", map[string]string{
		"dup_log.txt": fileContent,
	})

	dupParts := msg4["parts"].([]any)
	dupFilePart := dupParts[1].(map[string]any)
	dupAssetId := dupFilePart["asset_id"].(string)

	assert.NotEmpty(t, dupAssetId)
	assert.Equal(t, assetId, dupAssetId, "Content-addressed storage should return same Asset ID for duplicate content")

	// Act
	page1 := getMessages(t, client, baseURL, sessionId, 1, "", true)

	// Assert
	assert.Len(t, page1.Items, 1)
	assert.Equal(t, msg4["id"], page1.Items[0].Id.String())
	assert.True(t, page1.HasMore)
	assert.NotEmpty(t, page1.NextCursor)

	var assetIdStr string
	for _, p := range page1.Items[0].Parts {
		if p.AssetId != nil {
			assetIdStr = p.AssetId.String()
			break
		}
	}
	assert.NotEmpty(t, assetIdStr, "Message 4 should have an asset")

	presigned, ok := page1.PublicUrls[uuid.MustParse(assetIdStr)]
	assert.True(t, ok, "Public URL map should contain asset ID")
	assert.NotEmpty(t, presigned.Url)
	assert.Contains(t, presigned.Url, TEST_BUCKET)

	// Act
	page2 := getMessages(t, client, baseURL, sessionId, 10, page1.NextCursor, false)

	// Assert
	assert.Len(t, page2.Items, 3)
	assert.False(t, page2.HasMore)
	assert.Equal(t, msg3["id"], page2.Items[0].Id.String())
	assert.Equal(t, msg2["id"], page2.Items[1].Id.String())
	assert.Equal(t, msg1["id"], page2.Items[2].Id.String())

	// Act
	req, _ = http.NewRequest("POST", fmt.Sprintf("%s/api/v1/sessions/%s/checkpoint", baseURL, sessionId), nil)
	rsp, err = client.Do(req)
	require.NoError(t, err)

	// Assert
	assert.Equal(t, http.StatusAccepted, rsp.StatusCode)

	assert.Eventually(t, func() bool {
		var status string
		err := db.QueryRow(`
            SELECT status 
            FROM jobs 
            WHERE payload->>'session_id' = $1 
              AND type = 'extract_session' 
            ORDER BY created_at DESC 
            LIMIT 1`,
			sessionId,
		).Scan(&status)
		return err == nil && status == "success"
	}, 5*time.Second, 100*time.Millisecond, "Extraction Job should succeed")

	assert.Eventually(t, func() bool {
		var count int
		err := db.QueryRow("SELECT count(*) FROM tasks WHERE session_id = $1", sessionId).Scan(&count)
		return err == nil && count > 0
	}, 5*time.Second, 100*time.Millisecond, "Tasks should be extracted")

	var skillCount int
	_ = db.QueryRow("SELECT count(*) FROM skills WHERE space_id = $1", spaceId).Scan(&skillCount)
	assert.Equal(t, 0, skillCount, "Checkpoint should not create skills")

	// Act
	req, _ = http.NewRequest("POST", fmt.Sprintf("%s/api/v1/sessions/%s/finish", baseURL, sessionId), nil)
	rsp, err = client.Do(req)
	require.NoError(t, err)

	// Assert
	assert.Equal(t, http.StatusAccepted, rsp.StatusCode)

	assert.Eventually(t, func() bool {
		var status string
		err := db.QueryRow(`
        SELECT status 
        FROM jobs 
        WHERE payload->>'session_id' = $1 
          AND type = 'distill_session' 
        ORDER BY created_at DESC 
        LIMIT 1`,
			sessionId,
		).Scan(&status)

		return err == nil && status == "success"
	}, 5*time.Second, 100*time.Millisecond, "Distill Job should succeed")

	assert.Eventually(t, func() bool {
		var trigger string
		err := db.QueryRow("SELECT trigger FROM skills WHERE space_id = $1", spaceId).Scan(&trigger)
		return err == nil && trigger == "how to restart redis"
	}, 5*time.Second, 100*time.Millisecond, "Skill should be created in DB")
}
