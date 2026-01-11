package integration

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	_ "github.com/lib/pq"
	"github.com/stretchr/testify/require"

	"github.com/mark3labs/mcp-go/client"
	mcpclient "github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"
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

type mcpTestAdapter struct {
	server *mcpserver.MCPServer
}

func (a *mcpTestAdapter) Handle(handler any) error {
	switch h := handler.(type) {
	case mcpserver.ServerTool:
		a.server.AddTools(h)
	default:
		return fmt.Errorf("unknown handler type in test adapter: %T", handler)
	}

	return nil
}

func setupMcpServer(t *testing.T) (*mcpclient.Client, *sql.DB, *s3.Client) {
	t.Helper()

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

	rawMcpServer := mcpserver.NewMCPServer("test-mcp", "1.0.0")

	adapter := &mcpTestAdapter{server: rawMcpServer}

	_ = cmd.RegisterV1McpHandlers(ctx, adapter, spaceSvc, sessSvc, fileSvc)

	ts := httptest.NewServer(mcpserver.NewStreamableHTTPServer(rawMcpServer))
	t.Cleanup(ts.Close)

	cl, err := client.NewStreamableHttpClient(ts.URL + "/sse")
	require.NoError(t, err)

	err = cl.Start(ctx)
	require.NoError(t, err)

	initReq := mcp.InitializeRequest{}
	initReq.Params.ProtocolVersion = mcp.LATEST_PROTOCOL_VERSION
	initReq.Params.ClientInfo = mcp.Implementation{Name: "test-client", Version: "0.1.0"}
	_, err = cl.Initialize(ctx, initReq)
	require.NoError(t, err)

	return cl, db, s3Client
}

func setupHttpServer(t *testing.T) (*http.Client, string, *sql.DB, *s3.Client) {
	t.Helper()

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

	r, _ := cmd.RegisterV1HttpHandlers(
		ctx,
		spaceSvc,
		sessSvc,
		fileSvc,
	)

	ts := httptest.NewServer(r)
	t.Cleanup(ts.Close)

	return ts.Client(), ts.URL, db, s3Client
}

// TODO: remove
func createWithHttp(t *testing.T, client *http.Client, url string, data map[string]any) map[string]any {
	body, _ := json.Marshal(data)
	rsp, err := client.Post(url, "application/json", bytes.NewBuffer(body))
	require.NoError(t, err)
	defer rsp.Body.Close()

	require.Equal(t, http.StatusCreated, rsp.StatusCode)

	var out map[string]any
	json.NewDecoder(rsp.Body).Decode(&out)

	return out
}
