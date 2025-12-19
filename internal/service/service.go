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
	isRunning bool
	mtx       sync.RWMutex
}

func (s *Service) Run(stop chan struct{}, startCb func() error, stopCb func(context.Context) error) error {
	s.mtx.RLock()
	if s.isRunning {
		s.mtx.RUnlock()
		return errors.New("service already running")
	}
	s.mtx.RUnlock()

	if err := s.Start(startCb); err != nil {
		return fmt.Errorf("failed to start service: %w", err)
	}

	<-stop

	return s.Stop(stopCb)
}

func (s *Service) Start(startCb func() error) error {
	s.mtx.Lock()
	defer s.mtx.Unlock()

	if s.isRunning {
		return errors.New("service already started")
	}

	if startCb != nil {
		if err := startCb(); err != nil {
			return err
		}
	}

	s.isRunning = true

	return nil
}

func (s *Service) Stop(stopCb func(context.Context) error) error {
	stopCtx, stopCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer stopCancel()
	return s.stop(stopCtx, stopCb)
}

func (s *Service) stop(ctx context.Context, stopCb func(context.Context) error) error {
	s.mtx.Lock()

	if !s.isRunning {
		s.mtx.Unlock()
		return errors.New("service not running")
	}

	s.isRunning = false

	s.mtx.Unlock()

	gracefulStopDone := make(chan struct{})
	go func() {
		if stopCb != nil {
			if err := stopCb(ctx); err != nil {
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

func New() *Service {
	return &Service{
		mtx: sync.RWMutex{},
	}
}
