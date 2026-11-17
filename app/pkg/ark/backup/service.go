package backup

import (
	"context"
	"log/slog"
	"syscall"

	"pkg/ark"
	"pkg/ark/cmdrunner"
	"pkg/ark/lifecycle"
)

// DiskUsageFn returns (freeBytes, totalBytes, error) for a path.
type DiskUsageFn func(path string) (int64, int64, error)

// DiskUsageForPath is the default implementation of DiskUsageFn using syscall.Statfs.
// It returns the free and total bytes for the filesystem containing the given path.
func DiskUsageForPath(path string) (int64, int64, error) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return 0, 0, err
	}
	free := int64(stat.Bavail) * int64(stat.Bsize)
	total := int64(stat.Blocks) * int64(stat.Bsize)
	return free, total, nil
}

// CheckFileInContainerFn checks if a file exists inside a container. Used for
// polling ephemeral backup containers for completion markers. The CLI wires
// this to a raw "docker exec container test -f path" call.
type CheckFileInContainerFn func(ctx context.Context, container, path string) (bool, error)

// LookPathFn checks whether a binary is available in PATH.
type LookPathFn func(name string) (string, error)

// Service provides backup create/restore/verify/prune operations.
type Service struct {
	engine    ark.ContainerEngine
	compose   ark.ComposeService
	lifecycle *lifecycle.Service
	runner    cmdrunner.Runner
	logger    *slog.Logger

	// Injected capabilities
	DiskUsage            DiskUsageFn
	CheckFileInContainer CheckFileInContainerFn
	LookPath             LookPathFn
}

// New creates a backup Service.
func New(
	engine ark.ContainerEngine,
	compose ark.ComposeService,
	lc *lifecycle.Service,
	runner cmdrunner.Runner,
	logger *slog.Logger,
) *Service {
	return &Service{
		engine:    engine,
		compose:   compose,
		lifecycle: lc,
		runner:    runner,
		logger:    logger,
	}
}
