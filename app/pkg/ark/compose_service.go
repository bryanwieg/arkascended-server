package ark

import (
	"context"
	"io"
	"time"
)

// ComposeService abstracts Docker Compose operations. Methods that operate on
// a running project take the compose file path (which identifies the project)
// as their first string argument. The SDK backend (sdkbackend) implements this
// interface. Backends map native errors to sentinel types (ErrNotFound, etc.)
// so callers can branch with errors.Is.
type ComposeService interface {
	// Project loading

	// LoadProject parses the compose file and returns a typed project.
	LoadProject(ctx context.Context, composeFile string) (*ComposeProject, error)

	// SaveProject writes a project loaded via LoadProject back to composeFile:
	// changes to a service's image, environment, labels and ports become line
	// edits that keep the rest of the file; other changes are refused. Add new
	// services with runtime.InsertService on the raw YAML instead.
	SaveProject(ctx context.Context, composeFile string, project *ComposeProject) error

	// Lifecycle

	// Up creates and starts services. The services slice may be empty to
	// start all services.
	Up(ctx context.Context, composeFile string, services []string, opts UpOptions) error

	// Stop gracefully stops services with the given timeout. A nil timeout
	// uses the backend default.
	Stop(ctx context.Context, composeFile string, services []string, timeout *time.Duration) error

	// Kill sends the given signal (e.g. "SIGKILL") to services.
	Kill(ctx context.Context, composeFile string, services []string, signal string) error

	// Down stops and removes containers, networks, and (optionally) volumes
	// for the project.
	Down(ctx context.Context, composeFile string) error

	// Remove force-removes stopped service containers.
	Remove(ctx context.Context, composeFile string, services []string) error

	// Create creates service containers without starting them.
	Create(ctx context.Context, composeFile string, services []string) error

	// Status

	// RunningServices returns the set of currently running service names.
	RunningServices(ctx context.Context, composeFile string) (map[string]struct{}, error)

	// ExitedServices returns the set of exited service names.
	ExitedServices(ctx context.Context, composeFile string) (map[string]struct{}, error)

	// AllServices returns the set of all service names (any state).
	AllServices(ctx context.Context, composeFile string) (map[string]struct{}, error)

	// ServiceContainers returns the project's containers keyed by service name (all states).
	ServiceContainers(ctx context.Context, composeFile string) (map[string]ServiceContainer, error)

	// Exec

	// Exec runs a command in a running service container and returns the
	// result. The container must already be running.
	Exec(ctx context.Context, composeFile, service string, command []string, opts ExecOptions) (ExecResult, error)

	// ExecInteractive runs a command in a running service container with
	// stdin/stdout/stderr attached to the terminal.
	ExecInteractive(ctx context.Context, composeFile, service string, command []string) error

	// RunOneOff starts a one-off container, runs the command, and returns
	// the result. The container is removed after completion if
	// opts.Remove is true.
	RunOneOff(ctx context.Context, composeFile, service string, command []string, opts RunOptions) (ExecResult, error)

	// RunOneOffDetached starts a one-off container in detached mode with
	// the given name. Returns immediately after the container starts.
	RunOneOffDetached(ctx context.Context, composeFile, service, containerName string, command []string, opts RunOptions) error

	// RunOneOffInteractive starts a one-off container with stdin/stdout/stderr
	// attached to the terminal.
	RunOneOffInteractive(ctx context.Context, composeFile, service string, command []string, opts RunOptions) error

	// Logs

	// Logs returns a reader for the service's log output.
	Logs(ctx context.Context, composeFile, service string, opts LogOptions) (io.ReadCloser, error)

	// Copy

	// CopyFromService copies a file or directory from a running service
	// container to a local host path.
	CopyFromService(ctx context.Context, composeFile, service, srcPath, dstPath string) error
}
