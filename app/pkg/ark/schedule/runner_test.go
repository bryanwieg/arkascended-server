package schedule

import (
	"context"
	"errors"
	"fmt"
	"os"
	"slices"
	"sync"
	"testing"
	"time"
)

// recorder is a RunOne that records schedule names.
type recorder struct {
	mu   sync.Mutex
	runs []string
	logs []string
}

func (rec *recorder) runOne(_ context.Context, s BackupSchedule) error {
	rec.mu.Lock()
	defer rec.mu.Unlock()
	rec.runs = append(rec.runs, s.Name)
	return nil
}

func (rec *recorder) logf(format string, args ...any) {
	rec.mu.Lock()
	defer rec.mu.Unlock()
	rec.logs = append(rec.logs, fmt.Sprintf(format, args...))
}

func (rec *recorder) names() []string {
	rec.mu.Lock()
	defer rec.mu.Unlock()
	return slices.Clone(rec.runs)
}

func runnerWith(t *testing.T, schedules ...BackupSchedule) (*Runner, *recorder) {
	t.Helper()
	svc := testService(t)
	if err := svc.SaveScheduleFile(ScheduleFile{Version: 1, Schedules: schedules}); err != nil {
		t.Fatal(err)
	}
	rec := &recorder{}
	return &Runner{Service: svc, RunOne: rec.runOne, Logf: rec.logf}, rec
}

func backupAt(name, cronExpr string) BackupSchedule {
	return BackupSchedule{Name: name, Type: "backup", Instance: "main", Enabled: true, Cron: cronExpr}
}

func utc(hhmm string) time.Time {
	tm, err := time.Parse("2006-01-02 15:04", "2026-11-21 "+hhmm)
	if err != nil {
		panic(err)
	}
	return tm
}

// withLocal sets time.Local for one test.
func withLocal(t *testing.T, loc *time.Location) {
	t.Helper()
	old := time.Local
	time.Local = loc
	t.Cleanup(func() { time.Local = old })
}

func TestStepRunsOnlyDueSchedules(t *testing.T) {
	withLocal(t, time.UTC)
	r, rec := runnerWith(t, backupAt("hourly", "0 * * * *"), backupAt("nightly", "0 3 * * *"))
	next := r.Step(context.Background(), utc("09:30"), utc("10:00"))
	if !next.Equal(utc("10:00")) {
		t.Fatalf("Step returned %v, want now", next)
	}
	r.Step(context.Background(), utc("10:00"), utc("10:59"))
	if got := rec.names(); !slices.Equal(got, []string{"hourly"}) {
		t.Fatalf("runs = %v, want [hourly]", got)
	}
}

func TestStepRunsInNameOrderAndSkipsDisabled(t *testing.T) {
	withLocal(t, time.UTC)
	r, rec := runnerWith(t)
	// A hand-edited file need not be sorted.
	if err := os.WriteFile(r.Service.ConfigPath, []byte(`version: 1
schedules:
  - {name: c, type: backup, instance: main, enabled: true, cron: "* * * * *"}
  - {name: b-off, type: backup, instance: main, enabled: false, cron: "* * * * *"}
  - {name: a, type: backup, instance: main, enabled: true, cron: "* * * * *"}
`), 0o644); err != nil {
		t.Fatal(err)
	}
	r.Step(context.Background(), utc("10:00"), utc("10:01"))
	if got := rec.names(); !slices.Equal(got, []string{"a", "c"}) {
		t.Fatalf("runs = %v, want [a c]", got)
	}
}

func TestStepSkipsInvalidCron(t *testing.T) {
	withLocal(t, time.UTC)
	r, rec := runnerWith(t, backupAt("bad", "@every 1m"), backupAt("good", "* * * * *"))
	r.Step(context.Background(), utc("10:00"), utc("10:01"))
	if got := rec.names(); !slices.Equal(got, []string{"good"}) {
		t.Fatalf("runs = %v, want [good]", got)
	}
	if len(rec.logs) != 1 {
		t.Fatalf("logs = %v, want one invalid-cron line", rec.logs)
	}
}

