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
)

func TestAPI_Artifact(t *testing.T) {
	if len(os.Getenv("INTEGRATION")) == 0 {
		t.Log("SKIPPING INTEGRATION TEST")
		return
	}

	// Arrange
	client, baseURL, db, s3Client := setupIntegrationServer(t)

	projRsp := createJson(t, client, baseURL+"/api/v1/projects", map[string]any{"name": "Integration Proj"})
	projId := projRsp["id"].(string)
	require.NotEmpty(t, projId)

	artRsp := createJson(t, client, baseURL+"/api/v1/artifacts", map[string]any{"project_id": projId})
	artId := artRsp["id"].(string)
	require.NotEmpty(t, artId)

	// Act
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, _ := writer.CreateFormFile("file", "config.yaml")
	part.Write([]byte("host: localhost"))
	writer.WriteField("path", "/etc")
	writer.Close()

	req, _ := http.NewRequest("POST", fmt.Sprintf("%s/api/v1/artifacts/%s/files", baseURL, artId), body)
	req.Header.Set("Content-Type", writer.FormDataContentType())

	rsp, err := client.Do(req)
	require.NoError(t, err)

	// Assert
	assert.Equal(t, http.StatusOK, rsp.StatusCode)

	var count int
	var assetPath string
	err = db.QueryRow(`
		SELECT count(*), a.path 
		FROM files f 
		JOIN assets a ON f.asset_id = a.id
		WHERE f.filename = 'config.yaml' AND f.path = '/etc' AND f.artifact_id = $1
		GROUP BY a.path
	`, artId).Scan(&count, &assetPath)

	assert.NoError(t, err)
	assert.Equal(t, 1, count, "File record should exist in DB")
	assert.NotEmpty(t, assetPath, "Asset path should be populated")

	_, err = s3Client.HeadObject(context.Background(), &s3.HeadObjectInput{
		Bucket: aws.String(TEST_BUCKET),
		Key:    aws.String(assetPath),
	})
	assert.NoError(t, err, "Object should exist in MinIO")

	// Act
	req, _ = http.NewRequest("GET", fmt.Sprintf("%s/api/v1/artifacts/%s/files", baseURL, artId), nil)
	rsp, err = client.Do(req)
	require.NoError(t, err)

	// Assert
	assert.Equal(t, http.StatusOK, rsp.StatusCode)

	var files []map[string]any
	json.NewDecoder(rsp.Body).Decode(&files)
	assert.Len(t, files, 1)
	assert.Equal(t, "config.yaml", files[0]["filename"])
	assert.Equal(t, "/etc", files[0]["path"])

	assetMap := files[0]["asset"].(map[string]any)
	assert.Equal(t, assetPath, assetMap["path"])
}
