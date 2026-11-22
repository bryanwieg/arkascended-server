package backup

// Tests: restore into a new instance takes its passwords from the flags, else from the backup's INI.

import (
	"archive/tar"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf16"

	"ark-cli/internal/core"

	"pkg/ark"
	"pkg/ark/arktest"

	"github.com/compose-spec/compose-go/v2/types"
)

// createArchiveWithINI writes a backup tar holding a map directory and, if ini is non-empty,
// Config/WindowsServer/GameUserSettings.ini with that content.
func createArchiveWithINI(t *testing.T, dir string, ini []byte) {
	t.Helper()
	f, err := os.Create(filepath.Join(dir, "src-backup-20240102-120000.tar"))
	if err != nil {
		t.Fatalf("create archive: %v", err)
	}
	defer f.Close()
	tw := tar.NewWriter(f)
	if err := tw.WriteHeader(&tar.Header{Typeflag: tar.TypeDir, Name: "./SavedArks/TheIsland_WP/", Mode: 0o755}); err != nil {
		t.Fatalf("write dir: %v", err)
	}
	if len(ini) > 0 {
		name := "./Config/WindowsServer/GameUserSettings.ini"
		if err := tw.WriteHeader(&tar.Header{Typeflag: tar.TypeReg, Name: name, Mode: 0o644, Size: int64(len(ini))}); err != nil {
			t.Fatalf("write ini header: %v", err)
		}
		if _, err := tw.Write(ini); err != nil {
			t.Fatalf("write ini: %v", err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("close tar: %v", err)
	}
}

func utf16LE(s string) []byte {
	out := []byte{0xFF, 0xFE}
	for _, u := range utf16.Encode([]rune(s)) {
		out = append(out, byte(u), byte(u>>8))
	}
	return out
}

func TestRunRestoreNewInstancePasswords(t *testing.T) {
	const ini = "[ServerSettings]\r\nRCONPort=27020\r\nServerAdminPassword=adminFromBackup\r\nServerPassword=joinFromBackup\r\n"
	tests := []struct {
		name        string
		ini         []byte
		extra       []string
		wantErr     string
		wantAdmin   string
		wantServer  string
		absentInEnv string
	}{
		{name: "from backup", ini: []byte(ini), wantAdmin: "adminFromBackup", wantServer: "joinFromBackup"},
		{name: "from UTF-16 backup", ini: utf16LE(ini), wantAdmin: "adminFromBackup", wantServer: "joinFromBackup"},
		{
			name: "flags override backup", ini: []byte(ini),
			extra:     []string{"--admin-password", "newAdmin", "--no-server-password"},
			wantAdmin: "newAdmin", absentInEnv: "joinFromBackup",
		},
		{name: "no INI and no flag", wantErr: "--admin-password"},
		{name: "flag without INI", extra: []string{"--admin-password", "newAdmin"}, wantAdmin: "newAdmin"},
		{name: "invalid backup password", ini: []byte("ServerAdminPassword=bad pw!\n"), wantErr: "--admin-password"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmp := t.TempDir()
			backupRoot := filepath.Join(tmp, "backups")
			backupsDir := filepath.Join(backupRoot, "src")
			if err := os.MkdirAll(backupsDir, 0o755); err != nil {
				t.Fatalf("mkdir: %v", err)
			}
			createArchiveWithINI(t, backupsDir, tt.ini)
			composeFile := filepath.Join(tmp, "compose.yaml")
			if err := os.WriteFile(composeFile, nil, 0o644); err != nil {
				t.Fatalf("write compose: %v", err)
			}
			setEnv(t, "ARK_BACKUP_DIR", backupRoot)
			setEnv(t, "COMPOSE_FILE", composeFile)

			withPatchedAppDeps(t, func(d *core.AppDeps) {
				patchRestoreBaseDeps(d, "new1", types.ServiceConfig{})
				d.ComposeService.(*arktest.MockCompose).RunOneOffFn = func(_ context.Context, _, _ string, command []string, opts ark.RunOptions) (ark.ExecResult, error) {
					writePreRestoreArtifactFromRunOneOff(t, command, opts)
					return ark.ExecResult{}, nil
				}
			}, func() {
				err := RunRestore(core.CurrentAppDeps(), restoreNewFlags("new1", "src", "latest", tt.extra...))
				written, readErr := os.ReadFile(composeFile)
				if readErr != nil {
					t.Fatalf("read compose: %v", readErr)
				}
				if tt.wantErr != "" {
					if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
						t.Fatalf("expected error mentioning %q, got %v", tt.wantErr, err)
					}
					if len(written) != 0 {
						t.Fatalf("compose file must stay untouched on error, got:\n%s", written)
					}
					return
				}
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				text := string(written)
				if !strings.Contains(text, tt.wantAdmin) || !strings.Contains(text, tt.wantServer) {
					t.Fatalf("compose lacks admin %q / server %q:\n%s", tt.wantAdmin, tt.wantServer, text)
				}
				if tt.absentInEnv != "" && strings.Contains(text, tt.absentInEnv) {
					t.Fatalf("compose must not contain %q:\n%s", tt.absentInEnv, text)
				}
			})
		})
	}
}

func TestRunRestoreRejectsPasswordFlagsForExistingInstance(t *testing.T) {
	withPatchedAppDeps(t, nil, func() {
		err := RunRestore(core.CurrentAppDeps(), restoreFlags("asa1", "asa1", "latest", "--admin-password", "pw"))
		if err == nil || !strings.Contains(err.Error(), "--new-instance") {
			t.Fatalf("expected --new-instance error, got %v", err)
		}
	})
}

// TestRestoreNewInstanceRejectsSchedulerName verifies a backup cannot be restored as ark-scheduler.
func TestRestoreNewInstanceRejectsSchedulerName(t *testing.T) {
	composeFile := filepath.Join(t.TempDir(), "compose.yaml")
	if err := os.WriteFile(composeFile, nil, 0o644); err != nil {
		t.Fatalf("write compose: %v", err)
	}
	err := scaffoldComposeService(composeFile, ark.SchedulerService, "TheIsland_WP", "adminpass", "")
	if err == nil || err.Error() != "Error: instance name ark-scheduler is reserved for the scheduler service." {
		t.Fatalf("got %v", err)
	}
	if written, _ := os.ReadFile(composeFile); len(written) != 0 {
		t.Fatalf("compose file must stay untouched, got:\n%s", written)
	}
}
