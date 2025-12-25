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
	"github.com/urfave/cli/v2"
	"github.com/w-h-a/gomento/internal/client/dispatcher"
	v1memory "github.com/w-h-a/gomento/internal/client/dispatcher/v1_memory"
	"github.com/w-h-a/gomento/internal/client/distiller"
	v1mock "github.com/w-h-a/gomento/internal/client/distiller/v1_mock"
	v1openai "github.com/w-h-a/gomento/internal/client/distiller/v1_openai"
	"github.com/w-h-a/gomento/internal/client/filer"
	v1s3 "github.com/w-h-a/gomento/internal/client/filer/v1_s3"
	"github.com/w-h-a/gomento/internal/client/persister"
	v1postgres "github.com/w-h-a/gomento/internal/client/persister/v1_postgres"
	v1projecthttphandler "github.com/w-h-a/gomento/internal/handler/http/v1_project"
	v1sessionhttphandler "github.com/w-h-a/gomento/internal/handler/http/v1_session"
	v1spacehttphandler "github.com/w-h-a/gomento/internal/handler/http/v1_space"
	"github.com/w-h-a/gomento/internal/server"
	httpserver "github.com/w-h-a/gomento/internal/server/http"
	v1projectservice "github.com/w-h-a/gomento/internal/service/v1_project"
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

		apiKey := c.String("api_key")

		dist, err := InitV1Distiller(
			ctx,
			apiKey,
			"gpt-3.5-turbo",
		)
		if err != nil {
			return err
		}

		workerService = v1workerservice.NewV1Service(
			p,
			disp,
			dist,
		)
		stopChannels["worker"] = make(chan struct{})
	}

	var projectService *v1projectservice.V1Service
	var spaceService *v1spaceservice.V1Service
	var sessionService *v1sessionservice.V1Service
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
			"us-east-1",
			"gomento-assets",
			"user",
			"password",
		)
		if err != nil {
			return err
		}

		projectService = v1projectservice.NewV1Service(p)
		stopChannels["project"] = make(chan struct{})
		spaceService = v1spaceservice.NewV1Service(p)
		stopChannels["space"] = make(chan struct{})
		sessionService = v1sessionservice.NewV1Service(
			p,
			disp,
			f,
			qname,
		)
		stopChannels["session"] = make(chan struct{})

		httpServer, err = InitV1HttpServer(
			ctx,
			":4000",
			projectService,
			spaceService,
			sessionService,
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
					return workerService.Subscribe(ctx, workerService.ProcessTask, qname)
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
			slog.InfoContext(ctx, "project service running")
			errCh <- projectService.Run(
				stopChannels["project"],
				nil,
				nil,
			)
		}()

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
func InitV1Distiller(
	ctx context.Context,
	apiKey string,
	model string,
) (distiller.V1Distiller, error) {
	if len(apiKey) > 0 {
		return v1openai.NewV1Distiller(
			distiller.WithApiKey(apiKey),
			distiller.WithModel(model),
		), nil
	}

	return v1mock.NewV1Distiller(), nil
}

// TODO: accept user configuration
func InitV1Filer(
	ctx context.Context,
	endpoint string,
	region string,
	container string,
	user string,
	password string,
) (filer.V1Filer, error) {
	return v1s3.NewV1Filer(
		filer.WithEndpoint(endpoint),
		filer.WithRegion(region),
		filer.WithContainer(container),
		filer.WithUser(user),
		filer.WithSecret(password),
	), nil
}

// TODO: accept user configuration
func InitV1HttpServer(
	ctx context.Context,
	addr string,
	proj *v1projectservice.V1Service,
	spac *v1spaceservice.V1Service,
	sess *v1sessionservice.V1Service,
) (server.Server, error) {
	srv := httpserver.NewServer(
		server.WithAddress(addr),
	)

	v1, err := InitV1Router(ctx, proj, spac, sess)
	if err != nil {
		return nil, fmt.Errorf("failed to init router: %w", err)
	}

	if err := srv.Handle(v1); err != nil {
		return nil, fmt.Errorf("failed to attach handler: %w", err)
	}

	return srv, nil
}

func InitV1Router(ctx context.Context, proj *v1projectservice.V1Service, spac *v1spaceservice.V1Service, sess *v1sessionservice.V1Service) (*mux.Router, error) {
	router := mux.NewRouter()
	v1 := router.PathPrefix("/api/v1").Subrouter()

	projectHandler := v1projecthttphandler.NewV1Handler(proj)
	v1.Methods("POST").Path("/projects").HandlerFunc(projectHandler.Create)

	spaceHandler := v1spacehttphandler.NewV1Handler(spac)
	v1.Methods("POST").Path("/spaces").HandlerFunc(spaceHandler.Create)

	sessionHandler := v1sessionhttphandler.NewV1Handler(sess)
	v1.Methods("POST").Path("/sessions").HandlerFunc(sessionHandler.Create)
	v1.Methods("POST").Path("/sessions/{session_id}/messages").HandlerFunc(sessionHandler.AddMessage)
	v1.Methods("GET").Path("/sessions/{session_id}/messages").HandlerFunc(sessionHandler.GetMessages)
	v1.Methods("POST").Path("/sessions/{session_id}/finish").HandlerFunc(sessionHandler.FinishSession)

	return v1, nil
}
