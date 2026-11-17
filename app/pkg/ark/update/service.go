package update

import (
	"log/slog"

	"pkg/ark"
	"pkg/ark/lifecycle"
)

// FetchTextFn fetches a URL and returns its body as a string.
type FetchTextFn func(url string) (string, error)

// Service provides update check/apply operations.
type Service struct {
	engine    ark.ContainerEngine
	compose   ark.ComposeService
	lifecycle *lifecycle.Service
	logger    *slog.Logger

	// FetchText is injected for HTTP operations (Steam API, Docker Hub).
	FetchText FetchTextFn
	// RunCommand runs the server files build script (SteamCMD download);
	// output streams to the terminal.
	RunCommand func([]string, ark.CommandOptions) (ark.ExecResult, error)
	// ContainerRuntime ("docker" or "podman") is passed to that script.
	ContainerRuntime string
	// DiskUsage returns (free, total) bytes for a path; checked before a build.
	DiskUsage func(path string) (int64, int64, error)
}

// New creates an update Service.
func New(
	engine ark.ContainerEngine,
	compose ark.ComposeService,
	lc *lifecycle.Service,
	logger *slog.Logger,
) *Service {
	return &Service{
		engine:    engine,
		compose:   compose,
		lifecycle: lc,
		logger:    logger,
	}
}
