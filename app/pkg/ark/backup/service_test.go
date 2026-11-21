package backup

import (
	"archive/tar"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"pkg/ark"
	"pkg/ark/arktest"
	"pkg/ark/cmdrunner"
)

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func newTestService(compose ark.ComposeService, engine ark.ContainerEngine) *Service {
	runner := &mockRunner{}
	return New(engine, compose, nil, runner, discardLogger())
}

// --- Create path tests ---

func TestCreateForTargetUsesExecWhenRunning(t *testing.T) {
	var calledExec bool
	tmpDir := t.TempDir()

	compose := &arktest.MockCompose{
		RunningServicesFn: func(_ context.Context, _ string) (map[string]struct{}, error) {
			return map[string]struct{}{"asa1": {}}, nil
		},
		ExecFn: func(_ context.Context, _, _ string, _ []string, _ ark.ExecOptions) (ark.ExecResult, error) {
			calledExec = true
			return ark.ExecResult{ExitCode: 0}, nil
		},
		CopyFromServiceFn: func(_ context.Context, _, _, _, dstPath string) error {
			return os.WriteFile(dstPath, []byte("backup"), 0o644)
		},
	}

	s := newTestService(compose, &arktest.MockEngine{})
	s.DiskUsage = func(_ string) (int64, int64, error) { return 100e9, 200e9, nil }

	result, err := s.CreateForTarget(context.Background(), CreateRequest{
		ComposeFile:       "compose.yaml",
		BackupRoot:        tmpDir,
		Format:            "tar.gz",
		Timestamp:         "20260101-120000",
		PID:               1,
		LockTimeout:       time.Second,
		BackupWaitTimeout: time.Second,
	}, "asa1")
	if err != nil {
		t.Fatalf("CreateForTarget: %v", err)
	}
	if !calledExec {
		t.Fatal("expected Exec to be called for running service")
	}
	if result.ArchivePath == "" {
		t.Fatal("expected non-empty archive path")
	}
}

func TestCreateForTargetUsesEphemeralWhenStopped(t *testing.T) {
	var calledRunDetached bool
	tmpDir := t.TempDir()

	compose := &arktest.MockCompose{
		RunningServicesFn: func(_ context.Context, _ string) (map[string]struct{}, error) {
			return map[string]struct{}{}, nil
		},
		RunOneOffDetachedFn: func(_ context.Context, _, _, _ string, _ []string, _ ark.RunOptions) error {
			calledRunDetached = true
			return nil
		},
	}
	engine := &arktest.MockEngine{
		InspectStateFn: func(_ context.Context, _ string) (ark.ContainerState, error) {
			return ark.ContainerState{Running: true}, nil
		},
		CopyFromContainerFn: func(_ context.Context, _, _, dstPath string) error {
			return os.WriteFile(dstPath, []byte("backup"), 0o644)
		},
	}
	s := newTestService(compose, engine)
	s.DiskUsage = func(_ string) (int64, int64, error) { return 100e9, 200e9, nil }
	s.CheckFileInContainer = func(_ context.Context, _, _ string) (bool, error) { return true, nil }

	result, err := s.CreateForTarget(context.Background(), CreateRequest{
		ComposeFile:       "compose.yaml",
		BackupRoot:        tmpDir,
		Format:            "tar.gz",
		Timestamp:         "20260101-120000",
		PID:               1,
		LockTimeout:       time.Second,
		BackupWaitTimeout: time.Second,
	}, "asa1")
	if err != nil {
		t.Fatalf("CreateForTarget: %v", err)
	}
	if !calledRunDetached {
		t.Fatal("expected RunOneOffDetached for stopped service")
	}
	if result.ArchivePath == "" {
		t.Fatal("expected non-empty archive path")
	}
}

