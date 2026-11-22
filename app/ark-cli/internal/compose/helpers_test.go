package compose

import (
	"bytes"
	"io"
	"os"
	"testing"

	"ark-cli/internal/core"

	"pkg/ark"
	"pkg/ark/runtime"

	"github.com/compose-spec/compose-go/v2/types"
)

// NOTE: This file contains local duplicates of testutil helpers (captureStdout,
// fakeProject, emptyServices) because the compose package cannot import testutil
// — testutil/deps.go imports compose, creating an import cycle. The other
// internal packages use testutil directly; compose is the sole exception.
// Mirror of testutil.CaptureStdout.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stdout = w
	defer func() { os.Stdout = old }()
	defer func() {
		if recovered := recover(); recovered != nil {
			_ = w.Close()
			panic(recovered)
		}
	}()

	fn()
	_ = w.Close()

	data, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("read stdout: %v", err)
	}
	return string(bytes.TrimSpace(data))
}

// defaultAppDepsForTest creates a *core.AppDeps with real production defaults
// plus compose-specific seams, mirroring what app.go does via DefaultAppDeps + WireDefaults.
func defaultAppDepsForTest() *core.AppDeps {
	d := core.DefaultAppDeps()
	WireDefaults(d)
	return d
}

// fillTestDefaults fills in derived fields on deps, delegating to core.FillDefaults.
func fillTestDefaults(d *core.AppDeps) {
	core.FillDefaults(d)
}

func cloneAppDepsForTest() *core.AppDeps {
	current := core.CurrentAppDeps()
	base := defaultAppDepsForTest()
	if current != nil {
		core.MergeNonZeroAppDepFields(base, current)
	}
	fillTestDefaults(base)
	return base
}

func withPatchedAppDeps(t *testing.T, patch func(*core.AppDeps), fn func()) {
	t.Helper()
	deps := cloneAppDepsForTest()
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
	fillTestDefaults(deps)
	core.WithAppDeps(deps, fn)
}

// Mirror of testutil.FakeProject — kept local due to import cycle (see note above).
func fakeProject(services types.Services) *ark.ComposeProject {
	return &ark.ComposeProject{
		Project: &types.Project{
			Services: services,
		},
	}
}

// Mirror of testutil.EmptyServices — kept local due to import cycle (see note above).
func emptyServices(names ...string) types.Services {
	svcs := types.Services{}
	for _, name := range names {
		svcs[name] = types.ServiceConfig{Name: name, Labels: types.Labels{ark.PortASALabel: "27015"}}
	}
	return svcs
}
