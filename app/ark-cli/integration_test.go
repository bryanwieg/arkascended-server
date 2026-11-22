package main

// Tests: end-to-end integration via fake docker binary: create/start/stop/remove/backup/restore lifecycle flows

import (
	cmdbackup "ark-cli/internal/backup"
	"ark-cli/internal/compose"
	"ark-cli/internal/core"
	"ark-cli/internal/create"
	"ark-cli/internal/reachability"
	"ark-cli/internal/remove"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"pkg/ark"
	"pkg/ark/arktest"
	backup "pkg/ark/backup"
	"pkg/ark/cmdrunner"

	"github.com/compose-spec/compose-go/v2/types"
)

// writeFakeDocker writes a shell script that stands in for the real `docker` binary.
// It handles the subset of docker commands exercised by CLI integration tests.
//
// Handled subcommands:
//
//	docker compose config --format json
//	    Returns a minimal JSON service config. Service name is FAKE_SERVICE (default: asa1).
//	docker compose ps [-a]
//	    Returns FAKE_RUNNING (running) or FAKE_CONTAINERS (all containers, -a flag).
//	docker compose exec
//	    Exits 0. Set FAKE_EXEC_FAIL=1 to exit 1. Set FAKE_EXEC_SLEEP_SECONDS=N to sleep.
//	docker compose logs
//	    Emits the startup-ready marker string. Set FAKE_READY_LOG=0 to suppress it.
//	docker compose run
//	    Exits 0. If -v mounts an output dir and --pre-restore is passed, writes a
//	    pre-restore sentinel file into the output dir.
//	docker compose create / up
//	    Always exits 0.
//	docker compose cp
//	    Exits 0 for container-to-host copies; writes "backup" to dst for host copies.
//	docker cp
//	    Writes "backup" to the destination path.
//	docker image inspect
//	    Exits 0. Set FAKE_IMAGE_MISSING=1 to exit 1.
//	docker rm
//	    Always exits 0.
//	docker volume rm
//	    Always exits 0.
//
// Environment variables:
//
//	FAKE_DOCKER_LOG         — path to an append-only log of every docker invocation
//	FAKE_SERVICE            — service name injected into compose config JSON (default: asa1)
//	FAKE_RUNNING            — text returned by compose ps (running containers)
//	FAKE_CONTAINERS         — text returned by compose ps -a (all containers)
//	FAKE_EXEC_FAIL          — set to "1" to make compose exec exit 1
//	FAKE_EXEC_SLEEP_SECONDS — sleep duration for compose exec before exiting
//	FAKE_READY_LOG          — set to "0" to omit the startup-ready marker from compose logs
//	FAKE_IMAGE_MISSING      — set to "1" to make docker image inspect exit 1
func writeFakeDocker(t *testing.T) string {
	t.Helper()
	binDir := t.TempDir()
	dockerPath := filepath.Join(binDir, "docker")
	script := strings.Join([]string{
		"#!/bin/sh",
		`if [ -n "$FAKE_DOCKER_LOG" ]; then`,
		`  printf '%s\n' "$0 $*" >> "$FAKE_DOCKER_LOG"`,
		"fi",
		`cmd="$1"`,
		"shift",
		`if [ "$cmd" = "compose" ]; then`,
		`  if [ "$1" = "-f" ]; then`,
		"    shift 2",
		"  fi",
		`  sub="$1"`,
		"  shift",
		`  case "$sub" in`,
		"    config)",
		`      if [ "$1" = "--format" ] && [ "$2" = "json" ]; then`,
		`        service="${FAKE_SERVICE:-asa1}"`,
		`        printf '{"services":{"%s":{"image":"ark-server:latest","ports":["27015:27015/udp","27020:27020/tcp"]}}}\n' "$service"`,
		"        exit 0",
		"      fi",
		"      ;;",
		"    ps)",
		`      if [ "$1" = "-a" ]; then`,
		`        echo "${FAKE_CONTAINERS:-}"`,
		"        exit 0",
		"      fi",
		`      echo "${FAKE_RUNNING:-}"`,
		"      exit 0",
		"      ;;",
		"    exec)",
		`      if [ "${FAKE_EXEC_FAIL:-}" = "1" ]; then`,
		"        exit 1",
		"      fi",
		`      if [ -n "${FAKE_EXEC_SLEEP_SECONDS:-}" ]; then`,
		`        sleep "${FAKE_EXEC_SLEEP_SECONDS}"`,
		"      fi",
		"      exit 0",
		"      ;;",
		"    logs)",
		`      if [ "${FAKE_READY_LOG:-1}" = "1" ]; then`,
		`        echo "has successfully started!"`,
		`        echo "Server has completed startup and is now advertising for join."`,
		"      fi",
		"      exit 0",
		"      ;;",
		"    run)",
		`      output_dir=""`,
		`      pre_restore=""`,
		`      while [ "$#" -gt 0 ]; do`,
		`        if [ "$1" = "-v" ] && [ $# -ge 2 ]; then`,
		`          case "$2" in`,
		`            *:/home/ark-user/maintenance/output) output_dir="${2%:/home/ark-user/maintenance/output}" ;;`,
		`          esac`,
		`          shift 2`,
		`          continue`,
		`        fi`,
		`        if [ "$1" = "--pre-restore" ] && [ $# -ge 2 ]; then`,
		`          pre_restore="$2"`,
		`          shift 2`,
		`          continue`,
		`        fi`,
		`        shift`,
		`      done`,
		`      if [ -n "$output_dir" ] && [ -n "$pre_restore" ]; then`,
		`        mkdir -p "$output_dir"`,
		`        pre_name="$(basename "$pre_restore")"`,
		`        echo "pre-restore" > "$output_dir/$pre_name"`,
		`      fi`,
		"      exit 0",
		"      ;;",
		"    create)",
		"      exit 0",
		"      ;;",
		"    up)",
		"      exit 0",
		"      ;;",
		"    cp)",
		`      src="$1"`,
		`      dst="$2"`,
		`      case "$dst" in`,
		`        *:*) exit 0 ;;`,
		"      esac",
		`      mkdir -p "$(dirname "$dst")"`,
		`      echo "backup" > "$dst"`,
		"      exit 0",
		"      ;;",
		"  esac",
		"fi",
		`if [ "$cmd" = "cp" ]; then`,
		`  dst="$2"`,
		`  mkdir -p "$(dirname "$dst")"`,
		`  echo "backup" > "$dst"`,
		"  exit 0",
		"fi",
		`if [ "$cmd" = "image" ] && [ "$1" = "inspect" ]; then`,
		`  if [ "${FAKE_IMAGE_MISSING:-}" = "1" ]; then`,
		"    exit 1",
		"  fi",
		"  exit 0",
		"fi",
		`if [ "$cmd" = "rm" ]; then`,
		"  exit 0",
		"fi",
		`if [ "$cmd" = "volume" ] && [ "$1" = "rm" ]; then`,
		"  exit 0",
		"fi",
		"exit 0",
	}, "\n")
	if err := os.WriteFile(dockerPath, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake docker: %v", err)
	}
	return binDir
}

