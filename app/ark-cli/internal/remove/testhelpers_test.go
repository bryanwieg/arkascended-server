package remove

// Test helpers for the remove package.
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

func withPatchedAppDeps(t *testing.T, patch func(*core.AppDeps), fn func()) {
	t.Helper()
	testutil.WithPatchedAppDeps(t, testutil.ShowConfig, patch, fn)
}

func fakeProject(services types.Services) *ark.ComposeProject {
	return testutil.FakeProject(services)
}

func emptyServices(names ...string) types.Services {
	return testutil.EmptyServices(names...)
}
