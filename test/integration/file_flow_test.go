package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"os"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	v1file "github.com/w-h-a/gomento/internal/service/v1_file"
)

func TestAPI_File_Flow(t *testing.T) {
	if len(os.Getenv("INTEGRATION")) == 0 {
		t.Log("SKIPPING INTEGRATION TEST")
		return
	}

	client, baseURL, db, s3Client := setupIntegrationServer(t)

	// ==========================================
	// Scenario 1: Global File Lifecycle
	// ==========================================
	t.Log("Step 1: Uploading a Global File")

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, _ := writer.CreateFormFile("file", "config.yaml")
	part.Write([]byte("env: global"))
	writer.WriteField("path", "/etc")
	writer.Close()

	req, _ := http.NewRequest("POST", fmt.Sprintf("%s/api/v1/files", baseURL), body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	rsp, err := client.Do(req)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, rsp.StatusCode)

	var globalFile map[string]any
	err = json.NewDecoder(rsp.Body).Decode(&globalFile)
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
	t.Log("Step 2: Uploading a Space-Scoped File")

	// Create Space
	spaceRsp := createJson(t, client, baseURL+"/api/v1/spaces", map[string]any{"name": "DevOps Space"})
	spaceId := spaceRsp["id"].(string)

	// Upload File to Space
	body = &bytes.Buffer{}
	writer = multipart.NewWriter(body)
	part, _ = writer.CreateFormFile("file", "config.yaml")
	part.Write([]byte("env: local"))
	writer.WriteField("path", "/etc")
	writer.Close()

	url := fmt.Sprintf("%s/api/v1/files?space_id=%s", baseURL, spaceId)
	req, _ = http.NewRequest("POST", url, body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	rsp, err = client.Do(req)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, rsp.StatusCode)

	var spaceFile map[string]any
	json.NewDecoder(rsp.Body).Decode(&spaceFile)

	assert.Equal(t, spaceId, spaceFile["space_id"])
	assert.NotEqual(t, globalId, spaceFile["id"], "Space file should have distinct ID from global file")

	// ==========================================
	// Scenario 3: Listing & Filtering
	// ==========================================
	t.Log("Step 3: Verifying List Isolation")

	// List Global Only
	req, _ = http.NewRequest("GET", fmt.Sprintf("%s/api/v1/files", baseURL), nil)
	rsp, err = client.Do(req)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, rsp.StatusCode)
	var globalList v1file.ListFilesOutput
	json.NewDecoder(rsp.Body).Decode(&globalList)

	assert.Len(t, globalList.Items, 1)
	assert.Equal(t, globalId, globalList.Items[0].Id.String())

	// List Space Only
	req, _ = http.NewRequest("GET", fmt.Sprintf("%s/api/v1/files?space_id=%s", baseURL, spaceId), nil)
	rsp, err = client.Do(req)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, rsp.StatusCode)
	var spaceList v1file.ListFilesOutput
	json.NewDecoder(rsp.Body).Decode(&spaceList)

	assert.Len(t, spaceList.Items, 1)
	assert.Equal(t, spaceFile["id"], spaceList.Items[0].Id.String())

	// ==========================================
	// Scenario 4: Get By ID & Presigned URL
	// ==========================================
	t.Log("Step 4: Fetching File with Presigned URL")

	req, _ = http.NewRequest("GET", fmt.Sprintf("%s/api/v1/files/%s?with_url=true", baseURL, globalId), nil)
	rsp, err = client.Do(req)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, rsp.StatusCode)

	var getRes map[string]any
	json.NewDecoder(rsp.Body).Decode(&getRes)

	f := getRes["file"].(map[string]any)
	assert.Equal(t, globalId, f["id"])
	assert.NotEmpty(t, getRes["public_url"])
	assert.Contains(t, getRes["public_url"], TEST_BUCKET)

	// ==========================================
	// Scenario 5: Ad-Hoc Connection (Moving Global to Space)
	// ==========================================
	t.Log("Step 5: Connecting Global File to Space")

	// 1. Upload new orphan
	body = &bytes.Buffer{}
	writer = multipart.NewWriter(body)
	part, _ = writer.CreateFormFile("file", "orphan.txt")
	part.Write([]byte("orphan data"))
	writer.Close()

	req, _ = http.NewRequest("POST", fmt.Sprintf("%s/api/v1/files", baseURL), body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	rsp, _ = client.Do(req)
	require.Equal(t, http.StatusOK, rsp.StatusCode)

	var orphanFile map[string]any
	json.NewDecoder(rsp.Body).Decode(&orphanFile)
	orphanId := orphanFile["id"].(string)

	// 2. Connect
	connectBody, _ := json.Marshal(map[string]string{"space_id": spaceId})
	req, _ = http.NewRequest("POST", fmt.Sprintf("%s/api/v1/files/%s/connect_to_space", baseURL, orphanId), bytes.NewBuffer(connectBody))
	rsp, err = client.Do(req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rsp.StatusCode)

	// 3. Verify it moved
	// Check Global List
	req, _ = http.NewRequest("GET", fmt.Sprintf("%s/api/v1/files", baseURL), nil)
	rsp, _ = client.Do(req)
	json.NewDecoder(rsp.Body).Decode(&globalList)
	foundOrphanInGlobal := false
	for _, item := range globalList.Items {
		if item.Id.String() == orphanId {
			foundOrphanInGlobal = true
		}
	}
	assert.False(t, foundOrphanInGlobal, "Orphan file should no longer appear in global list")

	// Check Space List
	req, _ = http.NewRequest("GET", fmt.Sprintf("%s/api/v1/files?space_id=%s", baseURL, spaceId), nil)
	rsp, _ = client.Do(req)
	json.NewDecoder(rsp.Body).Decode(&spaceList)

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
	t.Log("Step 6: Filtering Files by Path")

	// 1. Upload file to a specific directory in the space
	body = &bytes.Buffer{}
	writer = multipart.NewWriter(body)
	part, _ = writer.CreateFormFile("file", "main.go")
	part.Write([]byte("package main"))
	writer.WriteField("path", "src/cmd")
	writer.Close()

	url = fmt.Sprintf("%s/api/v1/files?space_id=%s", baseURL, spaceId)
	req, _ = http.NewRequest("POST", url, body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	rsp, err = client.Do(req)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, rsp.StatusCode)

	// 2. Filter for that directory
	req, _ = http.NewRequest("GET", fmt.Sprintf("%s/api/v1/files?space_id=%s&path_prefix=src", baseURL, spaceId), nil)
	rsp, err = client.Do(req)
	require.NoError(t, err)

	var filteredList v1file.ListFilesOutput
	json.NewDecoder(rsp.Body).Decode(&filteredList)

	assert.Len(t, filteredList.Items, 1)
	assert.Equal(t, "main.go", filteredList.Items[0].Filename)
}
