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
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	_ "github.com/lib/pq"
	"github.com/stretchr/testify/require"

	"github.com/w-h-a/gomento/cmd"
	v1file "github.com/w-h-a/gomento/internal/service/v1_file"
	v1session "github.com/w-h-a/gomento/internal/service/v1_session"
	v1space "github.com/w-h-a/gomento/internal/service/v1_space"
	v1worker "github.com/w-h-a/gomento/internal/service/v1_worker"
)

const (
	DB_CONN      = "postgres://user:password@localhost:5432/gomento?sslmode=disable"
	MINIO_END    = "http://localhost:9000"
	MINIO_PUBLIC = "http://localhost:9000"
	MINIO_USER   = "user"
	MINIO_PASS   = "password"
	TEST_BUCKET  = "gomento-assets"
)

func setupIntegrationServer(t *testing.T) (*http.Client, string, *sql.DB, *s3.Client) {
	ctx := context.Background()

	db, err := sql.Open("postgres", DB_CONN)
	require.NoError(t, err)
	require.NoError(t, db.Ping())
	_, err = db.Exec(`TRUNCATE TABLE jobs, skills, message_assets, messages, tasks, sessions, spaces, assets, files CASCADE`)
	require.NoError(t, err)

	s3Config, _ := config.LoadDefaultConfig(ctx,
		config.WithRegion("us-east-1"),
		config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(MINIO_USER, MINIO_PASS, "")),
		config.WithEndpointResolverWithOptions(aws.EndpointResolverWithOptionsFunc(
			func(service, region string, options ...any) (aws.Endpoint, error) {
				return aws.Endpoint{URL: MINIO_PUBLIC}, nil
			},
		)),
	)
	s3Client := s3.NewFromConfig(s3Config, func(o *s3.Options) { o.UsePathStyle = true })

	list, err := s3Client.ListObjectsV2(ctx, &s3.ListObjectsV2Input{
		Bucket: aws.String(TEST_BUCKET),
	})
	require.NoError(t, err)
	for _, obj := range list.Contents {
		_, _ = s3Client.DeleteObject(ctx, &s3.DeleteObjectInput{
			Bucket: aws.String(TEST_BUCKET),
			Key:    obj.Key,
		})
	}

	p, _ := cmd.InitV1Persister(ctx, DB_CONN)
	d, _ := cmd.InitV1Dispatcher(ctx)
	i, _ := cmd.InitV1Interpreter(ctx, "", "")
	e, _ := cmd.InitEmbedder(ctx, "", "")
	f, _ := cmd.InitV1Filer(
		ctx,
		MINIO_END,
		MINIO_PUBLIC,
		"us-east-1",
		TEST_BUCKET,
		MINIO_USER,
		MINIO_PASS,
	)

	spaceSvc := v1space.NewV1Service(p, e)
	sessSvc := v1session.NewV1Service(p, d, f, e, "worker")
	fileSvc := v1file.NewV1Service(p, f)
	workerSvc := v1worker.NewV1Service(p, d, i, e)

	go func() { workerSvc.Subscribe(ctx, workerSvc.ProcessJob, "worker") }()

	r, _ := cmd.InitV1Router(
		ctx,
		spaceSvc,
		sessSvc,
		fileSvc,
	)

	ts := httptest.NewServer(r)
	t.Cleanup(ts.Close)

	return ts.Client(), ts.URL, db, s3Client
}

func createJson(t *testing.T, client *http.Client, url string, data map[string]any) map[string]any {
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
) v1session.ListMessagesOutput {
	url := fmt.Sprintf("%s/api/v1/sessions/%s/messages?limit=%d&cursor=%s&with_asset_public_url=%v", baseURL, sessId, limit, cursor, withUrl)
	req, _ := http.NewRequest("GET", url, nil)
	resp, err := client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	require.Equal(t, http.StatusOK, resp.StatusCode)

	var res v1session.ListMessagesOutput
	err = json.NewDecoder(resp.Body).Decode(&res)
	require.NoError(t, err)

	return res
}
