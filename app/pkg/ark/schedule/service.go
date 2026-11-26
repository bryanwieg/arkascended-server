package schedule

import (
	"log/slog"
	"path/filepath"
	"time"
)

// Service manages schedule configuration, state, and locking. State, log and
// lock paths are derived from RepoDir, the compose file's directory (see ForComposeFile).
type Service struct {
	RepoDir string
	Now     func() time.Time
	logger  *slog.Logger
}

// New creates a Service with sensible defaults derived from repoDir.
func New(repoDir string, logger *slog.Logger) *Service {
	if logger == nil {
		logger = slog.Default()
	}
	return &Service{
		RepoDir: repoDir,
		Now:     func() time.Time { return time.Now().UTC() },
		logger:  logger,
	}
}

// ForComposeFile returns a copy whose state, log and lock paths are under the
// compose file's directory, whatever the working directory.
func (s *Service) ForComposeFile(composeFile string) *Service {
	abs, err := filepath.Abs(composeFile)
	if err != nil {
		abs = composeFile // only when the working directory is gone; the relative path is the best left
	}
	forFile := *s
	forFile.RepoDir = filepath.Dir(abs)
	return &forFile
}
