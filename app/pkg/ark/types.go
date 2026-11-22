package ark

import (
	"fmt"
	"sort"
	"time"

	"github.com/compose-spec/compose-go/v2/types"
)

// ComposeProject wraps a parsed compose project. The Project field is the
// canonical typed representation from compose-go.
type ComposeProject struct {
	Project *types.Project
}

// An ARK instance is a service with label PortASALabel (set by create instance); any other service, such
// as the scheduler (label RoleLabel=SchedulerRole) or a web UI, is not one.
const (
	PortASALabel     = "ark.port.asa"
	RoleLabel        = "ark.role"
	SchedulerRole    = "scheduler"
	SchedulerService = "ark-scheduler" // service name; no container_name (compose names it <project>-ark-scheduler-1, so two projects on one engine never clash)
)

func isInstance(svc types.ServiceConfig) bool {
	return svc.Labels[PortASALabel] != ""
}

func isScheduler(svc types.ServiceConfig) bool {
	return svc.Labels[RoleLabel] == SchedulerRole
}

// ServiceNames returns the sorted names of the project's ARK instances.
func (p *ComposeProject) ServiceNames() []string {
	if p == nil || p.Project == nil {
		return nil
	}
	names := make([]string, 0, len(p.Project.Services))
	for name, svc := range p.Project.Services {
		if isInstance(svc) {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names
}

// EnsureService returns an error if the named service is not an ARK instance of the project.
func (p *ComposeProject) EnsureService(service string) error {
	if p == nil || p.Project == nil {
		return fmt.Errorf("Error: compose service not found: %s", service)
	}
	svc, ok := p.Project.Services[service]
	if !ok {
		return fmt.Errorf("Error: compose service not found: %s", service)
	}
	if isScheduler(svc) {
		return fmt.Errorf("Error: %s is the scheduler service, not an ARK instance", service)
	}
	if !isInstance(svc) {
		return fmt.Errorf("Error: %s is not an ARK instance (no %s label)", service, PortASALabel)
	}
	return nil
}

// HasScheduler reports whether a service carries the scheduler label.
func (p *ComposeProject) HasScheduler() bool {
	if p == nil || p.Project == nil {
		return false
	}
	for _, svc := range p.Project.Services {
		if isScheduler(svc) {
			return true
		}
	}
	return false
}

// ContainerState describes the runtime state of a single container.
type ContainerState struct {
	Running    bool
	Restarting bool
	ExitCode   int
	StartedAt  time.Time // zero when never started or unparseable
}

// ServiceContainer is one compose service's container.
type ServiceContainer struct {
	Name  string // container name
	State string // compose ps state, lower case: running, restarting, exited, created, paused, dead
}

// ExecResult captures the output and exit code of a command executed inside a
// container.
type ExecResult struct {
	Stdout   string
	Stderr   string
	ExitCode int
}

// LogOptions controls how container or service logs are fetched.
type LogOptions struct {
	Tail       int
	Since      *time.Time
	Follow     bool
	Timestamps bool
}

// BuildRequest describes an image build.
type BuildRequest struct {
	Dockerfile string
	Tag        string
	ExtraTags  []string
	BuildArgs  map[string]string
	Context    string
	NoCache    bool
	UseBuildx  bool
}

// UpOptions controls compose up behavior.
type UpOptions struct {
	PullPolicy string
	Detach     bool
}

// ExecOptions controls compose exec behavior.
type ExecOptions struct {
	DisableTTY bool
	Env        []string
	User       string
}

// RunOptions controls compose run (one-off container) behavior.
type RunOptions struct {
	Remove     bool
	NoDeps     bool
	DisableTTY bool
	Entrypoint string
	User       string
	Volumes    []string
}

// CommandOptions controls host-command output behavior.
// Capture true means output is captured into ExecResult; false streams to stdout/stderr.
type CommandOptions struct {
	Capture bool
}
