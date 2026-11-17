package core

import (
	"context"
	"log/slog"

	"pkg/ark"
	"pkg/ark/backup"
	"pkg/ark/health"
	"pkg/ark/host"
	"pkg/ark/lifecycle"
	"pkg/ark/runtime"
	"pkg/ark/schedule"
	"pkg/ark/update"
)

// AppDeps groups side-effecting seams and top-level CLI wiring so tests can
// inject behavior without patching package globals directly.
type AppDeps struct {
	Ctx             context.Context
	Logger          *slog.Logger
	Engine          ark.ContainerEngine
	ComposeService  ark.ComposeService
	HealthChecker   health.InstanceHealthChecker
	Lifecycle       *lifecycle.Service
	BackupService   *backup.Service
	UpdateService   *update.Service
	ScheduleService *schedule.Service

	// Host kernel config and port check seams
	HostDeps host.HostDeps

	// Runtime helper -- composes RuntimeDeps + HostDeps + SDK services
	RuntimeHelper runtime.RuntimeHelper

	// CLI-specific I/O seams (for testability)
	Runtime                        RuntimeDeps
	RestartStartInstance           func(runtime.StartCommandRequest) error
	RestartStopInstance            func(runtime.StopCommandRequest) error
	RunBackupFromSchedule          func(string, []string) error
	InstallManagedSchedulesCrontab func(string) error
	RunningServiceSnapshot         func(string) (map[string]struct{}, error)
	DiskUsageForPath               func(string) (int64, int64, error)
	StopService                    func(string, string) error
	RemoveVolume                   func(string, string) error
}

// RuntimeDeps holds primitive function seams for command execution.
type RuntimeDeps struct {
	ContainerRuntime runtime.ContainerRuntime
	RunCommand       func([]string, ark.CommandOptions) (ark.ExecResult, error)
	LookPath         func(string) (string, error)
	RconEnvArgs      func() []string
}

// Context returns the app's context, falling back to context.Background()
// if none was set (e.g. in tests).
func (d *AppDeps) Context() context.Context {
	if d.Ctx != nil {
		return d.Ctx
	}
	return context.Background()
}
