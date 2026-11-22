package scheduler

// Tests: check scheduler with a missing, unreadable, stale, healthy and unhealthy report (exit and output),
// the --json shape, and the warning when the compose file's scheduler image is not this binary's.

import (
	"encoding/json"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"ark-cli/internal/testutil"

	"pkg/ark/health"
	"pkg/ark/schedule"
)

// newCheckFixture is a compose dir with instance asa1 and a scheduler running image; the binary is a plain file.
func newCheckFixture(t *testing.T, image string) (*enableFixture, string) {
	t.Helper()
	binary := filepath.Join(t.TempDir(), "ark-cli")
	if err := os.WriteFile(binary, []byte("ark-cli build"), 0o755); err != nil {
		t.Fatal(err)
	}
	f := newEnableFixture(t, binary)
	text := "services:\n  asa1:\n    image: ark-server:abc123\n  ark-scheduler:\n    image: " + image + "\n    labels:\n      ark.role: scheduler\n"
	if err := os.WriteFile(f.file, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
	return f, binary
}

func writeHealthReport(t *testing.T, f *enableFixture, r health.Report) {
	t.Helper()
	data, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	path := healthPath(f.file)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
}

func sampleReport(healthy bool) health.Report {
	rcon := health.CheckOK
	details := map[string]string(nil)
	restarts := []health.RestartRecord{}
	if !healthy {
		rcon = health.CheckFail
		details = map[string]string{"rcon": "RCON timed out"}
		restarts = []health.RestartRecord{{Instance: "asa1", Time: time.Now().Add(-10 * time.Minute), Reason: "rcon"}}
	}
	return health.Report{
		Time:    time.Now().Add(-time.Minute),
		Healthy: healthy,
		Instances: map[string]health.InstanceReport{"asa1": {
			State:   "running",
			Checks:  map[string]string{"container": health.CheckOK, "ready": health.CheckOK, "rcon": rcon, "udp": health.CheckOK},
			Details: details,
		}},
		Schedules:       map[string]string{"nightly-asa1": health.CheckOK},
		ScheduleDetails: map[string]string{},
		Restarts:        restarts,
	}
}

func TestCheckSchedulerReportErrors(t *testing.T) {
	for _, tc := range []struct {
		name, content, want string
	}{
		{"missing", "", "Error: no scheduler health report at "},
		{"unreadable", "{", "Error: unreadable scheduler health report: "},
		{"stale", `{"time": "2026-01-01T00:00:00Z", "healthy": true}`, "Error: scheduler health report is stale (last update 2026-01-01T00:00:00Z)"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f, _ := newCheckFixture(t, "ark-scheduler:000000000000")
			path := healthPath(f.file)
			if tc.content != "" {
				if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, []byte(tc.content), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			var err error
			testutil.CaptureStderr(t, func() { err = f.run(t, RunCheckScheduler) })
			if err == nil || !strings.HasPrefix(err.Error(), tc.want) {
				t.Fatalf("err = %v, want prefix %q", err, tc.want)
			}
			if tc.name == "missing" && !strings.Contains(err.Error(), path+"; is the scheduler running?") {
				t.Fatalf("err = %v, want the path %s", err, path)
			}
		})
	}
}

func TestCheckSchedulerHealthyAndUnhealthy(t *testing.T) {
	for _, healthy := range []bool{true, false} {
		f, _ := newCheckFixture(t, "ark-scheduler:000000000000")
		writeHealthReport(t, f, sampleReport(healthy))
		var err error
		out := testutil.CaptureStdout(t, func() {
			testutil.CaptureStderr(t, func() { err = f.run(t, RunCheckScheduler) })
		})
		wants := []string{"INSTANCE  STATE    CHECK", "asa1      running  container", "-         -        nightly-asa1  ok"}
		if healthy {
			if err != nil {
				t.Fatalf("healthy: err = %v", err)
			}
		} else {
			if err == nil || err.Error() != "Error: scheduler reports unhealthy" {
				t.Fatalf("unhealthy: err = %v", err)
			}
			wants = append(wants, "rcon          fail    RCON timed out", "Restarted asa1 at ")
		}
		for _, want := range wants {
			if !strings.Contains(out, want) {
				t.Fatalf("healthy=%v: output missing %q:\n%s", healthy, want, out)
			}
		}
	}
}

func TestCheckSchedulerJSON(t *testing.T) {
	f, _ := newCheckFixture(t, "ark-scheduler:000000000000")
	writeHealthReport(t, f, sampleReport(true))
	var err error
	out := testutil.CaptureStdout(t, func() {
		testutil.CaptureStderr(t, func() { err = f.run(t, RunCheckScheduler, "--json") })
	})
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, out)
	}
	keys := slices.Sorted(maps.Keys(got))
	if want := []string{"healthy", "instances", "restarts", "schedule_details", "schedules", "time"}; !slices.Equal(keys, want) {
		t.Fatalf("keys = %v, want %v", keys, want)
	}
	if rcon := got["instances"].(map[string]any)["asa1"].(map[string]any)["checks"].(map[string]any)["rcon"]; rcon != "ok" {
		t.Fatalf("instances.asa1.checks.rcon = %v", rcon)
	}
}

func TestCheckSchedulerWarnsOnImageMismatch(t *testing.T) {
	f, binary := newCheckFixture(t, "ark-scheduler:000000000000")
	ref, err := schedule.ImageRef(binary)
	if err != nil {
		t.Fatal(err)
	}
	writeHealthReport(t, f, sampleReport(true))
	stderr := testutil.CaptureStderr(t, func() {
		testutil.CaptureStdout(t, func() { err = f.run(t, RunCheckScheduler) })
	})
	want := "Warning: the scheduler runs ark-scheduler:000000000000; run 'ark-cli enable scheduler' to update it to " + ref
	if err != nil || !strings.Contains(stderr, want) {
		t.Fatalf("err = %v, stderr = %q, want %q", err, stderr, want)
	}

	// The same image, with ark-cli started through a symlink: no warning.
	g, binary := newCheckFixture(t, ref)
	link := filepath.Join(t.TempDir(), "ark-cli-link")
	if err := os.Symlink(binary, link); err != nil {
		t.Fatal(err)
	}
	executablePath = func() (string, error) { return link, nil }
	writeHealthReport(t, g, sampleReport(true))
	stderr = testutil.CaptureStderr(t, func() {
		testutil.CaptureStdout(t, func() { err = g.run(t, RunCheckScheduler) })
	})
	if err != nil || strings.Contains(stderr, "Warning") {
		t.Fatalf("err = %v, stderr = %q", err, stderr)
	}
}

func TestCheckSchedulerNoImageWarningForUnmanagedImage(t *testing.T) {
	f, _ := newCheckFixture(t, "ark-scheduler:dev") // the dev compose's image; enable scheduler would replace it
	writeHealthReport(t, f, sampleReport(true))
	var err error
	stderr := testutil.CaptureStderr(t, func() {
		testutil.CaptureStdout(t, func() { err = f.run(t, RunCheckScheduler) })
	})
	if err != nil || strings.Contains(stderr, "Warning") {
		t.Fatalf("err = %v, stderr = %q", err, stderr)
	}
}