func patchFakeCLIHostDeps(t *testing.T) {
	t.Helper()
	patchAppDepsForTest(t, func(d *core.AppDeps) {
		d.DiskUsageForPath = func(_ string) (int64, int64, error) {
			return 100 * 1024 * 1024 * 1024, 200 * 1024 * 1024 * 1024, nil
		}
		// MockCompose so tests need no container engine; unconfigured
		// methods are no-ops.
		d.ComposeService = &arktest.MockCompose{
			LoadProjectFn: func(_ context.Context, composeFile string) (*ark.ComposeProject, error) {
				// Try to load from file first (create may have written one).
				if composeFile != "" {
					if _, err := os.Stat(composeFile); err == nil {
						project, err := loadComposeFromPathForTest(composeFile)
						if err == nil && project != nil && len(project.Services) > 0 {
							return &ark.ComposeProject{Project: project}, nil
						}
					}
				}
				// Fall back to a minimal project with the expected service.
				project := &types.Project{
					Name: "test",
					Services: types.Services{
						"asa1": types.ServiceConfig{
							Name:   "asa1",
							Labels: types.Labels{ark.PortASALabel: "27015"},
							Image:  "ark-server:latest",
							Ports: []types.ServicePortConfig{
								{Target: 27015, Published: "27015", Protocol: "udp"},
								{Target: 27020, Published: "27020", Protocol: "tcp"},
							},
						},
					},
				}
				return &ark.ComposeProject{Project: project}, nil
			},
			LogsFn: func(_ context.Context, _, _ string, _ ark.LogOptions) (io.ReadCloser, error) {
				msg := "has successfully started!\nServer has completed startup and is now advertising for join.\n"
				return io.NopCloser(strings.NewReader(msg)), nil
			},
			RunningServicesFn: func(_ context.Context, _ string) (map[string]struct{}, error) {
				running := os.Getenv("FAKE_RUNNING")
				if running == "" {
					return map[string]struct{}{}, nil
				}
				svcs := strings.Fields(running)
				result := make(map[string]struct{}, len(svcs))
				for _, s := range svcs {
					result[s] = struct{}{}
				}
				return result, nil
			},
			ExitedServicesFn: func(_ context.Context, _ string) (map[string]struct{}, error) {
				return map[string]struct{}{}, nil
			},
			ExecFn: func(_ context.Context, _, _ string, _ []string, _ ark.ExecOptions) (ark.ExecResult, error) {
				if os.Getenv("FAKE_EXEC_FAIL") == "1" {
					return ark.ExecResult{ExitCode: 1}, nil
				}
				if sleepSeconds := os.Getenv("FAKE_EXEC_SLEEP_SECONDS"); sleepSeconds != "" {
					if d, err := time.ParseDuration(sleepSeconds + "s"); err == nil {
						time.Sleep(d)
					}
				}
				return ark.ExecResult{ExitCode: 0}, nil
			},
			RunOneOffFn: func(_ context.Context, composeFile, _ string, cmd []string, opts ark.RunOptions) (ark.ExecResult, error) {
				// Handle pre-restore backup: write the sentinel file to the
				// mounted host output directory when --pre-restore is passed.
				for i, arg := range cmd {
					if arg == "--pre-restore" && i+1 < len(cmd) {
						preContainerPath := cmd[i+1]
						// Map the container pre-restore path to the host path
						// by checking the volume mounts.
						for _, vol := range opts.Volumes {
							parts := strings.SplitN(vol, ":", 3)
							if len(parts) >= 2 {
								hostDir := parts[0]
								containerMount := parts[1]
								if strings.HasPrefix(preContainerPath, containerMount) {
									hostPath := filepath.Join(hostDir, filepath.Base(preContainerPath))
									if err := os.MkdirAll(filepath.Dir(hostPath), 0o755); err == nil {
										_ = os.WriteFile(hostPath, []byte("pre-restore backup\n"), 0o644)
									}
								}
							}
						}
					}
				}
				return ark.ExecResult{ExitCode: 0, Stdout: "backup\n"}, nil
			},
			RunOneOffDetachedFn: func(_ context.Context, _, _, _ string, _ []string, _ ark.RunOptions) error {
				return nil
			},
			CopyFromServiceFn: func(_ context.Context, _, _, srcPath, dstPath string) error {
				if err := os.MkdirAll(filepath.Dir(dstPath), 0o755); err != nil {
					return err
				}
				return os.WriteFile(dstPath, []byte("backup\n"), 0o644)
			},
		}
		// MockEngine for fallback paths (backup, copy, etc.).
		d.Engine = &arktest.MockEngine{
			// ark-server:latest is built (build ID 200), so start pins instead of building.
			ImageExistsFn: func(context.Context, string) (bool, error) { return true, nil },
			ImageLabelsFn: func(context.Context, string) (map[string]string, error) {
				return map[string]string{"io.ark.server.build_id": "200"}, nil
			},
			CopyFromContainerFn: func(_ context.Context, _, srcPath, dstPath string) error {
				if err := os.MkdirAll(filepath.Dir(dstPath), 0o755); err != nil {
					return err
				}
				return os.WriteFile(dstPath, []byte("backup\n"), 0o644)
			},
		}
		// Use a mock backupService so ReadArchiveMapName returns empty
		// (the mock output produces non-tar archives).
		mockRunner := &arktest.MockCommandRunner{
			RunFn: func(_ context.Context, cmd []string, _ cmdrunner.RunOptions) (cmdrunner.RunResult, error) {
				if len(cmd) > 0 && cmd[0] == "tar" {
					return cmdrunner.RunResult{ExitCode: 0, Stdout: ""}, nil
				}
				return cmdrunner.RunResult{ExitCode: 0}, nil
			},
		}
		d.BackupService = backup.New(d.Engine, d.ComposeService, nil, mockRunner, d.Logger)
		d.BackupService.DiskUsage = func(_ string) (int64, int64, error) {
			return 100 * 1024 * 1024 * 1024, 200 * 1024 * 1024 * 1024, nil
		}
		d.BackupService.CheckFileInContainer = func(_ context.Context, _, _ string) (bool, error) {
			return true, nil
		}
		d.BackupService.LookPath = func(_ string) (string, error) { return "/usr/bin/tar", nil }
	})
}

