package show

// Test helpers for the show package.
// Thin wrappers delegating to internal/testutil.

import (
	"testing"

	"ark-cli/internal/core"
	"ark-cli/internal/testutil"

	"pkg/ark"

	"github.com/compose-spec/compose-go/v2/types"
)

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
	testutil.WithPatchedAppDeps(t, testutil.ShowConfig, patch, fn)
}

func fakeProject(services types.Services) *ark.ComposeProject {
	return testutil.FakeProject(services)
}

func fakeProjectWithVolumes(services types.Services, volumes types.Volumes) *ark.ComposeProject {
	return testutil.FakeProjectWithVolumes(services, volumes)
}

func testPorts(specs ...string) []types.ServicePortConfig {
	return testutil.TestPorts(specs...)
}

func checkTableRow(t *testing.T, line string, want []string) {
	t.Helper()
	testutil.CheckTableRow(t, line, want)
}

func testEnv(pairs ...string) types.MappingWithEquals {
	return testutil.TestEnv(pairs...)
}
