package health

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/compose-spec/compose-go/v2/types"

	"pkg/ark"
	"pkg/ark/arktest"
)

// fakeInstance is one instance's state in a monitorEnv; change fields between rounds.
type fakeInstance struct {
	state      string // compose state; "" = no container
	startedAgo time.Duration
	ready      bool
	rconOK     bool
	locked     bool
	noUDP      bool
}

type monitorEnv struct {
	now        time.Time
	instances  map[string]*fakeInstance
	schedules  []CheckResult
	composeErr error
	restartErr error
	restarted  []string
	probed     []string // services whose RCON was exec'd
	logs       []string
}

// envReady answers IsReady from the env's per-instance ready flag.
type envReady struct {
	InstanceHealthChecker
	env *monitorEnv
}

func (r envReady) IsReady(_ context.Context, _, service string, _ *time.Time) (bool, error) {
	return r.env.instances[service].ready, nil
}

func newMonitorEnv(t *testing.T, instances map[string]*fakeInstance) (*monitorEnv, *Monitor) {
	env := &monitorEnv{now: probeNow, instances: instances}
	services := types.Services{}
	for name, inst := range instances {
		svc := types.ServiceConfig{Name: name}
		if !inst.noUDP {
			svc.Ports = []types.ServicePortConfig{{Target: 7777, Protocol: "udp"}}
		}
		services[name] = svc
	}
	compose := &arktest.MockCompose{
		ServiceContainersFn: func(context.Context, string) (map[string]ark.ServiceContainer, error) {
			out := map[string]ark.ServiceContainer{}
			for name, inst := range env.instances {
				if inst.state != "" {
					out[name] = ark.ServiceContainer{Name: name + "-1", State: inst.state}
				}
			}
			return out, nil
		},
		ExecFn: func(_ context.Context, _, service string, _ []string, _ ark.ExecOptions) (ark.ExecResult, error) {
			env.probed = append(env.probed, service)
			if env.instances[service].rconOK {
				return ark.ExecResult{}, nil
			}
			return ark.ExecResult{ExitCode: 1, Stderr: "Error: RCON command timed out"}, nil
		},
	}
	engine := &arktest.MockEngine{InspectStateFn: func(_ context.Context, container string) (ark.ContainerState, error) {
		inst := env.instances[strings.TrimSuffix(container, "-1")]
		return ark.ContainerState{Running: true, StartedAt: probeNow.Add(-inst.startedAgo)}, nil
	}}
	m := &Monitor{
		Probe: &Probe{Compose: compose, Engine: engine, Checker: envReady{env: env},
			UDP: func(string, int, time.Duration) bool { return true }, Now: func() time.Time { return env.now }},
		ComposeFile: "dc.yaml",
		LoadProject: func(context.Context) (*ark.ComposeProject, error) {
			if env.composeErr != nil {
				return nil, env.composeErr
			}
			return &ark.ComposeProject{Project: &types.Project{Services: services}}, nil
		},
		Schedules: func(time.Time) []CheckResult { return env.schedules },
		LockHeld:  func(instance string) (bool, error) { return env.instances[instance].locked, nil },
		Restart: func(_ context.Context, instance string) error {
			if env.restartErr != nil {
				return env.restartErr
			}
			env.restarted = append(env.restarted, instance)
			return nil
		},
		HealthPath: filepath.Join(t.TempDir(), "state", "scheduler", "health.json"),
		Now:        func() time.Time { return env.now },
		Logf:       func(format string, args ...any) { env.logs = append(env.logs, fmt.Sprintf(format, args...)) },
	}
	return env, m
}

// round runs one monitor round, then advances the clock a minute.
func (env *monitorEnv) round(m *Monitor) Report {
	r := m.Round(context.Background())
	env.now = env.now.Add(time.Minute)
	return r
}

func healthy() *fakeInstance {
	return &fakeInstance{state: "running", startedAgo: time.Hour, ready: true, rconOK: true}
}

func hung() *fakeInstance {
	return &fakeInstance{state: "running", startedAgo: time.Hour, ready: true}
}

var allSkipped = map[string]string{"container": CheckSkipped, "ready": CheckSkipped, "rcon": CheckSkipped, "udp": CheckSkipped}

func TestMonitorRestartsAfterFiveHungRounds(t *testing.T) {
	env, m := newMonitorEnv(t, map[string]*fakeInstance{"asa1": hung()})
	for i := 1; i <= 4; i++ {
		env.round(m)
		if len(env.restarted) != 0 {
			t.Fatalf("restarted after %d hung rounds", i)
		}
	}
	r := env.round(m)
	if !slices.Equal(env.restarted, []string{"asa1"}) {
		t.Fatalf("restarted = %v after 5 hung rounds", env.restarted)
	}
	want := []RestartRecord{{Instance: "asa1", Time: probeNow.Add(4 * time.Minute), Reason: "rcon"}}
	if !reflect.DeepEqual(r.Restarts, want) {
		t.Fatalf("Restarts = %+v", r.Restarts)
	}
	if !slices.Contains(env.logs, "Restarting asa1: rcon") {
		t.Fatalf("logs = %q", env.logs)
	}
	for range 4 { // the counter was reset
		env.round(m)
	}
	if len(env.restarted) != 1 {
		t.Fatalf("restarted = %v, want one restart", env.restarted)
	}
}