// TestCLIIntegrationCreateStartStop exercises the fake-docker CLI flow for create/start/stop.
// - Expect: compose file is written, and start/stop return without error using the shim docker.
func TestCLIIntegrationCreateStartStop(t *testing.T) {
	tempDir := t.TempDir()
	composePath := filepath.Join(tempDir, "docker-compose.yaml")
	setEnv(t, "COMPOSE_FILE", composePath)
	setEnv(t, "FAKE_SERVICE", "asa1")
	setEnv(t, "FAKE_RUNNING", "asa1")
	fakeBin := writeFakeDocker(t)
	setEnv(t, "PATH", fakeBin+string(os.PathListSeparator)+os.Getenv("PATH"))
	patchFakeCLIHostDeps(t)

	if err := create.RunCreate(core.CurrentAppDeps(), []string{
		"--name", "asa1",
		"--admin-password", "adminpass",
		"--server-password", "serverpass",
	}); err != nil {
		t.Fatalf("runCreate: %v", err)
	}
	if _, err := os.Stat(composePath); err != nil {
		t.Fatalf("compose file missing: %v", err)
	}
	if err := compose.RunStart(core.CurrentAppDeps(), []string{"--instance", "asa1"}); err != nil {
		t.Fatalf("runStart: %v", err)
	}
	if err := compose.RunStop(core.CurrentAppDeps(), []string{"--instance", "asa1"}); err != nil {
		t.Fatalf("runStop: %v", err)
	}
}

