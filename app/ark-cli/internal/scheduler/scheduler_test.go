package scheduler

// Tests: run scheduler start/stop on a cancelled context (both loops and the health endpoint); a taken health
// port fails the command; a running backup keeps an uncancelled context; the monitor's restart holds the
// instance lock around stop (2m timeout) and start, starts after a failed stop, and is busy when another
// holder has the lock

import (
	"context"
	"errors"
	"io/fs"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"ark-cli/internal/core"
	"ark-cli/internal/testutil"

	"pkg/ark"
	"pkg/ark/arktest"
	"pkg/ark/backup"
	"pkg/ark/health"
	"pkg/ark/lifecycle"
	"pkg/ark/schedule"
)

// withSchedulerDeps runs fn with a compose file holding instance asa1 and the health endpoint on a free port;
// it returns the compose file.
func withSchedulerDeps(t *testing.T, ctx context.Context, svc *schedule.Service, backup func(string, []string) error, fn func()) string {
	t.Helper()
	file := filepath.Join(t.TempDir(), "docker-compose.yaml")
	if err := os.WriteFile(file, []byte("services:\n  asa1:\n    image: ark-server:abc123\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	testutil.SetEnv(t, "COMPOSE_FILE", file)
	testutil.SetEnv(t, "ARK_BACKUP_DIR", "")
	orig := healthListenAddr
	healthListenAddr = "127.0.0.1:0"
	t.Cleanup(func() { healthListenAddr = orig })
	testutil.WithPatchedAppDeps(t, testutil.FullConfig, func(d *core.AppDeps) {
		d.Ctx = ctx
		d.ScheduleService = svc
		d.RunBackupFromSchedule = backup
	}, fn)
	return file
}

func TestRunSchedulerReturnsOnCancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	svc := schedule.New(t.TempDir(), nil)
	var file string
	out := testutil.CaptureStdout(t, func() {
		file = withSchedulerDeps(t, ctx, svc, nil, func() {
			if err := RunRunScheduler(core.CurrentAppDeps(), nil); err != nil {
				t.Fatalf("RunRunScheduler: %v", err)
			}
		})
	})
	for _, want := range []string{"Scheduler started (TZ ", "schedules " + svc.ConfigPath, "Scheduler stopped"} {
		if !strings.Contains(out, want) {
			t.Fatalf("output missing %q:\n%s", want, out)
		}
	}
	// The monitor's round on the cancelled context published nothing.
	if _, err := health.LoadReport(healthPath(file)); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("health report after a cancelled round: %v", err)
	}
}

func TestRunSchedulerFailsWhenHealthPortIsTaken(t *testing.T) {
	taken, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer taken.Close()
	withSchedulerDeps(t, context.Background(), schedule.New(t.TempDir(), nil), nil, func() {
		healthListenAddr = taken.Addr().String()
		err = RunRunScheduler(core.CurrentAppDeps(), nil)
	})
	if want := "Error: health endpoint cannot listen on " + taken.Addr().String() + ": "; err == nil || !strings.HasPrefix(err.Error(), want) {
		t.Fatalf("err = %v, want prefix %q", err, want)
	}
}

// restartCalls records the lifecycle calls of a monitor restart and whether the instance lock was held.
type restartCalls struct {
	stopErr error
	stops   []time.Duration
	ups     []ark.UpOptions
	held    []bool
}

func restartDeps(t *testing.T, lockDir string, calls *restartCalls) *core.AppDeps {
	t.Helper()
	lockHeld := func() bool {
		held, err := backup.InstanceLockHeld(lockDir, "asa1")
		return err == nil && held
	}
	mock := &arktest.MockCompose{
		StopFn: func(_ context.Context, _ string, services []string, timeout *time.Duration) error {
			if !slices.Equal(services, []string{"asa1"}) || timeout == nil {
				t.Errorf("stop services = %v, timeout = %v", services, timeout)
				return nil
			}
			calls.stops = append(calls.stops, *timeout)
			calls.held = append(calls.held, lockHeld())
			return calls.stopErr
		},
		UpFn: func(_ context.Context, _ string, services []string, opts ark.UpOptions) error {
			if !slices.Equal(services, []string{"asa1"}) {
				t.Errorf("up services = %v", services)
			}
			calls.ups = append(calls.ups, opts)
			calls.held = append(calls.held, lockHeld())
			return nil
		},
	}
	return &core.AppDeps{ComposeService: mock, Lifecycle: lifecycle.New(mock, &testutil.MockHealthChecker{}, slog.Default())}
}

