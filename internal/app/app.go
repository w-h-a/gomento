package app

import (
	"context"
	"fmt"
	gohttp "net/http"

	"github.com/gorilla/mux"
	mcp "github.com/mark3labs/mcp-go/server"
	"github.com/w-h-a/gomento/internal/client/dispatcher"
	v1memory "github.com/w-h-a/gomento/internal/client/dispatcher/v1_memory"
	"github.com/w-h-a/gomento/internal/client/embedder"
	"github.com/w-h-a/gomento/internal/client/embedder/mock"
	"github.com/w-h-a/gomento/internal/client/embedder/openai"
	"github.com/w-h-a/gomento/internal/client/filer"
	v1s3 "github.com/w-h-a/gomento/internal/client/filer/v1_s3"
	"github.com/w-h-a/gomento/internal/client/interpreter"
	v1mock "github.com/w-h-a/gomento/internal/client/interpreter/v1_mock"
	v1openai "github.com/w-h-a/gomento/internal/client/interpreter/v1_openai"
	"github.com/w-h-a/gomento/internal/client/persister"
	v1postgres "github.com/w-h-a/gomento/internal/client/persister/v1_postgres"
	v1filehttphandler "github.com/w-h-a/gomento/internal/handler/http/v1_file"
	v1sessionhttphandler "github.com/w-h-a/gomento/internal/handler/http/v1_session"
	v1spacehttphandler "github.com/w-h-a/gomento/internal/handler/http/v1_space"
	v1filemcphandler "github.com/w-h-a/gomento/internal/handler/mcp/v1_file"
	v1sessionmcphandler "github.com/w-h-a/gomento/internal/handler/mcp/v1_session"
	v1spacemcphandler "github.com/w-h-a/gomento/internal/handler/mcp/v1_space"
	"github.com/w-h-a/gomento/internal/server"
	httpserver "github.com/w-h-a/gomento/internal/server/http"
	mcpserver "github.com/w-h-a/gomento/internal/server/mcp"
	v1fileservice "github.com/w-h-a/gomento/internal/service/v1_file"
	v1sessionservice "github.com/w-h-a/gomento/internal/service/v1_session"
	v1spaceservice "github.com/w-h-a/gomento/internal/service/v1_space"
	v1workerservice "github.com/w-h-a/gomento/internal/service/v1_worker"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/exporters/stdout/stdoutlog"
	logsdk "go.opentelemetry.io/otel/sdk/log"
	tracesdk "go.opentelemetry.io/otel/sdk/trace"
)

// TODO: accept user configuration
func InitLogsExporter(ctx context.Context) (logsdk.Exporter, error) {
	return stdoutlog.New()
}

// TODO: accept user configuration
func InitTracesExporter(ctx context.Context, loc string) (tracesdk.SpanExporter, error) {
	return otlptracehttp.New(
		ctx,
		otlptracehttp.WithEndpoint(loc),
		otlptracehttp.WithInsecure(),
	)
}

// TODO: accept user configuration
func InitV1Persister(ctx context.Context, loc string) (persister.V1Persister, error) {
	return v1postgres.NewV1Persister(
		persister.WithLocation(loc),
	), nil
}

// TODO: accept user configuration
func InitV1Dispatcher(ctx context.Context) (dispatcher.V1Dispatcher, error) {
	return v1memory.NewV1Dispatcher(), nil
}

// TODO: accept user configuration
func InitV1Interpreter(
	ctx context.Context,
	apiKey string,
	model string,
) (interpreter.V1Interpreter, error) {
	if len(apiKey) > 0 {
		return v1openai.NewV1Interpreter(
			interpreter.WithApiKey(apiKey),
			interpreter.WithModel(model),
		), nil
	}

	return v1mock.NewV1Interpreter(
		v1mock.WithActionRsp(
			[]interpreter.TaskAction{
				{
					Type: interpreter.TaskActionInsert,
					Payload: map[string]any{
						"after_task_order": 0.0,
						"task_description": "Integration Test Task",
					},
				},
			},
		),
	), nil
}

// TODO: accept user configuration
func InitEmbedder(
	ctx context.Context,
	apiKey string,
	model string,
) (embedder.Embedder, error) {
	if len(apiKey) > 0 {
		return openai.NewEmbedder(
			embedder.WithApiKey(apiKey),
			embedder.WithModel(model),
		), nil
	}

	return mock.NewEmbedder(), nil
}

// TODO: accept user configuration
func InitV1Filer(
	ctx context.Context,
	endpoint string,
	publicEndpoint string,
	region string,
	container string,
	user string,
	password string,
) (filer.V1Filer, error) {
	return v1s3.NewV1Filer(
		filer.WithEndpoint(endpoint),
		filer.WithPublicEndpoint(publicEndpoint),
		filer.WithRegion(region),
		filer.WithContainer(container),
		filer.WithUser(user),
		filer.WithSecret(password),
	), nil
}