func TestMonitorBusyInstanceSkipped(t *testing.T) {
	inst := hung()
	inst.locked = true
	env, m := newMonitorEnv(t, map[string]*fakeInstance{"asa1": inst})
	var r Report
	for range 10 {
		r = env.round(m)
	}
	want := InstanceReport{State: StateBusy, Checks: allSkipped}
	if !reflect.DeepEqual(r.Instances["asa1"], want) || !r.Healthy {
		t.Fatalf("busy report = %+v healthy=%v", r.Instances["asa1"], r.Healthy)
	}
	if len(env.probed) != 0 || len(env.restarted) != 0 {
		t.Fatalf("busy instance probed %v, restarted %v", env.probed, env.restarted)
	}
}

func TestMonitorBusyRoundsKeepCounter(t *testing.T) {
	inst := hung()
	env, m := newMonitorEnv(t, map[string]*fakeInstance{"asa1": inst})
	for i, locked := range []bool{false, false, true, true, true, false, false} {
		inst.locked = locked
		env.round(m)
		if len(env.restarted) != 0 {
			t.Fatalf("restarted in round %d", i+1)
		}
	}
	env.round(m) // fifth hung round
	if len(env.restarted) != 1 {
		t.Fatalf("restarted = %v on the fifth hung round", env.restarted)
	}
}

func TestMonitorRestartBusyRetriesNextRound(t *testing.T) {
	env, m := newMonitorEnv(t, map[string]*fakeInstance{"asa1": hung()})
	for range 4 {
		env.round(m)
	}
	env.restartErr = fmt.Errorf("wrapped: %w", ErrInstanceBusy)
	r := env.round(m)
	if len(r.Restarts) != 0 || !slices.Contains(env.logs, "Not restarting asa1: busy") {
		t.Fatalf("busy restart: restarts %+v, logs %q", r.Restarts, env.logs)
	}
	env.restartErr = nil
	r = env.round(m)
	if len(env.restarted) != 1 || len(r.Restarts) != 1 {
		t.Fatalf("next round: restarted %v, restarts %+v", env.restarted, r.Restarts)
	}
}

func TestMonitorRestartErrorRecorded(t *testing.T) {
	env, m := newMonitorEnv(t, map[string]*fakeInstance{"asa1": hung()})
	env.restartErr = errors.New("stop failed")
	var r Report
	for range 5 {
		r = env.round(m)
	}
	if len(r.Restarts) != 1 || r.Restarts[0].Error != "stop failed" {
		t.Fatalf("Restarts = %+v", r.Restarts)
	}
}

func TestMonitorRestartCap(t *testing.T) {
	env, m := newMonitorEnv(t, map[string]*fakeInstance{"asa1": hung()})
	for range 10 {
		env.round(m)
	}
	if len(env.restarted) != 2 {
		t.Fatalf("restarted = %v, want 2", env.restarted)
	}
	for range 20 {
		env.round(m)
	}
	limit := 0
	for _, l := range env.logs {
		if l == "Not restarting asa1: restart limit reached" {
			limit++
		}
	}
	if len(env.restarted) != 2 || limit != 1 {
		t.Fatalf("capped: restarted %v, limit logged %d times", env.restarted, limit)
	}
	env.now = probeNow.Add(4*time.Minute + time.Hour) // an hour after the first restart
	env.round(m)
	if len(env.restarted) != 3 {
		t.Fatalf("restarted = %v, want a third restart after an hour", env.restarted)
	}
}

func TestMonitorRecoveryResetsCounter(t *testing.T) {
	inst := hung()
	env, m := newMonitorEnv(t, map[string]*fakeInstance{"asa1": inst})
	for _, ok := range []bool{false, false, false, false, true, false, false, false, false} {
		inst.rconOK = ok
		env.round(m)
	}
	if len(env.restarted) != 0 {
		t.Fatalf("restarted = %v after a recovered round", env.restarted)
	}
	env.round(m)
	if len(env.restarted) != 1 {
		t.Fatalf("restarted = %v, want one after 5 hung rounds", env.restarted)
	}
}

func TestMonitorStoppedAndStartingSkipped(t *testing.T) {
	env, m := newMonitorEnv(t, map[string]*fakeInstance{
		"asa1": {state: "exited"},
		"asa2": {},
		"asa3": {state: "running", startedAgo: 5 * time.Minute},
	})
	var r Report
	for range 10 {
		r = env.round(m)
	}
	want := map[string]InstanceReport{
		"asa1": {State: "stopped", Checks: allSkipped},
		"asa2": {State: "stopped", Checks: allSkipped},
		"asa3": {State: "starting", Checks: allSkipped},
	}
	if !reflect.DeepEqual(r.Instances, want) || !r.Healthy || len(env.restarted) != 0 {
		t.Fatalf("instances = %+v healthy=%v restarted=%v", r.Instances, r.Healthy, env.restarted)
	}
}