func TestStepLoadErrorRunsNothingAndDoesNotBurst(t *testing.T) {
	withLocal(t, time.UTC)
	r, rec := runnerWith(t, backupAt("hourly", "0 * * * *"))
	good, err := os.ReadFile(r.Service.ConfigPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(r.Service.ConfigPath, []byte("schedules: [unclosed"), 0o644); err != nil {
		t.Fatal(err)
	}
	prev := r.Step(context.Background(), utc("09:59"), utc("10:00"))
	if len(rec.names()) != 0 || len(rec.logs) != 1 {
		t.Fatalf("runs = %v, logs = %v; want no run and one log line", rec.names(), rec.logs)
	}
	if err := os.WriteFile(r.Service.ConfigPath, good, 0o644); err != nil {
		t.Fatal(err)
	}
	r.Step(context.Background(), prev, utc("10:01"))
	if got := rec.names(); len(got) != 0 {
		t.Fatalf("runs = %v, want none (the 10:00 slot fell in the failed step)", got)
	}
}

func TestStepEvaluatesCronInLocalTime(t *testing.T) {
	withLocal(t, time.FixedZone("UTC+5", 5*3600))
	r, rec := runnerWith(t, backupAt("nightly", "0 3 * * *"))
	// 21:30–22:01 UTC is 02:30–03:01 at UTC+5.
	r.Step(context.Background(), utc("21:30"), utc("22:01"))
	if got := rec.names(); !slices.Equal(got, []string{"nightly"}) {
		t.Fatalf("runs = %v, want [nightly]", got)
	}
}

func TestCatchUp(t *testing.T) {
	withLocal(t, time.UTC)
	r, rec := runnerWith(t,
		backupAt("missed", "0 3 * * *"),
		backupAt("never-ran", "0 3 * * *"),
		backupAt("on-time", "0 3 * * *"),
		backupAt("killed", "0 3 * * *"),
		backupAt("still-running", "0 3 * * *"),
	)
	svc := r.Service
	svc.WriteRunState(RunState{Name: "missed", LastStartedAt: "2026-11-20T03:00:00Z", LastStatus: "success"})
	svc.WriteRunState(RunState{Name: "on-time", LastStartedAt: "2026-11-21T03:00:00Z", LastStatus: "success"})
	svc.WriteRunState(RunState{Name: "killed", LastStartedAt: "2026-11-21T03:00:00Z", LastStatus: "running"})
	svc.WriteRunState(RunState{Name: "still-running", LastStartedAt: "2026-11-21T03:00:00Z", LastStatus: "running"})
	release, err := svc.AcquireLock("still-running")
	if err != nil {
		t.Fatal(err)
	}
	defer release()

	r.CatchUp(context.Background(), utc("10:00"))
	if got := rec.names(); !slices.Equal(got, []string{"killed", "missed"}) {
		t.Fatalf("runs = %v, want [killed missed]", got)
	}
}

func TestRunCatchesUpOnceThenSteps(t *testing.T) {
	withLocal(t, time.UTC)
	r, rec := runnerWith(t, backupAt("hourly", "0 * * * *"))
	r.Service.WriteRunState(RunState{Name: "hourly", LastStartedAt: "2026-11-21T08:00:00Z", LastStatus: "success"})
	calls := 0 // Run's goroutine is the only caller of Now
	r.Now = func() time.Time {
		calls++
		if calls == 1 {
			return utc("09:30") // start: catch up the 09:00 slot
		}
		return utc("10:00")
	}

	ctx, cancel := context.WithCancel(context.Background())
	tick := make(chan time.Time)
	done := make(chan struct{})
	go func() { r.Run(ctx, tick); close(done) }()

	tick <- time.Time{} // the tick value is ignored
	tick <- time.Time{} // second tick at the same time: nothing new is due
	cancel()
	<-done
	if got := rec.names(); !slices.Equal(got, []string{"hourly", "hourly"}) {
		t.Fatalf("runs = %v, want catch-up then the 10:00 run", got)
	}
}

func TestRunCancelWaitsForRunInProgress(t *testing.T) {
	withLocal(t, time.UTC)
	r, _ := runnerWith(t, backupAt("a", "* * * * *"), backupAt("b", "* * * * *"))
	calls := 0 // Run's goroutine is the only caller of Now
	r.Now = func() time.Time {
		calls++
		if calls == 1 {
			return utc("10:00") // CatchUp at start; no state, so no run
		}
		return utc("10:01")
	}
	started := make(chan struct{})
	finish := make(chan struct{})
	var ran []string
	var runCtxErr error
	r.RunOne = func(ctx context.Context, s BackupSchedule) error {
		ran = append(ran, s.Name)
		close(started)
		<-finish
		runCtxErr = ctx.Err()
		return errors.New("boom")
	}

	ctx, cancel := context.WithCancel(context.Background())
	tick := make(chan time.Time, 1)
	done := make(chan struct{})
	go func() { r.Run(ctx, tick); close(done) }()
	tick <- time.Time{}
	<-started
	cancel()
	select {
	case <-done:
		t.Fatal("Run returned while a run was in progress")
	case <-time.After(20 * time.Millisecond):
	}
	close(finish)
	<-done
	if !slices.Equal(ran, []string{"a"}) {
		t.Fatalf("runs = %v, want only [a] (b is skipped after cancel)", ran)
	}
	if runCtxErr != nil {
		t.Fatalf("RunOne context error = %v, want nil after cancel", runCtxErr)
	}
}