func InitV1Worker(
	ctx context.Context,
	disp dispatcher.V1Dispatcher,
	persisterLocation string,
	openAIAPIKey string,
	interpreterModel string,
	embedderModel string,
) (*v1workerservice.V1Service, error) {
	p, err := InitV1Persister(ctx, persisterLocation)
	if err != nil {
		return nil, err
	}

	i, err := InitV1Interpreter(
		ctx,
		openAIAPIKey,
		interpreterModel,
	)
	if err != nil {
		return nil, err
	}

	e, err := InitEmbedder(
		ctx,
		openAIAPIKey,
		embedderModel,
	)
	if err != nil {
		return nil, err
	}

	workerService := v1workerservice.NewV1Service(
		p,
		disp,
		i,
		e,
	)

	return workerService, nil
}

func InitV1SpaceService(
	ctx context.Context,
	persisterLocation string,
	openAIAPIKey string,
	embedderModel string,
) (*v1spaceservice.V1Service, error) {
	p, err := InitV1Persister(ctx, persisterLocation)
	if err != nil {
		return nil, err
	}

	e, err := InitEmbedder(
		ctx,
		openAIAPIKey,
		embedderModel,
	)
	if err != nil {
		return nil, err
	}

	spaceService := v1spaceservice.NewV1Service(
		p,
		e,
	)

	return spaceService, nil
}

func InitV1SessionService(
	ctx context.Context,
	disp dispatcher.V1Dispatcher,
	persisterLocation string,
	filerEndpoint string,
	filerPublicEndpoint string,
	filerRegion string,
	filerContainer string,
	filerUser string,
	filerPassword string,
	openAIAPIKey string,
	embedderModel string,
	qName string,
) (*v1sessionservice.V1Service, error) {
	p, err := InitV1Persister(ctx, persisterLocation)
	if err != nil {
		return nil, err
	}

	f, err := InitV1Filer(
		ctx,
		filerEndpoint,
		filerPublicEndpoint,
		filerRegion,
		filerContainer,
		filerUser,
		filerPassword,
	)
	if err != nil {
		return nil, err
	}

	e, err := InitEmbedder(
		ctx,
		openAIAPIKey,
		embedderModel,
	)
	if err != nil {
		return nil, err
	}

	sessionService := v1sessionservice.NewV1Service(
		p,
		disp,
		f,
		e,
		qName,
	)

	return sessionService, nil
}

func InitV1FileService(
	ctx context.Context,
	persisterLocation string,
	filerEndpoint string,
	filerPublicEndpoint string,
	filerRegion string,
	filerContainer string,
	filerUser string,
	filerPassword string,
) (*v1fileservice.V1Service, error) {
	p, err := InitV1Persister(ctx, persisterLocation)
	if err != nil {
		return nil, err
	}

	f, err := InitV1Filer(
		ctx,
		filerEndpoint,
		filerPublicEndpoint,
		filerRegion,
		filerContainer,
		filerUser,
		filerPassword,
	)
	if err != nil {
		return nil, err
	}

	filerService := v1fileservice.NewV1Service(
		p,
		f,
	)

	return filerService, nil
}

// TODO: accept user configuration
func InitV1McpServer(
	ctx context.Context,
	addr string,
	name string,
	version string,
	spac *v1spaceservice.V1Service,
	sess *v1sessionservice.V1Service,
	file *v1fileservice.V1Service,
) (server.Server, error) {
	srv := mcpserver.NewServer(
		server.WithAddress(addr),
		server.WithName(name),
		server.WithVersion(version),
	)

	if err := RegisterV1McpHandlers(ctx, srv, spac, sess, file); err != nil {
		return nil, fmt.Errorf("failed to init mcp router: %w", err)
	}

	return srv, nil
}