func TestCreateForTargetDoesNotFallBackToHelperWhileRunning(t *testing.T) {
	// The helper container can't trigger an RCON save, so it must never archive a live server.
	var calledExec, calledRunDetached bool
	tmpDir := t.TempDir()

	compose := &arktest.MockCompose{
		RunningServicesFn: func(_ context.Context, _ string) (map[string]struct{}, error) {
			return map[string]struct{}{"asa1": {}}, nil
		},
		ExecFn: func(_ context.Context, _, _ string, _ []string, _ ark.ExecOptions) (ark.ExecResult, error) {
			calledExec = true
			return ark.ExecResult{ExitCode: 17}, nil
		},
		RunOneOffDetachedFn: func(_ context.Context, _, _, _ string, _ []string, _ ark.RunOptions) error {
			calledRunDetached = true
			return nil
		},
	}
	engine := &arktest.MockEngine{
		InspectStateFn: func(_ context.Context, _ string) (ark.ContainerState, error) {
			return ark.ContainerState{Running: true}, nil
		},
		CopyFromContainerFn: func(_ context.Context, _, _, dstPath string) error {
			return os.WriteFile(dstPath, []byte("backup"), 0o644)
		},
	}
	s := newTestService(compose, engine)
	s.DiskUsage = func(_ string) (int64, int64, error) { return 100e9, 200e9, nil }
	s.CheckFileInContainer = func(_ context.Context, _, _ string) (bool, error) { return true, nil }

	_, err := s.CreateForTarget(context.Background(), CreateRequest{
		ComposeFile:       "compose.yaml",
		BackupRoot:        tmpDir,
		Format:            "tar.gz",
		Timestamp:         "20260101-120000",
		PID:               1,
		LockTimeout:       time.Second,
		BackupWaitTimeout: time.Second,
	}, "asa1")
	if err == nil || !strings.Contains(err.Error(), "while the server is running") {
		t.Fatalf("expected running-server backup error, got %v", err)
	}
	if !calledExec {
		t.Fatal("expected Exec to be called first")
	}
	if calledRunDetached {
		t.Fatal("helper container must not be started while the server is running")
	}
}

func TestCreateForTargetFailsWhenRunningStateUnknown(t *testing.T) {
	var calledRunDetached bool
	compose := &arktest.MockCompose{
		RunningServicesFn: func(_ context.Context, _ string) (map[string]struct{}, error) {
			return nil, errors.New("daemon unavailable")
		},
		RunOneOffDetachedFn: func(_ context.Context, _, _, _ string, _ []string, _ ark.RunOptions) error {
			calledRunDetached = true
			return nil
		},
	}
	s := newTestService(compose, &arktest.MockEngine{})
	_, err := s.CreateForTarget(context.Background(), CreateRequest{
		ComposeFile:       "compose.yaml",
		BackupRoot:        t.TempDir(),
		Format:            "tar.gz",
		Timestamp:         "20260101-120000",
		PID:               1,
		LockTimeout:       time.Second,
		BackupWaitTimeout: time.Second,
	}, "asa1")
	if err == nil || !strings.Contains(err.Error(), "could not determine whether the service is running") {
		t.Fatalf("expected running-state error, got %v", err)
	}
	if calledRunDetached {
		t.Fatal("helper container must not be started when running state is unknown")
	}
}

// --- Restore tests ---