// TestCLIIntegrationRemove runs create then remove via the CLI flow.
// - Expect: compose file no longer contains the service or its volume entry.
func TestCLIIntegrationRemove(t *testing.T) {
	tempDir := t.TempDir()
	composePath := filepath.Join(tempDir, "docker-compose.yaml")
	setEnv(t, "COMPOSE_FILE", composePath)
	setEnv(t, "FAKE_SERVICE", "asa1")
	setEnv(t, "FAKE_RUNNING", "")
	fakeBin := writeFakeDocker(t)
	setEnv(t, "PATH", fakeBin+string(os.PathListSeparator)+os.Getenv("PATH"))
	patchFakeCLIHostDeps(t)

	if err := create.RunCreate(core.CurrentAppDeps(), []string{
		"--name", "asa1",
		"--admin-password", "adminpass",
		"--server-password", "serverpass",
	}); err != nil {
		t.Fatalf("runCreate: %v", err)
	}
	if err := remove.RunRemove(core.CurrentAppDeps(), []string{"--instance", "asa1", "--skip-backup"}); err != nil {
		t.Fatalf("runRemove: %v", err)
	}
	contents, err := os.ReadFile(composePath)
	if err != nil {
		t.Fatalf("read compose: %v", err)
	}
	if strings.Contains(string(contents), "asa1:") {
		t.Fatal("expected service to be removed from compose file")
	}
	if strings.Contains(string(contents), "asa1-saved:") {
		t.Fatal("expected volume to be removed from compose file")
	}
}

