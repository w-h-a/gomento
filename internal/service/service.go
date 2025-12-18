package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"
)

type Service struct {
	options   Options
	isRunning bool
	mtx       sync.RWMutex
}

func (s *Service) Run(stop chan struct{}) error {
	s.mtx.RLock()
	if s.isRunning {
		s.mtx.RUnlock()
		return errors.New("service already running")
	}
	s.mtx.RUnlock()

	if err := s.Start(); err != nil {
		return fmt.Errorf("failed to start service: %w", err)
	}

	<-stop

	return s.Stop()
}

func (s *Service) Start() error {
	s.mtx.Lock()
	defer s.mtx.Unlock()

	if s.isRunning {
		return errors.New("service already started")
	}

	if s.options.StartCallback != nil {
		if err := s.options.StartCallback(); err != nil {
			return err
		}
	}

	s.isRunning = true

	return nil
}

func (s *Service) Stop() error {
	stopCtx, stopCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer stopCancel()
	return s.stop(stopCtx)
}

func (s *Service) stop(ctx context.Context) error {
	s.mtx.Lock()

	if !s.isRunning {
		s.mtx.Unlock()
		return errors.New("service not running")
	}

	s.isRunning = false

	s.mtx.Unlock()

	gracefulStopDone := make(chan struct{})
	go func() {
		if s.options.StopCallback != nil {
			if err := s.options.StopCallback(ctx); err != nil {
				slog.ErrorContext(ctx, "stop callback error", "error", err)
			}
		}
		close(gracefulStopDone)
	}()

	var stopErr error

	select {
	case <-gracefulStopDone:
	case <-ctx.Done():
		stopErr = ctx.Err()
	}

	return stopErr
}

func New(opts ...Option) *Service {
	options := NewOptions(opts...)

	return &Service{
		options: options,
	}
}
