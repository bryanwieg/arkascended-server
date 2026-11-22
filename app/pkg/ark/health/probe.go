package health

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/compose-spec/compose-go/v2/types"

	"pkg/ark"
)

// StartGrace is how long a started instance may go without the ready log line
// before its RCON answer decides whether it is up (design 11).
const StartGrace = 20 * time.Minute

const (
	rconTimeout = 15 * time.Second
	udpTimeout  = 3 * time.Second
)

// CheckResult is one named health check.
type CheckResult struct {
	Name   string `json:"name"` // "container", "ready", "rcon", "udp", or a schedule name
	OK     bool   `json:"ok"`
	Detail string `json:"detail,omitempty"`
}

// InstanceHealth is the result of probing one instance.
type InstanceHealth struct {
	Instance string        `json:"instance"`
	State    string        `json:"state"` // "stopped", "starting", "running", "restarting" ("busy" is set by the monitor)
	Checks   []CheckResult `json:"checks,omitempty"`
}

// OK reports whether every check passed.
func (h InstanceHealth) OK() bool {
	for _, c := range h.Checks {
		if !c.OK {
			return false
		}
	}
	return true
}

// Hung reports a failed ready or rcon check, which a restart may fix. A failed
// udp check alone never counts (design 13).
func (h InstanceHealth) Hung() bool {
	for _, c := range h.Checks {
		if !c.OK && (c.Name == "ready" || c.Name == "rcon") {
			return true
		}
	}
	return false
}

// Probe checks one instance's container state, ready log line, RCON and UDP port.
type Probe struct {
	Compose ark.ComposeService
	Engine  ark.ContainerEngine
	Checker InstanceHealthChecker
	UDP     func(host string, port int, timeout time.Duration) bool
	Now     func() time.Time

	readyAt map[string]time.Time // container name → StartedAt of the run seen ready
}

// Check returns the health of service, whose container is c (found false when it has none).
func (p *Probe) Check(ctx context.Context, composeFile, service string, svc types.ServiceConfig, c ark.ServiceContainer, found bool) InstanceHealth {
	if !found || (c.State != "running" && c.State != "restarting") {
		return InstanceHealth{Instance: service, State: "stopped"}
	}
	if c.State == "restarting" {
		return InstanceHealth{Instance: service, State: "restarting",
			Checks: []CheckResult{{Name: "container", Detail: "restart loop"}}}
	}
	st, err := p.Engine.InspectState(ctx, c.Name)
	if err != nil {
		return InstanceHealth{Instance: service, State: c.State,
			Checks: []CheckResult{{Name: "container", Detail: err.Error()}}}
	}

	ready := p.ready(ctx, composeFile, service, c.Name, st.StartedAt)
	sinceStart := p.Now().Sub(st.StartedAt)
	if !ready && sinceStart < StartGrace {
		return InstanceHealth{Instance: service, State: "starting"}
	}

	rcon := p.rcon(ctx, composeFile, service)
	readyCheck := CheckResult{Name: "ready", OK: ready || rcon.OK}
	if !readyCheck.OK {
		readyCheck.Detail = fmt.Sprintf("not ready %dm after start", int(sinceStart.Minutes()))
	}
	if !ready && rcon.OK {
		// The ready line may have left the log (rotation, journald); RCON answering proves it.
		p.readyAt[c.Name] = st.StartedAt
	}
	checks := []CheckResult{readyCheck, rcon}
	if port, ok := udpPort(svc); ok {
		checks = append(checks, p.udp(service, port))
	}
	return InstanceHealth{Instance: service, State: "running", Checks: checks}
}

// ready reports the ready log line since startedAt, cached per container run.
// A log read error counts as not ready.
func (p *Probe) ready(ctx context.Context, composeFile, service, container string, startedAt time.Time) bool {
	if p.readyAt == nil {
		p.readyAt = map[string]time.Time{}
	}
	if at, ok := p.readyAt[container]; ok && at.Equal(startedAt) {
		return true
	}
	ok, err := p.Checker.IsReady(ctx, composeFile, service, &startedAt)
	if err != nil || !ok {
		return false
	}
	p.readyAt[container] = startedAt
	return true
}

func (p *Probe) rcon(ctx context.Context, composeFile, service string) CheckResult {
	ctx, cancel := context.WithTimeout(ctx, rconTimeout)
	defer cancel()
	argv := []string{"/usr/bin/python3.14", "/home/ark-user/scripts/runtime/ark-rcon.py", "listplayers"}
	res, err := p.Compose.Exec(ctx, composeFile, service, argv, ark.ExecOptions{DisableTTY: true})
	switch {
	case err != nil:
		return CheckResult{Name: "rcon", Detail: err.Error()}
	case res.ExitCode != 0:
		detail := strings.TrimSpace(res.Stderr)
		if detail == "" {
			detail = fmt.Sprintf("exit code %d", res.ExitCode)
		}
		return CheckResult{Name: "rcon", Detail: detail}
	}
	return CheckResult{Name: "rcon", OK: true}
}

func (p *Probe) udp(service string, port int) CheckResult {
	if p.UDP(service, port, udpTimeout) {
		return CheckResult{Name: "udp", OK: true}
	}
	return CheckResult{Name: "udp", Detail: fmt.Sprintf("no answer on %s:%d/udp", service, port)}
}

// udpPort returns the container side of the service's first UDP port.
func udpPort(svc types.ServiceConfig) (int, bool) {
	for _, pc := range svc.Ports {
		if pc.Protocol == "udp" {
			return int(pc.Target), true
		}
	}
	return 0, false
}
