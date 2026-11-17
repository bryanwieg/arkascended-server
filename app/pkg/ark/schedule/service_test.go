package schedule

import (
	"encoding/json"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func testService(t *testing.T) *Service {
	t.Helper()
	dir := t.TempDir()
	s := New(dir, discardLogger())
	return s
}

// --- Types ---

func TestScheduleFileDefaults(t *testing.T) {
	var f ScheduleFile
	if f.Version != 0 {
		t.Fatal("zero value should have version 0")
	}
}

// --- Store: load/save ---

func TestLoadScheduleFileNotExists(t *testing.T) {
	s := testService(t)
	file, err := s.LoadScheduleFile()
	if err != nil {
		t.Fatal(err)
	}
	if file.Version != 1 || len(file.Schedules) != 0 {
		t.Fatalf("expected empty v1 file, got: %+v", file)
	}
}

func TestSaveAndLoadScheduleFile(t *testing.T) {
	s := testService(t)
	file := ScheduleFile{
		Version: 1,
		Schedules: []BackupSchedule{
			{Name: "nightly", Type: "backup", Instance: "asa1", Enabled: true, Cron: "0 3 * * *"},
		},
	}
	if err := s.SaveScheduleFile(file); err != nil {
		t.Fatal(err)
	}
	loaded, err := s.LoadScheduleFile()
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Schedules) != 1 || loaded.Schedules[0].Name != "nightly" {
		t.Fatalf("unexpected loaded file: %+v", loaded)
	}
}

func TestSaveScheduleFileSortsByName(t *testing.T) {
	s := testService(t)
	file := ScheduleFile{
		Schedules: []BackupSchedule{
			{Name: "zebra", Type: "backup"},
			{Name: "alpha", Type: "backup"},
		},
	}
	if err := s.SaveScheduleFile(file); err != nil {
		t.Fatal(err)
	}
	loaded, err := s.LoadScheduleFile()
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Schedules[0].Name != "alpha" || loaded.Schedules[1].Name != "zebra" {
		t.Fatalf("not sorted: %v", loaded.Schedules)
	}
}

func TestSaveScheduleFileSetsVersionDefault(t *testing.T) {
	s := testService(t)
	if err := s.SaveScheduleFile(ScheduleFile{}); err != nil {
		t.Fatal(err)
	}
	loaded, err := s.LoadScheduleFile()
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Version != 1 {
		t.Fatalf("expected version 1, got %d", loaded.Version)
	}
}

// --- Store: crontab ---

func TestWriteManagedScheduleCrontab(t *testing.T) {
	s := testService(t)
	file := ScheduleFile{
		Schedules: []BackupSchedule{
			{Name: "nightly", Type: "backup", Instance: "asa1", Enabled: true, Cron: "0 3 * * *", Format: "tar.gz"},
			{Name: "disabled", Type: "backup", Instance: "asa2", Enabled: false, Cron: "0 4 * * *"},
		},
	}
	if err := s.WriteManagedScheduleCrontab(file); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(s.ManagedCrontabPath)
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	if !strings.Contains(content, "# ark-cli managed schedules") {
		t.Fatal("missing header")
	}
	if !strings.Contains(content, "nightly") {
		t.Fatal("missing nightly schedule")
	}
	if strings.Contains(content, "disabled") {
		t.Fatal("disabled schedule should not appear in crontab")
	}
}

func TestSaveScheduleAndCrontab(t *testing.T) {
	s := testService(t)
	file := ScheduleFile{
		Schedules: []BackupSchedule{
			{Name: "hourly", Type: "backup", Instance: "asa1", Enabled: true, Cron: "0 * * * *"},
		},
	}
	if err := s.SaveScheduleAndCrontab(file); err != nil {
		t.Fatal(err)
	}
	// Both files should exist
	if _, err := os.Stat(s.ConfigPath); err != nil {
		t.Fatalf("config file missing: %v", err)
	}
	if _, err := os.Stat(s.ManagedCrontabPath); err != nil {
		t.Fatalf("crontab file missing: %v", err)
	}
}

// --- Store: find/load by name ---

