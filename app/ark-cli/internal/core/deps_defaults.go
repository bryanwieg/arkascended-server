package core

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"

	"pkg/ark"
	"pkg/ark/backup"
	"pkg/ark/cmdrunner"
	"pkg/ark/health"
	"pkg/ark/host"
	"pkg/ark/lifecycle"
	"pkg/ark/runtime"
	"pkg/ark/schedule"
	"pkg/ark/update"
)

// DefaultAppDeps returns an *AppDeps with production defaults for
// host, runtime, and SDK-internal seams. CLI-orchestration function
// seams (RestartStartInstance, RunBackupFromSchedule, etc.) are left
// nil and must be wired by the caller (typically app.go) to avoid
// import cycles between core and cmd.
func DefaultAppDeps() *AppDeps {
	return &AppDeps{
		Ctx: context.Background(),
		HostDeps: host.HostDeps{
			GetNofileHardLimit: host.GetNofileHardLimit,
			FetchText:          host.FetchTextImpl,
			Stdout:             os.Stdout,
			Stderr:             os.Stderr,
		},
		Runtime: RuntimeDeps{
			ContainerRuntime: runtime.RuntimeDocker,
		},
		DiskUsageForPath: backup.DiskUsageForPath,
	}
}

// FillDefaults populates derived fields (Logger, HealthChecker, Lifecycle,
// BackupService, UpdateService, ScheduleService, RuntimeHelper) from the
// base fields. It is idempotent — fields already set are preserved.
// Call after wiring all seams (Engine, ComposeService, RunCommand, etc.).
func FillDefaults(d *AppDeps) {
	if d.Logger == nil {
		d.Logger = slog.Default()
	}
	if d.HealthChecker == nil && d.ComposeService != nil {
		d.HealthChecker = health.New(d.ComposeService, d.Engine, d.Logger)
	}
	if d.Lifecycle == nil && d.ComposeService != nil && d.HealthChecker != nil {
		d.Lifecycle = lifecycle.New(d.ComposeService, d.HealthChecker, d.Logger)
	}
	if d.BackupService == nil && d.ComposeService != nil {
		runner := cmdrunner.NewDefaultRunner(d.Logger)
		bs := backup.New(d.Engine, d.ComposeService, d.Lifecycle, runner, d.Logger)
		bs.DiskUsage = d.DiskUsageForPath
		bs.LookPath = d.Runtime.LookPath
		// Copy the file out through the engine API instead of `<runtime> exec
		// test -f`: the scheduler image has no engine binary, and a copy works
		// on exited containers too. Any copy error means "not there yet".
		bs.CheckFileInContainer = func(ctx context.Context, container, path string) (bool, error) {
			dir, err := ark.TempDir("ark-ready")
			if err != nil {
				return false, err
			}
			defer os.RemoveAll(dir)
			return d.Engine.CopyFromContainer(ctx, container, path, filepath.Join(dir, "ready")) == nil, nil
		}
		d.BackupService = bs
	}
	if d.UpdateService == nil {
		us := update.New(d.Engine, d.ComposeService, d.Lifecycle, d.Logger)
		us.FetchText = d.HostDeps.FetchText
		us.RunCommand = d.Runtime.RunCommand
		us.ContainerRuntime = string(d.Runtime.ContainerRuntime)
		us.DiskUsage = d.DiskUsageForPath
		d.UpdateService = us
	}
	if d.ScheduleService == nil {
		d.ScheduleService = schedule.New(RepoRoot(), d.Logger)
	}
	// (Re)build RuntimeHelper from current fields so patched deps take effect.
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

// RepoRoot returns the current working directory as the repo root.
// This is a package-level var so tests can override it.
var RepoRoot = func() string {
	cwd, err := os.Getwd()
	if err != nil {
		return "."
	}
	return cwd
}

// MergeNonZeroAppDepFields copies non-zero fields from src into dst.
func MergeNonZeroAppDepFields(dst, src *AppDeps) {
	if dst == nil || src == nil {
		return
	}
	if src.Ctx != nil {
		dst.Ctx = src.Ctx
	}
	if src.Engine != nil {
		dst.Engine = src.Engine
	}
	if src.ComposeService != nil {
		dst.ComposeService = src.ComposeService
	}
	if src.HealthChecker != nil {
		dst.HealthChecker = src.HealthChecker
	}
	if src.Lifecycle != nil {
		dst.Lifecycle = src.Lifecycle
	}
	if src.BackupService != nil {
		dst.BackupService = src.BackupService
	}
	if src.UpdateService != nil {
		dst.UpdateService = src.UpdateService
	}
	if src.ScheduleService != nil {
		dst.ScheduleService = src.ScheduleService
	}
	MergeHostDeps(&dst.HostDeps, src.HostDeps)
	MergeRuntimeDeps(&dst.Runtime, src.Runtime)
	if src.RestartStartInstance != nil {
		dst.RestartStartInstance = src.RestartStartInstance
	}
	if src.RestartStopInstance != nil {
		dst.RestartStopInstance = src.RestartStopInstance
	}
	if src.RunBackupFromSchedule != nil {
		dst.RunBackupFromSchedule = src.RunBackupFromSchedule
	}
	if src.RunningServiceSnapshot != nil {
		dst.RunningServiceSnapshot = src.RunningServiceSnapshot
	}
	if src.DiskUsageForPath != nil {
		dst.DiskUsageForPath = src.DiskUsageForPath
	}
	if src.StopService != nil {
		dst.StopService = src.StopService
	}
	if src.RemoveVolume != nil {
		dst.RemoveVolume = src.RemoveVolume
	}
}

// MergeRuntimeDeps copies non-zero fields from src into dst.
func MergeRuntimeDeps(dst *RuntimeDeps, src RuntimeDeps) {
	if src.RunCommand != nil {
		dst.RunCommand = src.RunCommand
	}
	if src.LookPath != nil {
		dst.LookPath = src.LookPath
	}
	if src.ContainerRuntime != "" {
		dst.ContainerRuntime = src.ContainerRuntime
	}
}

// MergeHostDeps copies non-zero fields from src into dst.
func MergeHostDeps(dst *host.HostDeps, src host.HostDeps) {
	if src.RunCommand != nil {
		dst.RunCommand = src.RunCommand
	}
	if src.LookPath != nil {
		dst.LookPath = src.LookPath
	}
	if src.ReadKernelParam != nil {
		dst.ReadKernelParam = src.ReadKernelParam
	}
	if src.GetNofileHardLimit != nil {
		dst.GetNofileHardLimit = src.GetNofileHardLimit
	}
	if src.OpenUDPProbeListener != nil {
		dst.OpenUDPProbeListener = src.OpenUDPProbeListener
	}
	if src.FetchText != nil {
		dst.FetchText = src.FetchText
	}
	if src.Logger != nil {
		dst.Logger = src.Logger
	}
	if src.Stdout != nil {
		dst.Stdout = src.Stdout
	}
	if src.Stderr != nil {
		dst.Stderr = src.Stderr
	}
}
