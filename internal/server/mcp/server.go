package mcp

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"sync"
	"time"

	mcpserver "github.com/mark3labs/mcp-go/server"
	"github.com/w-h-a/gomento/internal/server"
)

type mcpServer struct {
	options    server.Options
	mcpServer  *mcpserver.MCPServer
	httpServer *http.Server
	errCh      chan error
	exit       chan struct{}
	isRunning  bool
	mtx        sync.RWMutex
}

func (s *mcpServer) Handle(handler any) error {
	s.mtx.Lock()
	defer s.mtx.Unlock()

	if s.isRunning {
		return errors.New("cannot set handler after server has started")
	}

	switch h := handler.(type) {
	case mcpserver.ServerTool:
		finalHandler := h.Handler
		if ms, ok := ToolMiddlewareFrom(s.options.Context); ok && len(ms) > 0 {
			for i := len(ms) - 1; i >= 0; i-- {
				if ms[i] != nil {
					finalHandler = ms[i](finalHandler)
				}
			}
		}
		h.Handler = finalHandler
		s.mcpServer.AddTools(h)
	case mcpserver.ServerResource:
		finalHandler := h.Handler
		if ms, ok := ResourceMiddlewareFrom(s.options.Context); ok && len(ms) > 0 {
			for i := len(ms) - 1; i >= 0; i-- {
				if ms[i] != nil {
					finalHandler = ms[i](finalHandler)
				}
			}
		}
		h.Handler = finalHandler
		s.mcpServer.AddResource(h.Resource, h.Handler)
	default:
		return fmt.Errorf("invalid handler type for MCP server: %T", handler)
	}

	return nil
}

func (s *mcpServer) Run(stop chan struct{}) error {
	s.mtx.RLock()
	if s.isRunning {
		s.mtx.RUnlock()
		return errors.New("server already running")
	}
	s.mtx.RUnlock()

	if err := s.Start(); err != nil {
		return fmt.Errorf("failed to start server: %w", err)
	}

	select {
	case err := <-s.errCh:
		stopCtx, stopCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer stopCancel()
		_ = s.stop(stopCtx)
		return err
	case <-stop:
		stopCtx, stopCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer stopCancel()
		return s.stop(stopCtx)
	}
}

func (s *mcpServer) Start() error {
	s.mtx.Lock()
	defer s.mtx.Unlock()

	if s.isRunning {
		return errors.New("server already started")
	}

	listener, err := net.Listen("tcp", s.options.Address)
	if err != nil {
		return err
	}

	s.options.Address = listener.Addr().String()

	s.exit = make(chan struct{})
	s.errCh = make(chan error, 1)

	mux := http.NewServeMux()
	streamHandler := mcpserver.NewStreamableHTTPServer(s.mcpServer)
	mux.Handle("/mcp", streamHandler)

	s.httpServer = &http.Server{Handler: mux}

	go func() {
		if err := s.httpServer.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			s.errCh <- fmt.Errorf("mcp sse error: %w", err)
		}
		close(s.exit)
	}()

	s.isRunning = true

	return nil
}

func (s *mcpServer) Stop() error {
	stopCtx, stopCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer stopCancel()
	return s.stop(stopCtx)
}

func (s *mcpServer) stop(ctx context.Context) error {
	s.mtx.Lock()

	if !s.isRunning {
		s.mtx.Unlock()
		return errors.New("server not running")
	}

	s.isRunning = false
	httpServer := s.httpServer
	exit := s.exit

	s.mtx.Unlock()

	shutdownErr := httpServer.Shutdown(ctx)

	var stopErr error

	select {
	case <-exit:
	case <-ctx.Done():
		stopErr = ctx.Err()
	}

	if shutdownErr != nil && !errors.Is(shutdownErr, http.ErrServerClosed) && !errors.Is(shutdownErr, context.DeadlineExceeded) {
		return fmt.Errorf("http server shutdown error: %w", shutdownErr)
	}

	select {
	case err := <-s.errCh:
		return err
	default:
		return stopErr
	}
}

func NewServer(opts ...server.Option) server.Server {
	options := server.NewOptions(opts...)

	// TODO: validate options

	s := &mcpServer{
		options:   options,
		mcpServer: mcpserver.NewMCPServer(options.Name, options.Version),
		mtx:       sync.RWMutex{},
	}

	return s
}
