package integration

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	_ "github.com/lib/pq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/w-h-a/gomento/cmd"
	v1project "github.com/w-h-a/gomento/internal/service/v1_project"
	v1session "github.com/w-h-a/gomento/internal/service/v1_session"
	v1space "github.com/w-h-a/gomento/internal/service/v1_space"
	v1worker "github.com/w-h-a/gomento/internal/service/v1_worker"
)

const (
	DB_CONN     = "postgres://user:password@localhost:5432/gomento?sslmode=disable"
	MINIO_END   = "http://localhost:9000"
	MINIO_USER  = "user"
	MINIO_PASS  = "password"
	TEST_BUCKET = "gomento-assets"
)

func TestAPI_FullUserFlow(t *testing.T) {
	if len(os.Getenv("INTEGRATION")) == 0 {
		t.Log("SKIPPING INTEGRATION TEST")
		return
	}

	// Arrange
	client, baseURL, db, s3Client := setupIntegrationServer(t)

	projResp := createJson(t, client, baseURL+"/api/v1/projects", map[string]string{"name": "Integration Proj"})
	projId := projResp["id"].(string)
	require.NotEmpty(t, projId)

	spaceResp := createJson(t, client, baseURL+"/api/v1/spaces", map[string]string{"project_id": projId, "name": "Integration Space"})
	spaceId := spaceResp["id"].(string)
	require.NotEmpty(t, spaceId)

	sessResp := createJson(t, client, baseURL+"/api/v1/sessions", map[string]string{"project_id": projId, "space_id": spaceId})
	sessionId := sessResp["id"].(string)
	require.NotEmpty(t, sessionId)

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
	req, _ := http.NewRequest("POST", fmt.Sprintf("%s/api/v1/sessions/%s/finish", baseURL, sessionId), nil)
	resp, err := client.Do(req)
	require.NoError(t, err)

	// Assert
	assert.Equal(t, http.StatusAccepted, resp.StatusCode)

	assert.Eventually(t, func() bool {
		var count int
		db.QueryRow("SELECT count(*) FROM skills WHERE space_id = $1", spaceId).Scan(&count)
		return count == 1
	}, 5*time.Second, 100*time.Millisecond, "Skill should be created in DB")

	var trigger string
	db.QueryRow("SELECT trigger FROM skills WHERE space_id = $1", spaceId).Scan(&trigger)
	assert.Equal(t, "how to restart redis", trigger)
}

func setupIntegrationServer(t *testing.T) (*http.Client, string, *sql.DB, *s3.Client) {
	ctx := context.Background()

	db, err := sql.Open("postgres", DB_CONN)
	require.NoError(t, err)
	require.NoError(t, db.Ping())
	_, err = db.Exec(`TRUNCATE TABLE skills, message_assets, messages, sessions, spaces, projects, assets CASCADE`)
	require.NoError(t, err)

	s3Config, _ := config.LoadDefaultConfig(ctx,
		config.WithRegion("us-east-1"),
		config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(MINIO_USER, MINIO_PASS, "")),
		config.WithEndpointResolverWithOptions(aws.EndpointResolverWithOptionsFunc(
			func(service, region string, options ...any) (aws.Endpoint, error) {
				return aws.Endpoint{URL: MINIO_END}, nil
			},
		)),
	)
	s3Client := s3.NewFromConfig(s3Config, func(o *s3.Options) { o.UsePathStyle = true })

	p, _ := cmd.InitV1Persister(ctx, DB_CONN)
	disp, _ := cmd.InitV1Dispatcher(ctx)
	dist, _ := cmd.InitV1Distiller(ctx, "", "")
	u, _ := cmd.InitV1Uploader(
		ctx,
		MINIO_END,
		"us-east-1",
		TEST_BUCKET,
		MINIO_USER,
		MINIO_PASS,
	)

	projSvc := v1project.NewV1Service(p)
	spaceSvc := v1space.NewV1Service(p)
	sessSvc := v1session.NewV1Service(p, disp, u, "worker")
	workerSvc := v1worker.NewV1Service(p, disp, dist)

	go func() { workerSvc.Subscribe(ctx, workerSvc.ProcessTask, "worker") }()

	r, _ := cmd.InitV1Router(
		ctx,
		projSvc,
		spaceSvc,
		sessSvc,
	)

	ts := httptest.NewServer(r)
	t.Cleanup(ts.Close)

	return ts.Client(), ts.URL, db, s3Client
}

func createJson(t *testing.T, client *http.Client, url string, data map[string]string) map[string]any {
	body, _ := json.Marshal(data)
	rsp, err := client.Post(url, "application/json", bytes.NewBuffer(body))
	require.NoError(t, err)
	defer rsp.Body.Close()

	require.Equal(t, http.StatusCreated, rsp.StatusCode)

	var out map[string]any
	json.NewDecoder(rsp.Body).Decode(&out)

	return out
}

func sendMessage(
	t *testing.T,
	client *http.Client,
	baseURL string,
	sessId string,
	role string,
	text string,
	files map[string]string,
) map[string]any {
	var b bytes.Buffer
	w := multipart.NewWriter(&b)

	w.WriteField("role", role)

	parts := []map[string]string{
		{"type": "text", "text": text},
	}

	for fname, content := range files {
		field := "file_" + fname
		parts = append(parts, map[string]string{
			"type":       "file",
			"file_field": field,
		})

		fw, _ := w.CreateFormFile(field, fname)
		io.Copy(fw, strings.NewReader(content))
	}

	partsJson, _ := json.Marshal(parts)
	w.WriteField("parts", string(partsJson))
	w.Close()

	req, _ := http.NewRequest("POST", fmt.Sprintf("%s/api/v1/sessions/%s/messages", baseURL, sessId), &b)
	req.Header.Set("Content-Type", w.FormDataContentType())

	resp, err := client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	require.Equal(t, http.StatusOK, resp.StatusCode)

	var out map[string]any
	json.NewDecoder(resp.Body).Decode(&out)

	return out
}