func TestMonitorNoUDPPortSkipped(t *testing.T) {
	inst := healthy()
	inst.noUDP = true
	env, m := newMonitorEnv(t, map[string]*fakeInstance{"asa1": inst})
	r := env.round(m)
	want := map[string]string{"container": CheckOK, "ready": CheckOK, "rcon": CheckOK, "udp": CheckSkipped}
	if !reflect.DeepEqual(r.Instances["asa1"].Checks, want) || !r.Healthy {
		t.Fatalf("checks = %v healthy=%v", r.Instances["asa1"].Checks, r.Healthy)
	}
}

func TestMonitorOneRestartPerRound(t *testing.T) {
	env, m := newMonitorEnv(t, map[string]*fakeInstance{"asa1": hung(), "asa2": hung()})
	for range 5 {
		env.round(m)
	}
	if !slices.Equal(env.restarted, []string{"asa1"}) {
		t.Fatalf("round 5 restarted %v, want only asa1", env.restarted)
	}
	env.round(m)
	if !slices.Equal(env.restarted, []string{"asa1", "asa2"}) {
		t.Fatalf("round 6 restarted %v, want asa2 next", env.restarted)
	}
}

func TestMonitorEmptyCollectionsSerialize(t *testing.T) {
	env, m := newMonitorEnv(t, map[string]*fakeInstance{"asa1": healthy()})
	data, err := json.Marshal(env.round(m))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"restarts":[]`, `"schedule_details":{}`, `"schedules":{}`} {
		if !strings.Contains(string(data), want) {
			t.Fatalf("report %s lacks %s", data, want)
		}
	}
}

func TestMonitorComposeError(t *testing.T) {
	env, m := newMonitorEnv(t, map[string]*fakeInstance{"asa1": healthy()})
	env.composeErr = errors.New("Error: compose file not found")
	r := env.round(m)
	if r.Healthy || r.Error != "Error: compose file not found" || len(r.Instances) != 0 {
		t.Fatalf("report = %+v", r)
	}
}

func TestMonitorReportFileLoadable(t *testing.T) {
	env, m := newMonitorEnv(t, map[string]*fakeInstance{"asa1": hung()})
	env.schedules = []CheckResult{{Name: "nightly", Detail: "last run failed: boom"}}
	var r Report
	for range 5 {
		r = env.round(m)
	}
	got, err := LoadReport(m.HealthPath)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, r) {
		t.Fatalf("LoadReport =\n %+v\nwant\n %+v", got, r)
	}
	if latest, ok := m.Latest(); !ok || !reflect.DeepEqual(latest, r) {
		t.Fatalf("Latest = %+v, %v", latest, ok)
	}
}

func TestMonitorLatestRace(t *testing.T) {
	_, m := newMonitorEnv(t, map[string]*fakeInstance{"asa1": healthy()})
	ctx, cancel := context.WithCancel(context.Background())
	tick := make(chan time.Time)
	done := make(chan struct{})
	go func() {
		m.Run(ctx, tick)
		close(done)
	}()
	for range 5 {
		tick <- probeNow
		if r, ok := m.Latest(); !ok || !r.Healthy {
			t.Errorf("Latest = %+v, %v", r, ok)
		}
	}
	cancel()
	<-done
}

func TestMonitorLogsHealthChanges(t *testing.T) {
	inst := healthy()
	env, m := newMonitorEnv(t, map[string]*fakeInstance{"asa1": inst})
	env.round(m)
	env.round(m)
	inst.rconOK = false
	env.round(m)
	env.round(m)
	inst.rconOK = true
	env.round(m)
	want := []string{
		"Health: healthy",
		"Health: unhealthy: asa1 rcon: Error: RCON command timed out",
		"Health: healthy",
	}
	if !slices.Equal(env.logs, want) {
		t.Fatalf("logs =\n %q\nwant\n %q", env.logs, want)
	}
}

// TestMonitorReportGolden pins the JSON shape of design.md decision 12.
func TestMonitorReportGolden(t *testing.T) {
	env, m := newMonitorEnv(t, map[string]*fakeInstance{"asa1": hung()})
	env.schedules = []CheckResult{{Name: "nightly-asa1", OK: true, Detail: "running"}}
	var r Report
	for range 5 {
		r = env.round(m)
	}
	got, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	want := `{
  "time": "2026-11-21T12:04:00Z",
  "healthy": false,
  "instances": {
    "asa1": {
      "state": "running",
      "checks": {
        "container": "ok",
        "rcon": "fail",
        "ready": "ok",
        "udp": "ok"
      },
      "details": {
        "rcon": "Error: RCON command timed out"
      }
    }
  },
  "schedules": {
    "nightly-asa1": "ok"
  },
  "schedule_details": {},
  "restarts": [
    {
      "instance": "asa1",
      "time": "2026-11-21T12:04:00Z",
      "reason": "rcon"
    }
  ]
}`
	if string(got) != want {
		t.Fatalf("report JSON =\n%s\nwant\n%s", got, want)
	}
}
