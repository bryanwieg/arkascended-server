package runtime

import (
	"context"
	"io"
	"log/slog"
	"time"

	"pkg/ark"
	"pkg/ark/backup"
	"pkg/ark/health"
	"pkg/ark/host"
	"pkg/ark/lifecycle"
	"pkg/ark/schedule"
	"pkg/ark/update"
)

// ContainerRuntime represents the container runtime type.
type ContainerRuntime string

const (
	RuntimeDocker ContainerRuntime = "docker"
	RuntimePodman ContainerRuntime = "podman"
)

// RuntimeDeps holds primitive function seams for command execution and host introspection.
type RuntimeDeps struct {
	ContainerRuntime     ContainerRuntime
	RunCommand           func([]string, ark.CommandOptions) (ark.ExecResult, error)
	LookPath             func(string) (string, error)
	ReadKernelParam      func(string) (int, error)
	GetNofileHardLimit   func() (uint64, error)
	OpenUDPProbeListener func(int) (func(), error)
}

// RuntimeHelper composes RuntimeDeps with SDK services and I/O.
// Most runtime functions that currently live on *appDeps become methods on RuntimeHelper.
type RuntimeHelper struct {
	Ctx             context.Context
	Logger          *slog.Logger
	Runtime         RuntimeDeps
	Host            host.HostDeps // named field — avoids name collisions with RuntimeDeps
	Engine          ark.ContainerEngine
	ComposeService  ark.ComposeService
	HealthChecker   health.InstanceHealthChecker
	Lifecycle       *lifecycle.Service
	UpdateService   *update.Service
	BackupService   *backup.Service
	ScheduleService *schedule.Service
	Stdout          io.Writer
	Stderr          io.Writer
	RepoRoot        string // replaces repoRoot() global
}

// StartCommandRequest holds parameters for starting an instance.
type StartCommandRequest struct {
	Instance        string
	ComposeFileFlag string
	NoWait          bool
	WaitTimeout     time.Duration
}

// StopCommandRequest holds parameters for stopping an instance.
type StopCommandRequest struct {
	Instance        string
	ComposeFileFlag string
	Timeout         time.Duration
	Kill            bool
}

// DownCommandRequest holds parameters for taking an instance down.
type DownCommandRequest struct {
	Instance        string
	ComposeFileFlag string
	Timeout         time.Duration
	Kill            bool
}
