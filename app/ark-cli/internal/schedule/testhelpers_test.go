package schedule

// Test helpers for the schedule package.
// Thin wrappers delegating to internal/testutil, plus schedule-specific helpers.

import (
	"testing"

	"ark-cli/internal/core"
	"ark-cli/internal/testutil"

	"pkg/ark"
	"pkg/ark/runtime"

	"github.com/compose-spec/compose-go/v2/types"
)

// scheduleConfig wires all services and resets all on patch, plus the
// schedule-specific WireDefaults seam.
var scheduleConfig = testutil.TestDepsConfig{
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

func withPatchedAppDeps(t *testing.T, patch func(*core.AppDeps), fn func()) {
	t.Helper()
	testutil.WithPatchedAppDeps(t, scheduleConfig, patch, fn)
}

func fakeProject(services types.Services) *ark.ComposeProject {
	return testutil.FakeProject(services)
}

func emptyServices(names ...string) types.Services {
	return testutil.EmptyServices(names...)
}

// patchAppDepsForTest patches the current AppDeps for the lifetime of the test,
// restoring the previous value on cleanup. This is schedule-specific because
// some schedule tests use SetCurrentAppDeps instead of WithAppDeps.
func patchAppDepsForTest(t *testing.T, patch func(*core.AppDeps)) {
	t.Helper()
	prev := core.CurrentAppDeps()
	deps := testutil.CloneAppDepsForTest(scheduleConfig)
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
	testutil.FillTestDefaults(deps, scheduleConfig)
	core.SetCurrentAppDeps(deps)
	t.Cleanup(func() {
		core.SetCurrentAppDeps(prev)
	})
}
