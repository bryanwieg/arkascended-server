package main

// Tests: utility functions: RCON env args, compose base command, tar path traversal safety, source path resolution

import (
	"archive/tar"
	cmdbackup "ark-cli/internal/backup"
	"ark-cli/internal/compose"
	"ark-cli/internal/core"
	"context"
	"os"
	"path/filepath"
	"testing"

	"pkg/ark/arktest"
	backup "pkg/ark/backup"
)

// TestRconEnvArgsEmpty verifies rconEnvArgs returns no flags when env vars are unset.
// - Expect: an empty argument slice.
func TestRconEnvArgsEmpty(t *testing.T) {
	setEnv(t, "RCON_HOST", "")
	setEnv(t, "RCON_PORT", "")
	setEnv(t, "RCON_PATH", "")
	if args := compose.RconEnvArgsImpl(); len(args) != 0 {
		t.Fatalf("expected empty args, got %v", args)
	}
}

// TestRconEnvArgsValues verifies rconEnvArgs includes RCON env vars in order.
// - Expect: args contain RCON_HOST and RCON_PORT; RCON_PATH (the removed rcon-cli path) is not forwarded.
func TestRconEnvArgsValues(t *testing.T) {
	setEnv(t, "RCON_HOST", "127.0.0.1")
	setEnv(t, "RCON_PORT", "27020")
	setEnv(t, "RCON_PATH", "/rcon")
	args := compose.RconEnvArgsImpl()
	expected := []string{
		"-e", "RCON_HOST=127.0.0.1",
		"-e", "RCON_PORT=27020",
	}
	if len(args) != len(expected) {
		t.Fatalf("unexpected args: %v", args)
	}
	for i, value := range expected {
		if args[i] != value {
			t.Fatalf("unexpected args: %v", args)
		}
	}
}

// TestRequireComposeMissing verifies missing compose files are rejected.
// - Expect: requireCompose returns an error when the file does not exist.
func TestRequireComposeMissing(t *testing.T) {
	tmp := t.TempDir()
	if err := compose.RequireComposeFile(filepath.Join(tmp, "missing.yaml")); err == nil {
		t.Fatal("expected error")
	}
}

// TestEnsureTarWithinBaseRejectsEscape checks tar path traversal protection.
// - Expect: an error when a tar header escapes the target base.
func TestEnsureTarWithinBaseRejectsEscape(t *testing.T) {
	member := &tar.Header{Name: "../evil"}
	if err := backup.EnsureTarWithinBase([]*tar.Header{member}, t.TempDir()); err == nil {
		t.Fatal("expected error")
	}
}

// TestEnsureNoOverwriteRejectsExisting ensures restore refuses to overwrite existing paths.
// - Expect: an error when a target path already exists.
func TestEnsureNoOverwriteRejectsExisting(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "existing.txt")
	if err := os.WriteFile(path, []byte("data"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	member := &tar.Header{Name: "existing.txt"}
	if err := backup.EnsureNoOverwrite([]*tar.Header{member}, tmp); err == nil {
		t.Fatal("expected error")
	}
}

// TestResolveSourcePathsRepoRelative verifies repo-relative paths resolve inside the repo.
// - Expect: Abs points to the repo path and Rel remains repo-relative.
func TestResolveSourcePathsRepoRelative(t *testing.T) {
	cwd, _ := os.Getwd()
	tmp := t.TempDir()
	if err := os.Chdir(tmp); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	defer func() { _ = os.Chdir(cwd) }()

	path := filepath.Join(tmp, "data")
	if err := os.Mkdir(path, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	withPatchedAppDeps(t, func(d *core.AppDeps) {
		d.Engine = &arktest.MockEngine{
			VolumeMountpointFn: func(_ context.Context, _ string) (string, bool) { return "", false },
		}
	}, func() {
		resolved, err := cmdbackup.ResolveSourcePaths(core.CurrentAppDeps(), []string{"data"})
		if err != nil {
			t.Fatalf("resolve: %v", err)
		}
		if len(resolved) != 1 {
			t.Fatalf("expected 1 path")
		}
		if resolved[0].Abs != path || resolved[0].Rel != "data" {
			t.Fatalf("unexpected resolved: %v", resolved)
		}
	})
}

// TestResolveSourcePathsRejectsOutsideRepo checks that outside paths are rejected.
// - Expect: resolveSourcePaths returns an error for paths outside the repo root.
func TestResolveSourcePathsRejectsOutsideRepo(t *testing.T) {
	cwd, _ := os.Getwd()
	tmp := t.TempDir()
	if err := os.Chdir(tmp); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	defer func() { _ = os.Chdir(cwd) }()

	outside := filepath.Join(filepath.Dir(tmp), "outside-file")
	if err := os.WriteFile(outside, []byte("data"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	withPatchedAppDeps(t, func(d *core.AppDeps) {
		d.Engine = &arktest.MockEngine{
			VolumeMountpointFn: func(_ context.Context, _ string) (string, bool) { return "", false },
		}
	}, func() {
		if _, err := cmdbackup.ResolveSourcePaths(core.CurrentAppDeps(), []string{outside}); err == nil {
			t.Fatal("expected error")
		}
	})
}

// TestResolveSourcePathsVolumeMount verifies volume names resolve via docker volume inspect.
// - Expect: Abs uses the mountpoint and Rel stays as the volume name.
func TestResolveSourcePathsVolumeMount(t *testing.T) {
	cwd, _ := os.Getwd()
	tmp := t.TempDir()
	if err := os.Chdir(tmp); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	defer func() { _ = os.Chdir(cwd) }()

	volume := filepath.Join(tmp, "volume")
	if err := os.Mkdir(volume, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(volume, "file"), []byte("data"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	withPatchedAppDeps(t, func(d *core.AppDeps) {
		d.Engine = &arktest.MockEngine{
			VolumeMountpointFn: func(_ context.Context, _ string) (string, bool) { return volume, true },
		}
	}, func() {
		resolved, err := cmdbackup.ResolveSourcePaths(core.CurrentAppDeps(), []string{"ark-volume"})
		if err != nil {
			t.Fatalf("resolve: %v", err)
		}
		if len(resolved) != 1 {
			t.Fatalf("expected 1 path")
		}
		if resolved[0].Abs != volume || resolved[0].Rel != "ark-volume" {
			t.Fatalf("unexpected resolved: %v", resolved)
		}
	})
}

// TestResolveVolumeMountpointNoDocker checks VolumeMountpoint when docker is missing.
// - Expect: no mountpoint is returned when docker is unavailable.
func TestResolveVolumeMountpointNoDocker(t *testing.T) {
	withPatchedAppDeps(t, func(d *core.AppDeps) {
		d.Runtime.LookPath = func(_ string) (string, error) { return "", os.ErrNotExist }
		d.Engine = &arktest.MockEngine{
			VolumeMountpointFn: func(_ context.Context, _ string) (string, bool) { return "", false },
		}
	}, func() {
		if _, ok := core.CurrentAppDeps().Engine.VolumeMountpoint(context.Background(), "volume"); ok {
			t.Fatal("expected no mountpoint")
		}
	})
}

// TestEnsureComposeServiceMissing validates missing services are rejected.
// - Expect: ensureComposeService returns an error for a nonexistent service.
func TestEnsureComposeServiceMissing(t *testing.T) {
	project := fakeProject(emptyServices("asa1"))
	if err := project.EnsureService("missing"); err == nil {
		t.Fatal("expected error")
	}
}
