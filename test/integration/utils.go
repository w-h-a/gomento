package integration

import (
	"context"
	"database/sql"
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
	"github.com/w-h-a/gomento/internal/app/gomento"
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
	_, err = db.Exec(`TRUNCATE TABLE jobs, skills, message_assets, messages, tasks, sessions, spaces, assets, files, file_chunks CASCADE`)
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

	p, _ := gomento.InitV1Persister(ctx, DB_CONN)
	b, _ := gomento.InitV1Buffer(ctx)
	d, _ := gomento.InitV1Dispatcher(ctx)
	i, _ := gomento.InitV1Interpreter(ctx, "", "")
	e, _ := gomento.InitEmbedder(ctx, "", "")
	f, _ := gomento.InitV1Filer(
		ctx,
		MINIO_END,
		MINIO_PUBLIC,
		"us-east-1",
		TEST_BUCKET,
		MINIO_USER,
		MINIO_PASS,
	)

	spaceSvc := v1space.NewV1Service(p, e)
	sessSvc := v1session.NewV1Service(p, b, d, f, "session", "file")
	fileSvc := v1file.NewV1Service(p, d, f, "file")
	workerSvc := v1worker.NewV1Service(p, b, d, f, i, e)

	go func() {
		workerSvc.Subscribe(ctx, workerSvc.ProcessJob, "session")
		workerSvc.Subscribe(ctx, workerSvc.ProcessJob, "file")
	}()

	ingestionCtx, ingestionCancel := context.WithCancel(ctx)
	t.Cleanup(ingestionCancel)
	go workerSvc.StartIngestion(ingestionCtx)

	rawMcpServer := mcpserver.NewMCPServer("test-mcp", "1.0.0")

	adapter := &mcpTestAdapter{server: rawMcpServer}

	_ = gomento.RegisterV1McpHandlers(ctx, adapter, spaceSvc, sessSvc, fileSvc)

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
	_, err = db.Exec(`TRUNCATE TABLE jobs, skills, message_assets, messages, tasks, sessions, spaces, assets, files, file_chunks CASCADE`)
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

	p, _ := gomento.InitV1Persister(ctx, DB_CONN)
	b, _ := gomento.InitV1Buffer(ctx)
	d, _ := gomento.InitV1Dispatcher(ctx)
	i, _ := gomento.InitV1Interpreter(ctx, "", "")
	e, _ := gomento.InitEmbedder(ctx, "", "")
	f, _ := gomento.InitV1Filer(
		ctx,
		MINIO_END,
		MINIO_PUBLIC,
		"us-east-1",
		TEST_BUCKET,
		MINIO_USER,
		MINIO_PASS,
	)

	spaceSvc := v1space.NewV1Service(p, e)
	sessSvc := v1session.NewV1Service(p, b, d, f, "session", "file")
	fileSvc := v1file.NewV1Service(p, d, f, "file")
	workerSvc := v1worker.NewV1Service(p, b, d, f, i, e)

	go func() {
		workerSvc.Subscribe(ctx, workerSvc.ProcessJob, "session")
		workerSvc.Subscribe(ctx, workerSvc.ProcessJob, "file")
	}()

	ingestionCtx, ingestionCancel := context.WithCancel(ctx)
	t.Cleanup(ingestionCancel)
	go workerSvc.StartIngestion(ingestionCtx)

	r, _ := gomento.RegisterV1HttpHandlers(
		ctx,
		spaceSvc,
		sessSvc,
		fileSvc,
	)

	ts := httptest.NewServer(r)
	t.Cleanup(ts.Close)

	return ts.Client(), ts.URL, db, s3Client
}
