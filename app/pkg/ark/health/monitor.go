package health

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"pkg/ark"
)

const (
	HungRoundsBeforeRestart = 5
	MaxRestartsPerHour      = 2
)

// ReportStaleAfter is the age after which a report means the monitor is stuck; a restart round
// (2 min stop + start + probes) must fit with margin.
const ReportStaleAfter = 5 * time.Minute

// StateBusy is the state of an instance whose lock is held (backup, lifecycle command, rollout).
const StateBusy = "busy"

// Check values in a Report.
const (
	CheckOK      = "ok"
	CheckFail    = "fail"
	CheckSkipped = "skipped"
)

// ErrInstanceBusy is returned by Monitor.Restart when the instance lock is held.
var ErrInstanceBusy = errors.New("instance lock held")

// instanceChecks are the checks every InstanceReport lists.
var instanceChecks = []string{"container", "ready", "rcon", "udp"}

// Report is the serialized health (file, HTTP, check scheduler --json). Maps keep it addressable by
// Gatus JSON paths: [BODY].instances.<name>.checks.rcon.
type Report struct {
	Time            time.Time                 `json:"time"`
	Healthy         bool                      `json:"healthy"`
	Error           string                    `json:"error,omitempty"` // compose project could not be loaded/listed
	Instances       map[string]InstanceReport `json:"instances"`
	Schedules       map[string]string         `json:"schedules"`        // name → "ok" | "fail"
	ScheduleDetails map[string]string         `json:"schedule_details"` // always present, {} when empty
	Restarts        []RestartRecord           `json:"restarts"`         // last hour; always present, [] when empty (Gatus len())
}

// InstanceReport is one instance's entry in a Report.
type InstanceReport struct {
	State   string            `json:"state"`
	Checks  map[string]string `json:"checks"`            // always all of container, ready, rcon, udp → "ok" | "fail" | "skipped"
	Details map[string]string `json:"details,omitempty"` // check → detail, failed checks only
}

// RestartRecord is one automatic restart.
type RestartRecord struct {
	Instance string    `json:"instance"`
	Time     time.Time `json:"time"`
	Reason   string    `json:"reason"`
	Error    string    `json:"error,omitempty"`
}

// Monitor runs health rounds over every instance and schedule and restarts hung instances.
// Round and Run must be called from one goroutine; Latest is safe from any.
type Monitor struct {
	Probe       *Probe
	ComposeFile string
	LoadProject func(ctx context.Context) (*ark.ComposeProject, error)
	Schedules   func(now time.Time) []CheckResult                // wraps schedule.Service.Health
	LockHeld    func(instance string) (bool, error)              // an error counts as held
	Restart     func(ctx context.Context, instance string) error // takes the instance lock itself; ErrInstanceBusy when held
	HealthPath  string
	Now         func() time.Time
	Logf        func(format string, args ...any)

	mu        sync.Mutex
	latest    Report
	hasLatest bool

	hungRounds  map[string]int       // instance → consecutive hung rounds
	restarts    []RestartRecord      // last hour
	limitLogged map[string]time.Time // instance → last "restart limit reached" log
	lastSummary string               // health line logged last
}

// Run does a Round at start and on each tick until ctx is done.
func (m *Monitor) Run(ctx context.Context, tick <-chan time.Time) {
	m.Round(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick:
			m.Round(ctx)
		}
	}
}

// Latest returns the last completed round's report; false before the first round.
func (m *Monitor) Latest() (Report, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.latest, m.hasLatest
}

// Round checks every instance and schedule, publishes the report and restarts at most one hung instance.
func (m *Monitor) Round(ctx context.Context) Report {
	if m.hungRounds == nil {
		m.hungRounds = map[string]int{}
		m.limitLogged = map[string]time.Time{}
	}
	now := m.Now()
	m.restarts = slices.DeleteFunc(m.restarts, func(r RestartRecord) bool { return now.Sub(r.Time) >= time.Hour })

	healths, err := m.probeAll(ctx)
	m.countHung(healths)
	report := m.buildReport(now, healths, err)
	m.logChange(report)
	m.publish(report)

	inst, ok := m.restartCandidate(healths, now)
	if !ok {
		return report
	}
	rec, ok := m.restart(ctx, inst)
	if !ok {
		return report
	}
	m.restarts = append(m.restarts, rec)
	restarted := report
	restarted.Restarts = slices.Clone(m.restarts)
	m.publish(restarted)
	return restarted
}

// probeAll probes each instance in name order; a locked instance is busy and not probed.
func (m *Monitor) probeAll(ctx context.Context) ([]InstanceHealth, error) {
	project, err := m.LoadProject(ctx)
	if err != nil {
		return nil, err
	}
	containers, err := m.Probe.Compose.ServiceContainers(ctx, m.ComposeFile)
	if err != nil {
		return nil, err
	}
	healths := []InstanceHealth{}
	for _, name := range project.ServiceNames() {
		if held, err := m.LockHeld(name); err != nil || held {
			healths = append(healths, InstanceHealth{Instance: name, State: StateBusy})
			continue
		}
		c, found := containers[name]
		healths = append(healths, m.Probe.Check(ctx, m.ComposeFile, name, project.Project.Services[name], c, found))
	}
	return healths, nil
}