func TestRestoreRequiresServiceStopped(t *testing.T) {
	tmpDir := t.TempDir()
	backupsDir := filepath.Join(tmpDir, "asa1")
	os.MkdirAll(backupsDir, 0o755)
	os.WriteFile(filepath.Join(backupsDir, "asa1-backup-20260101-120000.tar.gz"), []byte("x"), 0o644)

	compose := &arktest.MockCompose{
		RunningServicesFn: func(_ context.Context, _ string) (map[string]struct{}, error) {
			return map[string]struct{}{"asa1": {}}, nil
		},
	}
	s := newTestService(compose, &arktest.MockEngine{})

	_, err := s.Restore(context.Background(), RestoreRequest{
		ComposeFile: "compose.yaml",
		Service:     "asa1",
		Archive:     "latest",
		BackupRoot:  tmpDir,
	})
	if err == nil {
		t.Fatal("expected error for running service")
	}
	if !strings.Contains(err.Error(), "must be stopped") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRestoreAutoStartCallsUp(t *testing.T) {
	tmpDir := t.TempDir()
	backupsDir := filepath.Join(tmpDir, "asa1")
	os.MkdirAll(backupsDir, 0o755)
	os.WriteFile(filepath.Join(backupsDir, "asa1-backup-20260101-120000.tar.gz"), []byte("x"), 0o644)

	var upCalled bool
	compose := &arktest.MockCompose{
		RunningServicesFn: func(_ context.Context, _ string) (map[string]struct{}, error) {
			return map[string]struct{}{}, nil
		},
		RunOneOffFn: func(_ context.Context, _, service string, _ []string, opts ark.RunOptions) (ark.ExecResult, error) {
			timestamp := time.Now().UTC().Format("20060102-150405")
			for _, vol := range opts.Volumes {
				parts := strings.SplitN(vol, ":", 2)
				if len(parts) >= 2 && strings.Contains(parts[1], "maintenance/output") {
					hostDir := parts[0]
					os.WriteFile(filepath.Join(hostDir, fmt.Sprintf("%s-pre-restore-%s.tar.gz", service, timestamp)), []byte("x"), 0o644)
				}
			}
			return ark.ExecResult{ExitCode: 0}, nil
		},
		UpFn: func(_ context.Context, _ string, _ []string, _ ark.UpOptions) error {
			upCalled = true
			return nil
		},
	}
	s := newTestService(compose, &arktest.MockEngine{})

	_, err := s.Restore(context.Background(), RestoreRequest{
		ComposeFile: "compose.yaml",
		Service:     "asa1",
		Archive:     "latest",
		BackupRoot:  tmpDir,
		AutoStart:   true,
	})
	if err != nil {
		t.Fatalf("Restore with AutoStart: %v", err)
	}
	if !upCalled {
		t.Fatal("expected Up to be called for AutoStart restore")
	}
}

func TestRestoreFailsWhenBackupLockHeld(t *testing.T) {
	tmpDir := t.TempDir()
	backupsDir := filepath.Join(tmpDir, "asa1")
	os.MkdirAll(backupsDir, 0o755)
	os.WriteFile(filepath.Join(backupsDir, "asa1-backup-20260101-120000.tar.gz"), []byte("x"), 0o644)
	release, err := AcquireInstanceLock(backupsDir, "asa1", 0)
	if err != nil {
		t.Fatal(err)
	}
	defer release()

	compose := &arktest.MockCompose{
		RunningServicesFn: func(_ context.Context, _ string) (map[string]struct{}, error) {
			return map[string]struct{}{}, nil
		},
		RunOneOffFn: func(context.Context, string, string, []string, ark.RunOptions) (ark.ExecResult, error) {
			t.Fatal("restore ran while the backup lock was held")
			return ark.ExecResult{}, nil
		},
	}
	s := newTestService(compose, &arktest.MockEngine{})
	_, err = s.Restore(context.Background(), RestoreRequest{
		ComposeFile: "compose.yaml", Service: "asa1", Archive: "latest", BackupRoot: tmpDir,
	})
	if err == nil || !strings.Contains(err.Error(), "backup lock is held") {
		t.Fatalf("err = %v, want lock-held error", err)
	}
}

// LockHeld: the caller (a rollout) holds the lock on another descriptor; restore must not take it again.
func TestRestoreWithLockHeldRunsUnderCallersLock(t *testing.T) {
	tmpDir := t.TempDir()
	backupsDir := filepath.Join(tmpDir, "asa1")
	os.MkdirAll(backupsDir, 0o755)
	os.WriteFile(filepath.Join(backupsDir, "asa1-backup-20260101-120000.tar.gz"), []byte("x"), 0o644)
	release, err := AcquireInstanceLock(InstanceLockDir(tmpDir, "asa1"), "asa1", 0)
	if err != nil {
		t.Fatal(err)
	}
	defer release()

	compose := &arktest.MockCompose{
		RunningServicesFn: func(_ context.Context, _ string) (map[string]struct{}, error) {
			return map[string]struct{}{}, nil
		},
		RunOneOffFn: func(_ context.Context, _, service string, _ []string, opts ark.RunOptions) (ark.ExecResult, error) {
			hostDir, _, _ := strings.Cut(opts.Volumes[1], ":")
			name := fmt.Sprintf("%s-pre-restore-%s.tar.gz", service, time.Now().UTC().Format("20060102-150405"))
			return ark.ExecResult{}, os.WriteFile(filepath.Join(hostDir, name), []byte("x"), 0o644)
		},
	}
	s := newTestService(compose, &arktest.MockEngine{})
	_, err = s.Restore(context.Background(), RestoreRequest{
		ComposeFile: "compose.yaml", Service: "asa1", Archive: "latest", BackupRoot: tmpDir, LockHeld: true,
	})
	if err != nil {
		t.Fatalf("Restore with LockHeld: %v", err)
	}
}

func TestCreateForTargetLockHeld(t *testing.T) {
	for _, lockHeld := range []bool{true, false} {
		t.Run(fmt.Sprint("LockHeld=", lockHeld), func(t *testing.T) {
			tmpDir := t.TempDir()
			release, err := AcquireInstanceLock(InstanceLockDir(tmpDir, "asa1"), "asa1", 0)
			if err != nil {
				t.Fatal(err)
			}
			defer release()
			compose := &arktest.MockCompose{
				RunningServicesFn: func(_ context.Context, _ string) (map[string]struct{}, error) {
					return map[string]struct{}{"asa1": {}}, nil
				},
				ExecFn: func(_ context.Context, _, _ string, _ []string, _ ark.ExecOptions) (ark.ExecResult, error) {
					return ark.ExecResult{ExitCode: 0}, nil
				},
				CopyFromServiceFn: func(_ context.Context, _, _, _, dstPath string) error {
					return os.WriteFile(dstPath, []byte("backup"), 0o644)
				},
			}
			s := newTestService(compose, &arktest.MockEngine{})
			_, err = s.CreateForTarget(context.Background(), CreateRequest{
				ComposeFile: "compose.yaml", BackupRoot: tmpDir, Format: "tar.gz", Timestamp: "20260101-120000",
				PID: 1, BackupWaitTimeout: time.Second, LockHeld: lockHeld,
			}, "asa1")
			if lockHeld && err != nil {
				t.Fatalf("CreateForTarget with LockHeld: %v", err)
			}
			if !lockHeld && (err == nil || !strings.Contains(err.Error(), "backup lock is held")) {
				t.Fatalf("err = %v, want lock-held error", err)
			}
		})
	}
}

// --- Cleanup tests ---

func TestCleanupArtifactSourceEphemeralRemovesContainer(t *testing.T) {
	tmpDir := t.TempDir()
	var removedContainer string
	engine := &arktest.MockEngine{
		RemoveContainerFn: func(_ context.Context, container string) error {
			removedContainer = container
			return nil
		},
	}
	s := newTestService(&arktest.MockCompose{}, engine)

	s.cleanupArtifactSource(context.Background(), "compose.yaml", "asa1",
		ArtifactPaths{ContainerName: "asa1-helper", ContainerPath: filepath.Join(tmpDir, "backup.tar.gz")},
		true,
	)
	if removedContainer != "asa1-helper" {
		t.Fatalf("expected RemoveContainer(%q), got %q", "asa1-helper", removedContainer)
	}
}

func TestCleanupArtifactSourceInContainerUsesExec(t *testing.T) {
	tmpDir := t.TempDir()
	backupPath := filepath.Join(tmpDir, "backup.tar.gz")
	var gotService string
	var gotCmd []string
	compose := &arktest.MockCompose{
		ExecFn: func(_ context.Context, _, service string, command []string, _ ark.ExecOptions) (ark.ExecResult, error) {
			gotService = service
			gotCmd = command
			return ark.ExecResult{}, nil
		},
	}
	s := newTestService(compose, &arktest.MockEngine{})

	s.cleanupArtifactSource(context.Background(), "compose.yaml", "asa1",
		ArtifactPaths{ContainerPath: backupPath},
		false,
	)
	if gotService != "asa1" {
		t.Fatalf("expected exec on service asa1, got %q", gotService)
	}
	if len(gotCmd) < 3 || gotCmd[0] != "rm" || gotCmd[2] != backupPath {
		t.Fatalf("unexpected command: %v", gotCmd)
	}
}

// --- Format tests ---

func TestIsValidFormat(t *testing.T) {
	for _, tc := range []struct {
		format string
		want   bool
	}{
		{"tar", true},
		{"tar.gz", true},
		{"tar.zst", true},
		{"zip", false},
		{"", false},
	} {
		if got := IsValidFormat(tc.format); got != tc.want {
			t.Errorf("IsValidFormat(%q) = %v, want %v", tc.format, got, tc.want)
		}
	}
}

func TestDetectArchiveFormat(t *testing.T) {
	for _, tc := range []struct {
		name, want string
	}{
		{"backup.tar.gz", "tar.gz"},
		{"backup.tar.zst", "tar.zst"},
		{"backup.tar", "tar"},
		{"backup.zip", ""},
	} {
		if got := DetectArchiveFormat(tc.name); got != tc.want {
			t.Errorf("DetectArchiveFormat(%q) = %q, want %q", tc.name, got, tc.want)
		}
	}
}

func TestTrimArchiveSuffix(t *testing.T) {
	if got := TrimArchiveSuffix("asa1-backup-20260101.tar.gz"); got != "asa1-backup-20260101" {
		t.Errorf("TrimArchiveSuffix = %q", got)
	}
}

// --- Archive tests ---

func TestResolveArchiveLatest(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "asa1-backup-20260101-000001.tar.gz"), []byte("x"), 0o644)
	os.WriteFile(filepath.Join(dir, "asa1-backup-20260102-000001.tar.gz"), []byte("y"), 0o644)

	got, err := ResolveArchive(dir, "latest")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(got, "20260102-000001.tar.gz") {
		t.Fatalf("expected newest archive, got %q", got)
	}
}

