package testutil

// Shared test utilities for ark-cli domain packages.
// This package is imported by test files across internal packages
// to avoid duplicating common test helpers.

import (
	"context"
	"log/slog"
	"os"
	"testing"

	"ark-cli/internal/compose"
	"ark-cli/internal/core"

	"pkg/ark"
	"pkg/ark/arktest"
	"pkg/ark/backup"
	"pkg/ark/cmdrunner"
	"pkg/ark/health"
	"pkg/ark/lifecycle"
	"pkg/ark/runtime"
	"pkg/ark/schedule"
	"pkg/ark/update"
)

// TestDepsConfig controls how test dependency wiring works for a domain package.
type TestDepsConfig struct {
	// ExtraWireFuncs are called after compose.WireDefaults during CloneAppDepsForTest.
	ExtraWireFuncs []func(*core.AppDeps)
	// WireBackupService controls whether FillTestDefaults wires BackupService.
	WireBackupService bool
	// WireUpdateService controls whether FillTestDefaults wires UpdateService.
	WireUpdateService bool
	// WireScheduleService controls whether FillTestDefaults wires ScheduleService.
	WireScheduleService bool
	// ResetBackupOnPatch controls whether WithPatchedAppDeps clears BackupService
	// (when not explicitly patched) before rebuilding.
	ResetBackupOnPatch bool
	// ResetUpdateOnPatch controls whether WithPatchedAppDeps clears UpdateService
	// (when not explicitly patched) before rebuilding.
	ResetUpdateOnPatch bool
}

// Pre-defined configs for common wiring patterns.

// FullConfig wires all services and resets all on patch. Used by cmd, backup, schedule.
var FullConfig = TestDepsConfig{
	WireBackupService:   true,
	WireUpdateService:   true,
	WireScheduleService: true,
	ResetBackupOnPatch:  true,
	ResetUpdateOnPatch:  true,
}

// ComposeOnlyConfig wires only compose (no domain services). Used by reconfigure.
var ComposeOnlyConfig = TestDepsConfig{}

// UpdateOnlyConfig wires only UpdateService. Used by update.
var UpdateOnlyConfig = TestDepsConfig{
	WireUpdateService:  true,
	ResetUpdateOnPatch: true,
}

// ShowConfig wires all services but doesn't reset on patch. Used by ini, host, show, remove.
var ShowConfig = TestDepsConfig{
	WireBackupService:   true,
	WireUpdateService:   true,
	WireScheduleService: true,
}