func TestFindScheduleByName(t *testing.T) {
	file := ScheduleFile{Schedules: []BackupSchedule{
		{Name: "a"}, {Name: "b"}, {Name: "c"},
	}}
	sched, idx, ok := FindScheduleByName(file, "b")
	if !ok || idx != 1 || sched.Name != "b" {
		t.Fatalf("got %v, %d, %v", sched, idx, ok)
	}
	_, _, ok = FindScheduleByName(file, "missing")
	if ok {
		t.Fatal("expected not found")
	}
}

func TestLoadScheduleByName(t *testing.T) {
	s := testService(t)
	file := ScheduleFile{Schedules: []BackupSchedule{
		{Name: "nightly", Type: "backup", Instance: "asa1", Enabled: true, Cron: "0 3 * * *"},
	}}
	if err := s.SaveScheduleFile(file); err != nil {
		t.Fatal(err)
	}
	_, sched, idx, err := s.LoadScheduleByName("nightly")
	if err != nil {
		t.Fatal(err)
	}
	if sched.Name != "nightly" || idx != 0 {
		t.Fatalf("unexpected: %v %d", sched, idx)
	}
	_, _, _, err = s.LoadScheduleByName("missing")
	if err == nil {
		t.Fatal("expected error")
	}
}

// --- State ---

func TestWriteAndLoadRunState(t *testing.T) {
	s := testService(t)
	state := RunState{
		Name:           "nightly",
		LastStartedAt:  "2026-01-01T00:00:00Z",
		LastFinishedAt: "2026-01-01T00:05:00Z",
		LastStatus:     "success",
		LastExitCode:   0,
	}
	s.WriteRunState(state)
	loaded, ok := s.LoadRunState("nightly")
	if !ok {
		t.Fatal("expected to load state")
	}
	if loaded.LastStatus != "success" || loaded.LastExitCode != 0 {
		t.Fatalf("unexpected state: %+v", loaded)
	}
}

func TestLoadRunStateNotExists(t *testing.T) {
	s := testService(t)
	_, ok := s.LoadRunState("missing")
	if ok {
		t.Fatal("expected not found")
	}
}

func TestLoadRunStateSetsNameIfEmpty(t *testing.T) {
	s := testService(t)
	// Write state JSON with empty name
	path := s.StatePath("test")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	data, _ := json.Marshal(RunState{LastStatus: "success"})
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	loaded, ok := s.LoadRunState("test")
	if !ok {
		t.Fatal("expected to load")
	}
	if loaded.Name != "test" {
		t.Fatalf("expected name=test, got %q", loaded.Name)
	}
}

