package backup

import (
	"archive/tar"
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"ark-cli/internal/core"

	"pkg/ark"
	"pkg/ark/arktest"
	pkgbackup "pkg/ark/backup"
	"pkg/ark/cmdrunner"

	"github.com/compose-spec/compose-go/v2/types"
)

func TestWithInstanceBackupLockFailsWhenHeld(t *testing.T) {
	lockDir := t.TempDir()
	lockPath := filepath.Join(lockDir, ".asa1-backup.lock")
	if err := os.WriteFile(lockPath, []byte("held"), 0o600); err != nil {
		t.Fatalf("write lock: %v", err)
	}
	err := withInstanceBackupLock(lockDir, "asa1", 0, func() error { return nil })
	if err == nil {
		t.Fatal("expected lock-held error")
	}
	if !strings.Contains(err.Error(), "backup lock is held") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunVerifyBackupSuccess(t *testing.T) {
	tmp := t.TempDir()
	backupRoot := tmp
	instance := "asa1"
	backupsDir := filepath.Join(backupRoot, instance)
	if err := os.MkdirAll(backupsDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	createTarWithSQLite(t, backupsDir, instance+"-backup-20240201-120000.tar.gz")
	setEnv(t, "ARK_BACKUP_DIR", backupRoot)

	runner := &arktest.MockCommandRunner{
		RunFn: func(_ context.Context, cmd []string, _ cmdrunner.RunOptions) (cmdrunner.RunResult, error) {
			if len(cmd) > 0 && cmd[0] == "tar" && len(cmd) > 2 && cmd[1] == "-xf" {
				destDir := cmd[len(cmd)-1]
				savedDir := filepath.Join(destDir, "Saved")
				if err := os.MkdirAll(savedDir, 0o755); err == nil {
					_ = os.WriteFile(filepath.Join(savedDir, "test.sqlite"), []byte(""), 0o644)
				}
				return cmdrunner.RunResult{ExitCode: 0}, nil
			}
			if len(cmd) > 1 && cmd[0] == "sqlite3" {
				return cmdrunner.RunResult{ExitCode: 0, Stdout: "ok"}, nil
			}
			return cmdrunner.RunResult{ExitCode: 0, Stdout: "ok"}, nil
		},
	}
	bs := pkgbackup.New(nil, nil, nil, runner, slog.Default())
	bs.LookPath = func(_ string) (string, error) { return "/usr/bin/sqlite3", nil }

	withPatchedAppDeps(t, func(d *core.AppDeps) {
		d.BackupService = bs
	}, func() {
		if err := RunVerifyBackup(core.CurrentAppDeps(), []string{"--instance", instance, "--backup", "latest"}); err != nil {
			t.Fatalf("RunVerifyBackup: %v", err)
		}
	})
}

func TestRunVerifyBackupInvalidArchive(t *testing.T) {
	tmp := t.TempDir()
	backupRoot := tmp
	instance := "asa1"
	backupsDir := filepath.Join(backupRoot, instance)
	if err := os.MkdirAll(backupsDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	corruptPath := filepath.Join(backupsDir, instance+"-backup-20240201-120000.tar.gz")
	if err := os.WriteFile(corruptPath, []byte("not a real tar"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	setEnv(t, "ARK_BACKUP_DIR", backupRoot)

	runner := &arktest.MockCommandRunner{
		RunFn: func(_ context.Context, cmd []string, _ cmdrunner.RunOptions) (cmdrunner.RunResult, error) {
			if len(cmd) > 0 && cmd[0] == "tar" && len(cmd) > 2 && cmd[1] == "-xf" {
				return cmdrunner.RunResult{ExitCode: 1, Stderr: "tar: This does not look like a tar archive"}, nil
			}
			return cmdrunner.RunResult{ExitCode: 0, Stdout: "ok"}, nil
		},
	}
	bs := pkgbackup.New(nil, nil, nil, runner, slog.Default())

	withPatchedAppDeps(t, func(d *core.AppDeps) {
		d.BackupService = bs
	}, func() {
		err := RunVerifyBackup(core.CurrentAppDeps(), []string{"--instance", instance, "--backup", "latest"})
		if err == nil {
			t.Fatal("expected error for corrupt archive")
		}
		if !strings.Contains(err.Error(), "failed to extract backup archive") {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}

func TestRunPruneBackupsSingleInstance(t *testing.T) {
	tmp := t.TempDir()
	backupRoot := tmp
	instance := "asa1"
	instanceDir := filepath.Join(backupRoot, instance)
	if err := os.MkdirAll(instanceDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	for i := 1; i <= 3; i++ {
		name := filepath.Join(instanceDir, instance+"-backup-20240201-"+strconv.Itoa(i)+".tar.gz")
		if err := os.WriteFile(name, []byte("data"), 0o644); err != nil {
			t.Fatalf("write: %v", err)
		}
	}
	setEnv(t, "ARK_BACKUP_DIR", backupRoot)

	runner := &arktest.MockCommandRunner{
		RunFn: func(_ context.Context, _ []string, _ cmdrunner.RunOptions) (cmdrunner.RunResult, error) {
			return cmdrunner.RunResult{ExitCode: 0, Stdout: "ok"}, nil
		},
	}
	bs := pkgbackup.New(nil, nil, nil, runner, slog.Default())
	bs.DiskUsage = func(_ string) (int64, int64, error) {
		return pkgbackup.MinFreeBackupBytes + 1024, 0, nil
	}

	withPatchedAppDeps(t, func(d *core.AppDeps) {
		d.BackupService = bs
	}, func() {
		if err := RunPruneBackups(core.CurrentAppDeps(), []string{"--instance", instance}); err != nil {
			t.Fatalf("RunPruneBackups: %v", err)
		}
	})
}

func TestRunPruneBackupsAllInstances(t *testing.T) {
	tmp := t.TempDir()
	backupRoot := tmp
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

	runner := &arktest.MockCommandRunner{
		RunFn: func(_ context.Context, _ []string, _ cmdrunner.RunOptions) (cmdrunner.RunResult, error) {
			return cmdrunner.RunResult{ExitCode: 0, Stdout: "ok"}, nil
		},
	}
	bs := pkgbackup.New(nil, nil, nil, runner, slog.Default())
	bs.DiskUsage = func(_ string) (int64, int64, error) {
		return pkgbackup.MinFreeBackupBytes + 1024, 0, nil
	}

	withPatchedAppDeps(t, func(d *core.AppDeps) {
		d.ComposeService = &arktest.MockCompose{
			LoadProjectFn: func(_ context.Context, _ string) (*ark.ComposeProject, error) {
				return fakeProject(types.Services{"a": {Name: "a"}, "b": {Name: "b"}}), nil
			},
		}
		d.BackupService = bs
	}, func() {
		if err := RunPruneBackups(core.CurrentAppDeps(), []string{"--instance", "all", "--compose-file", "docker-compose.yaml"}); err != nil {
			t.Fatalf("RunPruneBackups: %v", err)
		}
	})
}

func createTarWithSQLite(t *testing.T, dir, filename string) string {
	t.Helper()
	archivePath := filepath.Join(dir, filename)
	f, err := os.Create(archivePath)
	if err != nil {
		t.Fatalf("create archive: %v", err)
	}
	defer f.Close()
	tw := tar.NewWriter(f)
	if err := tw.WriteHeader(&tar.Header{
		Typeflag: tar.TypeReg,
		Name:     "Saved/test.sqlite",
		Size:     0,
		Mode:     0o644,
	}); err != nil {
		t.Fatalf("write tar header: %v", err)
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("close tar: %v", err)
	}
	return archivePath
}