// FillTestDefaults fills in derived fields on deps for testing,
// controlled by the given config.
func FillTestDefaults(d *core.AppDeps, cfg TestDepsConfig) {
	if d.Logger == nil {
		d.Logger = slog.Default()
	}
	if d.ComposeService == nil {
		d.ComposeService = &arktest.MockCompose{
			LoadProjectFn: func(ctx context.Context, composeFile string) (*ark.ComposeProject, error) {
				project, err := LoadComposeFromPathForTest(composeFile)
				if err != nil {
					return nil, err
				}
				return &ark.ComposeProject{Project: project}, nil
			},
			SaveProjectFn: func(_ context.Context, composeFile string, project *ark.ComposeProject) error {
				return SaveComposeForTest(composeFile, project.Project)
			},
		}
	}
	if d.HealthChecker == nil && d.ComposeService != nil {
		d.HealthChecker = health.New(d.ComposeService, d.Engine, d.Logger)
	}
	if d.Lifecycle == nil && d.ComposeService != nil && d.HealthChecker != nil {
		d.Lifecycle = lifecycle.New(d.ComposeService, d.HealthChecker, d.Logger)
	}
	if cfg.WireBackupService && d.BackupService == nil && d.ComposeService != nil {
		runner := cmdrunner.NewDefaultRunner(d.Logger)
		bs := backup.New(d.Engine, d.ComposeService, d.Lifecycle, runner, d.Logger)
		bs.DiskUsage = d.DiskUsageForPath
		bs.LookPath = d.Runtime.LookPath
		bs.CheckFileInContainer = func(ctx context.Context, container, path string) (bool, error) {
			rt := d.Runtime.ContainerRuntime
			if rt == "" {
				rt = runtime.RuntimeDocker
			}
			cmd := []string{string(rt), "exec", container, "test", "-f", path}
			result, err := d.Runtime.RunCommand(cmd, ark.CommandOptions{Capture: true})
			if err != nil {
				return false, err
			}
			return result.ExitCode == 0, nil
		}
		d.BackupService = bs
	}
	if cfg.WireUpdateService && d.UpdateService == nil {
		us := update.New(d.Engine, d.ComposeService, d.Lifecycle, d.Logger)
		us.FetchText = d.HostDeps.FetchText
		us.RunCommand = d.Runtime.RunCommand
		us.ContainerRuntime = string(d.Runtime.ContainerRuntime)
		us.DiskUsage = d.DiskUsageForPath
		d.UpdateService = us
	}
	if cfg.WireScheduleService && d.ScheduleService == nil {
		d.ScheduleService = schedule.New(RepoRoot(), d.Logger)
	}
	d.RuntimeHelper = runtime.RuntimeHelper{
		Ctx:    d.Ctx,
		Logger: d.Logger,
		Runtime: runtime.RuntimeDeps{
			ContainerRuntime: runtime.ContainerRuntime(d.Runtime.ContainerRuntime),
			RunCommand:       d.Runtime.RunCommand,
			LookPath:         d.Runtime.LookPath,
		},
		Host:            d.HostDeps,
		Engine:          d.Engine,
		ComposeService:  d.ComposeService,
		HealthChecker:   d.HealthChecker,
		Lifecycle:       d.Lifecycle,
		UpdateService:   d.UpdateService,
		BackupService:   d.BackupService,
		ScheduleService: d.ScheduleService,
		RepoRoot:        RepoRoot(),
		Stdout:          os.Stdout,
		Stderr:          os.Stderr,
	}
}

// CloneAppDepsForTest creates a *core.AppDeps with compose wire defaults,
// merges non-zero fields from the current deps (if any), and fills derived
// fields for testing. Merging from current deps preserves any seams set by
// patchAppDepsForTest so that subsequent withPatchedAppDeps calls inherit
// them.
func CloneAppDepsForTest(cfg TestDepsConfig) *core.AppDeps {
	d := core.DefaultAppDeps()
	compose.WireDefaults(d)
	for _, fn := range cfg.ExtraWireFuncs {
		fn(d)
	}
	if current := core.CurrentAppDeps(); current != nil {
		MergeNonZeroAppDepFields(d, current)
	}
	FillTestDefaults(d, cfg)
	return d
}

// WithPatchedAppDeps creates a test AppDeps, applies the patch function,
// then runs fn with the patched deps as the current deps.
func WithPatchedAppDeps(t *testing.T, cfg TestDepsConfig, patch func(*core.AppDeps), fn func()) {
	t.Helper()
	deps := CloneAppDepsForTest(cfg)
	// Save pre-patch pointers for services that depend on patched base fields
	// (Engine, ComposeService). If the test explicitly replaces them, preserve
	// the test's value; otherwise clear so FillTestDefaults rebuilds them.
	backupBefore := deps.BackupService
	updateBefore := deps.UpdateService
	lifecycleBefore := deps.Lifecycle
	if patch != nil {
		patch(deps)
	}
	if cfg.ResetBackupOnPatch && deps.BackupService == backupBefore {
		deps.BackupService = nil
	}
	if cfg.ResetUpdateOnPatch && deps.UpdateService == updateBefore {
		deps.UpdateService = nil
	}
	// Lifecycle depends on ComposeService. If the patch replaced ComposeService
	// but left Lifecycle unchanged, clear it so FillTestDefaults rebuilds it with
	// the new mock.
	if deps.Lifecycle == lifecycleBefore {
		deps.Lifecycle = nil
	}
	// ScheduleService and HealthChecker do NOT depend on Engine/ComposeService,
	// so never clear them.
	deps.RuntimeHelper = runtime.RuntimeHelper{}
	FillTestDefaults(deps, cfg)
	core.WithAppDeps(deps, fn)
}

// RepoRoot delegates to core.RepoRoot for test wiring.
func RepoRoot() string {
	return core.RepoRoot()
}