// TestCLIIntegrationBackupRestore uses the CLI flow to create a backup and restore latest.
// - Expect: a backup archive is created and a pre-restore archive is written on restore.
func TestCLIIntegrationBackupRestore(t *testing.T) {
	tempDir := t.TempDir()
	composePath := filepath.Join(tempDir, "docker-compose.yaml")
	backupRoot := filepath.Join(tempDir, "backups")
	setEnv(t, "COMPOSE_FILE", composePath)
	setEnv(t, "ARK_BACKUP_DIR", backupRoot)
	setEnv(t, "FAKE_SERVICE", "asa1")
	setEnv(t, "FAKE_RUNNING", "asa1")
	setEnv(t, "FAKE_CONTAINERS", "asa1")
	fakeBin := writeFakeDocker(t)
	setEnv(t, "PATH", fakeBin+string(os.PathListSeparator)+os.Getenv("PATH"))
	patchFakeCLIHostDeps(t)

	if err := create.RunCreate(core.CurrentAppDeps(), []string{
		"--name", "asa1",
		"--admin-password", "adminpass",
		"--server-password", "serverpass",
	}); err != nil {
		t.Fatalf("runCreate: %v", err)
	}

	out := captureStdout(t, func() {
		if err := cmdbackup.RunBackup(core.CurrentAppDeps(), []string{"--instance", "asa1"}); err != nil {
			t.Fatalf("runBackup: %v", err)
		}
	})
	backupPath := strings.TrimSpace(out)
	if backupPath == "" {
		t.Fatal("expected backup output path")
	}
	if _, err := os.Stat(backupPath); err != nil {
		t.Fatalf("backup not created: %v", err)
	}

	setEnv(t, "FAKE_RUNNING", "")
	out = captureStdout(t, func() {
		if err := cmdbackup.RunRestore(core.CurrentAppDeps(), []string{"--instance", "asa1", "--archive", "latest"}); err != nil {
			t.Fatalf("runRestore: %v", err)
		}
	})
	if !strings.Contains(out, "Pre-restore backup") {
		t.Fatalf("unexpected restore output: %s", out)
	}
	preRestoreDir := filepath.Join(backupRoot, "asa1")
	entries, err := os.ReadDir(preRestoreDir)
	if err != nil {
		t.Fatalf("read pre-restore dir: %v", err)
	}
	found := false
	for _, entry := range entries {
		if strings.Contains(entry.Name(), "pre-restore") {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected pre-restore archive in backup directory")
	}
}

// TestCLIIntegrationBackupUsesEphemeralWhenStopped ensures backups run without a running service.
// - Expect: backup still succeeds when the service is not running.
func TestCLIIntegrationBackupUsesEphemeralWhenStopped(t *testing.T) {
	tempDir := t.TempDir()
	composePath := filepath.Join(tempDir, "docker-compose.yaml")
	backupRoot := filepath.Join(tempDir, "backups")
	setEnv(t, "COMPOSE_FILE", composePath)
	setEnv(t, "ARK_BACKUP_DIR", backupRoot)
	setEnv(t, "FAKE_SERVICE", "asa1")
	setEnv(t, "FAKE_RUNNING", "")
	fakeBin := writeFakeDocker(t)
	setEnv(t, "PATH", fakeBin+string(os.PathListSeparator)+os.Getenv("PATH"))
	patchFakeCLIHostDeps(t)

	if err := create.RunCreate(core.CurrentAppDeps(), []string{
		"--name", "asa1",
		"--admin-password", "adminpass",
		"--server-password", "serverpass",
	}); err != nil {
		t.Fatalf("runCreate: %v", err)
	}

	out := captureStdout(t, func() {
		if err := cmdbackup.RunBackup(core.CurrentAppDeps(), []string{"--instance", "asa1"}); err != nil {
			t.Fatalf("runBackup: %v", err)
		}
	})
	backupPath := strings.TrimSpace(out)
	if backupPath == "" {
		t.Fatal("expected backup output path")
	}
	if _, err := os.Stat(backupPath); err != nil {
		t.Fatalf("backup not created: %v", err)
	}
}

// TestCLIIntegrationBackupUsesExecWhenRunning verifies running services use the exec backup path.
// - Expect: backup succeeds and the backup artifact is created.
func TestCLIIntegrationBackupUsesExecWhenRunning(t *testing.T) {
	tempDir := t.TempDir()
	composePath := filepath.Join(tempDir, "docker-compose.yaml")
	backupRoot := filepath.Join(tempDir, "backups")
	setEnv(t, "COMPOSE_FILE", composePath)
	setEnv(t, "ARK_BACKUP_DIR", backupRoot)
	setEnv(t, "FAKE_SERVICE", "asa1")
	setEnv(t, "FAKE_RUNNING", "asa1")
	patchFakeCLIHostDeps(t)

	if err := create.RunCreate(core.CurrentAppDeps(), []string{
		"--name", "asa1",
		"--admin-password", "adminpass",
		"--server-password", "serverpass",
	}); err != nil {
		t.Fatalf("runCreate: %v", err)
	}

	out := captureStdout(t, func() {
		if err := cmdbackup.RunBackup(core.CurrentAppDeps(), []string{"--instance", "asa1"}); err != nil {
			t.Fatalf("runBackup: %v", err)
		}
	})
	backupPath := strings.TrimSpace(out)
	if backupPath == "" {
		t.Fatal("expected backup output path")
	}
	if _, err := os.Stat(backupPath); err != nil {
		t.Fatalf("backup not created: %v", err)
	}
}

// TestCLIIntegrationBackupRetentionPrunesByCount verifies CLI backup retention
// prunes oldest archives in the destination when retain-count is exceeded.
func TestCLIIntegrationBackupRetentionPrunesByCount(t *testing.T) {
	tempDir := t.TempDir()
	composePath := filepath.Join(tempDir, "docker-compose.yaml")
	backupRoot := filepath.Join(tempDir, "backups")
	setEnv(t, "COMPOSE_FILE", composePath)
	setEnv(t, "ARK_BACKUP_DIR", backupRoot)
	setEnv(t, "FAKE_SERVICE", "asa1")
	setEnv(t, "FAKE_RUNNING", "asa1")
	fakeBin := writeFakeDocker(t)
	setEnv(t, "PATH", fakeBin+string(os.PathListSeparator)+os.Getenv("PATH"))
	patchFakeCLIHostDeps(t)

	if err := create.RunCreate(core.CurrentAppDeps(), []string{
		"--name", "asa1",
		"--admin-password", "adminpass",
		"--server-password", "serverpass",
	}); err != nil {
		t.Fatalf("runCreate: %v", err)
	}

	backupNames := []string{
		"backup-20260101-000001",
		"backup-20260101-000002",
		"backup-20260101-000003",
	}
	for _, backupName := range backupNames {
		if err := cmdbackup.RunBackup(core.CurrentAppDeps(), []string{
			"--instance", "asa1",
			"--name", backupName,
			"--retain-count", "2",
			"--retain-days", "0",
			"--retain-size-gb", "0",
		}); err != nil {
			t.Fatalf("cmdbackup.RunBackup(core.CurrentAppDeps(), %s): %v", backupName, err)
		}
	}

	backupsDir := filepath.Join(backupRoot, "asa1")
	entries, err := os.ReadDir(backupsDir)
	if err != nil {
		t.Fatalf("read backups dir: %v", err)
	}
	// --name archives get "-<now>" appended; key by the name part. All three share
	// that timestamp, so retention orders them by mtime.
	found := map[string]bool{}
	for _, entry := range entries {
		found[backup.TimestampPattern.ReplaceAllString(backup.TrimArchiveSuffix(entry.Name()), "")] = true
	}

	oldest := "asa1-backup-20260101-000001"
	newer := "asa1-backup-20260101-000002"
	newest := "asa1-backup-20260101-000003"
	if found[oldest] {
		t.Fatalf("expected oldest backup to be pruned: %s", oldest)
	}
	if !found[newer] || !found[newest] {
		t.Fatalf("expected newest backups to remain; found=%v", found)
	}
}

// TestCLIIntegrationBackupLockContention verifies concurrent backups for the same
// instance are serialized and lock-timeout enforcement surfaces clear errors.
func TestCLIIntegrationBackupLockContention(t *testing.T) {
	tempDir := t.TempDir()
	composePath := filepath.Join(tempDir, "docker-compose.yaml")
	backupRoot := filepath.Join(tempDir, "backups")
	setEnv(t, "COMPOSE_FILE", composePath)
	setEnv(t, "ARK_BACKUP_DIR", backupRoot)
	setEnv(t, "FAKE_SERVICE", "asa1")
	setEnv(t, "FAKE_RUNNING", "asa1")
	setEnv(t, "FAKE_EXEC_SLEEP_SECONDS", "3")
	fakeBin := writeFakeDocker(t)
	setEnv(t, "PATH", fakeBin+string(os.PathListSeparator)+os.Getenv("PATH"))
	patchFakeCLIHostDeps(t)

	if err := create.RunCreate(core.CurrentAppDeps(), []string{
		"--name", "asa1",
		"--admin-password", "adminpass",
		"--server-password", "serverpass",
	}); err != nil {
		t.Fatalf("runCreate: %v", err)
	}

	firstErrCh := make(chan error, 1)
	go func() {
		firstErrCh <- cmdbackup.RunBackup(core.CurrentAppDeps(), []string{
			"--instance", "asa1",
			"--name", "lock-holder",
			"--retain-count", "0",
			"--retain-days", "0",
			"--retain-size-gb", "0",
		})
	}()

	lockDir := filepath.Join(backupRoot, "asa1")
	deadline := time.Now().Add(5 * time.Second)
	for {
		if held, _ := backup.InstanceLockHeld(lockDir, "asa1"); held {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for the backup lock in %s", lockDir)
		}
		time.Sleep(100 * time.Millisecond)
	}

	err := cmdbackup.RunBackup(core.CurrentAppDeps(), []string{
		"--instance", "asa1",
		"--name", "lock-contender",
		"--lock-timeout", "0s",
		"--retain-count", "0",
		"--retain-days", "0",
		"--retain-size-gb", "0",
	})
	if err == nil {
		t.Fatal("expected lock contention error for second backup")
	}
	if !strings.Contains(err.Error(), "backup lock is held") {
		t.Fatalf("unexpected lock error: %v", err)
	}

	if firstErr := <-firstErrCh; firstErr != nil {
		t.Fatalf("first backup failed unexpectedly: %v", firstErr)
	}
}

// TestCLIIntegrationBackupFallsBackWhenExecFails verifies backup fallback behavior.
// - Expect: when exec backup fails, CLI starts detached helper container and still produces backup.
// The helper container can't trigger an RCON save, so a failed in-server backup must error
// instead of archiving the live server's files from outside.
func TestCLIIntegrationBackupFailsWithoutFallbackWhenExecFails(t *testing.T) {
	tempDir := t.TempDir()
	composePath := filepath.Join(tempDir, "docker-compose.yaml")
	backupRoot := filepath.Join(tempDir, "backups")
	setEnv(t, "COMPOSE_FILE", composePath)
	setEnv(t, "ARK_BACKUP_DIR", backupRoot)
	setEnv(t, "FAKE_SERVICE", "asa1")
	setEnv(t, "FAKE_RUNNING", "asa1")
	setEnv(t, "FAKE_EXEC_FAIL", "1")
	patchFakeCLIHostDeps(t)

	if err := create.RunCreate(core.CurrentAppDeps(), []string{
		"--name", "asa1",
		"--admin-password", "adminpass",
		"--server-password", "serverpass",
	}); err != nil {
		t.Fatalf("runCreate: %v", err)
	}

	err := cmdbackup.RunBackup(core.CurrentAppDeps(), []string{"--instance", "asa1"})
	if err == nil || !strings.Contains(err.Error(), "while the server is running") {
		t.Fatalf("expected running-server backup error, got %v", err)
	}
	// The lock file stays behind by design; only archives count.
	if archives, _ := filepath.Glob(filepath.Join(backupRoot, "asa1", "*.tar.gz")); len(archives) != 0 {
		t.Fatalf("expected no backup archive, found %v", archives)
	}
}

// TestCLIIntegrationCheckPortInstance checks the instance path for check_port.
// - Expect: output reports the UDP port as OPEN using the stubbed port check.
func TestCLIIntegrationCheckReachabilityInstance(t *testing.T) {
	tempDir := t.TempDir()
	composePath := filepath.Join(tempDir, "docker-compose.yaml")
	setEnv(t, "COMPOSE_FILE", composePath)
	setEnv(t, "FAKE_SERVICE", "asa1")
	setEnv(t, "FAKE_RUNNING", "asa1")
	fakeBin := writeFakeDocker(t)
	setEnv(t, "PATH", fakeBin+string(os.PathListSeparator)+os.Getenv("PATH"))

	patchAppDepsForTest(t, func(d *core.AppDeps) {
		d.ComposeService = &arktest.MockCompose{
			LoadProjectFn: func(_ context.Context, composeFile string) (*ark.ComposeProject, error) {
				project, err := loadComposeFromPathForTest(composeFile)
				if err != nil {
					return nil, err
				}
				return &ark.ComposeProject{Project: project}, nil
			},
			RunningServicesFn: func(_ context.Context, _ string) (map[string]struct{}, error) {
				return map[string]struct{}{"asa1": {}}, nil
			},
		}
		d.HostDeps.FetchText = func(url string) (string, error) {
			if strings.Contains(url, "ipify") {
				return "203.0.113.1", nil
			}
			return "open", nil
		}
	})

	if err := os.WriteFile(composePath, []byte("services:\n  asa1:\n    labels: {ark.port.asa: '27015'}\n    image: ark-server:latest\n    ports:\n      - \"27015:27015/udp\"\n      - \"27020:27020/tcp\"\n"), 0o644); err != nil {
		t.Fatalf("write compose: %v", err)
	}

	out := captureStdout(t, func() {
		if err := reachability.RunCheckReachability(core.CurrentAppDeps(), []string{"--instance", "asa1"}); err != nil {
			t.Fatalf("RunCheckReachability: %v", err)
		}
	})
	if !strings.Contains(out, "ASA") || !strings.Contains(out, "27015") {
		t.Fatalf("unexpected output: %s", out)
	}
	if !strings.Contains(out, "RCON") || !strings.Contains(out, "27020") {
		t.Fatalf("unexpected output: %s", out)
	}
	if !strings.Contains(strings.ToUpper(out), "OPEN") {
		t.Fatalf("expected OPEN in output: %s", out)
	}
}
