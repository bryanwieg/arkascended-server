package health

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/compose-spec/compose-go/v2/types"

	"pkg/ark"
	"pkg/ark/arktest"
)

// fakeReady is an InstanceHealthChecker whose IsReady answers ready and counts calls.
type fakeReady struct {
	InstanceHealthChecker
	ready bool
	calls int
}

func (f *fakeReady) IsReady(context.Context, string, string, *time.Time) (bool, error) {
	f.calls++
	return f.ready, nil
}

var (
	probeNow  = time.Date(2026, 11, 21, 12, 0, 0, 0, time.UTC)
	udpSvc    = types.ServiceConfig{Ports: []types.ServicePortConfig{{Target: 27015, Protocol: "tcp"}, {Target: 7777, Protocol: "udp"}}}
	running   = ark.ServiceContainer{Name: "asa1-1", State: "running"}
	rconOK    = ark.ExecResult{}
	rconFails = ark.ExecResult{ExitCode: 1, Stderr: " connection refused\n"}
)

type probeCase struct {
	startedAgo time.Duration
	ready      bool
	rcon       ark.ExecResult
	udpOK      bool
}

func newProbe(pc probeCase) (*Probe, *fakeReady, *[]string) {
	checker := &fakeReady{ready: pc.ready}
	var udpTargets []string
	p := &Probe{
		Compose: &arktest.MockCompose{ExecFn: func(_ context.Context, _, _ string, cmd []string, opts ark.ExecOptions) (ark.ExecResult, error) {
			if cmd[len(cmd)-1] != "listplayers" || !opts.DisableTTY {
				return ark.ExecResult{}, errors.New("unexpected exec")
			}
			return pc.rcon, nil
		}},
		Engine: &arktest.MockEngine{InspectStateFn: func(context.Context, string) (ark.ContainerState, error) {
			return ark.ContainerState{Running: true, StartedAt: probeNow.Add(-pc.startedAgo)}, nil
		}},
		Checker: checker,
		UDP: func(host string, port int, _ time.Duration) bool {
			udpTargets = append(udpTargets, host)
			return pc.udpOK && port == 7777
		},
		Now: func() time.Time { return probeNow },
	}
	return p, checker, &udpTargets
}

func TestProbeCheck(t *testing.T) {
	ok := func(name string) CheckResult { return CheckResult{Name: name, OK: true} }
	tests := []struct {
		name      string
		pc        probeCase
		svc       types.ServiceConfig
		container ark.ServiceContainer
		found     bool
		want      InstanceHealth
		hung      bool
	}{
		{name: "no container", found: false, want: InstanceHealth{Instance: "asa1", State: "stopped"}},
		{name: "exited", container: ark.ServiceContainer{Name: "asa1-1", State: "exited"}, found: true,
			want: InstanceHealth{Instance: "asa1", State: "stopped"}},
		{name: "restarting", container: ark.ServiceContainer{Name: "asa1-1", State: "restarting"}, found: true,
			want: InstanceHealth{Instance: "asa1", State: "restarting", Checks: []CheckResult{{Name: "container", Detail: "restart loop"}}}},
		{name: "starting within grace", pc: probeCase{startedAgo: 5 * time.Minute}, container: running, found: true,
			want: InstanceHealth{Instance: "asa1", State: "starting"}},
		{name: "not ready after grace", pc: probeCase{startedAgo: 25 * time.Minute, rcon: rconFails, udpOK: true}, svc: udpSvc, container: running, found: true,
			want: InstanceHealth{Instance: "asa1", State: "running", Checks: []CheckResult{
				{Name: "ready", Detail: "not ready 25m after start"}, {Name: "rcon", Detail: "connection refused"}, ok("udp")}},
			hung: true},
		{name: "rcon fail", pc: probeCase{startedAgo: time.Hour, ready: true, rcon: rconFails, udpOK: true}, svc: udpSvc, container: running, found: true,
			want: InstanceHealth{Instance: "asa1", State: "running", Checks: []CheckResult{
				ok("ready"), {Name: "rcon", Detail: "connection refused"}, ok("udp")}},
			hung: true},
		{name: "udp fail", pc: probeCase{startedAgo: time.Hour, ready: true, rcon: rconOK}, svc: udpSvc, container: running, found: true,
			want: InstanceHealth{Instance: "asa1", State: "running", Checks: []CheckResult{
				ok("ready"), ok("rcon"), {Name: "udp", Detail: "no answer on asa1:7777/udp"}}}},
		{name: "all ok", pc: probeCase{startedAgo: time.Hour, ready: true, rcon: rconOK, udpOK: true}, svc: udpSvc, container: running, found: true,
			want: InstanceHealth{Instance: "asa1", State: "running", Checks: []CheckResult{ok("ready"), ok("rcon"), ok("udp")}}},
		{name: "no udp port", pc: probeCase{startedAgo: time.Hour, ready: true, rcon: rconOK}, container: running, found: true,
			want: InstanceHealth{Instance: "asa1", State: "running", Checks: []CheckResult{ok("ready"), ok("rcon")}}},
		{name: "past grace without ready line, rcon ok", pc: probeCase{startedAgo: 25 * time.Minute, rcon: rconOK, udpOK: true}, svc: udpSvc, container: running, found: true,
			want: InstanceHealth{Instance: "asa1", State: "running", Checks: []CheckResult{ok("ready"), ok("rcon"), ok("udp")}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, _, udpTargets := newProbe(tt.pc)
			got := p.Check(context.Background(), "dc.yaml", "asa1", tt.svc, tt.container, tt.found)
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("Check =\n %+v\nwant\n %+v", got, tt.want)
			}
			if got.Hung() != tt.hung {
				t.Fatalf("Hung() = %v, want %v", got.Hung(), tt.hung)
			}
			if got.OK() != allOK(tt.want.Checks) {
				t.Fatalf("OK() = %v", got.OK())
			}
			for _, host := range *udpTargets {
				if host != "asa1" {
					t.Fatalf("UDP probed %q, want the service name", host)
				}
			}
		})
	}
}

