package health

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"time"

	"pkg/ark"
)

const (
	// StartedMarker is the log line emitted when the server process starts.
	StartedMarker = "has successfully started!"

	// ReadyMarker is the log line emitted when the server is advertising
	// for player connections.
	ReadyMarker = "Server has completed startup and is now advertising for join."

	defaultLogTail = 300
)

// InstanceHealthChecker provides the health-check interface used by command handlers.
// *Checker in this package satisfies this interface.
type InstanceHealthChecker interface {
	IsStarted(ctx context.Context, composeFile, service string, since *time.Time) (bool, error)
	IsReady(ctx context.Context, composeFile, service string, since *time.Time) (bool, error)
	WaitForStarted(ctx context.Context, composeFile, service string, opts WaitOptions) error
	WaitForReady(ctx context.Context, composeFile, service string, opts WaitOptions) error
	WaitForStopped(ctx context.Context, composeFile, service string, opts WaitOptions) error
}

// Checker provides health-check and readiness-polling for compose services.
type Checker struct {
	compose ark.ComposeService
	engine  ark.ContainerEngine // optional; finds the current run's start time
	logger  *slog.Logger
}

// New creates a Checker backed by the given ComposeService. engine may be nil;
// then a nil since falls back to the last defaultLogTail log lines.
func New(compose ark.ComposeService, engine ark.ContainerEngine, logger *slog.Logger) *Checker {
	return &Checker{compose: compose, engine: engine, logger: logger}
}

// LogsContainMarker scans the service's logs for the given marker string.
// If since is nil, it checks every line since the container last started, so
// a marker from a previous run (in-place restart) never counts and a
// long-running server's marker is never pushed out of a fixed tail.
func (c *Checker) LogsContainMarker(ctx context.Context, composeFile, service, marker string, since *time.Time) (bool, error) {
	if since == nil {
		since = c.currentRunStart(ctx, composeFile, service)
	}
	opts := ark.LogOptions{Tail: defaultLogTail, Since: since}
	if since != nil {
		opts.Tail = 0 // everything since the start
	}
	reader, err := c.compose.Logs(ctx, composeFile, service, opts)
	if err != nil {
		return false, err
	}
	defer reader.Close()
	data, err := io.ReadAll(reader)
	if err != nil {
		return false, err
	}
	return strings.Contains(string(data), marker), nil
}

// currentRunStart returns when the service's container last started, or nil
// when it can't tell (no engine, no container, never started).
func (c *Checker) currentRunStart(ctx context.Context, composeFile, service string) *time.Time {
	if c.engine == nil {
		return nil
	}
	containers, err := c.compose.ServiceContainers(ctx, composeFile)
	if err != nil {
		return nil
	}
	ct, ok := containers[service]
	if !ok || ct.Name == "" {
		return nil
	}
	state, err := c.engine.InspectState(ctx, ct.Name)
	if err != nil || state.StartedAt.IsZero() {
		return nil
	}
	return &state.StartedAt
}

// IsStarted returns true if the service's logs contain the startup marker.
func (c *Checker) IsStarted(ctx context.Context, composeFile, service string, since *time.Time) (bool, error) {
	return c.LogsContainMarker(ctx, composeFile, service, StartedMarker, since)
}

// IsReady returns true if the service's logs contain the advertising marker.
func (c *Checker) IsReady(ctx context.Context, composeFile, service string, since *time.Time) (bool, error) {
	return c.LogsContainMarker(ctx, composeFile, service, ReadyMarker, since)
}

// IsRunning returns true if the service is in the running state.
func (c *Checker) IsRunning(ctx context.Context, composeFile, service string) (bool, error) {
	services, err := c.compose.RunningServices(ctx, composeFile)
	if err != nil {
		return false, err
	}
	_, ok := services[service]
	return ok, nil
}

// CheckNotExited returns an error if the service has exited.
func (c *Checker) CheckNotExited(ctx context.Context, composeFile, service string) error {
	exited, err := c.compose.ExitedServices(ctx, composeFile)
	if err != nil {
		return nil // can't tell; don't block
	}
	if _, ok := exited[service]; ok {
		return fmt.Errorf("Error: service exited prematurely before becoming ready: %s", service)
	}
	return nil
}

// WaitOptions controls polling behavior for Wait* methods.
type WaitOptions struct {
	Timeout  time.Duration
	Interval time.Duration
	Since    *time.Time // if non-nil, only check logs after this time
}

// WaitForStarted polls until the service's logs contain the started marker.
func (c *Checker) WaitForStarted(ctx context.Context, composeFile, service string, opts WaitOptions) error {
	return c.waitForLogMarker(ctx, composeFile, service, StartedMarker, opts,
		fmt.Sprintf("Error: timed out waiting for instance to start: %s", service))
}

// WaitForReady polls until the service's logs contain the advertising marker.
func (c *Checker) WaitForReady(ctx context.Context, composeFile, service string, opts WaitOptions) error {
	return c.waitForLogMarker(ctx, composeFile, service, ReadyMarker, opts,
		fmt.Sprintf("Error: timed out waiting for instance to become ready: %s", service))
}

// WaitForStopped polls until the service is no longer running.
func (c *Checker) WaitForStopped(ctx context.Context, composeFile, service string, opts WaitOptions) error {
	return poll(ctx, opts.Timeout, opts.Interval, func() (bool, error) {
		running, err := c.IsRunning(ctx, composeFile, service)
		if err != nil {
			return false, err
		}
		return !running, nil
	}, fmt.Sprintf("Error: timed out waiting for instance to stop: %s", service))
}

func (c *Checker) waitForLogMarker(ctx context.Context, composeFile, service, marker string, opts WaitOptions, timeoutMsg string) error {
	return poll(ctx, opts.Timeout, opts.Interval, func() (bool, error) {
		running, err := c.IsRunning(ctx, composeFile, service)
		if err != nil {
			return false, err
		}
		if running {
			found, err := c.LogsContainMarker(ctx, composeFile, service, marker, opts.Since)
			if err != nil {
				return false, err
			}
			return found, nil
		}
		// Not running — check if it exited prematurely.
		if err := c.CheckNotExited(ctx, composeFile, service); err != nil {
			return false, err
		}
		return false, nil
	}, timeoutMsg)
}

// poll repeatedly calls check until it returns true, the timeout expires, or
// the context is cancelled.
func poll(ctx context.Context, timeout, interval time.Duration, check func() (bool, error), timeoutMsg string) error {
	deadline := time.Now().Add(timeout)
	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		done, err := check()
		if err != nil {
			return err
		}
		if done {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("%s", timeoutMsg)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(interval):
		}
	}
}
