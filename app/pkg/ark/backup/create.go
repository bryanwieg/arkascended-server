package backup

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"pkg/ark"
)

// ArtifactPaths holds the paths used during a backup create operation.
type ArtifactPaths struct {
	DestDir            string
	LockDir            string
	OutputPath         string
	ContainerPath      string
	ContainerReadyPath string
	ContainerName      string
}

// BuildArtifactPaths computes all paths for a backup create operation.
func BuildArtifactPaths(
	target string,
	backupRoot string,
	destOverride string,
	nameOverride string,
	format string,
	timestamp string,
	pid int,
) ArtifactPaths {
	destDir := filepath.Join(backupRoot, target)
	if destOverride != "" {
		destDir = destOverride
	}
	namePrefix := fmt.Sprintf("%s-backup-%s", target, timestamp)
	if nameOverride != "" {
		// Timestamped so retention orders it by age, not ahead of every regular backup.
		namePrefix = fmt.Sprintf("%s-%s-%s", target, nameOverride, timestamp)
	}
	filename := namePrefix + "." + format
	containerPath := filepath.ToSlash(filepath.Join("/home/ark-user/maintenance", filename))
	return ArtifactPaths{
		DestDir:            destDir,
		LockDir:            InstanceLockDir(backupRoot, target),
		OutputPath:         filepath.Join(destDir, filename),
		ContainerPath:      containerPath,
		ContainerReadyPath: containerPath + ".ready",
		ContainerName:      fmt.Sprintf("%s-backup-%s-%d", target, timestamp, pid),
	}
}

// CreateForTarget runs a backup for a single target service within a lock.
func (s *Service) CreateForTarget(ctx context.Context, req CreateRequest, target string) (*CreateResult, error) {
	paths := BuildArtifactPaths(
		target,
		req.BackupRoot,
		req.DestOverride,
		req.NameOverride,
		req.Format,
		req.Timestamp,
		req.PID,
	)

	if err := os.MkdirAll(paths.DestDir, 0o755); err != nil {
		return nil, err
	}

	var result *CreateResult
	err := withInstanceLockUnless(req.LockHeld, paths.LockDir, target, req.LockTimeout, func() error {
		usingEphemeral, err := s.startBackupArchiveProduction(ctx, req, target, paths)
		if err != nil {
			return err
		}

		defer s.cleanupArtifactSource(ctx, req.ComposeFile, target, paths, usingEphemeral)

		if err := s.waitForArchive(ctx, req.ComposeFile, target, paths, usingEphemeral, req.BackupWaitTimeout); err != nil {
			return err
		}

		if err := s.copyCompletedArchive(ctx, req.ComposeFile, target, paths, usingEphemeral); err != nil {
			return err
		}

		if s.DiskUsage != nil {
			if err := ApplyRetention(paths.DestDir, target, req.Retention, s.DiskUsage, s.logger); err != nil {
				return err
			}
		}

		result = &CreateResult{ArchivePath: paths.OutputPath}
		return nil
	})
	return result, err
}

func (s *Service) startBackupArchiveProduction(
	ctx context.Context,
	req CreateRequest,
	target string,
	paths ArtifactPaths,
) (bool, error) {
	// The helper container can't see the server's processes, so it skips the RCON save and
	// archives whatever is on disk. Only use it when the server is known to be stopped;
	// a live server must be backed up from inside, or not at all.
	running, err := s.compose.RunningServices(ctx, req.ComposeFile)
	if err != nil {
		return false, fmt.Errorf("Error: backup failed for %s: could not determine whether the service is running: %w", target, err)
	}
	if _, ok := running[target]; ok {
		s.logger.Info("service is running; attempting in-container exec backup", "service", target)
		if err := s.runBackupInRunning(ctx, req.ComposeFile, target, paths.ContainerPath); err != nil {
			return false, fmt.Errorf("Error: backup failed for %s while the server is running: %w", target, err)
		}
		return false, nil
	}

	s.logger.Info("service is not running; using helper container", "service", target)
	if helperErr := s.runBackupInEphemeral(ctx, req.ComposeFile, target, paths); helperErr != nil {
		return false, fmt.Errorf(
			"Error: backup failed for %s (service not running; helper failure: %w)",
			target, helperErr,
		)
	}
	s.logger.Info("helper container started", "service", target)
	return true, nil
}

