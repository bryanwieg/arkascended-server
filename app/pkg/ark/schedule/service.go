package schedule

import (
	"log/slog"
	"path/filepath"
	"time"
)

// Service manages schedule configuration, state, and locking. Path fields
// can be overridden; defaults are derived from RepoDir.
type Service struct {
	RepoDir            string
	ConfigPath         string
	ManagedCrontabPath string
	CLICommand         []string
	Now                func() time.Time
	logger             *slog.Logger
}

// New creates a Service with sensible defaults derived from repoDir.
func New(repoDir string, logger *slog.Logger) *Service {
	if logger == nil {
		logger = slog.Default()
	}
	return &Service{
		RepoDir:            repoDir,
		ConfigPath:         filepath.Join(repoDir, ".ark-manage", "schedules.yaml"),
		ManagedCrontabPath: filepath.Join(repoDir, ".ark-manage", "generated", "schedules.crontab"),
		CLICommand:         []string{"ark-cli"},
		Now:                func() time.Time { return time.Now().UTC() },
		logger:             logger,
	}
}
