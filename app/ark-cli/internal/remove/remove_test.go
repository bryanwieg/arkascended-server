package remove

// Tests: remove instance command: backup skip flag, running-instance protection, force stop, all-instances flag conflict

import (
	"ark-cli/internal/core"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"pkg/ark"
	"pkg/ark/arktest"
)

func writeCompose(path, instance string) error {
	text := "services:\n" +
		"  " + instance + ":\n" +
		"    image: ark-server:latest\n" +
		"volumes:\n" +
		"  " + instance + "-saved:\n"
	return os.WriteFile(path, []byte(text), 0o644)
}

// TestRemoveSkipsBackupWhenFlagged verifies --skip-backup bypasses backup but still removes the service.
// - Expect: service and volume entries are removed from compose without running backup.
func TestRemoveSkipsBackupWhenFlagged(t *testing.T) {
	instance := "asa1"
	tmp := t.TempDir()
	composeFile := filepath.Join(tmp, "compose.yaml")
	if err := writeCompose(composeFile, instance); err != nil {
		t.Fatalf("write: %v", err)
	}
	setEnv(t, "COMPOSE_FILE", composeFile)
	withPatchedAppDeps(t, func(d *core.AppDeps) {
		d.ComposeService = &arktest.MockCompose{
			LoadProjectFn: func(_ context.Context, _ string) (*ark.ComposeProject, error) {
				return fakeProject(emptyServices(instance)), nil
			},
			RunningServicesFn: func(_ context.Context, _ string) (map[string]struct{}, error) {
				return map[string]struct{}{}, nil
			},
		}
		d.RemoveVolume = func(_, _ string) error { return nil }
	}, func() {
		if err := RunRemove(core.CurrentAppDeps(), []string{"--instance", instance, "--skip-backup"}); err != nil {
			t.Fatalf("runRemove: %v", err)
		}
	})

	updated, err := os.ReadFile(composeFile)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if strings.Contains(string(updated), "asa1:") {
		t.Fatal("expected service removed")
	}
	if strings.Contains(string(updated), "asa1-saved:") {
		t.Fatal("expected volume removed")
	}
}

// TestRemoveRunningRequiresForce checks that running services require --kill.
// - Expect: removal fails with an error when the service is running.
func TestRemoveRunningRequiresForce(t *testing.T) {
	instance := "asa1"
	tmp := t.TempDir()
	composeFile := filepath.Join(tmp, "compose.yaml")
	if err := writeCompose(composeFile, instance); err != nil {
		t.Fatalf("write: %v", err)
	}
	setEnv(t, "COMPOSE_FILE", composeFile)
	withPatchedAppDeps(t, func(d *core.AppDeps) {
		d.ComposeService = &arktest.MockCompose{
			LoadProjectFn: func(_ context.Context, _ string) (*ark.ComposeProject, error) {
				return fakeProject(emptyServices(instance)), nil
			},
			RunningServicesFn: func(_ context.Context, _ string) (map[string]struct{}, error) {
				return map[string]struct{}{instance: {}}, nil
			},
		}
		d.RemoveVolume = func(_, _ string) error { return nil }
	}, func() {
		if err := RunRemove(core.CurrentAppDeps(), []string{"--instance", instance, "--skip-backup"}); err == nil {
			t.Fatal("expected error")
		}
	})
}

// TestRemoveKillCallsImmediateStop confirms --kill triggers compose kill.
// - Expect: compose kill is called for the target instance before removal.
func TestRemoveKillCallsImmediateStop(t *testing.T) {
	instance := "asa1"
	tmp := t.TempDir()
	composeFile := filepath.Join(tmp, "compose.yaml")
	if err := writeCompose(composeFile, instance); err != nil {
		t.Fatalf("write: %v", err)
	}
	setEnv(t, "COMPOSE_FILE", composeFile)
	var killed bool
	withPatchedAppDeps(t, func(d *core.AppDeps) {
		d.ComposeService = &arktest.MockCompose{
			LoadProjectFn: func(_ context.Context, _ string) (*ark.ComposeProject, error) {
				return fakeProject(emptyServices(instance)), nil
			},
			RunningServicesFn: func(_ context.Context, _ string) (map[string]struct{}, error) {
				return map[string]struct{}{instance: {}}, nil
			},
			KillFn: func(_ context.Context, _ string, _ []string, _ string) error {
				killed = true
				return nil
			},
		}
		d.RemoveVolume = func(_, _ string) error { return nil }
	}, func() {
		if err := RunRemove(core.CurrentAppDeps(), []string{"--instance", instance, "--skip-backup", "--kill"}); err != nil {
			t.Fatalf("runRemove: %v", err)
		}
	})
	if !killed {
		t.Fatal("expected compose Kill to be called")
	}
}