func allOK(checks []CheckResult) bool {
	for _, c := range checks {
		if !c.OK {
			return false
		}
	}
	return true
}

func TestProbeCheckInspectError(t *testing.T) {
	p, _, _ := newProbe(probeCase{})
	p.Engine = &arktest.MockEngine{InspectStateFn: func(context.Context, string) (ark.ContainerState, error) {
		return ark.ContainerState{}, errors.New("no such container")
	}}
	got := p.Check(context.Background(), "dc.yaml", "asa1", udpSvc, running, true)
	want := []CheckResult{{Name: "container", Detail: "no such container"}}
	if !reflect.DeepEqual(got.Checks, want) || got.Hung() {
		t.Fatalf("Check = %+v", got)
	}
}

func TestHungIgnoresUDP(t *testing.T) {
	h := InstanceHealth{Checks: []CheckResult{{Name: "ready", OK: true}, {Name: "rcon", OK: true}, {Name: "udp"}}}
	if h.Hung() || h.OK() {
		t.Fatalf("udp-only failure: Hung()=%v OK()=%v, want false false", h.Hung(), h.OK())
	}
}

func TestProbeReadyCachedPerStart(t *testing.T) {
	startedAt := probeNow.Add(-time.Hour)
	p, checker, _ := newProbe(probeCase{ready: true, rcon: rconOK})
	p.Engine = &arktest.MockEngine{InspectStateFn: func(context.Context, string) (ark.ContainerState, error) {
		return ark.ContainerState{Running: true, StartedAt: startedAt}, nil
	}}
	check := func() {
		p.Check(context.Background(), "dc.yaml", "asa1", types.ServiceConfig{}, running, true)
	}
	check()
	check()
	if checker.calls != 1 {
		t.Fatalf("IsReady called %d times for one container start, want 1", checker.calls)
	}
	startedAt = probeNow.Add(-30 * time.Minute)
	check()
	if checker.calls != 2 {
		t.Fatalf("IsReady called %d times after a new start, want 2", checker.calls)
	}
}

func TestProbeReadyByRCONIsCached(t *testing.T) {
	p, checker, _ := newProbe(probeCase{startedAgo: 25 * time.Minute, rcon: rconOK})
	first := p.Check(context.Background(), "dc.yaml", "asa1", types.ServiceConfig{}, running, true)
	if !first.OK() || first.Hung() {
		t.Fatalf("past grace with RCON answering: %+v", first)
	}
	p.Check(context.Background(), "dc.yaml", "asa1", types.ServiceConfig{}, running, true)
	if checker.calls != 1 {
		t.Fatalf("IsReady called %d times, want 1 (ready cached after RCON answered)", checker.calls)
	}
}
