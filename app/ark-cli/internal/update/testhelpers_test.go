package update

// Test helpers for the update package.
// Thin wrappers delegating to internal/testutil.

import (
	"context"
	"errors"
	"testing"

	"ark-cli/internal/core"
	"ark-cli/internal/testutil"

	"pkg/ark"
	"pkg/ark/update"

	"github.com/compose-spec/compose-go/v2/types"
)

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	return testutil.CaptureStdout(t, fn)
}

func withPatchedAppDeps(t *testing.T, patch func(*core.AppDeps), fn func()) {
	t.Helper()
	// Offline unless a test sets its own: local builds look up Steam and Docker Hub.
	offline := func(d *core.AppDeps) {
		d.HostDeps.FetchText = func(string) (string, error) { return "", errors.New("offline in tests") }
		patch(d)
	}
	testutil.WithPatchedAppDeps(t, testutil.UpdateOnlyConfig, offline, fn)
}

func fakeProject(services types.Services) *ark.ComposeProject {
	return testutil.FakeProject(services)
}

func emptyServices(names ...string) types.Services {
	return testutil.EmptyServices(names...)
}

// fakeBackups makes rollouts skip the real backup and restore (no containers in
// unit tests); restored records the archives restored.
func fakeBackups(t *testing.T) (restored *[]string) {
	t.Helper()
	orig := newRolloutSteps
	t.Cleanup(func() { newRolloutSteps = orig })
	restored = &[]string{}
	newRolloutSteps = func(d *core.AppDeps, composeFile string) update.RolloutSteps {
		steps := orig(d, composeFile)
		steps.Backup = func(context.Context, string) (string, error) { return "/backups/pre-update.tar.gz", nil }
		steps.Restore = func(_ context.Context, _, archive string) error {
			*restored = append(*restored, archive)
			return nil
		}
		return steps
	}
	return restored
}
