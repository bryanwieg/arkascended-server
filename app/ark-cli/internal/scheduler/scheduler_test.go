package scheduler

// Tests: run scheduler start/stop on a cancelled context; a running backup keeps an uncancelled context

import (
	"context"
	"strings"
	"testing"

	"ark-cli/internal/core"
	"ark-cli/internal/testutil"

	"pkg/ark/schedule"
)

func withSchedulerDeps(t *testing.T, ctx context.Context, svc *schedule.Service, backup func(string, []string) error, fn func()) {
	t.Helper()
	testutil.WithPatchedAppDeps(t, testutil.FullConfig, func(d *core.AppDeps) {
		d.Ctx = ctx
		d.ScheduleService = svc
		d.RunBackupFromSchedule = backup
	}, fn)
}

func TestRunSchedulerReturnsOnCancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	svc := schedule.New(t.TempDir(), nil)
	out := testutil.CaptureStdout(t, func() {
		withSchedulerDeps(t, ctx, svc, nil, func() {
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