func TestResolveArchiveByName(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "asa1-backup-20260101-000001.tar.gz"), []byte("x"), 0o644)

	got, err := ResolveArchive(dir, "asa1-backup-20260101-000001")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(got, "asa1-backup-20260101-000001.tar.gz") {
		t.Fatalf("unexpected: %q", got)
	}
}

func TestResolveArchiveNotFound(t *testing.T) {
	dir := t.TempDir()
	_, err := ResolveArchive(dir, "nonexistent")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestIsArchiveForInstance(t *testing.T) {
	if !IsArchiveForInstance("asa1", "asa1-backup-20260101.tar.gz") {
		t.Error("expected true")
	}
	if IsArchiveForInstance("asa1", "asa2-backup-20260101.tar.gz") {
		t.Error("expected false for different instance")
	}
	if IsArchiveForInstance("asa1", "asa1-backup-20260101.zip") {
		t.Error("expected false for non-archive suffix")
	}
}

func TestParseArchiveMapName(t *testing.T) {
	listing := "SavedArks/TheIsland_WP/\nSavedArks/TheIsland_WP/TheIsland_WP.ark\n"
	if got := ParseArchiveMapName(listing); got != "TheIsland_WP" {
		t.Errorf("ParseArchiveMapName = %q, want TheIsland_WP", got)
	}
	if got := ParseArchiveMapName("some/other/file\n"); got != "" {
		t.Errorf("expected empty, got %q", got)
	}
}

func TestParseArchiveMetadata(t *testing.T) {
	raw := `{"server_version":"35.21","build_id":"12345","instance_name":"test","map_name":"TheIsland_WP","backup_timestamp":"20260101-000001","environment":{"MAP_NAME":"TheIsland_WP"}}`
	m, err := ParseArchiveMetadata(raw)
	if err != nil {
		t.Fatal(err)
	}
	if m.ServerVersion != "35.21" {
		t.Fatalf("expected 35.21, got %s", m.ServerVersion)
	}
	if m.InstanceName != "test" {
		t.Fatalf("expected test, got %s", m.InstanceName)
	}
	if m.MapName != "TheIsland_WP" {
		t.Fatalf("expected TheIsland_WP, got %s", m.MapName)
	}
	if m.Environment["MAP_NAME"] != "TheIsland_WP" {
		t.Fatalf("expected TheIsland_WP, got %s", m.Environment["MAP_NAME"])
	}
}

func TestParseArchiveMetadataInvalidJSON(t *testing.T) {
	_, err := ParseArchiveMetadata("not json")
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestReadArchiveMetadataReturnsEmptyForMissing(t *testing.T) {
	runner := &mockRunner{
		RunFn: func(_ context.Context, _ []string, _ cmdrunner.RunOptions) (cmdrunner.RunResult, error) {
			return cmdrunner.RunResult{ExitCode: 1}, nil
		},
	}
	s := New(nil, nil, nil, runner, discardLogger())
	m, err := s.ReadArchiveMetadata(context.Background(), "/nonexistent.tar.gz")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m.ServerVersion != "" || m.InstanceName != "" {
		t.Fatalf("expected empty metadata, got %+v", m)
	}
}

func TestReadArchiveMetadataParsesValid(t *testing.T) {
	runner := &mockRunner{
		RunFn: func(_ context.Context, cmd []string, _ cmdrunner.RunOptions) (cmdrunner.RunResult, error) {
			for _, arg := range cmd {
				if arg == "./backup-metadata.json" {
					return cmdrunner.RunResult{
						ExitCode: 0,
						Stdout:   `{"server_version":"35.21","build_id":"12345","instance_name":"test","map_name":"TheIsland_WP","backup_timestamp":"20260101-000001","environment":{"MAP_NAME":"TheIsland_WP"}}`,
					}, nil
				}
			}
			return cmdrunner.RunResult{ExitCode: 1}, nil
		},
	}
	s := New(nil, nil, nil, runner, discardLogger())
	m, err := s.ReadArchiveMetadata(context.Background(), "/backup.tar.gz")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m.ServerVersion != "35.21" {
		t.Fatalf("expected 35.21, got %s", m.ServerVersion)
	}
	if m.InstanceName != "test" {
		t.Fatalf("expected test, got %s", m.InstanceName)
	}
}

func TestEnsureTarWithinBase(t *testing.T) {
	base := t.TempDir()
	safe := []*tar.Header{{Name: "SavedArks/file.ark"}}
	if err := EnsureTarWithinBase(safe, base); err != nil {
		t.Fatalf("expected safe: %v", err)
	}

	escape := []*tar.Header{{Name: "../../etc/passwd"}}
	if err := EnsureTarWithinBase(escape, base); err == nil {
		t.Fatal("expected error for path traversal")
	}
}

func TestDirectoryForInstance(t *testing.T) {
	dir := t.TempDir()
	instanceDir := filepath.Join(dir, "asa1")
	os.MkdirAll(instanceDir, 0o755)

	got, err := DirectoryForInstance(dir, "asa1")
	if err != nil {
		t.Fatal(err)
	}
	if got != instanceDir {
		t.Fatalf("expected %q, got %q", instanceDir, got)
	}

	_, err = DirectoryForInstance(dir, "nonexistent")
	if err == nil {
		t.Fatal("expected error for missing directory")
	}
}

// --- Retention policy tests ---

func TestApplyRetentionPrunesByCount(t *testing.T) {
	dir := t.TempDir()
	for i := 1; i <= 5; i++ {
		name := fmt.Sprintf("asa1-backup-20260101-%06d.tar.gz", i)
		os.WriteFile(filepath.Join(dir, name), []byte("data"), 0o644)
	}

	policy := RetentionPolicy{RetainCount: 3}
	diskUsage := func(_ string) (int64, int64, error) { return 100e9, 200e9, nil }

	err := ApplyRetention(dir, "asa1", policy, diskUsage, discardLogger())
	if err != nil {
		t.Fatal(err)
	}

	entries, _ := os.ReadDir(dir)
	if len(entries) != 3 {
		names := []string{}
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Fatalf("expected 3 remaining, got %d: %v", len(entries), names)
	}

	// Newest 3 should remain
	for _, name := range []string{
		"asa1-backup-20260101-000005.tar.gz",
		"asa1-backup-20260101-000004.tar.gz",
		"asa1-backup-20260101-000003.tar.gz",
	} {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Errorf("expected %s to remain", name)
		}
	}
}

func TestDryRunRetentionDoesNotDelete(t *testing.T) {
	dir := t.TempDir()
	for i := 1; i <= 3; i++ {
		name := fmt.Sprintf("asa1-backup-20260101-%06d.tar.gz", i)
		os.WriteFile(filepath.Join(dir, name), []byte("data"), 0o644)
	}

	policy := RetentionPolicy{RetainCount: 1}
	diskUsage := func(_ string) (int64, int64, error) { return 100e9, 200e9, nil }

	results, err := DryRunRetention(dir, "asa1", policy, diskUsage, discardLogger())
	if err != nil {
		t.Fatal(err)
	}

	removeCount := 0
	for _, r := range results {
		if r.Remove {
			removeCount++
		}
	}
	// RetainCount=1, but minKeep=2 (normal free space), so only oldest 1 gets removed
	if removeCount != 1 {
		t.Fatalf("expected 1 to be marked for removal, got %d", removeCount)
	}

	// Files should still exist (dry run doesn't delete)
	entries, _ := os.ReadDir(dir)
	if len(entries) != 3 {
		t.Fatalf("expected all 3 files to still exist, got %d", len(entries))
	}
}

// --- BuildArtifactPaths tests ---

func TestBuildArtifactPaths(t *testing.T) {
	paths := BuildArtifactPaths("asa1", "/backups", "", "", "tar.gz", "20260101-120000", 42)

	if paths.DestDir != "/backups/asa1" {
		t.Errorf("DestDir = %q", paths.DestDir)
	}
	if paths.OutputPath != "/backups/asa1/asa1-backup-20260101-120000.tar.gz" {
		t.Errorf("OutputPath = %q", paths.OutputPath)
	}
	if paths.ContainerName != "asa1-backup-20260101-120000-42" {
		t.Errorf("ContainerName = %q", paths.ContainerName)
	}
}

func TestBuildArtifactPathsWithOverrides(t *testing.T) {
	paths := BuildArtifactPaths("asa1", "/backups", "/custom/dest", "my-backup", "tar.zst", "20260101-120000", 1)

	if paths.DestDir != "/custom/dest" {
		t.Errorf("DestDir = %q", paths.DestDir)
	}
	if !strings.HasSuffix(paths.OutputPath, "asa1-my-backup-20260101-120000.tar.zst") {
		t.Errorf("OutputPath = %q", paths.OutputPath)
	}
}

// --- BuildRestoreExecutionPaths tests ---

func TestBuildRestoreExecutionPaths(t *testing.T) {
	paths, err := BuildRestoreExecutionPaths("asa1", "/backups/asa1/asa1-backup.tar.gz", "/backups/asa1", "20260101-120000")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(paths.ContainerArchive, "asa1-restore-input-20260101-120000.tar.gz") {
		t.Errorf("ContainerArchive = %q", paths.ContainerArchive)
	}
	if !strings.Contains(paths.HostPreRestore, "asa1-pre-restore-20260101-120000.tar.gz") {
		t.Errorf("HostPreRestore = %q", paths.HostPreRestore)
	}
}

// --- Lock tests ---

func TestWithInstanceLockBasic(t *testing.T) {
	dir := t.TempDir()
	var called bool
	err := WithInstanceLock(dir, "asa1", time.Second, func() error {
		called = true
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if !called {
		t.Fatal("expected fn to be called")
	}
}

func TestWithInstanceLockPropagatesError(t *testing.T) {
	dir := t.TempDir()
	want := errors.New("inner error")
	err := WithInstanceLock(dir, "asa1", time.Second, func() error {
		return want
	})
	if !errors.Is(err, want) {
		t.Fatalf("expected inner error, got: %v", err)
	}
}

// --- EnsureServiceContainer tests ---

func TestEnsureServiceContainerExisting(t *testing.T) {
	compose := &arktest.MockCompose{
		AllServicesFn: func(_ context.Context, _ string) (map[string]struct{}, error) {
			return map[string]struct{}{"asa1": {}}, nil
		},
	}
	s := newTestService(compose, &arktest.MockEngine{})

	err := s.EnsureServiceContainer(context.Background(), "compose.yaml", "asa1")
	if err != nil {
		t.Fatal(err)
	}
}

func TestEnsureServiceContainerCreatesWhenMissing(t *testing.T) {
	var created bool
	compose := &arktest.MockCompose{
		AllServicesFn: func(_ context.Context, _ string) (map[string]struct{}, error) {
			return map[string]struct{}{}, nil
		},
		CreateFn: func(_ context.Context, _ string, _ []string) error {
			created = true
			return nil
		},
	}
	s := newTestService(compose, &arktest.MockEngine{})

	err := s.EnsureServiceContainer(context.Background(), "compose.yaml", "asa1")
	if err != nil {
		t.Fatal(err)
	}
	if !created {
		t.Fatal("expected Create to be called")
	}
}

// --- EnsureHeadroomForStart tests ---

func TestEnsureHeadroomSufficientSpace(t *testing.T) {
	dir := t.TempDir()
	diskUsage := func(_ string) (int64, int64, error) { return 100e9, 200e9, nil }

	err := EnsureHeadroomForStart(dir, []string{"asa1"}, diskUsage, discardLogger())
	if err != nil {
		t.Fatal(err)
	}
}

func TestEnsureHeadroomInsufficientWithNoBackups(t *testing.T) {
	dir := t.TempDir()
	diskUsage := func(_ string) (int64, int64, error) { return 500e6, 200e9, nil } // 500MB free

	err := EnsureHeadroomForStart(dir, []string{"asa1"}, diskUsage, discardLogger())
	if err == nil {
		t.Fatal("expected error for low disk with no backups")
	}
	if !strings.Contains(err.Error(), "insufficient disk space") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// --- mockRunner for cmdrunner.Runner ---

type mockRunner struct {
	RunFn func(ctx context.Context, cmd []string, opts cmdrunner.RunOptions) (cmdrunner.RunResult, error)
}

func (m *mockRunner) Run(ctx context.Context, cmd []string, opts cmdrunner.RunOptions) (cmdrunner.RunResult, error) {
	if m.RunFn != nil {
		return m.RunFn(ctx, cmd, opts)
	}
	return cmdrunner.RunResult{}, nil
}

func (m *mockRunner) RunInteractive(_ context.Context, _ []string) error {
	return nil
}

// Pre-restore, pre-update and --name archives sort by their own timestamp, so
// retention doesn't delete a fresh undo point ahead of older regular backups.
func TestListArchivesOrdersEveryArchiveByNameTimestamp(t *testing.T) {
	dir := t.TempDir()
	names := []string{
		"asa1-backup-20260103-000000.tar.gz",
		"asa1-pre-restore-20260105-000000.tar.gz",
		"asa1-pre-update-20260104-000000.tar.gz",
		"asa1-backup-20260101-000000.tar.gz",
		"asa1-mine-20260102-000000.tar.zst",
	}
	for _, n := range names {
		if err := os.WriteFile(filepath.Join(dir, n), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	got, err := ListArchives(dir)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{names[1], names[2], names[0], names[4], names[3]}
	for i := range want {
		if filepath.Base(got[i]) != want[i] {
			t.Fatalf("order = %v, want %v", got, want)
		}
	}
}
