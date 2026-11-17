package show

// Tests: list backups command: sorted output, missing directory, no-archives case, mtime sort-key fallback

import (
	"ark-cli/internal/core"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"pkg/ark/backup"
)

// TestListBackupsOutputsSorted verifies backup listing is ordered newest-first.
// - Expect: output names are sorted by timestamp descending.
func TestListBackupsOutputsSorted(t *testing.T) {
	instance := "asa-server-1"
	tmp := t.TempDir()
	backupRoot := filepath.Join(tmp, "backups")
	backupsDir := filepath.Join(backupRoot, instance)
	if err := os.MkdirAll(backupsDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	newer := filepath.Join(backupsDir, "asa-server-1-backup-20240102-120000.tar.gz")
	older := filepath.Join(backupsDir, "asa-server-1-backup-20240101-120000.tar.gz")
	if err := os.WriteFile(older, []byte("data"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := os.WriteFile(newer, []byte("data"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	setEnv(t, "ARK_BACKUP_DIR", backupRoot)

	out := captureStdout(t, func() {
		if err := RunListBackups(core.CurrentAppDeps(), []string{"--instance", instance}); err != nil {
			t.Fatalf("runListBackups: %v", err)
		}
	})
	newerBase := filepath.Base(newer)
	olderBase := filepath.Base(older)
	newerIdx := strings.Index(out, newerBase)
	olderIdx := strings.Index(out, olderBase)
	if newerIdx == -1 || olderIdx == -1 {
		t.Fatalf("expected both archives in output:\n%s", out)
	}
	if newerIdx > olderIdx {
		t.Fatalf("expected newer archive first, got:\n%s", out)
	}
}

// TestListBackupsMissingDirectory checks missing backup directory handling.
// - Expect: runListBackups returns an error when the instance directory is absent.
func TestListBackupsMissingDirectory(t *testing.T) {
	backupRoot := filepath.Join(t.TempDir(), "backups")
	setEnv(t, "ARK_BACKUP_DIR", backupRoot)
	if err := RunListBackups(core.CurrentAppDeps(), []string{"--instance", "missing"}); err == nil {
		t.Fatal("expected error")
	}
}

// TestListBackupsNoArchives checks empty backup directory handling.
// - Expect: runListBackups returns an error when no archives exist.
func TestListBackupsNoArchives(t *testing.T) {
	instance := "asa-server-2"
	backupRoot := filepath.Join(t.TempDir(), "backups")
	backupsDir := filepath.Join(backupRoot, instance)
	if err := os.MkdirAll(backupsDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	setEnv(t, "ARK_BACKUP_DIR", backupRoot)
	if err := RunListBackups(core.CurrentAppDeps(), []string{"--instance", instance}); err == nil {
		t.Fatal("expected error")
	}
}

// TestListBackupSortKeyFallsBackToMtime verifies sorting falls back to mtime without timestamps.
// - Expect: newer mtime sorts ahead when filenames lack valid timestamps.
func TestListBackupSortKeyFallsBackToMtime(t *testing.T) {
	tmp := t.TempDir()
	newer := filepath.Join(tmp, "asa-backup-20240101-120000x.tar")
	older := filepath.Join(tmp, "asa-backup-20240101-120000y.tar")
	if err := os.WriteFile(newer, []byte("data"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := os.WriteFile(older, []byte("data"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := os.Chtimes(older, time.Unix(1, 0), time.Unix(1, 0)); err != nil {
		t.Fatalf("chtimes: %v", err)
	}
	if err := os.Chtimes(newer, time.Unix(2, 0), time.Unix(2, 0)); err != nil {
		t.Fatalf("chtimes: %v", err)
	}
	files := []string{older, newer}
	sort.Slice(files, func(i, j int) bool {
		left, _ := os.Stat(files[i])
		right, _ := os.Stat(files[j])
		return backup.CompareArchiveSort(files[i], left, files[j], right)
	})
	if files[0] != newer {
		t.Fatalf("expected %s first, got %s", newer, files[0])
	}
}
