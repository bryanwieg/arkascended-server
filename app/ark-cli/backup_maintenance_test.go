package main

// Tests: backup verify and prune commands: SQLite integrity checks, retention application across single and all services

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	cmdbackup "ark-cli/internal/backup"
	"ark-cli/internal/core"
	"pkg/ark"
	"pkg/ark/arktest"
	"pkg/ark/backup"
	"pkg/ark/cmdrunner"
)

func setupBackupArchiveFixture(t *testing.T, instance string) (string, string, string) {
	t.Helper()
	backupRoot := filepath.Join(t.TempDir(), "backups")
	setEnv(t, "ARK_BACKUP_DIR", backupRoot)
	instanceDir := filepath.Join(backupRoot, instance)
	if err := os.MkdirAll(instanceDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	archivePath := filepath.Join(instanceDir, instance+"-backup-20240201-120000.tar.gz")
	if err := os.WriteFile(archivePath, []byte("archive"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	return backupRoot, instanceDir, archivePath
}

func newTestBackupService(t *testing.T) *backup.Service {
	t.Helper()
	runner := &arktest.MockCommandRunner{
		RunFn: func(_ context.Context, cmd []string, _ cmdrunner.RunOptions) (cmdrunner.RunResult, error) {
			return cmdrunner.RunResult{ExitCode: 0, Stdout: "ok"}, nil
		},
	}
	bs := backup.New(nil, nil, nil, runner, slog.Default())
	bs.DiskUsage = func(_ string) (int64, int64, error) {
		return backup.MinFreeBackupBytes + 1024, 0, nil
	}
	bs.LookPath = func(_ string) (string, error) { return "/usr/bin/sqlite3", nil }
	return bs
}

func TestRunVerifyBackupChecksSQLiteFiles(t *testing.T) {
	_, _, _ = setupBackupArchiveFixture(t, "asa1")

	checked := []string{}
	runner := &arktest.MockCommandRunner{
		RunFn: func(_ context.Context, cmd []string, opts cmdrunner.RunOptions) (cmdrunner.RunResult, error) {
			if len(cmd) > 0 && cmd[0] == "tar" && len(cmd) > 2 && cmd[1] == "-xf" {
				// Simulate extraction: create .sqlite files in the dest dir.
				destDir := cmd[len(cmd)-1]
				savedDir := filepath.Join(destDir, "Saved")
				if err := os.MkdirAll(savedDir, 0o755); err == nil {
					_ = os.WriteFile(filepath.Join(savedDir, "foo.sqlite"), []byte(""), 0o644)
					_ = os.WriteFile(filepath.Join(savedDir, "bar.sqlite"), []byte(""), 0o644)
				}
				return cmdrunner.RunResult{ExitCode: 0}, nil
			}
			if len(cmd) > 1 && cmd[0] == "sqlite3" {
				checked = append(checked, cmd[1])
				return cmdrunner.RunResult{ExitCode: 0, Stdout: "ok"}, nil
			}
			return cmdrunner.RunResult{ExitCode: 0, Stdout: "ok"}, nil
		},
	}

	var out string
	withPatchedAppDeps(t, func(d *core.AppDeps) {
		d.BackupService = backup.New(nil, nil, nil, runner, slog.Default())
		d.BackupService.LookPath = func(_ string) (string, error) { return "/usr/bin/sqlite3", nil }
	}, func() {
		out = captureStdout(t, func() {
			if err := cmdbackup.RunVerifyBackup(core.CurrentAppDeps(), []string{"--instance", "asa1", "--backup", "latest"}); err != nil {
				t.Fatalf("runVerifyBackup: %v", err)
			}
		})
	})

	if len(checked) != 2 {
		t.Fatalf("expected 2 sqlite checks, got %d", len(checked))
	}
	if !strings.Contains(out, "Verification passed") {
		t.Fatalf("unexpected output: %s", out)
	}
}

func TestRunVerifyBackupFailsOnSQLiteIntegrityError(t *testing.T) {
	_, _, _ = setupBackupArchiveFixture(t, "asa1")

	runner := &arktest.MockCommandRunner{
		RunFn: func(_ context.Context, cmd []string, opts cmdrunner.RunOptions) (cmdrunner.RunResult, error) {
			if len(cmd) > 0 && cmd[0] == "tar" {
				// Simulate extraction: create a .sqlite file.
				if len(cmd) > 2 {
					destDir := cmd[len(cmd)-1]
					savedDir := filepath.Join(destDir, "Saved")
					if err := os.MkdirAll(savedDir, 0o755); err == nil {
						_ = os.WriteFile(filepath.Join(savedDir, "foo.sqlite"), []byte(""), 0o644)
					}
				}
				return cmdrunner.RunResult{ExitCode: 0}, nil
			}
			if len(cmd) > 1 && cmd[0] == "sqlite3" {
				return cmdrunner.RunResult{ExitCode: 1, Stderr: "integrity_check failed"}, nil
			}
			return cmdrunner.RunResult{ExitCode: 0, Stdout: "ok"}, nil
		},
	}

	withPatchedAppDeps(t, func(d *core.AppDeps) {
		d.BackupService = backup.New(nil, nil, nil, runner, slog.Default())
		d.BackupService.LookPath = func(_ string) (string, error) { return "/usr/bin/sqlite3", nil }
	}, func() {
		if err := cmdbackup.RunVerifyBackup(core.CurrentAppDeps(), []string{"asa1", "latest"}); err == nil {
			t.Fatal("expected error")
		}
	})
}

func TestRunPruneBackupsSingleCallsRetention(t *testing.T) {
	_, instanceDir, _ := setupBackupArchiveFixture(t, "asa1")

	// Create additional backups so retention has something to work with.
	for i := 1; i <= 3; i++ {
		name := filepath.Join(instanceDir, "asa1-backup-20240201-"+strconv.Itoa(i)+".tar.gz")
		if err := os.WriteFile(name, []byte("data"), 0o644); err != nil {
			t.Fatalf("write: %v", err)
		}
	}

	var out string
	withPatchedAppDeps(t, func(d *core.AppDeps) {
		d.BackupService = newTestBackupService(t)
	}, func() {
		out = captureStdout(t, func() {
			if err := cmdbackup.RunPruneBackups(core.CurrentAppDeps(), []string{"--instance", "asa1", "--retain-count", "5", "--retain-days", "10", "--retain-size-gb", "2"}); err != nil {
				t.Fatalf("runPruneBackups: %v", err)
			}
		})
	})

	if !strings.Contains(out, "Pruned backups for asa1") {
		t.Fatalf("unexpected output: %s", out)
	}
}

func TestRunPruneBackupsAllUsesComposeServices(t *testing.T) {
	backupRoot := filepath.Join(t.TempDir(), "backups")
	setEnv(t, "ARK_BACKUP_DIR", backupRoot)
	for _, name := range []string{"a", "b"} {
		dir := filepath.Join(backupRoot, name)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(filepath.Join(dir, name+"-backup-20240101-000000.tar.gz"), []byte("x"), 0o644); err != nil {
			t.Fatalf("write: %v", err)
		}
	}

	withPatchedAppDeps(t, func(d *core.AppDeps) {
		d.ComposeService = &arktest.MockCompose{
			LoadProjectFn: func(_ context.Context, _ string) (*ark.ComposeProject, error) {
				return fakeProject(emptyServices("a", "b")), nil
			},
		}
		d.BackupService = newTestBackupService(t)
	}, func() {
		if err := cmdbackup.RunPruneBackups(core.CurrentAppDeps(), []string{"--instance", "all", "--compose-file", "compose.yaml"}); err != nil {
			t.Fatalf("runPruneBackups: %v", err)
		}
	})
}