func TestMonitorRestartHoldsInstanceLock(t *testing.T) {
	testutil.SetEnv(t, "ARK_BACKUP_DIR", "")
	file := filepath.Join(t.TempDir(), "docker-compose.yaml")
	lockDir := backup.InstanceLockDir(core.BackupRootDirForComposeFile(file), "asa1")
	calls := &restartCalls{}
	m := newMonitor(restartDeps(t, lockDir, calls), file, time.Now(), t.Logf)

	if err := m.Restart(context.Background(), "asa1"); err != nil {
		t.Fatalf("Restart: %v", err)
	}
	if !slices.Equal(calls.stops, []time.Duration{2 * time.Minute}) || len(calls.ups) != 1 || calls.ups[0].PullPolicy != "never" {
		t.Fatalf("stops = %v, ups = %+v", calls.stops, calls.ups)
	}
	if !slices.Equal(calls.held, []bool{true, true}) {
		t.Fatalf("lock held during stop, start = %v", calls.held)
	}
	if held, err := backup.InstanceLockHeld(lockDir, "asa1"); err != nil || held {
		t.Fatalf("lock after restart held = %v, %v", held, err)
	}

	// Another holder (a backup, a lifecycle command): busy, nothing stopped.
	release, err := backup.AcquireInstanceLock(lockDir, "asa1", 0)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	busy := &restartCalls{}
	m = newMonitor(restartDeps(t, lockDir, busy), file, time.Now(), t.Logf)
	if err := m.Restart(context.Background(), "asa1"); !errors.Is(err, health.ErrInstanceBusy) {
		t.Fatalf("Restart with the lock held = %v, want ErrInstanceBusy", err)
	}
	if len(busy.stops)+len(busy.ups) != 0 {
		t.Fatalf("busy restart called stop %v, up %v", busy.stops, busy.ups)
	}
}

func TestMonitorRestartStartsAfterFailedStop(t *testing.T) {
	testutil.SetEnv(t, "ARK_BACKUP_DIR", "")
	file := filepath.Join(t.TempDir(), "docker-compose.yaml")
	lockDir := backup.InstanceLockDir(core.BackupRootDirForComposeFile(file), "asa1")
	stopErr := errors.New("stop timed out")
	calls := &restartCalls{stopErr: stopErr}
	m := newMonitor(restartDeps(t, lockDir, calls), file, time.Now(), t.Logf)
	if err := m.Restart(context.Background(), "asa1"); !errors.Is(err, stopErr) {
		t.Fatalf("Restart = %v, want the stop error", err)
	}
	if len(calls.ups) != 1 {
		t.Fatalf("ups = %+v, want a start after the failed stop", calls.ups)
	}
}

func TestRunSchedulerSignalDoesNotCancelRunningBackup(t *testing.T) {
	svc := schedule.New(t.TempDir(), nil)
	s := schedule.BackupSchedule{Name: "nightly", Type: "backup", Instance: "asa1", Enabled: true, Cron: "0 3 * * *"}
	if err := svc.SaveScheduleFile(schedule.ScheduleFile{Version: 1, Schedules: []schedule.BackupSchedule{s}}); err != nil {
		t.Fatal(err)
	}
	// Left "running" with its lock free, so the start-up catch-up reruns it.
	svc.WriteRunState(schedule.RunState{Name: s.Name, LastStartedAt: "2026-11-21T03:00:00Z", LastStatus: "running"})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	called := false
	var backupCtxErr error
	testutil.CaptureStdout(t, func() {
		withSchedulerDeps(t, ctx, svc, func(string, []string) error {
			called = true
			cancel() // the signal arrives while the backup runs
			backupCtxErr = core.CurrentAppDeps().Context().Err()
			return nil
		}, func() {
			if err := RunRunScheduler(core.CurrentAppDeps(), nil); err != nil {
				t.Fatalf("RunRunScheduler: %v", err)
			}
		})
	})
	if !called {
		t.Fatal("catch-up did not run the backup")
	}
	if backupCtxErr != nil {
		t.Fatalf("backup context cancelled by the signal: %v", backupCtxErr)
	}
	if state, _ := svc.LoadRunState(s.Name); state.LastStatus != "success" {
		t.Fatalf("state = %+v, want success", state)
	}
}

// TestUntilNextMinute verifies the schedule runner wakes on wall-clock minute starts.
// - Expect: the wait ends at the next hh:mm:00, a full minute when already on one.
func TestUntilNextMinute(t *testing.T) {
	base := time.Date(2026, 11, 22, 3, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		now  time.Time
		want time.Duration
	}{
		{base.Add(59 * time.Second), time.Second},
		{base.Add(1500 * time.Millisecond), 58500 * time.Millisecond},
		{base, time.Minute},
	} {
		if got := untilNextMinute(tc.now); got != tc.want {
			t.Errorf("untilNextMinute(%s) = %s, want %s", tc.now.Format("15:04:05.000"), got, tc.want)
		}
	}
}
