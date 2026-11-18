package backup

// Tests: restore edge cases: missing pre-restore artifact detection, auto-start error propagation, map guard

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ark-cli/internal/core"
	"ark-cli/internal/testutil"

	"pkg/ark"
	"pkg/ark/arktest"
	pkgbackup "pkg/ark/backup"

	"github.com/compose-spec/compose-go/v2/types"
)

func patchRestoreBaseDeps(d *core.AppDeps, instance string, svcConfig types.ServiceConfig) {
	svcConfig.Name = instance
	d.ComposeService = &arktest.MockCompose{
		LoadProjectFn: func(_ context.Context, _ string) (*ark.ComposeProject, error) {
			return fakeProject(types.Services{instance: svcConfig}), nil
		},
		RunningServicesFn: func(_ context.Context, _ string) (map[string]struct{}, error) {
			return map[string]struct{}{}, nil
		},
		AllServicesFn: func(_ context.Context, _ string) (map[string]struct{}, error) {
			return map[string]struct{}{instance: {}}, nil
		},
	}
}

func restoreFlags(instance, srcInstance, backupName string, extras ...string) []string {
	archive := backupName
	if srcInstance != "" {
		archive = srcInstance + ":" + backupName
	}
	args := []string{"--instance", instance, "--archive", archive}
	args = append(args, extras...)
	return args
}

func restoreNewFlags(newInstance, srcInstance, backupName string, extras ...string) []string {
	args := []string{"--new-instance", newInstance, "--archive", srcInstance + ":" + backupName}
	args = append(args, extras...)
	return args
}

// --- Flag validation tests ---

func TestRunRestoreRejectsNoFlags(t *testing.T) {
	withPatchedAppDeps(t, nil, func() {
		err := RunRestore(core.CurrentAppDeps(), nil)
		if err == nil {
			t.Fatal("expected error")
		}
		if !strings.Contains(err.Error(), "--instance") || !strings.Contains(err.Error(), "--new-instance") {
			t.Fatalf("expected --instance/--new-instance error, got: %v", err)
		}
	})
}

func TestRunRestoreRejectsBothInstanceAndNewInstance(t *testing.T) {
	withPatchedAppDeps(t, nil, func() {
		err := RunRestore(core.CurrentAppDeps(), []string{"--instance", "a", "--new-instance", "b", "--archive", "a:latest"})
		if err == nil {
			t.Fatal("expected error")
		}
		if !strings.Contains(err.Error(), "mutually exclusive") {
			t.Fatalf("expected mutual exclusion error, got: %v", err)
		}
	})
}

func TestRunRestoreRejectsNoArchiveFlag(t *testing.T) {
	withPatchedAppDeps(t, nil, func() {
		err := RunRestore(core.CurrentAppDeps(), []string{"--instance", "asa1"})
		if err == nil {
			t.Fatal("expected error")
		}
		if !strings.Contains(err.Error(), "--archive") {
			t.Fatalf("expected --archive error, got: %v", err)
		}
	})
}

func TestRunRestoreRejectsPositionalArgs(t *testing.T) {
	withPatchedAppDeps(t, nil, func() {
		err := RunRestore(core.CurrentAppDeps(), []string{"asa1", "latest"})
		if err == nil {
			t.Fatal("expected error")
		}
	})
}

func TestRunRestoreNewInstanceRequiresColonInArchive(t *testing.T) {
	withPatchedAppDeps(t, nil, func() {
		err := RunRestore(core.CurrentAppDeps(), []string{"--new-instance", "new1", "--archive", "latest"})
		if err == nil {
			t.Fatal("expected error")
		}
		if !strings.Contains(err.Error(), "invalid --archive format") {
			t.Fatalf("expected archive format error, got: %v", err)
		}
	})
}

func TestParseArchiveFlag(t *testing.T) {
	i := "instance"
	niSet := "new-instance"
	niEmpty := ""

	src, name := parseArchiveFlag("src:backup", &i, &niSet)
	if src != "src" || name != "backup" {
		t.Fatalf("parseArchiveFlag(src:backup) = %q, %q", src, name)
	}

	// No colon + new-instance flag set → no default source, returns empty.
	src, name = parseArchiveFlag("backup", &i, &niSet)
	if src != "" || name != "" {
		t.Fatalf("parseArchiveFlag(backup) with new-instance set = %q, %q", src, name)
	}

	// No colon + new-instance empty → defaults to --instance value.
	src, name = parseArchiveFlag("backup", &i, &niEmpty)
	if src != "instance" || name != "backup" {
		t.Fatalf("parseArchiveFlag(backup) new-instance empty = %q, %q", src, name)
	}

	// No colon + new-instance set + empty instance → returns empty.
	emptyI := ""
	src, name = parseArchiveFlag("backup", &emptyI, &niSet)
	if src != "" || name != "" {
		t.Fatalf("parseArchiveFlag(backup) new-instance set, empty instance = %q, %q", src, name)
	}
}

