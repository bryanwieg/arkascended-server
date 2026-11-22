package schedule

import (
	"os"
	"slices"
	"testing"
	"time"
)

func TestHealth(t *testing.T) {
	withLocal(t, time.UTC)
	now := time.Date(2026, 11, 21, 10, 0, 0, 0, time.UTC)
	weekAgo := now.Add(-7 * 24 * time.Hour)
	cases := []struct {
		name     string
		cron     string
		state    *RunState
		lock     bool
		mtime    time.Time
		since    time.Time
		wantOK   bool
		wantText string
	}{
		{name: "lock held", cron: "0 3 * * *", lock: true, state: &RunState{LastStatus: "failed"}, mtime: weekAgo, since: weekAgo, wantOK: true, wantText: "running"},
		{name: "running with lock free", cron: "0 3 * * *", state: &RunState{LastStatus: "running", LastStartedAt: "2026-11-21T03:00:00Z"}, mtime: weekAgo, since: weekAgo, wantText: "last run did not finish (started 2026-11-21T03:00:00Z)"},
		{name: "last run failed", cron: "0 3 * * *", state: &RunState{LastStatus: "failed", LastStartedAt: "2026-11-21T03:00:00Z", LastError: "disk full"}, mtime: weekAgo, since: weekAgo, wantText: "last run failed: disk full"},
		{name: "invalid cron", cron: "nope", mtime: weekAgo, since: weekAgo, wantText: "invalid cron"},
		{name: "ran on time", cron: "0 3 * * *", state: &RunState{LastStatus: "success", LastStartedAt: "2026-11-21T03:00:00Z"}, mtime: weekAgo, since: weekAgo, wantOK: true},
		{name: "overdue", cron: "0 3 * * *", state: &RunState{LastStatus: "success", LastStartedAt: "2026-11-19T03:00:00Z"}, mtime: weekAgo, since: weekAgo, wantText: "overdue since 2026-11-20T03:00:00Z"},
		{name: "within grace", cron: "0 9 * * *", state: &RunState{LastStatus: "success", LastStartedAt: "2026-11-20T09:00:00Z"}, mtime: weekAgo, since: weekAgo, wantOK: true},
		{name: "old run, file just modified", cron: "0 3 * * *", state: &RunState{LastStatus: "success", LastStartedAt: weekAgo.Format(time.RFC3339)}, mtime: now.Add(-time.Minute), since: weekAgo, wantOK: true},
		{name: "never run, measured from since", cron: "0 3 * * *", mtime: weekAgo, since: now.Add(-time.Hour), wantOK: true},
		{name: "never run, overdue from since", cron: "0 3 * * *", mtime: weekAgo, since: now.Add(-2 * 24 * time.Hour), wantText: "overdue since 2026-11-20T03:00:00Z"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc := testService(t)
			s := backupAt("nightly", tc.cron)
			if err := svc.SaveScheduleFile(ScheduleFile{Version: 1, Schedules: []BackupSchedule{s}}); err != nil {
				t.Fatal(err)
			}
			if err := os.Chtimes(svc.ConfigPath, tc.mtime, tc.mtime); err != nil {
				t.Fatal(err)
			}
			if tc.state != nil {
				st := *tc.state
				st.Name = s.Name
				svc.WriteRunState(st)
			}
			if tc.lock {
				release, err := svc.AcquireLock(s.Name)
				if err != nil {
					t.Fatal(err)
				}
				defer release()
			}
			want := []ScheduleHealth{{Name: "nightly", OK: tc.wantOK, Detail: tc.wantText}}
			if got := svc.Health(now, tc.since); !slices.Equal(got, want) {
				t.Fatalf("Health = %+v, want %+v", got, want)
			}
		})
	}
}

func TestHealthOnlyEnabledBackupsInNameOrder(t *testing.T) {
	withLocal(t, time.UTC)
	now := time.Date(2026, 11, 21, 10, 0, 0, 0, time.UTC)
	svc := testService(t)
	disabled := backupAt("disabled", "0 3 * * *")
	disabled.Enabled = false
	other := backupAt("other", "0 3 * * *")
	other.Type = "other"
	if err := svc.SaveScheduleFile(ScheduleFile{Version: 1, Schedules: []BackupSchedule{
		backupAt("zeta", "0 3 * * *"), disabled, other, backupAt("alpha", "0 3 * * *"),
	}}); err != nil {
		t.Fatal(err)
	}
	got := svc.Health(now, now)
	want := []ScheduleHealth{{Name: "alpha", OK: true}, {Name: "zeta", OK: true}}
	if !slices.Equal(got, want) {
		t.Fatalf("Health = %+v, want %+v", got, want)
	}
}

func TestHealthFileLoadError(t *testing.T) {
	svc := testService(t)
	if err := os.MkdirAll(svc.ConfigPath, 0o755); err != nil { // a directory cannot be read as the file
		t.Fatal(err)
	}
	got := svc.Health(time.Now(), time.Now())
	if len(got) != 1 || got[0].Name != "schedules" || got[0].OK || got[0].Detail == "" {
		t.Fatalf("Health = %+v, want one failed schedules entry", got)
	}
}
