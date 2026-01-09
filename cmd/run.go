package cmd

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"github.com/gorilla/mux"
	mcp "github.com/mark3labs/mcp-go/server"
	"github.com/urfave/cli/v2"
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
	"github.com/w-h-a/gomento/internal/server"
	httpserver "github.com/w-h-a/gomento/internal/server/http"
	mcpserver "github.com/w-h-a/gomento/internal/server/mcp"
	v1fileservice "github.com/w-h-a/gomento/internal/service/v1_file"
	v1sessionservice "github.com/w-h-a/gomento/internal/service/v1_session"
	v1spaceservice "github.com/w-h-a/gomento/internal/service/v1_space"
	v1workerservice "github.com/w-h-a/gomento/internal/service/v1_worker"
)

func Run(c *cli.Context) error {
	ctx := c.Context

	stopChannels := map[string]chan struct{}{}

	mode := c.String("mode")
	qname := c.String("qname")

	if len(qname) == 0 {
		qname = "worker"
	}

	apiKey := c.String("api_key")

	disp, err := InitV1Dispatcher(ctx)
	if err != nil {
		return err
	}

	var workerService *v1workerservice.V1Service
	if mode == "" || mode == "worker" {
		slog.InfoContext(ctx, "initiating worker")

		p, err := InitV1Persister(ctx, "postgres://user:password@localhost:5432/gomento?sslmode=disable")
		if err != nil {
			return err
		}

		i, err := InitV1Interpreter(
			ctx,
			apiKey,
			"gpt-3.5-turbo",
		)
		if err != nil {
			return err
		}

		e, err := InitEmbedder(
			ctx,
			apiKey,
			"text-embedding-3-small",
		)
		if err != nil {
			return err
		}

		workerService = v1workerservice.NewV1Service(
			p,
			disp,
			i,
			e,
		)
		stopChannels["worker"] = make(chan struct{})
	}

	var spaceService *v1spaceservice.V1Service
	var sessionService *v1sessionservice.V1Service
	var fileService *v1fileservice.V1Service
	var httpServer server.Server
	if mode == "" || mode == "server" {
		slog.InfoContext(ctx, "initiating http server")

		p, err := InitV1Persister(ctx, "postgres://user:password@localhost:5432/gomento?sslmode=disable")
		if err != nil {
			return err
		}

		f, err := InitV1Filer(
			ctx,
			"http://localhost:9000",
			"http://localhost:9000",
			"us-east-1",
			"gomento-assets",
			"user",
			"password",
		)
		if err != nil {
			return err
		}

		e, err := InitEmbedder(
			ctx,
			apiKey,
			"text-embedding-3-small",
		)
		if err != nil {
			return err
		}

		spaceService = v1spaceservice.NewV1Service(p, e)
		stopChannels["space"] = make(chan struct{})
		sessionService = v1sessionservice.NewV1Service(
			p,
			disp,
			f,
			e,
			qname,
		)
		stopChannels["session"] = make(chan struct{})
		fileService = v1fileservice.NewV1Service(
			p,
			f,
		)
		stopChannels["file"] = make(chan struct{})

		httpServer, err = InitV1HttpServer(
			ctx,
			":4000",
			spaceService,
			sessionService,
			fileService,
		)
		stopChannels["httpserver"] = make(chan struct{})
	}

	var wg sync.WaitGroup
	errCh := make(chan error, len(stopChannels))
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	if workerService != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			slog.InfoContext(ctx, "worker running")
			errCh <- workerService.Run(
				stopChannels["worker"],
				func() error {
					return workerService.Subscribe(ctx, workerService.ProcessJob, qname)
				},
				func(ctx context.Context) error {
					return workerService.Close(ctx)
				},
			)
		}()
	}

	if httpServer != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			slog.InfoContext(ctx, "space service running")
			errCh <- spaceService.Run(
				stopChannels["space"],
				nil,
				nil,
			)
		}()

		wg.Add(1)
		go func() {
			defer wg.Done()
			slog.InfoContext(ctx, "session service running")
			errCh <- sessionService.Run(
				stopChannels["session"],
				nil,
				nil,
			)
		}()

		wg.Add(1)
		go func() {
			defer wg.Done()
			slog.InfoContext(ctx, "file service running")
			errCh <- fileService.Run(
				stopChannels["file"],
				nil,
				nil,
			)
		}()

		wg.Add(1)
		go func() {
			defer wg.Done()
			slog.InfoContext(ctx, "http server running")
			errCh <- httpServer.Run(stopChannels["httpserver"])
		}()
	}

	select {
	case err := <-errCh:
		if err != nil {
			slog.ErrorContext(ctx, "startup failure", "error", err)
			return err
		}
	case <-sigChan:
		for _, stop := range stopChannels {
			close(stop)
		}
	}

	wg.Wait()

	close(errCh)
	for err := range errCh {
		if err != nil {
			slog.ErrorContext(ctx, "close failure", "error", err)
		}
	}

	slog.InfoContext(ctx, "shutdown complete")

	return nil
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
	// space handler

	sessionHandler := v1sessionmcphandler.NewV1Handler(sess)
	sessTools := []mcp.ServerTool{
		sessionHandler.CreateTool(),
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

	if err := srv.Handle(v1); err != nil {
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
	v1.Methods("POST").Path("/sessions/{session_id}/checkpoint").HandlerFunc(sessionHandler.CheckpointSession)
	v1.Methods("GET").Path("/sessions/{session_id}/tasks").HandlerFunc(sessionHandler.ListTasks)
	v1.Methods("POST").Path("/sessions/{session_id}/finish").HandlerFunc(sessionHandler.FinishSession)

	fileHandler := v1filehttphandler.NewV1Handler(file)
	v1.Methods("POST").Path("/files").HandlerFunc(fileHandler.UploadFile)
	v1.Methods("GET").Path("/files").HandlerFunc(fileHandler.ListFiles)
	v1.Methods("GET").Path("/files/{file_id}").HandlerFunc(fileHandler.GetFile)
	v1.Methods("POST").Path("/files/{file_id}/connect_to_space").HandlerFunc(fileHandler.ConnectToSpace)

	return v1, nil
}
