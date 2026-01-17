package http

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/w-h-a/gomento/internal/server"
)

type httpServer struct {
	options   server.Options
	server    *http.Server
	errCh     chan error
	exit      chan struct{}
	isRunning bool
	mtx       sync.RWMutex
}

func (s *httpServer) Handle(handler any) error {
	s.mtx.Lock()
	defer s.mtx.Unlock()

	if s.isRunning {
		return errors.New("cannot set handler after server has started")
	}

	if s.server.Handler != nil {
		return errors.New("handler already set")
	}

	h, ok := handler.(http.Handler)
	if !ok {
		return fmt.Errorf("invalid handler type: expected http.Handler, got %T", handler)
	}

	if ms, ok := MiddlewareFrom(s.options.Context); ok && len(ms) > 0 {
		for i := len(ms) - 1; i >= 0; i-- {
			if ms[i] != nil {
				h = ms[i](h)
			}
		}
	}

	s.server.Handler = h

	return nil
}

func (s *httpServer) Run(stop chan struct{}) error {
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

func (s *httpServer) Start() error {
	s.mtx.Lock()
	defer s.mtx.Unlock()

	if s.isRunning {
		return errors.New("server already started")
	}

	if s.server.Handler == nil {
		return errors.New("handler not set")
	}

	listener, err := net.Listen("tcp", s.options.Address)
	if err != nil {
		return err
	}

	s.options.Address = listener.Addr().String()

	s.exit = make(chan struct{})
	s.errCh = make(chan error, 1)

	go func() {
		if err := s.server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			s.errCh <- fmt.Errorf("http server ListenAndServe error: %w", err)
		}
		close(s.exit)
	}()

	s.isRunning = true

	return nil
}

func (s *httpServer) Stop() error {
	stopCtx, stopCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer stopCancel()
	return s.stop(stopCtx)
}

func (s *httpServer) stop(ctx context.Context) error {
	s.mtx.Lock()

	if !s.isRunning {
		s.mtx.Unlock()
		return errors.New("server not running")
	}

	s.isRunning = false
	srv := s.server
	exit := s.exit

	s.mtx.Unlock()

	shutdownErr := srv.Shutdown(ctx)

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

	s := &httpServer{
		options: options,
		server:  &http.Server{},
		mtx:     sync.RWMutex{},
	}

	return s
}
