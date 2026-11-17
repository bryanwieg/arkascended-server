package backup

// Test helpers for the backup package.
// Thin wrappers delegating to internal/testutil, plus package-specific helpers.

import (
	"archive/tar"
	"os"
	"path/filepath"
	"testing"

	"ark-cli/internal/core"
	"ark-cli/internal/testutil"

	"pkg/ark"

	"github.com/compose-spec/compose-go/v2/types"
)

// backupConfig wires all services and resets all on patch, plus the
// backup-specific RunBackupFromSchedule seam.
var backupConfig = testutil.TestDepsConfig{
	ExtraWireFuncs: []func(*core.AppDeps){
		func(d *core.AppDeps) { d.RunBackupFromSchedule = RunBackupFromScheduleImpl },
	},
	WireBackupService:   true,
	WireUpdateService:   true,
	WireScheduleService: true,
	ResetBackupOnPatch:  true,
	ResetUpdateOnPatch:  true,
}

func setEnv(t *testing.T, key, value string) {
	t.Helper()
	testutil.SetEnv(t, key, value)
}

func withPatchedAppDeps(t *testing.T, patch func(*core.AppDeps), fn func()) {
	t.Helper()
	testutil.WithPatchedAppDeps(t, backupConfig, patch, fn)
}

func fakeProject(services types.Services) *ark.ComposeProject {
	return testutil.FakeProject(services)
}

func testEnv(pairs ...string) types.MappingWithEquals {
	return testutil.TestEnv(pairs...)
}

func createBackupArchive(t *testing.T, backupsDir, filename string) string {
	t.Helper()
	return testutil.CreateBackupArchive(t, backupsDir, filename)
}

func writePreRestoreArtifactFromRunOneOff(t *testing.T, command []string, opts ark.RunOptions) {
	t.Helper()
	testutil.WritePreRestoreArtifact(t, command, opts)
}

// createTarArchiveWithMap creates a tar archive containing a SavedArks/<mapName>/
// directory entry. This is package-specific because backup tests need to
// simulate backup archives with specific map directories.
func createTarArchiveWithMap(t *testing.T, dir, filename, mapName string) string {
	t.Helper()
	archivePath := filepath.Join(dir, filename)
	f, err := os.Create(archivePath)
	if err != nil {
		t.Fatalf("create archive: %v", err)
	}
	defer f.Close()
	tw := tar.NewWriter(f)
	if err := tw.WriteHeader(&tar.Header{
		Typeflag: tar.TypeDir,
		Name:     "SavedArks/" + mapName + "/",
		Mode:     0o755,
	}); err != nil {
		t.Fatalf("write tar header: %v", err)
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("close tar: %v", err)
	}
	return archivePath
}
