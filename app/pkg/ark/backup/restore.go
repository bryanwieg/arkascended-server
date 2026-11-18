package backup

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"pkg/ark"
)

// RestoreExecutionPaths holds the paths used during a restore operation.
type RestoreExecutionPaths struct {
	HostArchive       string
	HostPreRestoreDir string
	ContainerArchive  string
	ContainerPreStore string
	HostPreRestore    string
}

// RestoreResult reports what happened during a restore operation.
type RestoreResult struct {
	PreRestorePath string
}

// resolveRestoreArchive returns the backups directory and archive path for a
// restore request. When req.ArchivePath is set, it uses that directly instead
// of resolving from the instance's backup directory.
func (s *Service) resolveRestoreArchive(req RestoreRequest) (string, string, error) {
	if req.ArchivePath != "" {
		if _, err := os.Stat(req.ArchivePath); err != nil {
			return "", "", fmt.Errorf("Error: backup archive not found: %s", req.ArchivePath)
		}
		return filepath.Dir(req.ArchivePath), req.ArchivePath, nil
	}
	backupsDir, err := DirectoryForInstance(req.BackupRoot, req.Service)
	if err != nil {
		return "", "", err
	}
	archive, err := ResolveArchive(backupsDir, req.Archive)
	if err != nil {
		return "", "", err
	}
	return backupsDir, archive, nil
}

// Restore runs a backup restore for a single instance. Returns the pre-restore
// backup path and any error.
func (s *Service) Restore(ctx context.Context, req RestoreRequest) (*RestoreResult, error) {
	backupsDir, archive, err := s.resolveRestoreArchive(req)
	if err != nil {
		return nil, err
	}

	// Check service is stopped.
	running, err := s.compose.RunningServices(ctx, req.ComposeFile)
	if err != nil {
		return nil, err
	}
	if _, ok := running[req.Service]; ok {
		return nil, fmt.Errorf("Error: service must be stopped before restore: %s. Stop it first with 'ark-cli stop instance %s'.",
			req.Service, req.Service)
	}

	preRestoreDir := backupsDir
	if err := ensureContainerWritableDir(preRestoreDir); err != nil {
		return nil, err
	}

	timestamp := time.Now().UTC().Format("20060102-150405")
	restorePaths, err := BuildRestoreExecutionPaths(req.Service, archive, preRestoreDir, timestamp)
	if err != nil {
		return nil, err
	}

	if err := s.runRestoreInMaintenanceContainer(ctx, req.ComposeFile, req.Service, restorePaths); err != nil {
		return nil, err
	}
	if _, err := os.Stat(restorePaths.HostPreRestore); err != nil {
		return nil, fmt.Errorf("Error: pre-restore backup not created: %s", restorePaths.HostPreRestore)
	}

	if req.AutoStart {
		if err := s.compose.Up(ctx, req.ComposeFile, []string{req.Service}, ark.UpOptions{}); err != nil {
			return nil, err
		}
	}

	return &RestoreResult{PreRestorePath: restorePaths.HostPreRestore}, nil
}

func (s *Service) runRestoreInMaintenanceContainer(
	ctx context.Context,
	composeFile, service string,
	paths RestoreExecutionPaths,
	extraArgs ...string,
) error {
	restoreCmd := append([]string{
		"/usr/bin/python3.14",
		"/home/ark-user/scripts/runtime/ark-restore.py",
		"--archive", paths.ContainerArchive,
		"--pre-restore", paths.ContainerPreStore,
	}, extraArgs...)
	result, err := s.compose.RunOneOff(ctx, composeFile, service,
		restoreCmd,
		maintenanceRunOptions(
			fmt.Sprintf("%s:%s:ro", paths.HostArchive, paths.ContainerArchive),
			fmt.Sprintf("%s:%s", paths.HostPreRestoreDir, containerOutputDir),
		),
	)
	if err != nil {
		return err
	}
	if result.ExitCode != 0 {
		detail := strings.TrimSpace(result.Stderr)
		if detail == "" {
			detail = fmt.Sprintf("restore command failed with exit code %d", result.ExitCode)
		}
		return fmt.Errorf("Error: restore failed: %s", detail)
	}
	return nil
}

// ensureContainerWritableDir creates dir and opens it to the container's ark-user (uid 3000).
func ensureContainerWritableDir(dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	return os.Chmod(dir, 0o777)
}

// BuildRestoreExecutionPaths computes the host and container paths for a restore.
func BuildRestoreExecutionPaths(instance, archive, preRestoreDir, timestamp string) (RestoreExecutionPaths, error) {
	archiveSuffix := filepath.Ext(archive)
	if format := DetectArchiveFormat(archive); format != "" {
		archiveSuffix = "." + format
	}
	hostArchive, err := filepath.Abs(archive)
	if err != nil {
		return RestoreExecutionPaths{}, err
	}
	hostPreRestoreDir, err := filepath.Abs(preRestoreDir)
	if err != nil {
		return RestoreExecutionPaths{}, err
	}
	containerArchive := filepath.ToSlash(
		filepath.Join("/home/ark-user/maintenance", fmt.Sprintf("%s-restore-input-%s%s", instance, timestamp, archiveSuffix)),
	)
	preRestoreName := fmt.Sprintf("%s-pre-restore-%s.tar.gz", instance, timestamp)
	containerPreRestore := filepath.ToSlash(filepath.Join("/home/ark-user/maintenance/output", preRestoreName))
	return RestoreExecutionPaths{
		HostArchive:       hostArchive,
		HostPreRestoreDir: hostPreRestoreDir,
		ContainerArchive:  containerArchive,
		ContainerPreStore: containerPreRestore,
		HostPreRestore:    filepath.Join(preRestoreDir, preRestoreName),
	}, nil
}

// maintenanceRunOptions runs a removed one-off container as ark-user with the given extra mounts.
func maintenanceRunOptions(volumes ...string) ark.RunOptions {
	return ark.RunOptions{Remove: true, NoDeps: true, DisableTTY: true, User: "3000:3000", Volumes: volumes}
}
