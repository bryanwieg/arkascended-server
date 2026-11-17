package cmd

// Tests: test infrastructure helpers delegating to internal/testutil,
// plus cmd-specific helpers that don't have testutil equivalents.

import (
	"testing"

	"ark-cli/internal/core"
	"ark-cli/internal/testutil"

	"pkg/ark"
	"pkg/ark/runtime"

	"github.com/compose-spec/compose-go/v2/types"
)

// cmdConfig wires all services and resets all on patch, plus the
// cmd-specific WireDefaults seam.
var cmdConfig = testutil.TestDepsConfig{
	ExtraWireFuncs:      []func(*core.AppDeps){WireDefaults},
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

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	return testutil.CaptureStdout(t, fn)
}

func captureStderr(t *testing.T, fn func()) string {
	t.Helper()
	return testutil.CaptureStderr(t, fn)
}

func withPatchedAppDeps(t *testing.T, patch func(*core.AppDeps), fn func()) {
	t.Helper()
	testutil.WithPatchedAppDeps(t, cmdConfig, patch, fn)
}

// patchAppDepsForTest patches the current AppDeps for the lifetime of the test,
// restoring the previous value on cleanup. This is cmd-specific because
// some cmd tests use SetCurrentAppDeps instead of WithAppDeps.
func patchAppDepsForTest(t *testing.T, patch func(*core.AppDeps)) {
	t.Helper()
	prev := core.CurrentAppDeps()
	deps := testutil.CloneAppDepsForTest(cmdConfig)
	backupBefore := deps.BackupService
	updateBefore := deps.UpdateService
	if patch != nil {
		patch(deps)
	}
	if deps.BackupService == backupBefore {
		deps.BackupService = nil
	}
	if deps.UpdateService == updateBefore {
		deps.UpdateService = nil
	}
	deps.RuntimeHelper = runtime.RuntimeHelper{}
	testutil.FillTestDefaults(deps, cmdConfig)
	core.SetCurrentAppDeps(deps)
	t.Cleanup(func() {
		core.SetCurrentAppDeps(prev)
	})
}

func fakeProject(services types.Services) *ark.ComposeProject {
	return testutil.FakeProject(services)
}

func fakeProjectWithVolumes(services types.Services, volumes types.Volumes) *ark.ComposeProject {
	return testutil.FakeProjectWithVolumes(services, volumes)
}

func emptyServices(names ...string) types.Services {
	return testutil.EmptyServices(names...)
}

func strPtr(s string) *string { return testutil.StrPtr(s) }

func testEnv(pairs ...string) types.MappingWithEquals {
	return testutil.TestEnv(pairs...)
}

// testPorts and parseTestPortSpec are cmd-only helpers that delegate to testutil.
func testPorts(specs ...string) []types.ServicePortConfig {
	return testutil.TestPorts(specs...)
}

func parseTestPortSpec(spec string) types.ServicePortConfig {
	return testutil.ParseTestPortSpec(spec)
}

func checkTableRow(t *testing.T, line string, want []string) {
	t.Helper()
	testutil.CheckTableRow(t, line, want)
}

func createBackupArchive(t *testing.T, backupsDir, filename string) string {
	t.Helper()
	return testutil.CreateBackupArchive(t, backupsDir, filename)
}

func writePreRestoreArtifactFromRunOneOff(t *testing.T, command []string, opts ark.RunOptions) {
	t.Helper()
	testutil.WritePreRestoreArtifact(t, command, opts)
}

func loadComposeFromPathForTest(path string) (*types.Project, error) {
	return testutil.LoadComposeFromPathForTest(path)
}

func newMockHealthChecker() *testutil.MockHealthChecker {
	return testutil.NewMockHealthChecker()
}
