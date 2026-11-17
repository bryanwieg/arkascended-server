package compose

import (
	"ark-cli/internal/core"
	"time"

	"pkg/ark/runtime"
)

const (
	DefaultStartReadyWaitTimeout  = 5 * time.Minute
	defaultStartReadyWaitInterval = 2 * time.Second
	DefaultStopTimeoutSeconds     = 900
)

type startCommandRequest = runtime.StartCommandRequest
type stopCommandRequest = runtime.StopCommandRequest
type downCommandRequest = runtime.DownCommandRequest

// DownCommandRequest is the exported alias for runtime.DownCommandRequest.
type DownCommandRequest = runtime.DownCommandRequest

type StartExecutionPlan struct {
	ComposeFile        string
	Services           []string
	PullPolicy         string
	NoWait             bool
	WaitTimeout        time.Duration
	PreRunningSnapshot map[string]struct{}
	StartLogSince      time.Time
}

type ComposeLifecycleCommandService struct {
	deps *core.AppDeps
}

func NewComposeLifecycleCommandService(deps *core.AppDeps) ComposeLifecycleCommandService {
	return ComposeLifecycleCommandService{deps: deps}
}

func (s ComposeLifecycleCommandService) runWithDeps(fn func() error) error {
	return core.WithAppDepsErr(s.deps, fn)
}