func (s *Service) runBackupInRunning(ctx context.Context, composeFile, service, containerPath string) error {
	result, err := s.compose.Exec(ctx, composeFile, service,
		[]string{"/usr/bin/python3.14", "/home/ark-user/scripts/runtime/ark-backup.py", "--output", containerPath},
		ark.ExecOptions{DisableTTY: true},
	)
	if err != nil {
		return err
	}
	if result.ExitCode != 0 {
		detail := strings.TrimSpace(result.Stderr)
		if detail == "" {
			detail = strings.TrimSpace(result.Stdout)
		}
		return fmt.Errorf("backup script exited %d: %s", result.ExitCode, detail)
	}
	return nil
}

func (s *Service) runBackupInEphemeral(ctx context.Context, composeFile, service string, paths ArtifactPaths) error {
	shellCmd := fmt.Sprintf(
		"/usr/bin/python3.14 /home/ark-user/scripts/runtime/ark-backup.py --output %s && touch %s && sleep 600",
		paths.ContainerPath,
		paths.ContainerReadyPath,
	)
	return s.compose.RunOneOffDetached(ctx, composeFile, service, paths.ContainerName,
		[]string{"-c", shellCmd},
		ark.RunOptions{Entrypoint: "/bin/sh"},
	)
}

func (s *Service) cleanupArtifactSource(ctx context.Context, composeFile, target string, paths ArtifactPaths, usingEphemeral bool) {
	if usingEphemeral {
		if s.engine != nil {
			_ = s.engine.RemoveContainer(ctx, paths.ContainerName)
		}
		return
	}
	_, _ = s.compose.Exec(ctx, composeFile, target,
		[]string{"rm", "-f", paths.ContainerPath},
		ark.ExecOptions{DisableTTY: true},
	)
}

func (s *Service) waitForArchive(
	ctx context.Context,
	composeFile, service string,
	paths ArtifactPaths,
	usingEphemeral bool,
	waitTimeout time.Duration,
) error {
	deadline := time.Now().Add(waitTimeout)
	for {
		ready, err := s.archiveReady(ctx, composeFile, service, paths, usingEphemeral)
		if err != nil {
			return err
		}
		if ready {
			s.surfaceEphemeralLogs(ctx, paths.ContainerName, usingEphemeral)
			return nil
		}
		if err := s.ensureEphemeralHelperRunning(ctx, paths.ContainerName, usingEphemeral); err != nil {
			return err
		}
		if time.Now().After(deadline) {
			s.surfaceEphemeralLogs(ctx, paths.ContainerName, usingEphemeral)
			return errors.New("Error: backup archive not found in container after waiting.")
		}
		time.Sleep(2 * time.Second)
	}
}

func (s *Service) archiveReady(
	ctx context.Context,
	composeFile, service string,
	paths ArtifactPaths,
	usingEphemeral bool,
) (bool, error) {
	if usingEphemeral {
		if s.CheckFileInContainer == nil {
			return false, errors.New("CheckFileInContainer not configured")
		}
		return s.CheckFileInContainer(ctx, paths.ContainerName, paths.ContainerReadyPath)
	}
	result, err := s.compose.Exec(ctx, composeFile, service,
		[]string{"test", "-f", paths.ContainerPath},
		ark.ExecOptions{DisableTTY: true},
	)
	if err != nil {
		return false, err
	}
	return result.ExitCode == 0, nil
}

func (s *Service) ensureEphemeralHelperRunning(ctx context.Context, containerName string, usingEphemeral bool) error {
	if !usingEphemeral || s.containerIsRunning(ctx, containerName) {
		return nil
	}
	s.surfaceEphemeralLogs(ctx, containerName, true)
	return errors.New("Error: backup helper container exited before completion.")
}

func (s *Service) surfaceEphemeralLogs(ctx context.Context, containerName string, usingEphemeral bool) {
	if !usingEphemeral || s.engine == nil {
		return
	}
	reader, err := s.engine.ContainerLogs(ctx, containerName, ark.LogOptions{})
	if err != nil {
		return
	}
	defer reader.Close()
	scanner := bufio.NewScanner(reader)
	for scanner.Scan() {
		s.logger.Info(scanner.Text())
	}
}

func (s *Service) containerIsRunning(ctx context.Context, containerName string) bool {
	if s.engine == nil {
		return false
	}
	state, err := s.engine.InspectState(ctx, containerName)
	if err != nil {
		return false
	}
	return state.Running
}

func (s *Service) copyCompletedArchive(
	ctx context.Context,
	composeFile, target string,
	paths ArtifactPaths,
	usingEphemeral bool,
) error {
	if !usingEphemeral {
		return s.compose.CopyFromService(ctx, composeFile, target, paths.ContainerPath, paths.OutputPath)
	}
	if s.engine == nil {
		return fmt.Errorf("engine not initialized")
	}
	return s.engine.CopyFromContainer(ctx, paths.ContainerName, paths.ContainerPath, paths.OutputPath)
}
