package show

// JSON output purity tests for show commands.
// See docs/dev/explanation/logging-architecture.md#json-output-contract.
// These tests guard the untested JSON output surface identified in the
// logging audit (Gap 8): verify each --json branch produces valid JSON,
// matches the documented struct shape, and does not mix text-mode
// output (table headers, YAML-ish detail lines) into the JSON stream.

import (
	"ark-cli/internal/core"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestListBackupsInstanceJSON(t *testing.T) {
	instance := "asa1"
	tmp := t.TempDir()
	backupRoot := filepath.Join(tmp, "backups")
	backupsDir := filepath.Join(backupRoot, instance)
	if err := os.MkdirAll(backupsDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	archive := filepath.Join(backupsDir, "asa1-backup-20260101-120000.tar.gz")
	if err := os.WriteFile(archive, []byte("payload"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	setEnv(t, "ARK_BACKUP_DIR", backupRoot)

	out := captureStdout(t, func() {
		if err := RunListBackups(core.CurrentAppDeps(), []string{"--instance", instance, "--json"}); err != nil {
			t.Fatalf("RunListBackups: %v", err)
		}
	})

	var parsed []core.BackupEntryJSON
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("stdout is not valid JSON: %v\nstdout:\n%s", err, out)
	}
	if len(parsed) != 1 {
		t.Fatalf("expected 1 entry; got %d\nstdout:\n%s", len(parsed), out)
	}
	if parsed[0].Name != filepath.Base(archive) {
		t.Errorf("entry[0].Name = %q; want %q", parsed[0].Name, filepath.Base(archive))
	}
	if parsed[0].SizeBytes != int64(len("payload")) {
		t.Errorf("entry[0].SizeBytes = %d; want %d", parsed[0].SizeBytes, len("payload"))
	}

	// Must not contain text-mode table headers.
	for _, forbidden := range []string{"NAME", "SIZE", "DATE"} {
		if strings.Contains(out, forbidden) {
			t.Errorf("JSON output contains text-mode header %q; channels crossed.\nstdout:\n%s", forbidden, out)
		}
	}
}

func TestListBackupsInstanceJSONEmpty(t *testing.T) {
	instance := "asa1"
	tmp := t.TempDir()
	backupRoot := filepath.Join(tmp, "backups")
	// listBackupsForInstance returns an error when no archives exist; we
	// confirm the error path does not accidentally emit partial JSON.
	if err := os.MkdirAll(filepath.Join(backupRoot, instance), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	setEnv(t, "ARK_BACKUP_DIR", backupRoot)

	out := captureStdout(t, func() {
		// Expect an error; the handler should return it without printing.
		_ = RunListBackups(core.CurrentAppDeps(), []string{"--instance", instance, "--json"})
	})
	if strings.TrimSpace(out) != "" {
		t.Errorf("error path emitted output to stdout; expected empty.\nstdout:\n%s", out)
	}
}

func TestShowBackupJSON(t *testing.T) {
	instance := "asa1"
	tmp := t.TempDir()
	backupRoot := filepath.Join(tmp, "backups")
	backupsDir := filepath.Join(backupRoot, instance)
	if err := os.MkdirAll(backupsDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	archiveName := "asa1-backup-20260101-120000.tar.gz"
	archive := filepath.Join(backupsDir, archiveName)
	payload := []byte("payload-bytes")
	if err := os.WriteFile(archive, payload, 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	setEnv(t, "ARK_BACKUP_DIR", backupRoot)

	out := captureStdout(t, func() {
		if err := RunShowBackup(core.CurrentAppDeps(), []string{"--instance", instance, "--backup", archiveName, "--json"}); err != nil {
			t.Fatalf("RunShowBackup: %v", err)
		}
	})

	var parsed core.BackupDetailJSON
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("stdout is not valid JSON: %v\nstdout:\n%s", err, out)
	}
	if parsed.Instance != instance {
		t.Errorf("Instance = %q; want %q", parsed.Instance, instance)
	}
	if parsed.Name != archiveName {
		t.Errorf("Name = %q; want %q", parsed.Name, archiveName)
	}
	if parsed.SizeBytes != int64(len(payload)) {
		t.Errorf("SizeBytes = %d; want %d", parsed.SizeBytes, len(payload))
	}
	if parsed.Path == "" {
		t.Errorf("Path is empty; want the archive path")
	}

	// Must not contain the text-mode YAML-ish detail lines.
	for _, forbidden := range []string{"instance: " + instance, "name: " + archiveName, "size_bytes:"} {
		if strings.Contains(out, forbidden) {
			t.Errorf("JSON output contains text-mode detail line %q; channels crossed.\nstdout:\n%s", forbidden, out)
		}
	}
}
