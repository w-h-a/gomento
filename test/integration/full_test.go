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
	"github.com/google/uuid"
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
	req, _ := http.NewRequest("POST", fmt.Sprintf("%s/api/v1/sessions/%s/finish", baseURL, sessionId), nil)
	resp, err := client.Do(req)
	require.NoError(t, err)

	// Assert
	assert.Equal(t, http.StatusAccepted, resp.StatusCode)

	assert.Eventually(t, func() bool {
		var status string
		err := db.QueryRow("SELECT status FROM tasks WHERE session_id = $1", sessionId).Scan(&status)
		return err == nil && status == "success"
	}, 5*time.Second, 100*time.Millisecond, "Persistent Task should succeed")

	assert.Eventually(t, func() bool {
		var trigger string
		err := db.QueryRow("SELECT trigger FROM skills WHERE space_id = $1", spaceId).Scan(&trigger)
		return err == nil && trigger == "how to restart redis"
	}, 5*time.Second, 100*time.Millisecond, "Skill should be created in DB")
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
	f, _ := cmd.InitV1Filer(
		ctx,
		MINIO_END,
		"us-east-1",
		TEST_BUCKET,
		MINIO_USER,
		MINIO_PASS,
	)

	projSvc := v1project.NewV1Service(p)
	spaceSvc := v1space.NewV1Service(p)
	sessSvc := v1session.NewV1Service(p, disp, f, "worker")
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

func getMessages(
	t *testing.T,
	client *http.Client,
	baseURL string,
	sessId string,
	limit int,
	cursor string,
	withUrl bool,
) v1session.GetMessagesOutput {
	url := fmt.Sprintf("%s/api/v1/sessions/%s/messages?limit=%d&cursor=%s&with_asset_public_url=%v", baseURL, sessId, limit, cursor, withUrl)
	req, _ := http.NewRequest("GET", url, nil)
	resp, err := client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	require.Equal(t, http.StatusOK, resp.StatusCode)

	var res v1session.GetMessagesOutput
	err = json.NewDecoder(resp.Body).Decode(&res)
	require.NoError(t, err)

	return res
}