func RegisterV1McpHandlers(
	ctx context.Context,
	registrar mcpserver.Registrar,
	spac *v1spaceservice.V1Service,
	sess *v1sessionservice.V1Service,
	file *v1fileservice.V1Service,
) error {
	spaceHandler := v1spacemcphandler.NewV1Handler(spac)
	spaceTools := []mcp.ServerTool{
		spaceHandler.CreateTool(),
		spaceHandler.ListSpacesTool(),
		spaceHandler.GetSpaceTool(),
		spaceHandler.SearchSkillsTool(),
		spaceHandler.SearchMessagesTool(),
	}
	for _, t := range spaceTools {
		if err := registrar.Handle(t); err != nil {
			return err
		}
	}

	sessionHandler := v1sessionmcphandler.NewV1Handler(sess)
	sessTools := []mcp.ServerTool{
		sessionHandler.CreateTool(),
		sessionHandler.ListSessionsTool(),
		sessionHandler.GetSessionTool(),
		sessionHandler.ConnectToSpaceTool(),
		sessionHandler.AddMessageTool(),
		sessionHandler.ListMessagesTool(),
		sessionHandler.ExtractTasksTool(),
		sessionHandler.ListTasksTool(),
		sessionHandler.DistillSkillTool(),
	}
	for _, t := range sessTools {
		if err := registrar.Handle(t); err != nil {
			return err
		}
	}

	fileHandler := v1filemcphandler.NewV1Handler(file)
	fileTools := []mcp.ServerTool{
		fileHandler.UploadFileTool(),
		fileHandler.ListFilesTool(),
		fileHandler.GetFileTool(),
		fileHandler.ConnectToSpaceTool(),
	}
	for _, t := range fileTools {
		if err := registrar.Handle(t); err != nil {
			return err
		}
	}

	return nil
}

// TODO: accept user configuration
func InitV1HttpServer(
	ctx context.Context,
	addr string,
	spac *v1spaceservice.V1Service,
	sess *v1sessionservice.V1Service,
	file *v1fileservice.V1Service,
) (server.Server, error) {
	srv := httpserver.NewServer(
		server.WithAddress(addr),
	)

	v1, err := RegisterV1HttpHandlers(ctx, spac, sess, file)
	if err != nil {
		return nil, fmt.Errorf("failed to init router: %w", err)
	}

	handler := otelhttp.NewHandler(
		v1,
		"",
		otelhttp.WithSpanNameFormatter(func(operation string, r *gohttp.Request) string { return r.URL.Path }),
		otelhttp.WithTracerProvider(otel.GetTracerProvider()),
		otelhttp.WithPropagators(otel.GetTextMapPropagator()),
		otelhttp.WithFilter(func(r *gohttp.Request) bool { return r.URL.Path != "/api/v1/status" }),
	)

	if err := srv.Handle(handler); err != nil {
		return nil, fmt.Errorf("failed to attach handler: %w", err)
	}

	return srv, nil
}

func RegisterV1HttpHandlers(
	ctx context.Context,
	spac *v1spaceservice.V1Service,
	sess *v1sessionservice.V1Service,
	file *v1fileservice.V1Service,
) (*mux.Router, error) {
	router := mux.NewRouter()
	v1 := router.PathPrefix("/api/v1").Subrouter()

	spaceHandler := v1spacehttphandler.NewV1Handler(spac)
	v1.Methods("POST").Path("/spaces").HandlerFunc(spaceHandler.Create)
	v1.Methods("GET").Path("/spaces").HandlerFunc(spaceHandler.ListSpaces)
	v1.Methods("GET").Path("/spaces/{space_id}").HandlerFunc(spaceHandler.GetSpace)
	v1.Methods("GET").Path("/spaces/{space_id}/skills").HandlerFunc(spaceHandler.SearchSkills)
	v1.Methods("GET").Path("/spaces/{space_id}/messages").HandlerFunc(spaceHandler.SearchMessages)

	sessionHandler := v1sessionhttphandler.NewV1Handler(sess)
	v1.Methods("POST").Path("/sessions").HandlerFunc(sessionHandler.Create)
	v1.Methods("GET").Path("/sessions").HandlerFunc(sessionHandler.ListSessions)
	v1.Methods("GET").Path("/sessions/{session_id}").HandlerFunc(sessionHandler.GetSession)
	v1.Methods("POST").Path("/sessions/{session_id}/connect_to_space").HandlerFunc(sessionHandler.ConnectToSpace)
	v1.Methods("POST").Path("/sessions/{session_id}/messages").HandlerFunc(sessionHandler.AddMessage)
	v1.Methods("GET").Path("/sessions/{session_id}/messages").HandlerFunc(sessionHandler.ListMessages)
	v1.Methods("POST").Path("/sessions/{session_id}/extract").HandlerFunc(sessionHandler.ExtractTasks)
	v1.Methods("GET").Path("/sessions/{session_id}/tasks").HandlerFunc(sessionHandler.ListTasks)
	v1.Methods("POST").Path("/sessions/{session_id}/distill").HandlerFunc(sessionHandler.DistillSkill)

	fileHandler := v1filehttphandler.NewV1Handler(file)
	v1.Methods("POST").Path("/files").HandlerFunc(fileHandler.UploadFile)
	v1.Methods("GET").Path("/files").HandlerFunc(fileHandler.ListFiles)
	v1.Methods("GET").Path("/files/{file_id}").HandlerFunc(fileHandler.GetFile)
	v1.Methods("POST").Path("/files/{file_id}/connect_to_space").HandlerFunc(fileHandler.ConnectToSpace)

	return v1, nil
}