// countHung updates the consecutive hung-round counters; busy rounds leave them unchanged.
func (m *Monitor) countHung(healths []InstanceHealth) {
	for _, h := range healths {
		switch {
		case h.State == StateBusy:
		case h.Hung():
			m.hungRounds[h.Instance]++
		default:
			delete(m.hungRounds, h.Instance)
		}
	}
}

func (m *Monitor) buildReport(now time.Time, healths []InstanceHealth, err error) Report {
	report := Report{
		Time:            now,
		Instances:       map[string]InstanceReport{},
		Schedules:       map[string]string{},
		ScheduleDetails: map[string]string{},
		Restarts:        slices.Clone(m.restarts),
	}
	if report.Restarts == nil {
		report.Restarts = []RestartRecord{}
	}
	if err != nil {
		report.Error = err.Error()
	}
	for _, h := range healths {
		report.Instances[h.Instance] = instanceReport(h)
	}
	for _, s := range m.Schedules(now) {
		if s.OK {
			report.Schedules[s.Name] = CheckOK
		} else {
			report.Schedules[s.Name] = CheckFail
			report.ScheduleDetails[s.Name] = s.Detail
		}
	}
	report.Healthy = len(problems(report)) == 0
	return report
}

func instanceReport(h InstanceHealth) InstanceReport {
	checks := map[string]string{}
	for _, name := range instanceChecks {
		checks[name] = CheckSkipped
	}
	if h.State == "running" {
		checks["container"] = CheckOK
	}
	var details map[string]string
	for _, c := range h.Checks {
		if c.OK {
			checks[c.Name] = CheckOK
			continue
		}
		checks[c.Name] = CheckFail
		if details == nil {
			details = map[string]string{}
		}
		details[c.Name] = c.Detail
	}
	return InstanceReport{State: h.State, Checks: checks, Details: details}
}

// problems lists what makes a report unhealthy, in a stable order.
func problems(r Report) []string {
	var out []string
	if r.Error != "" {
		out = append(out, "compose: "+r.Error)
	}
	for _, name := range sortedKeys(r.Instances) {
		inst := r.Instances[name]
		for _, check := range instanceChecks {
			if inst.Checks[check] == CheckFail {
				out = append(out, name+" "+check+": "+inst.Details[check])
			}
		}
	}
	for _, name := range sortedKeys(r.Schedules) {
		if r.Schedules[name] != CheckOK {
			out = append(out, "schedule "+name+": "+r.ScheduleDetails[name])
		}
	}
	return out
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// logChange logs the health line when it differs from the previous round's.
func (m *Monitor) logChange(r Report) {
	summary := "Health: healthy"
	if p := problems(r); len(p) > 0 {
		summary = "Health: unhealthy: " + strings.Join(p, "; ")
	}
	if summary != m.lastSummary {
		m.Logf("%s", summary)
	}
	m.lastSummary = summary
}

// publish stores the report for Latest and writes it to HealthPath atomically.
func (m *Monitor) publish(r Report) {
	m.mu.Lock()
	m.latest, m.hasLatest = r, true
	m.mu.Unlock()
	if err := writeReport(m.HealthPath, r); err != nil {
		m.Logf("Warning: cannot write health report %s: %v", m.HealthPath, err)
	}
}

func writeReport(path string, r Report) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, append(data, '\n'), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// LoadReport reads a report written by a Monitor.
func LoadReport(path string) (Report, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Report{}, err
	}
	var r Report
	if err := json.Unmarshal(data, &r); err != nil {
		return Report{}, err
	}
	return r, nil
}

// restartCandidate returns the first instance, in name order, hung long enough and under the hourly cap.
func (m *Monitor) restartCandidate(healths []InstanceHealth, now time.Time) (InstanceHealth, bool) {
	for _, h := range healths {
		if m.hungRounds[h.Instance] < HungRoundsBeforeRestart {
			continue
		}
		recent := 0
		for _, r := range m.restarts {
			if r.Instance == h.Instance {
				recent++
			}
		}
		if recent < MaxRestartsPerHour {
			return h, true
		}
		if last, ok := m.limitLogged[h.Instance]; !ok || now.Sub(last) >= time.Hour {
			m.Logf("Not restarting %s: restart limit reached", h.Instance)
			m.limitLogged[h.Instance] = now
		}
	}
	return InstanceHealth{}, false
}

// restart restarts a hung instance; false when it was busy (nothing recorded, counter kept).
func (m *Monitor) restart(ctx context.Context, h InstanceHealth) (RestartRecord, bool) {
	var failed []string
	for _, c := range h.Checks {
		if !c.OK {
			failed = append(failed, c.Name)
		}
	}
	reason := strings.Join(failed, ", ")
	m.Logf("Restarting %s: %s", h.Instance, reason)
	rec := RestartRecord{Instance: h.Instance, Time: m.Now(), Reason: reason}
	err := m.Restart(ctx, h.Instance)
	if errors.Is(err, ErrInstanceBusy) {
		m.Logf("Not restarting %s: busy", h.Instance)
		return RestartRecord{}, false
	}
	if err != nil {
		m.Logf("Warning: restart of %s failed: %v", h.Instance, err)
		rec.Error = err.Error()
	}
	delete(m.hungRounds, h.Instance)
	return rec, true
}
