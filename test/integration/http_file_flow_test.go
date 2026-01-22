package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	v1file "github.com/w-h-a/gomento/internal/service/v1_file"
)

func TestAPI_Http_File_Flow(t *testing.T) {
	if len(os.Getenv("INTEGRATION")) == 0 {
		t.Log("SKIPPING INTEGRATION TEST")
		return
	}

	client, baseURL, db, s3Client := setupHttpServer(t)

	// ==========================================
	// Scenario 1: Global File Lifecycle
	// ==========================================
	t.Log("Step 1: Uploading a Global File via HTTP")

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, _ := writer.CreateFormFile("file", "config.yaml")
	part.Write([]byte("env: global"))
	writer.WriteField("path", "/etc")
	writer.Close()

	req, _ := http.NewRequest("POST", fmt.Sprintf("%s/api/v1/files", baseURL), body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	glbRsp, err := client.Do(req)
	require.NoError(t, err)
	defer glbRsp.Body.Close()
	require.Equal(t, http.StatusOK, glbRsp.StatusCode)

	var globalFile map[string]any
	err = json.NewDecoder(glbRsp.Body).Decode(&globalFile)
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
	t.Log("Step 2: Uploading a Space-Scoped File via HTTP")

	// Create Space
	spaceId := uuid.New()
	_, err = db.Exec("INSERT INTO spaces (id, name, created_at) VALUES ($1, $2, NOW())", spaceId, "DevOps Space")
	require.NoError(t, err)

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
	spcRsp, err := client.Do(req)
	require.NoError(t, err)
	defer spcRsp.Body.Close()
	require.Equal(t, http.StatusOK, spcRsp.StatusCode)

	var spaceFile map[string]any
	json.NewDecoder(spcRsp.Body).Decode(&spaceFile)

	assert.Equal(t, spaceId.String(), spaceFile["space_id"])
	assert.NotEqual(t, globalId, spaceFile["id"], "Space file should have distinct ID from global file")

	err = db.QueryRow("SELECT count(*) FROM files WHERE id = $1 AND space_id = $2", spaceFile["id"], spaceId).Scan(&count)
	assert.NoError(t, err)
	assert.Equal(t, 1, count, "Space file must exist in DB and be linked to space")

	// ==========================================
	// Scenario 3: Listing & Filtering
	// ==========================================
	t.Log("Step 3: Verifying List Isolation via HTTP")

	// List Global Only
	req, _ = http.NewRequest("GET", fmt.Sprintf("%s/api/v1/files", baseURL), nil)
	getRsp, err := client.Do(req)
	require.NoError(t, err)
	defer getRsp.Body.Close()
	require.Equal(t, http.StatusOK, getRsp.StatusCode)
	var globalList v1file.ListFilesOutput
	json.NewDecoder(getRsp.Body).Decode(&globalList)

	assert.Len(t, globalList.Items, 1)
	assert.Equal(t, globalId, globalList.Items[0].Id.String())

	// List Space Only
	req, _ = http.NewRequest("GET", fmt.Sprintf("%s/api/v1/files?space_id=%s", baseURL, spaceId), nil)
	getRsp2, err := client.Do(req)
	require.NoError(t, err)
	defer getRsp2.Body.Close()
	require.Equal(t, http.StatusOK, getRsp2.StatusCode)

	var spaceList v1file.ListFilesOutput
	json.NewDecoder(getRsp2.Body).Decode(&spaceList)

	assert.Len(t, spaceList.Items, 1)
	assert.Equal(t, spaceFile["id"], spaceList.Items[0].Id.String())

	// ==========================================
	// Scenario 4: Get By ID & Presigned URL
	// ==========================================
	t.Log("Step 4: Fetching File with Presigned URL via HTTP")

	req, _ = http.NewRequest("GET", fmt.Sprintf("%s/api/v1/files/%s?with_url=true", baseURL, globalId), nil)
	getRsp3, err := client.Do(req)
	require.NoError(t, err)
	defer getRsp3.Body.Close()
	require.Equal(t, http.StatusOK, getRsp3.StatusCode)

	var getRes map[string]any
	json.NewDecoder(getRsp3.Body).Decode(&getRes)

	f := getRes["file"].(map[string]any)
	assert.Equal(t, globalId, f["id"])
	assert.NotEmpty(t, getRes["public_url"])
	assert.Contains(t, getRes["public_url"], TEST_BUCKET)

	// ==========================================
	// Scenario 5: Ad-Hoc Connection (Moving Global to Space)
	// ==========================================
	t.Log("Step 5: Connecting Global File to Space via HTTP")

	// 1. Upload new orphan
	body = &bytes.Buffer{}
	writer = multipart.NewWriter(body)
	part, _ = writer.CreateFormFile("file", "orphan.txt")
	part.Write([]byte("orphan data"))
	writer.Close()

	req, _ = http.NewRequest("POST", fmt.Sprintf("%s/api/v1/files", baseURL), body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	orphanRsp, err := client.Do(req)
	require.NoError(t, err)
	defer orphanRsp.Body.Close()
	require.Equal(t, http.StatusOK, orphanRsp.StatusCode)

	var orphanFile map[string]any
	json.NewDecoder(orphanRsp.Body).Decode(&orphanFile)
	orphanId := orphanFile["id"].(string)

	// 2. Connect
	connectBody, _ := json.Marshal(map[string]string{"space_id": spaceId.String()})
	req, _ = http.NewRequest("POST", fmt.Sprintf("%s/api/v1/files/%s/connect_to_space", baseURL, orphanId), bytes.NewBuffer(connectBody))
	cnnRsp, err := client.Do(req)
	require.NoError(t, err)
	defer cnnRsp.Body.Close()
	assert.Equal(t, http.StatusOK, cnnRsp.StatusCode)

	// 3. Verify it moved
	// Check Global List
	req, _ = http.NewRequest("GET", fmt.Sprintf("%s/api/v1/files", baseURL), nil)
	glblList, err := client.Do(req)
	require.NoError(t, err)
	defer glblList.Body.Close()

	json.NewDecoder(glblList.Body).Decode(&globalList)

	foundOrphanInGlobal := false
	for _, item := range globalList.Items {
		if item.Id.String() == orphanId {
			foundOrphanInGlobal = true
		}
	}

	assert.False(t, foundOrphanInGlobal, "Orphan file should no longer appear in global list")

	// Check Space List
	req, _ = http.NewRequest("GET", fmt.Sprintf("%s/api/v1/files?space_id=%s", baseURL, spaceId), nil)
	spcList, err := client.Do(req)
	require.NoError(t, err)
	defer spcList.Body.Close()

	json.NewDecoder(spcList.Body).Decode(&spaceList)

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
	t.Log("Step 6: Filtering Files by Path via HTTP")

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
	uplRsp, err := client.Do(req)
	require.NoError(t, err)
	defer uplRsp.Body.Close()
	require.Equal(t, http.StatusOK, uplRsp.StatusCode)

	// 2. Filter for that directory
	req, _ = http.NewRequest("GET", fmt.Sprintf("%s/api/v1/files?space_id=%s&path_prefix=src", baseURL, spaceId), nil)
	fltRsp, err := client.Do(req)
	require.NoError(t, err)
	defer fltRsp.Body.Close()

	var filteredList v1file.ListFilesOutput
	json.NewDecoder(fltRsp.Body).Decode(&filteredList)

	assert.Len(t, filteredList.Items, 1)
	assert.Equal(t, "main.go", filteredList.Items[0].Filename)

	// ==========================================
	// Scenario 7: Download Content with Line Filtering
	// ==========================================
	t.Log("Step 7: Downloading Content via HTTP")

	// 1. Upload a multi-line file
	body = &bytes.Buffer{}
	writer = multipart.NewWriter(body)
	part, _ = writer.CreateFormFile("file", "lines.txt")
	part.Write([]byte("line1\nline2\nline3\nline4"))
	writer.Close()

	req, _ = http.NewRequest("POST", fmt.Sprintf("%s/api/v1/files", baseURL), body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	linesRsp, err := client.Do(req)
	require.NoError(t, err)
	defer linesRsp.Body.Close()

	var linesFile map[string]any
	json.NewDecoder(linesRsp.Body).Decode(&linesFile)
	linesId := linesFile["id"].(string)

	// 2. Download specific lines (2-3)
	downloadUrl := fmt.Sprintf("%s/api/v1/files/%s/content?start_line=2&end_line=3", baseURL, linesId)
	req, _ = http.NewRequest("GET", downloadUrl, nil)
	dlRsp, err := client.Do(req)
	require.NoError(t, err)
	defer dlRsp.Body.Close()

	assert.Equal(t, http.StatusOK, dlRsp.StatusCode)

	contentBytes, err := io.ReadAll(dlRsp.Body)
	require.NoError(t, err)

	expectedContent := "line2\nline3\n"
	assert.Equal(t, expectedContent, string(contentBytes))

	// ==========================================
	// Scenario 8: Verify Embedding Ingestion (Async)
	// ==========================================
	t.Log("Step 8: Verifying File Embedding Generation via HTTP Flow")

	require.Eventually(t, func() bool {
		var hasEmbedding bool
		err := db.QueryRow("SELECT embedding IS NOT NULL FROM files WHERE id = $1", linesId).Scan(&hasEmbedding)
		if err != nil {
			return false
		}
		return hasEmbedding
	}, 5*time.Second, 500*time.Millisecond, "File should eventually have an embedding generated by the worker")
}