// TestRemoveReturnsStatusCheckError ensures compose status read failures are not
// silently treated as "not running".
func TestRemoveReturnsStatusCheckError(t *testing.T) {
	instance := "asa1"
	tmp := t.TempDir()
	composeFile := filepath.Join(tmp, "compose.yaml")
	if err := writeCompose(composeFile, instance); err != nil {
		t.Fatalf("write: %v", err)
	}
	setEnv(t, "COMPOSE_FILE", composeFile)
	withPatchedAppDeps(t, func(d *core.AppDeps) {
		d.ComposeService = &arktest.MockCompose{
			LoadProjectFn: func(_ context.Context, _ string) (*ark.ComposeProject, error) {
				return fakeProject(emptyServices(instance)), nil
			},
			RunningServicesFn: func(_ context.Context, _ string) (map[string]struct{}, error) {
				return nil, errors.New("compose status failed")
			},
		}
	}, func() {
		if err := RunRemove(core.CurrentAppDeps(), []string{"--instance", instance, "--skip-backup"}); err == nil {
			t.Fatal("expected error")
		}
	})
}

func TestRemoveAllReturnsTeardownHint(t *testing.T) {
	err := RunRemove(core.CurrentAppDeps(), []string{"--instance", "all"})
	if err == nil {
		t.Fatal("expected error for remove all")
	}
	if !strings.Contains(err.Error(), "teardown instances") {
		t.Fatalf("expected teardown hint, got: %v", err)
	}
}

func TestRunTeardownSkipConfirmCallsDown(t *testing.T) {
	instance := "asa1"
	tmp := t.TempDir()
	composeFile := filepath.Join(tmp, "compose.yaml")
	if err := writeCompose(composeFile, instance); err != nil {
		t.Fatalf("write: %v", err)
	}
	setEnv(t, "COMPOSE_FILE", composeFile)
	var downCalled bool
	withPatchedAppDeps(t, func(d *core.AppDeps) {
		d.ComposeService = &arktest.MockCompose{
			LoadProjectFn: func(_ context.Context, _ string) (*ark.ComposeProject, error) {
				return fakeProject(emptyServices(instance)), nil
			},
			RunningServicesFn: func(_ context.Context, _ string) (map[string]struct{}, error) {
				return map[string]struct{}{}, nil
			},
			DownFn: func(_ context.Context, composeFilePath string) error {
				downCalled = true
				if composeFilePath != composeFile {
					t.Fatalf("expected Down called with %s, got %s", composeFile, composeFilePath)
				}
				return nil
			},
		}
	}, func() {
		if err := RunTeardown(core.CurrentAppDeps(), []string{"--skip-confirm"}); err != nil {
			t.Fatalf("runTeardown: %v", err)
		}
	})
	if !downCalled {
		t.Fatal("expected compose Down to be called")
	}
}

func TestRunTeardownNonTTYRequiresSkipConfirm(t *testing.T) {
	orig := core.IsInteractiveTerminal
	core.IsInteractiveTerminal = func() bool { return false }
	t.Cleanup(func() { core.IsInteractiveTerminal = orig })
	withPatchedAppDeps(t, nil, func() {
		err := RunTeardown(core.CurrentAppDeps(), []string{})
		if err == nil {
			t.Fatal("expected error without --skip-confirm")
		}
		if !strings.Contains(err.Error(), "skip-confirm") {
			t.Fatalf("expected skip-confirm error, got: %v", err)
		}
	})
}

func TestRunTeardownMissingComposeFile(t *testing.T) {
	tmp := t.TempDir()
	setEnv(t, "COMPOSE_FILE", filepath.Join(tmp, "nonexistent.yaml"))
	withPatchedAppDeps(t, nil, func() {
		err := RunTeardown(core.CurrentAppDeps(), []string{"--skip-confirm"})
		if err == nil {
			t.Fatal("expected error for missing compose file")
		}
	})
}
