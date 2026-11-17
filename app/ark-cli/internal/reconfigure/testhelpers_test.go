package reconfigure

// Test helpers for the reconfigure package.
// Thin wrappers delegating to internal/testutil.

import (
	"testing"

	"ark-cli/internal/core"
	"ark-cli/internal/testutil"
)

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	return testutil.CaptureStdout(t, fn)
}

func withPatchedAppDeps(t *testing.T, patch func(*core.AppDeps), fn func()) {
	t.Helper()
	testutil.WithPatchedAppDeps(t, testutil.ComposeOnlyConfig, patch, fn)
}
