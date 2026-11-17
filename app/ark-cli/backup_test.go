package main

// Tests: backup and restore commands: archive sorting, argument parsing, auto-stop/start, overwrite and conflict detection

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"ark-cli/internal/backup"
	"ark-cli/internal/core"
	"pkg/ark"
	"pkg/ark/arktest"
	pkgbackup "pkg/ark/backup"
)

func createBackupArchive(t *testing.T, backupsDir, filename string) string {
	t.Helper()
	archivePath := filepath.Join(backupsDir, filename)
	if err := os.WriteFile(archivePath, []byte("data"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	return archivePath
}

// writePreRestoreArtifactFromRunOneOff simulates the restore container creating
// a pre-restore backup file, matching the volume-mount and --pre-restore args
// passed via the RunOneOff interface.
func writePreRestoreArtifactFromRunOneOff(t *testing.T, command []string, opts ark.RunOptions) {
	t.Helper()

	hostPreRestoreDir := ""
	for _, v := range opts.Volumes {
		if strings.HasSuffix(v, ":/home/ark-user/maintenance/output") {
			hostPreRestoreDir = strings.TrimSuffix(v, ":/home/ark-user/maintenance/output")
			break
		}
	}

	for i := 0; i < len(command)-1; i++ {
		if command[i] != "--pre-restore" {
			continue
		}
		target := command[i+1]
		if hostPreRestoreDir != "" {
			target = filepath.Join(hostPreRestoreDir, filepath.Base(command[i+1]))
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			t.Fatalf("mkdir pre-restore parent: %v", err)
		}
		if err := os.WriteFile(target, []byte("pre-restore"), 0o644); err != nil {
			t.Fatalf("write pre-restore file: %v", err)
		}
		return
	}
}

func TestBackupSortOrdering(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name         string
		paths        []string
		applyMtime   func([]string) error
		wantFirstIdx int
	}{
		{
			name:         "timestamp in filename",
			paths:        []string{"asa-backup-20231201-120000.tar.gz", "asa-backup-20240101-120000.tar.gz"},
			wantFirstIdx: 1,
		},
		{
			name:  "mtime fallback",
			paths: []string{"random.tar.gz", "another.tar.gz"},
			applyMtime: func(paths []string) error {
				if err := os.Chtimes(paths[0], time.Unix(1, 0), time.Unix(1, 0)); err != nil {
					return err
				}
				return os.Chtimes(paths[1], time.Unix(2, 0), time.Unix(2, 0))
			},
			wantFirstIdx: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			tmp := t.TempDir()
			full := make([]string, 0, len(tt.paths))
			for _, name := range tt.paths {
				path := filepath.Join(tmp, name)
				if err := os.WriteFile(path, []byte("data"), 0o644); err != nil {
					t.Fatalf("write: %v", err)
				}
				full = append(full, path)
			}
			if tt.applyMtime != nil {
				if err := tt.applyMtime(full); err != nil {
					t.Fatalf("apply mtime: %v", err)
				}
			}

			files := append([]string(nil), full...)
			sort.Slice(files, func(i, j int) bool {
				left, _ := os.Stat(files[i])
				right, _ := os.Stat(files[j])
				return pkgbackup.CompareArchiveSort(files[i], left, files[j], right)
			})
			wantFirst := full[tt.wantFirstIdx]
			if files[0] != wantFirst {
				t.Fatalf("expected %s first, got %s", wantFirst, files[0])
			}
		})
	}
}

