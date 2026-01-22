package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"github.com/alecthomas/kong"
	"github.com/w-h-a/gomento/internal/app/gomento"
	"github.com/w-h-a/gomento/internal/server"
	v1fileservice "github.com/w-h-a/gomento/internal/service/v1_file"
	v1sessionservice "github.com/w-h-a/gomento/internal/service/v1_session"
	v1spaceservice "github.com/w-h-a/gomento/internal/service/v1_space"
	v1workerservice "github.com/w-h-a/gomento/internal/service/v1_worker"
	"go.opentelemetry.io/contrib/bridges/otelslog"
	"go.opentelemetry.io/otel"
	globallog "go.opentelemetry.io/otel/log/global"
	"go.opentelemetry.io/otel/propagation"
	logsdk "go.opentelemetry.io/otel/sdk/log"
	"go.opentelemetry.io/otel/sdk/resource"
	tracesdk "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.20.0"
)

type CLI struct {
	Mode         string `env:"MODE" default:""`
	Name         string `env:"NAME" default:"gomento"`
	Version      string `env:"VERSION" default:"v0.1.0"`
	SessionQName string `env:"SESSION_Q_NAME" default:"session"`
	FileQName    string `env:"FILE_Q_NAME" default:"file"`

	LogsExporterLocation   string `env:"LOGS_EXPORTER_LOCATION" default:"stdout"`
	TracesExporterLocation string `env:"TRACES_EXPORTER_LOCATION" default:"jaeger:4318"`
	PersisterLocation      string `env:"PERSISTER_LOCATION" default:"postgres://user:password@localhost:5432/gomento?sslmode=disable"`
	FilerEndpoint          string `env:"FILER_ENDPOINT" default:"http://localhost:9000"`
	FilerPublicEndpoint    string `env:"FILER_PUBLIC_ENDPOINT" default:"http://localhost:9000"`
	FilerRegion            string `env:"FILER_REGION" default:"us-east-1"`
	FilerContainer         string `env:"FILER_CONTAINER" default:"gomento-assets"`
	FilerUser              string `env:"FILER_USER" default:"user"`
	FilerPassword          string `env:"FILER_PASSWORD" default:"password"`
	OpenAIAPIKey           string `env:"OPENAI_API_KEY" default:""`
	InterpreterModel       string `env:"INTERPRETER_MODEL" default:"gpt-3.5-turbo"`
	EmbedderModel          string `env:"EMBEDDER_MODEL" default:"text-embedding-3-small"`
	McpServerAddr          string `env:"MCP_SERVER_ADDR" default:":4001"`
	HttpServerAddr         string `env:"HTTP_SERVER_ADDR" default:":4000"`

	RunAll RunAllCmd `cmd:"" default:"1"`
}

type RunAllCmd struct{}

