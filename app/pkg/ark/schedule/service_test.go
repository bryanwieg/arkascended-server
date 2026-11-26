package schedule

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"pkg/ark"
	"pkg/ark/arktest"

	"github.com/compose-spec/compose-go/v2/types"
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

// --- Store: load/save ---

const testComposeFile = "docker-compose.yaml"

// memCompose is an in-memory compose project holding the given x-ark-schedules value (nil: no block).
func memCompose(schedules any) *arktest.MockCompose {
	extensions := map[string]any{}
	if schedules != nil {
		extensions[ark.SchedulesExtensionKey] = schedules
	}
	return arktest.NewMemoryCompose(&ark.ComposeProject{Project: &types.Project{Extensions: extensions}})
}

func TestSchedulesRoundTripThroughCompose(t *testing.T) {
	s := testService(t)
	ctx := context.Background()
	compose := memCompose(nil)
	if file, err := s.LoadScheduleFile(ctx, compose, testComposeFile); err != nil || len(file.Schedules) != 0 {
		t.Fatalf("no block: got %+v, %v; want no schedules", file, err)
	}
	nightly := BackupSchedule{Name: "nightly", Type: TypeBackup, Instance: "asa1", Enabled: true, Cron: "0 3 * * *", Format: "tar.gz", RetainCount: 20, RetainDays: 7, RetainSizeGB: 5}
	chat := BackupSchedule{Name: "chat", Type: TypeRCON, Instance: "all", Cron: "@daily", Command: "ServerChat costs $5"}
	if err := s.SaveScheduleFile(ctx, compose, testComposeFile, ScheduleFile{Schedules: []BackupSchedule{nightly, chat}}); err != nil {
		t.Fatal(err)
	}
	loaded, err := s.LoadScheduleFile(ctx, compose, testComposeFile)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(loaded.Schedules, []BackupSchedule{chat, nightly}) {
		t.Fatalf("loaded %+v, want chat then nightly unchanged", loaded.Schedules)
	}
	project, _ := compose.LoadProject(ctx, testComposeFile)
	wantChat := map[string]any{"name": "chat", "type": "rcon", "instance": "all", "enabled": false, "cron": "@daily", "command": "ServerChat costs $5"}
	if items := project.Project.Extensions[ark.SchedulesExtensionKey].([]any); !reflect.DeepEqual(items[0], wantChat) {
		t.Fatalf("saved entry %#v, want %#v", items[0], wantChat)
	}

	if err := s.SaveScheduleFile(ctx, compose, testComposeFile, ScheduleFile{}); err != nil {
		t.Fatal(err)
	}
	project, _ = compose.LoadProject(ctx, testComposeFile)
	if _, ok := project.Project.Extensions[ark.SchedulesExtensionKey]; ok {
		t.Fatal("an empty list must remove the key")
	}

	ignored := []any{map[string]any{"name": "old", "compose_file": "x.yaml"}}
	if file, err := s.LoadScheduleFile(ctx, memCompose(ignored), testComposeFile); err != nil || file.Schedules[0].Name != "old" {
		t.Fatalf("unknown keys: got %+v, %v; want them ignored", file, err)
	}
	for name, value := range map[string]any{
		"not a list":      map[string]any{"name": "x"},
		"not mappings":    []any{"nightly"},
		"wrong type":      []any{map[string]any{"name": "x", "retain_count": "many"}},
		"null in a field": []any{map[string]any{"name": nil}},
	} {
		_, err := s.LoadScheduleFile(ctx, memCompose(value), testComposeFile)
		if err == nil || !strings.HasPrefix(err.Error(), "Error: x-ark-schedules in docker-compose.yaml: ") {
			t.Errorf("%s: err = %v, want the bad x-ark-schedules error", name, err)
		}
	}
}

func TestSaveScheduleFileSortsByName(t *testing.T) {
	compose := memCompose(nil)
	s := testService(t)
	file := ScheduleFile{
		Schedules: []BackupSchedule{
			{Name: "zebra", Type: "backup"},
			{Name: "alpha", Type: "backup"},
		},
	}
	if err := s.SaveScheduleFile(context.Background(), compose, testComposeFile, file); err != nil {
		t.Fatal(err)
	}
	loaded, err := s.LoadScheduleFile(context.Background(), compose, testComposeFile)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Schedules[0].Name != "alpha" || loaded.Schedules[1].Name != "zebra" {
		t.Fatalf("not sorted: %v", loaded.Schedules)
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
	compose := memCompose(nil)
	s := testService(t)
	file := ScheduleFile{Schedules: []BackupSchedule{
		{Name: "nightly", Type: "backup", Instance: "asa1", Enabled: true, Cron: "0 3 * * *"},
	}}
	if err := s.SaveScheduleFile(context.Background(), compose, testComposeFile, file); err != nil {
		t.Fatal(err)
	}
	_, sched, idx, err := s.LoadScheduleByName(context.Background(), compose, testComposeFile, "nightly")
	if err != nil {
		t.Fatal(err)
	}
	if sched.Name != "nightly" || idx != 0 {
		t.Fatalf("unexpected: %v %d", sched, idx)
	}
	_, _, _, err = s.LoadScheduleByName(context.Background(), compose, testComposeFile, "missing")
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
	if !s.LockHeld("nightly") {
		t.Fatal("LockHeld should be true while held")
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
	if s.LockHeld("nightly") {
		t.Fatal("LockHeld should be false after release")
	}
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
	for _, expr := range []string{"0 3 * * *", "*/15 * * * *", "@daily"} {
		if err := ValidateCronExpr(expr); err != nil {
			t.Errorf("ValidateCronExpr(%q): %v", expr, err)
		}
	}
	for _, expr := range []string{"61 * * * *", "0 3 * *", "", "bad", "@every 5m", "TZ=Asia/Tokyo 0 3 * * *", "CRON_TZ=UTC 0 3 * * *"} {
		err := ValidateCronExpr(expr)
		if err == nil || !strings.HasPrefix(err.Error(), fmt.Sprintf("Error: invalid cron expression %q: ", expr)) {
			t.Errorf("ValidateCronExpr(%q) = %v, want invalid cron expression error", expr, err)
		}
	}
}

func TestParseCronNext(t *testing.T) {
	sched, err := ParseCron("0 3 * * *")
	if err != nil {
		t.Fatal(err)
	}
	from := time.Date(2026, 11, 21, 4, 0, 0, 0, time.Local)
	want := time.Date(2026, 11, 22, 3, 0, 0, 0, time.Local)
	if got := sched.Next(from); !got.Equal(want) {
		t.Fatalf("Next(%v) = %v, want %v", from, got, want)
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
	}
	args := BackupFlagArgs(s)
	expected := []string{"--format", "tar.gz", "--retain-count", "20", "--retain-days", "7", "--retain-size-gb", "5"}
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

func TestBackupCommandPreview(t *testing.T) {
	got := strings.Join(BackupCommandPreview(BackupSchedule{Instance: "asa1", Format: "tar.gz"}), " ")
	if got != "ark-cli create backup --instance asa1 --format tar.gz" {
		t.Fatalf("preview: %q", got)
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