// --- Restore execution path tests ---

func TestRunRestoreFailsWhenPreRestoreArtifactIsMissing(t *testing.T) {
	instance := "asa1"
	tmp := t.TempDir()
	backupRoot := filepath.Join(tmp, "backups")
	backupsDir := filepath.Join(backupRoot, instance)
	if err := os.MkdirAll(backupsDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	createBackupArchive(t, backupsDir, "asa1-backup-20240102-120000.tar.gz")
	setEnv(t, "ARK_BACKUP_DIR", backupRoot)
	setEnv(t, "COMPOSE_FILE", "compose.yaml")

	var ranRunOneOff bool
	withPatchedAppDeps(t, func(d *core.AppDeps) {
		patchRestoreBaseDeps(d, instance, types.ServiceConfig{})
		d.ComposeService.(*arktest.MockCompose).RunOneOffFn = func(_ context.Context, _, _ string, _ []string, _ ark.RunOptions) (ark.ExecResult, error) {
			ranRunOneOff = true
			return ark.ExecResult{}, nil
		}
	}, func() {
		err := RunRestore(core.CurrentAppDeps(), restoreFlags(instance, instance, "latest"))
		if err == nil {
			t.Fatal("expected pre-restore error")
		}
		if !strings.Contains(err.Error(), "pre-restore backup not created") {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	if !ranRunOneOff {
		t.Fatal("expected restore RunOneOff call")
	}
}

func TestRunRestoreAutoStartUpErrorIncludesServiceName(t *testing.T) {
	instance := "asa1"
	tmp := t.TempDir()
	backupRoot := filepath.Join(tmp, "backups")
	backupsDir := filepath.Join(backupRoot, instance)
	if err := os.MkdirAll(backupsDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	createBackupArchive(t, backupsDir, "asa1-backup-20240102-120000.tar.gz")
	setEnv(t, "ARK_BACKUP_DIR", backupRoot)
	setEnv(t, "COMPOSE_FILE", "compose.yaml")

	withPatchedAppDeps(t, func(d *core.AppDeps) {
		patchRestoreBaseDeps(d, instance, types.ServiceConfig{})
		mock := d.ComposeService.(*arktest.MockCompose)
		mock.RunOneOffFn = func(_ context.Context, _, _ string, command []string, opts ark.RunOptions) (ark.ExecResult, error) {
			writePreRestoreArtifactFromRunOneOff(t, command, opts)
			return ark.ExecResult{}, nil
		}
		mock.UpFn = func(_ context.Context, _ string, _ []string, _ ark.UpOptions) error {
			return errors.New("compose up failed for service asa1")
		}
	}, func() {
		err := RunRestore(core.CurrentAppDeps(), restoreFlags(instance, instance, "latest", "--auto-start"))
		if err == nil {
			t.Fatal("expected compose up error")
		}
		if !strings.Contains(err.Error(), "compose up failed for service asa1") {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}

func TestRunRestoreRejectsMapMismatch(t *testing.T) {
	instance := "asa1"
	tmp := t.TempDir()
	backupRoot := filepath.Join(tmp, "backups")
	backupsDir := filepath.Join(backupRoot, instance)
	if err := os.MkdirAll(backupsDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	createTarArchiveWithMap(t, backupsDir, "asa1-backup-20240102-120000.tar", "TheIsland_WP")
	setEnv(t, "ARK_BACKUP_DIR", backupRoot)
	setEnv(t, "COMPOSE_FILE", "compose.yaml")

	svcConfig := types.ServiceConfig{
		Environment: testEnv("MAP_NAME=Aberration_P"),
	}
	withPatchedAppDeps(t, func(d *core.AppDeps) {
		patchRestoreBaseDeps(d, instance, svcConfig)
	}, func() {
		err := RunRestore(core.CurrentAppDeps(), restoreFlags(instance, instance, "latest"))
		if err == nil {
			t.Fatal("expected map mismatch error")
		}
		if !strings.Contains(err.Error(), "TheIsland_WP") || !strings.Contains(err.Error(), "Aberration_P") {
			t.Fatalf("expected both map names in error, got: %v", err)
		}
	})
}

func TestRunRestoreAllowsMatchingMap(t *testing.T) {
	instance := "asa1"
	tmp := t.TempDir()
	backupRoot := filepath.Join(tmp, "backups")
	backupsDir := filepath.Join(backupRoot, instance)
	if err := os.MkdirAll(backupsDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	createTarArchiveWithMap(t, backupsDir, "asa1-backup-20240102-120000.tar", "TheIsland_WP")
	setEnv(t, "ARK_BACKUP_DIR", backupRoot)
	setEnv(t, "COMPOSE_FILE", "compose.yaml")

	svcConfig := types.ServiceConfig{
		Environment: testEnv("MAP_NAME=TheIsland_WP"),
	}
	withPatchedAppDeps(t, func(d *core.AppDeps) {
		patchRestoreBaseDeps(d, instance, svcConfig)
		d.ComposeService.(*arktest.MockCompose).RunOneOffFn = func(_ context.Context, _, _ string, command []string, opts ark.RunOptions) (ark.ExecResult, error) {
			writePreRestoreArtifactFromRunOneOff(t, command, opts)
			return ark.ExecResult{}, nil
		}
	}, func() {
		if err := RunRestore(core.CurrentAppDeps(), restoreFlags(instance, instance, "latest")); err != nil {
			t.Fatalf("expected success for matching map, got: %v", err)
		}
	})
}

func TestRunRestoreSkipsMapCheckWhenNoMapNameInCompose(t *testing.T) {
	instance := "asa1"
	tmp := t.TempDir()
	backupRoot := filepath.Join(tmp, "backups")
	backupsDir := filepath.Join(backupRoot, instance)
	if err := os.MkdirAll(backupsDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	createTarArchiveWithMap(t, backupsDir, "asa1-backup-20240102-120000.tar", "TheIsland_WP")
	setEnv(t, "ARK_BACKUP_DIR", backupRoot)
	setEnv(t, "COMPOSE_FILE", "compose.yaml")

	withPatchedAppDeps(t, func(d *core.AppDeps) {
		patchRestoreBaseDeps(d, instance, types.ServiceConfig{})
		d.ComposeService.(*arktest.MockCompose).RunOneOffFn = func(_ context.Context, _, _ string, command []string, opts ark.RunOptions) (ark.ExecResult, error) {
			writePreRestoreArtifactFromRunOneOff(t, command, opts)
			return ark.ExecResult{}, nil
		}
	}, func() {
		if err := RunRestore(core.CurrentAppDeps(), restoreFlags(instance, instance, "latest")); err != nil {
			t.Fatalf("expected success when compose has no MAP_NAME, got: %v", err)
		}
	})
}

func TestRunRestoreInstanceWithExplicitSource(t *testing.T) {
	instance := "asa1"
	srcInstance := "asa2"
	tmp := t.TempDir()
	backupRoot := filepath.Join(tmp, "backups")

	// Backup in src instance dir
	srcBackupsDir := filepath.Join(backupRoot, srcInstance)
	if err := os.MkdirAll(srcBackupsDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	createBackupArchive(t, srcBackupsDir, "asa2-backup-20240102-120000.tar.gz")
	setEnv(t, "ARK_BACKUP_DIR", backupRoot)
	setEnv(t, "COMPOSE_FILE", "compose.yaml")

	svcConfig := types.ServiceConfig{}
	withPatchedAppDeps(t, func(d *core.AppDeps) {
		patchRestoreBaseDeps(d, instance, svcConfig)
		d.ComposeService.(*arktest.MockCompose).RunOneOffFn = func(_ context.Context, _, _ string, command []string, opts ark.RunOptions) (ark.ExecResult, error) {
			writePreRestoreArtifactFromRunOneOff(t, command, opts)
			return ark.ExecResult{}, nil
		}
	}, func() {
		if err := RunRestore(core.CurrentAppDeps(), restoreFlags(instance, srcInstance, "latest")); err != nil {
			t.Fatalf("expected success with explicit source, got: %v", err)
		}
	})
}

func TestRunRestoreNewInstanceUnknownSource(t *testing.T) {
	tmp := t.TempDir()
	backupRoot := filepath.Join(tmp, "backups")
	setEnv(t, "ARK_BACKUP_DIR", backupRoot)
	setEnv(t, "COMPOSE_FILE", "compose.yaml")

	withPatchedAppDeps(t, nil, func() {
		err := RunRestore(core.CurrentAppDeps(), restoreNewFlags("new1", "nonexistent", "latest"))
		if err == nil {
			t.Fatal("expected error for unknown source")
		}
	})
}

func TestParseArchiveMapName(t *testing.T) {
	tests := []struct {
		name    string
		listing string
		want    string
	}{
		{
			name:    "unix path with leading dot-slash",
			listing: "./SavedArks/TheIsland_WP/\n./SavedArks/TheIsland_WP/save.ark\n",
			want:    "TheIsland_WP",
		},
		{
			name:    "path without leading dot-slash",
			listing: "SavedArks/Aberration_P/\nSavedArks/Aberration_P/save.ark\n",
			want:    "Aberration_P",
		},
		{
			name:    "no SavedArks directory",
			listing: "Config/WindowsServer/Game.ini\n",
			want:    "",
		},
		{
			name:    "empty listing",
			listing: "",
			want:    "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := pkgbackup.ParseArchiveMapName(tt.listing)
			if got != tt.want {
				t.Fatalf("parseArchiveMapName(%q) = %q, want %q", tt.listing, got, tt.want)
			}
		})
	}
}

// --- Cluster restore ---

func clusterRestoreDeps(t *testing.T, running map[string]struct{}, commands *[][]string) func(*core.AppDeps) {
	t.Helper()
	composeMock := &arktest.MockCompose{
		LoadProjectFn: func(_ context.Context, _ string) (*ark.ComposeProject, error) {
			return fakeProject(clusterServices(true, "asa1", "asa2")), nil
		},
		RunningServicesFn: func(_ context.Context, _ string) (map[string]struct{}, error) {
			return running, nil
		},
		RunOneOffFn: func(_ context.Context, _, _ string, command []string, opts ark.RunOptions) (ark.ExecResult, error) {
			*commands = append(*commands, command)
			writePreRestoreArtifactFromRunOneOff(t, command, opts)
			return ark.ExecResult{}, nil
		},
	}
	return func(d *core.AppDeps) {
		d.ComposeService = composeMock
		d.BackupService = pkgbackup.New(&arktest.MockEngine{}, composeMock, nil, &arktest.MockCommandRunner{}, d.Logger)
	}
}

func seedClusterBackups(t *testing.T, names ...string) {
	t.Helper()
	root := t.TempDir()
	setEnv(t, "ARK_BACKUP_DIR", root)
	setEnv(t, "COMPOSE_FILE", "docker-compose.yaml")
	if err := os.MkdirAll(filepath.Join(root, "cluster"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range names {
		createBackupArchive(t, filepath.Join(root, "cluster"), name)
	}
}

func TestRunRestoreClusterLatest(t *testing.T) {
	seedClusterBackups(t, "cluster-backup-20260101-000000.tar.gz", "cluster-backup-20260102-000000.tar.gz")
	var commands [][]string
	withPatchedAppDeps(t, clusterRestoreDeps(t, map[string]struct{}{}, &commands), func() {
		out := testutil.CaptureStdout(t, func() {
			if err := RunRestore(core.CurrentAppDeps(), []string{"--instance", "cluster", "--archive", "latest"}); err != nil {
				t.Fatalf("RunRestore: %v", err)
			}
		})
		if !strings.Contains(out, "Restored cluster data (ark-cluster-1). Pre-restore backup:") {
			t.Fatalf("output = %q", out)
		}
	})
	if len(commands) != 1 || commands[0][len(commands[0])-1] != "--cluster" {
		t.Fatalf("commands = %v", commands)
	}
	if !strings.Contains(strings.Join(commands[0], " "), "cluster-restore-input-") {
		t.Fatalf("archive not passed: %v", commands[0])
	}
}

func TestRunRestoreClusterRefusesWhileInstanceRuns(t *testing.T) {
	seedClusterBackups(t, "cluster-backup-20260101-000000.tar.gz")
	var commands [][]string
	withPatchedAppDeps(t, clusterRestoreDeps(t, map[string]struct{}{"asa2": {}}, &commands), func() {
		err := RunRestore(core.CurrentAppDeps(), []string{"--instance", "cluster", "--archive", "latest"})
		if err == nil || !strings.Contains(err.Error(), "asa2") {
			t.Fatalf("got %v", err)
		}
	})
	if len(commands) != 0 {
		t.Fatalf("restore ran: %v", commands)
	}
}

func TestRunRestoreClusterRejectsFlags(t *testing.T) {
	for name, args := range map[string][]string{
		"auto-start": {"--instance", "cluster", "--archive", "latest", "--auto-start"},
		"src prefix": {"--instance", "cluster", "--archive", "asa1:latest"},
	} {
		t.Run(name, func(t *testing.T) {
			withPatchedAppDeps(t, nil, func() {
				if err := RunRestore(core.CurrentAppDeps(), args); err == nil {
					t.Fatal("expected an error")
				}
			})
		})
	}
}
