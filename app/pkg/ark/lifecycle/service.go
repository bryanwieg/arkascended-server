package lifecycle

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"pkg/ark"
	"pkg/ark/health"
)

// HealthChecker abstracts the health checking needed by lifecycle operations.
// *health.Checker implements this interface.
type HealthChecker interface {
	WaitForReady(ctx context.Context, composeFile, service string, opts health.WaitOptions) error
}

// Service provides start/stop/down orchestration for compose services.
type Service struct {
	compose ark.ComposeService
	health  HealthChecker
	logger  *slog.Logger
}

// New creates a lifecycle Service.
func New(compose ark.ComposeService, health HealthChecker, logger *slog.Logger) *Service {
	return &Service{compose: compose, health: health, logger: logger}
}

// Start brings up services via compose and optionally waits for readiness.
func (s *Service) Start(ctx context.Context, req StartRequest) (*StartResult, error) {
	s.logger.Debug("Start", "file", req.ComposeFile, "services", req.Services, "pull", req.PullPolicy)

	if err := s.compose.Up(ctx, req.ComposeFile, req.Services, ark.UpOptions{
		PullPolicy: req.PullPolicy,
	}); err != nil {
		return nil, fmt.Errorf("compose up: %w", err)
	}

	result := &StartResult{Started: req.Services}

	if !req.WaitForReady {
		return result, nil
	}

	timeout := req.WaitTimeout
	if timeout <= 0 {
		timeout = DefaultStartWaitTimeout
	}

	for _, svc := range req.Services {
		// No Since: the checker reads logs from the container's current start.
		opts := health.WaitOptions{
			Timeout:  timeout,
			Interval: DefaultStartWaitInterval,
		}
		if err := s.health.WaitForReady(ctx, req.ComposeFile, svc, opts); err != nil {
			return result, err
		}
	}

	return result, nil
}

// Stop gracefully stops services or kills them immediately.
func (s *Service) Stop(ctx context.Context, req StopRequest) error {
	s.logger.Debug("Stop", "file", req.ComposeFile, "services", req.Services, "kill", req.Kill)

	if req.Kill {
		return s.compose.Kill(ctx, req.ComposeFile, req.Services, "SIGKILL")
	}

	timeout := req.Timeout
	if timeout < 0 {
		timeout = DefaultStopTimeout
	}
	return s.compose.Stop(ctx, req.ComposeFile, req.Services, &timeout)
}

// Down stops (if running) and removes a service, or tears down the entire
// compose project if req.Service is empty.
func (s *Service) Down(ctx context.Context, req DownRequest) error {
	s.logger.Debug("Down", "file", req.ComposeFile, "service", req.Service)

	if req.Service == "" {
		return s.downAll(ctx, req)
	}
	return s.downService(ctx, req)
}

func (s *Service) downAll(ctx context.Context, req DownRequest) error {
	// Stop all running services first.
	running, err := s.compose.RunningServices(ctx, req.ComposeFile)
	if err != nil {
		return err
	}
	for svc := range running {
		if err := s.stopOne(ctx, req.ComposeFile, svc, req.Timeout, req.Kill); err != nil {
			return err
		}
	}
	return s.compose.Down(ctx, req.ComposeFile)
}

func (s *Service) downService(ctx context.Context, req DownRequest) error {
	// Stop if running.
	running, err := s.compose.RunningServices(ctx, req.ComposeFile)
	if err != nil {
		return err
	}
	if _, ok := running[req.Service]; ok {
		if err := s.stopOne(ctx, req.ComposeFile, req.Service, req.Timeout, req.Kill); err != nil {
			return err
		}
	}

	// Remove the container if it exists.
	all, err := s.compose.AllServices(ctx, req.ComposeFile)
	if err != nil {
		return err
	}
	if _, ok := all[req.Service]; !ok {
		return nil // already gone
	}
	return s.compose.Remove(ctx, req.ComposeFile, []string{req.Service})
}

func (s *Service) stopOne(ctx context.Context, composeFile, service string, timeout time.Duration, kill bool) error {
	return s.Stop(ctx, StopRequest{
		ComposeFile: composeFile,
		Services:    []string{service},
		Timeout:     timeout,
		Kill:        kill,
	})
}