func (c *RunAllCmd) Run(cli *CLI) error {
	ctx := context.Background()

	// resource
	resource, err := resource.New(
		ctx,
		resource.WithAttributes(
			semconv.ServiceName(cli.Name),
		),
		resource.WithProcess(),
	)
	if err != nil {
		return err
	}

	// logs
	logsExporter, err := gomento.InitLogsExporter(ctx)
	if err != nil {
		return err
	}

	lp := logsdk.NewLoggerProvider(
		logsdk.WithResource(resource),
		logsdk.WithProcessor(
			logsdk.NewBatchProcessor(logsExporter),
		),
	)

	defer lp.Shutdown(ctx)

	globallog.SetLoggerProvider(lp)

	logger := otelslog.NewLogger(
		cli.Name,
		otelslog.WithLoggerProvider(lp),
	)

	slog.SetDefault(logger)

	// traces
	traceExporter, err := gomento.InitTracesExporter(ctx, cli.TracesExporterLocation)
	if err != nil {
		return err
	}

	tp := tracesdk.NewTracerProvider(
		tracesdk.WithResource(resource),
		tracesdk.WithSampler(tracesdk.AlwaysSample()),
		tracesdk.WithSpanProcessor(
			tracesdk.NewBatchSpanProcessor(
				traceExporter,
			),
		),
	)

	defer tp.Shutdown(ctx)

	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(
		propagation.NewCompositeTextMapPropagator(
			propagation.TraceContext{},
			propagation.Baggage{},
		),
	)

	// setup & run
	stopChannels := map[string]chan struct{}{}

	disp, err := gomento.InitV1Dispatcher(ctx)
	if err != nil {
		return err
	}

	var workerService *v1workerservice.V1Service
	if cli.Mode == "" || cli.Mode == "worker" {
		slog.InfoContext(ctx, "initiating worker")

		var err error

		workerService, err = gomento.InitV1Worker(
			ctx,
			disp,
			cli.PersisterLocation,
			cli.FilerEndpoint,
			cli.FilerPublicEndpoint,
			cli.FilerRegion,
			cli.FilerContainer,
			cli.FilerUser,
			cli.FilerPassword,
			cli.OpenAIAPIKey,
			cli.InterpreterModel,
			cli.EmbedderModel,
		)
		if err != nil {
			return err
		}
		stopChannels["worker"] = make(chan struct{})
	}

	var spaceService *v1spaceservice.V1Service
	var sessionService *v1sessionservice.V1Service
	var fileService *v1fileservice.V1Service
	var mcpServer server.Server
	var httpServer server.Server
	if cli.Mode == "" || cli.Mode == "server" {
		slog.InfoContext(ctx, "initiating mcp & http servers")

		var err error

		spaceService, err = gomento.InitV1SpaceService(
			ctx,
			cli.PersisterLocation,
			cli.OpenAIAPIKey,
			cli.EmbedderModel,
		)
		if err != nil {
			return err
		}
		stopChannels["space"] = make(chan struct{})

		sessionService, err = gomento.InitV1SessionService(
			ctx,
			disp,
			cli.PersisterLocation,
			cli.FilerEndpoint,
			cli.FilerPublicEndpoint,
			cli.FilerRegion,
			cli.FilerContainer,
			cli.FilerUser,
			cli.FilerPassword,
			cli.OpenAIAPIKey,
			cli.EmbedderModel,
			cli.SessionQName,
		)
		if err != nil {
			return err
		}
		stopChannels["session"] = make(chan struct{})

		fileService, err = gomento.InitV1FileService(
			ctx,
			cli.PersisterLocation,
			disp,
			cli.FilerEndpoint,
			cli.FilerPublicEndpoint,
			cli.FilerRegion,
			cli.FilerContainer,
			cli.FilerUser,
			cli.FilerPassword,
			cli.OpenAIAPIKey,
			cli.EmbedderModel,
			cli.FileQName,
		)
		if err != nil {
			return err
		}
		stopChannels["file"] = make(chan struct{})

		mcpServer, err = gomento.InitV1McpServer(
			ctx,
			cli.McpServerAddr,
			cli.Name,
			cli.Version,
			spaceService,
			sessionService,
			fileService,
		)
		if err != nil {
			return err
		}
		stopChannels["mcpserver"] = make(chan struct{})

		httpServer, err = gomento.InitV1HttpServer(
			ctx,
			cli.HttpServerAddr,
			spaceService,
			sessionService,
			fileService,
		)
		if err != nil {
			return err
		}
		stopChannels["httpserver"] = make(chan struct{})
	}

	var wg sync.WaitGroup
	errCh := make(chan error, len(stopChannels))
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	if cli.Mode == "" || cli.Mode == "worker" {
		wg.Add(1)
		go func() {
			defer wg.Done()
			slog.InfoContext(ctx, "worker running")
			errCh <- workerService.Run(
				stopChannels["worker"],
				func() error {
					if err := workerService.Subscribe(ctx, workerService.ProcessJob, cli.SessionQName); err != nil {
						return err
					}
					if err := workerService.Subscribe(ctx, workerService.ProcessJob, cli.FileQName); err != nil {
						return err
					}
					return nil
				},
				func(ctx context.Context) error {
					return workerService.Close(ctx)
				},
			)
		}()
	}

	if cli.Mode == "" || cli.Mode == "server" {
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
			slog.InfoContext(ctx, "mcp server running")
			errCh <- mcpServer.Run(stopChannels["mcpserver"])
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

func main() {
	var cli CLI
	ctx := kong.Parse(&cli)
	err := ctx.Run()
	ctx.FatalIfErrorf(err)
}