func TestRunRestoreUsesLatest(t *testing.T) {
	instance := "asa1"
	tmp := t.TempDir()
	backupRoot := filepath.Join(tmp, "backups")
	backupsDir := filepath.Join(backupRoot, instance)
	if err := os.MkdirAll(backupsDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	_ = createBackupArchive(t, backupsDir, "asa1-backup-20240101-120000.tar.gz")
	newer := createBackupArchive(t, backupsDir, "asa1-backup-20240102-120000.tar.gz")
	setEnv(t, "ARK_BACKUP_DIR", backupRoot)
	setEnv(t, "COMPOSE_FILE", "compose.yaml")

	var runOneOffVolumes []string
	withPatchedAppDeps(t, func(d *core.AppDeps) {
		d.ComposeService = &arktest.MockCompose{
			LoadProjectFn: func(_ context.Context, _ string) (*ark.ComposeProject, error) {
				return fakeProject(emptyServices(instance)), nil
			},
			RunningServicesFn: func(_ context.Context, _ string) (map[string]struct{}, error) {
				return map[string]struct{}{}, nil
			},
			RunOneOffFn: func(_ context.Context, _, _ string, command []string, opts ark.RunOptions) (ark.ExecResult, error) {
				runOneOffVolumes = opts.Volumes
				writePreRestoreArtifactFromRunOneOff(t, command, opts)
				return ark.ExecResult{}, nil
			},
		}
	}, func() {
		if err := backup.RunRestore(core.CurrentAppDeps(), []string{"--instance", instance, "--archive", "latest"}); err != nil {
			t.Fatalf("runRestore: %v", err)
		}
	})

	if len(runOneOffVolumes) == 0 {
		t.Fatal("expected RunOneOff to be called with volume mounts")
	}
	foundNewer := false
	for _, v := range runOneOffVolumes {
		if strings.Contains(v, newer) {
			foundNewer = true
		}
	}
	if !foundNewer {
		t.Fatalf("expected newer archive %s in volumes, got %v", newer, runOneOffVolumes)
	}
}

func TestRunRestoreResolvesNameSuffix(t *testing.T) {
	instance := "asa1"
	tmp := t.TempDir()
	backupRoot := filepath.Join(tmp, "backups")
	backupsDir := filepath.Join(backupRoot, instance)
	if err := os.MkdirAll(backupsDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	tarPath := filepath.Join(backupsDir, "asa1-custom.tar")
	tarGzPath := filepath.Join(backupsDir, "asa1-custom.tar.gz")
	if err := os.WriteFile(tarPath, []byte("data"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := os.WriteFile(tarGzPath, []byte("data"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	setEnv(t, "ARK_BACKUP_DIR", backupRoot)
	setEnv(t, "COMPOSE_FILE", "compose.yaml")

	var runOneOffVolumes []string
	withPatchedAppDeps(t, func(d *core.AppDeps) {
		d.ComposeService = &arktest.MockCompose{
			LoadProjectFn: func(_ context.Context, _ string) (*ark.ComposeProject, error) {
				return fakeProject(emptyServices(instance)), nil
			},
			RunningServicesFn: func(_ context.Context, _ string) (map[string]struct{}, error) {
				return map[string]struct{}{}, nil
			},
			RunOneOffFn: func(_ context.Context, _, _ string, command []string, opts ark.RunOptions) (ark.ExecResult, error) {
				runOneOffVolumes = opts.Volumes
				writePreRestoreArtifactFromRunOneOff(t, command, opts)
				return ark.ExecResult{}, nil
			},
		}
	}, func() {
		if err := backup.RunRestore(core.CurrentAppDeps(), []string{"--instance", instance, "--archive", "asa1-custom"}); err != nil {
			t.Fatalf("runRestore: %v", err)
		}
	})

	if len(runOneOffVolumes) == 0 {
		t.Fatal("expected RunOneOff to be called with volume mounts")
	}
	foundTarGz := false
	foundTar := false
	for _, v := range runOneOffVolumes {
		if strings.Contains(v, tarGzPath) {
			foundTarGz = true
		}
		if strings.Contains(v, tarPath+":") {
			foundTar = true
		}
	}
	if !foundTarGz {
		t.Fatal("expected tar.gz to be used")
	}
	if foundTar {
		t.Fatal("did not expect tar to be used")
	}
}

func TestRunRestoreRejectsRunningService(t *testing.T) {
	instance := "asa1"
	tmp := t.TempDir()
	backupRoot := filepath.Join(tmp, "backups")
	backupsDir := filepath.Join(backupRoot, instance)
	if err := os.MkdirAll(backupsDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	archive := filepath.Join(backupsDir, "asa1-backup-20240102-120000.tar.gz")
	if err := os.WriteFile(archive, []byte("data"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	setEnv(t, "ARK_BACKUP_DIR", backupRoot)
	setEnv(t, "COMPOSE_FILE", "compose.yaml")

	var err error
	withPatchedAppDeps(t, func(d *core.AppDeps) {
		d.ComposeService = &arktest.MockCompose{
			LoadProjectFn: func(_ context.Context, _ string) (*ark.ComposeProject, error) {
				return fakeProject(emptyServices(instance)), nil
			},
			RunningServicesFn: func(_ context.Context, _ string) (map[string]struct{}, error) {
				return map[string]struct{}{instance: {}}, nil
			},
		}
	}, func() {
		err = backup.RunRestore(core.CurrentAppDeps(), []string{"--instance", instance, "--archive", "latest"})
	})
	if err == nil {
		t.Fatal("expected running-service error")
	}
	if !strings.Contains(err.Error(), "must be stopped before restore") {
		t.Fatalf("expected stop guidance, got: %v", err)
	}
}

func TestRunRestoreAutoStartCallsComposeUp(t *testing.T) {
	instance := "asa1"
	tmp := t.TempDir()
	backupRoot := filepath.Join(tmp, "backups")
	backupsDir := filepath.Join(backupRoot, instance)
	if err := os.MkdirAll(backupsDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	archive := filepath.Join(backupsDir, "asa1-backup-20240102-120000.tar.gz")
	if err := os.WriteFile(archive, []byte("data"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	setEnv(t, "ARK_BACKUP_DIR", backupRoot)
	setEnv(t, "COMPOSE_FILE", "compose.yaml")

	var ranRestore, calledUp bool
	withPatchedAppDeps(t, func(d *core.AppDeps) {
		d.ComposeService = &arktest.MockCompose{
			LoadProjectFn: func(_ context.Context, _ string) (*ark.ComposeProject, error) {
				return fakeProject(emptyServices(instance)), nil
			},
			RunningServicesFn: func(_ context.Context, _ string) (map[string]struct{}, error) {
				return map[string]struct{}{}, nil
			},
			RunOneOffFn: func(_ context.Context, _, _ string, command []string, opts ark.RunOptions) (ark.ExecResult, error) {
				ranRestore = true
				writePreRestoreArtifactFromRunOneOff(t, command, opts)
				return ark.ExecResult{}, nil
			},
			UpFn: func(_ context.Context, _ string, _ []string, _ ark.UpOptions) error {
				calledUp = true
				return nil
			},
		}
	}, func() {
		if err := backup.RunRestore(core.CurrentAppDeps(), []string{"--instance", instance, "--archive", "latest", "--auto-start"}); err != nil {
			t.Fatalf("runRestore: %v", err)
		}
	})

	if !ranRestore {
		t.Fatal("expected restore RunOneOff call")
	}
	if !calledUp {
		t.Fatal("expected compose Up call for auto-start")
	}
}
