package backup

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"pkg/ark"
)

// ClusterTarget is the reserved --instance keyword, backup directory and archive prefix for cluster data.
const ClusterTarget = "cluster"

// ClusterMountPath is where every instance mounts the shared cluster volume.
const ClusterMountPath = "/home/ark-user/ark-server/ShooterGame/Saved/clusters"

const containerOutputDir = "/home/ark-user/maintenance/output"

// ErrNoClusterVolume means no service in the compose file mounts anything at ClusterMountPath.
var ErrNoClusterVolume = errors.New("Error: no instance in the compose file mounts a cluster volume at Saved/clusters.")

// ClusterVolume is the shared cluster mount: Source as written in the compose file, Services sorted, never empty.
type ClusterVolume struct {
	Source   string
	Services []string
}

// ResolveClusterVolume finds the one cluster mount shared by the project's services.
func ResolveClusterVolume(project *ark.ComposeProject) (ClusterVolume, error) {
	names := project.ServiceNames()
	if slices.Contains(names, ClusterTarget) {
		return ClusterVolume{}, errors.New("Error: an instance is named 'cluster', which is reserved for cluster backups; rename it.")
	}
	sources := []string{}
	services := []string{}
	for _, name := range names {
		for _, vol := range project.Project.Services[name].Volumes {
			if vol.Target != ClusterMountPath {
				continue
			}
			services = append(services, name)
			if !slices.Contains(sources, vol.Source) {
				sources = append(sources, vol.Source)
			}
		}
	}
	switch len(sources) {
	case 0:
		return ClusterVolume{}, ErrNoClusterVolume
	case 1:
		return ClusterVolume{Source: sources[0], Services: services}, nil
	}
	slices.Sort(sources)
	return ClusterVolume{}, fmt.Errorf(
		"Error: instances mount different cluster volumes (%s); cluster backups support one per compose file.",
		strings.Join(sources, ", "),
	)
}

// CreateCluster backs up the cluster volume; req.Service and req.BackupWaitTimeout are ignored.
// It runs without a deadline: cancelling RunOneOff would leave its container running.
func (s *Service) CreateCluster(ctx context.Context, req CreateRequest, cv ClusterVolume) (*CreateResult, error) {
	paths := BuildArtifactPaths(ClusterTarget, req.BackupRoot, req.DestOverride, req.NameOverride, req.Format, req.Timestamp, req.PID)
	hostDest, err := filepath.Abs(paths.DestDir)
	if err != nil {
		return nil, err
	}
	if err := ensureContainerWritableDir(hostDest); err != nil {
		return nil, err
	}
	containerOutput := path.Join(containerOutputDir, filepath.Base(paths.OutputPath))
	err = WithInstanceLock(paths.LockDir, ClusterTarget, req.LockTimeout, func() error {
		result, err := s.compose.RunOneOff(ctx, req.ComposeFile, cv.Services[0],
			[]string{"/usr/bin/python3.14", "/home/ark-user/scripts/runtime/ark-cluster-backup.py", "--output", containerOutput},
			maintenanceRunOptions(hostDest+":"+containerOutputDir),
		)
		if err != nil {
			return err
		}
		if result.ExitCode != 0 {
			return fmt.Errorf("Error: cluster backup failed (exit code %d).", result.ExitCode)
		}
		if _, err := os.Stat(paths.OutputPath); err != nil {
			return fmt.Errorf("Error: cluster backup archive not created: %s", paths.OutputPath)
		}
		if s.DiskUsage == nil {
			return nil
		}
		return ApplyRetention(paths.DestDir, ClusterTarget, req.Retention, s.DiskUsage, s.logger)
	})
	if err != nil {
		return nil, err
	}
	return &CreateResult{ArchivePath: paths.OutputPath}, nil
}

// RestoreCluster restores req.ArchivePath into the cluster volume; req.Service, req.Archive and
// req.AutoStart are ignored. Every service that mounts the volume must be stopped.
func (s *Service) RestoreCluster(ctx context.Context, req RestoreRequest, cv ClusterVolume) (*RestoreResult, error) {
	if _, err := os.Stat(req.ArchivePath); err != nil {
		return nil, fmt.Errorf("Error: backup archive not found: %s", req.ArchivePath)
	}
	running, err := s.compose.RunningServices(ctx, req.ComposeFile)
	if err != nil {
		return nil, err
	}
	busy := slices.DeleteFunc(slices.Clone(cv.Services), func(name string) bool {
		_, ok := running[name]
		return !ok
	})
	if len(busy) > 0 {
		return nil, fmt.Errorf(
			"Error: stop every instance that uses the cluster before restoring it: %s. Use 'ark-cli stop instance <name>'.",
			strings.Join(busy, ", "),
		)
	}
	backupsDir := filepath.Dir(req.ArchivePath)
	var result *RestoreResult
	err = WithInstanceLock(filepath.Join(req.BackupRoot, ClusterTarget), ClusterTarget, 0, func() error {
		if err := ensureContainerWritableDir(backupsDir); err != nil {
			return err
		}
		paths, err := BuildRestoreExecutionPaths(ClusterTarget, req.ArchivePath, backupsDir, time.Now().UTC().Format("20060102-150405"))
		if err != nil {
			return err
		}
		if err := s.runRestoreInMaintenanceContainer(ctx, req.ComposeFile, cv.Services[0], paths, "--cluster"); err != nil {
			return err
		}
		if _, err := os.Stat(paths.HostPreRestore); err != nil {
			return fmt.Errorf("Error: pre-restore backup not created: %s", paths.HostPreRestore)
		}
		result = &RestoreResult{PreRestorePath: paths.HostPreRestore}
		return nil
	})
	return result, err
}