func TestAppendScheduleLog(t *testing.T) {
	s := testService(t)
	s.AppendScheduleLog("nightly", "line 1")
	s.AppendScheduleLog("nightly", "line 2")
	data, err := os.ReadFile(s.LogPath("nightly"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "line 1") || !strings.Contains(string(data), "line 2") {
		t.Fatalf("unexpected log: %s", string(data))
	}
}

// --- Lock ---

func TestAcquireAndReleaseLock(t *testing.T) {
	s := testService(t)
	release, err := s.AcquireLock("nightly")
	if err != nil {
		t.Fatal(err)
	}
	// Lock file should exist
	if _, err := os.Stat(s.LockPath("nightly")); err != nil {
		t.Fatal("lock file should exist")
	}
	// Second acquire should fail
	_, err = s.AcquireLock("nightly")
	if err == nil {
		t.Fatal("expected lock-held error")
	}
	if !strings.Contains(err.Error(), "lock held") {
		t.Fatalf("unexpected error: %v", err)
	}
	// Release and re-acquire
	release()
	release2, err := s.AcquireLock("nightly")
	if err != nil {
		t.Fatalf("re-acquire failed: %v", err)
	}
	release2()
}

// --- Paths ---

func TestPathDerivation(t *testing.T) {
	repo := t.TempDir()
	s := New(repo, discardLogger())
	if s.ConfigPath != repo+"/.ark-manage/schedules.yaml" {
		t.Fatalf("config path: %s", s.ConfigPath)
	}
	if s.ManagedCrontabPath != repo+"/.ark-manage/generated/schedules.crontab" {
		t.Fatalf("crontab path: %s", s.ManagedCrontabPath)
	}
	if s.LogPath("x") != repo+"/.ark-manage/logs/schedules/x.log" {
		t.Fatalf("log path: %s", s.LogPath("x"))
	}
	if s.StatePath("x") != repo+"/.ark-manage/state/schedules/x.last.json" {
		t.Fatalf("state path: %s", s.StatePath("x"))
	}
	if s.LockPath("x") != repo+"/.ark-manage/locks/schedule-x.lock" {
		t.Fatalf("lock path: %s", s.LockPath("x"))
	}
}

// --- Helpers ---

func TestValidateCronExpr(t *testing.T) {
	if err := ValidateCronExpr("0 3 * * *"); err != nil {
		t.Fatal(err)
	}
	if err := ValidateCronExpr("bad"); err == nil {
		t.Fatal("expected error for bad cron")
	}
}

func TestValidateRetention(t *testing.T) {
	if err := ValidateRetention(BackupSchedule{RetainCount: 5}); err != nil {
		t.Fatal(err)
	}
	if err := ValidateRetention(BackupSchedule{RetainCount: -1}); err == nil {
		t.Fatal("expected error for negative retention")
	}
}

func TestBackupFlagArgs(t *testing.T) {
	s := BackupSchedule{
		Format: "tar.gz", RetainCount: 20, RetainDays: 7, RetainSizeGB: 5,
		ComposeFile: "custom.yaml",
	}
	args := BackupFlagArgs(s)
	expected := []string{"--format", "tar.gz", "--retain-count", "20", "--retain-days", "7", "--retain-size-gb", "5", "--compose-file", "custom.yaml"}
	if len(args) != len(expected) {
		t.Fatalf("got %v", args)
	}
	for i, a := range args {
		if a != expected[i] {
			t.Fatalf("arg %d: got %q want %q", i, a, expected[i])
		}
	}
}

func TestBackupFlagArgsEmpty(t *testing.T) {
	args := BackupFlagArgs(BackupSchedule{})
	if len(args) != 0 {
		t.Fatalf("expected empty, got %v", args)
	}
}

func TestBackupCommandParts(t *testing.T) {
	repo := t.TempDir()
	s := New(repo, discardLogger())
	sched := BackupSchedule{Instance: "asa1", Format: "tar.gz"}
	preview, cmd := s.BackupCommandParts(sched)
	if preview[0] != "ark-cli" || preview[2] != "backup" || preview[3] != "--instance" || preview[4] != "asa1" {
		t.Fatalf("preview: %v", preview)
	}
	if cmd[0] != "ark-cli" || cmd[2] != "backup" {
		t.Fatalf("cmd: %v", cmd)
	}
}

func TestShellQuote(t *testing.T) {
	if got := ShellQuote("hello"); got != "'hello'" {
		t.Fatalf("got %q", got)
	}
	if got := ShellQuote(""); got != "''" {
		t.Fatalf("got %q", got)
	}
	if got := ShellQuote("it's"); !strings.Contains(got, `'"'"'`) {
		t.Fatalf("got %q", got)
	}
}

func TestShellJoin(t *testing.T) {
	got := ShellJoin([]string{"a", "b c"})
	if got != "'a' 'b c'" {
		t.Fatalf("got %q", got)
	}
}

// --- Errors ---

func TestErrNotFound(t *testing.T) {
	err := ErrNotFound("x")
	if !strings.Contains(err.Error(), "not found") || !strings.Contains(err.Error(), "x") {
		t.Fatalf("got %v", err)
	}
}

func TestErrUnsupportedType(t *testing.T) {
	err := ErrUnsupportedType("monthly")
	if !strings.Contains(err.Error(), "unsupported") {
		t.Fatalf("got %v", err)
	}
}

func TestErrValidationFailed(t *testing.T) {
	inner := ErrNotFound("x")
	err := ErrValidationFailed("nightly", inner)
	if !strings.Contains(err.Error(), "nightly") || !strings.Contains(err.Error(), "invalid") {
		t.Fatalf("got %v", err)
	}
}

// --- Now override ---

func TestServiceNowOverride(t *testing.T) {
	s := testService(t)
	fixed := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	s.Now = func() time.Time { return fixed }
	if s.Now() != fixed {
		t.Fatal("Now override not working")
	}
}
