package schedule

// Tests: schedule lifecycle: create/list/remove/show/update/run/validate, lock and status tracking

import (
	"ark-cli/internal/core"
	"ark-cli/internal/testutil"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"pkg/ark"
	"pkg/ark/arktest"
	"pkg/ark/backup"
	"pkg/ark/runtime"
	"pkg/ark/schedule"

	"github.com/compose-spec/compose-go/v2/types"
	"github.com/goccy/go-yaml"
)

func TestCreateListRemoveScheduleBackupPhase1(t *testing.T) {
	tmp := t.TempDir()
	overrideSchedulePathsForTest(t, tmp)
	setEnv(t, "COMPOSE_FILE", filepath.Join(tmp, "docker-compose.yaml"))

	if err := os.WriteFile(filepath.Join(tmp, "docker-compose.yaml"), []byte("services:\n  asa1:\n    labels: {ark.port.asa: '27015'}\n    image: ark-server:latest\n"), 0o644); err != nil {
		t.Fatalf("write compose: %v", err)
	}

	out := captureStdout(t, func() {
		withScheduleComposeDeps(t, emptyServices("asa1"), func(d *core.AppDeps) {
		}, func() {
			if err := runCreateScheduleBackup([]string{"--instance", "asa1", "--name", "nightly", "--cron", "0 3 * * *"}); err != nil {
				t.Fatalf("runCreateScheduleBackup: %v", err)
			}
		})
	})
	if !strings.Contains(out, "Created schedule nightly") {
		t.Fatalf("unexpected create output: %s", out)
	}

	configBytes, err := savedSchedulesYAML(t)
	if err != nil {
		t.Fatalf("read schedule config: %v", err)
	}
	configText := string(configBytes)
	for _, want := range []string{"name: nightly", "type: backup", "instance: asa1", "cron: 0 3 * * *"} {
		if !strings.Contains(configText, want) {
			t.Fatalf("config missing %q\n%s", want, configText)
		}
	}

	listOut := captureStdout(t, func() {
		if err := RunListSchedules(core.CurrentAppDeps(), []string{}); err != nil {
			t.Fatalf("runListSchedules: %v", err)
		}
	})
	if !strings.Contains(listOut, "nightly") || !strings.Contains(listOut, "backup") || !strings.Contains(listOut, "asa1") {
		t.Fatalf("unexpected list output: %s", listOut)
	}
	if !strings.Contains(listOut, "LAST_STATUS") {
		t.Fatalf("expected list output header to include LAST_STATUS: %s", listOut)
	}

	removeOut := captureStdout(t, func() {
		if err := RunRemoveSchedule(core.CurrentAppDeps(), []string{"--name", "nightly"}); err != nil {
			t.Fatalf("runRemoveSchedule: %v", err)
		}
	})
	if !strings.Contains(removeOut, "Removed schedule nightly") {
		t.Fatalf("unexpected remove output: %s", removeOut)
	}

	configBytes, err = savedSchedulesYAML(t)
	if err != nil {
		t.Fatalf("read schedule config after remove: %v", err)
	}
	if strings.Contains(string(configBytes), "nightly") {
		t.Fatalf("schedule still present in config:\n%s", string(configBytes))
	}
}

func TestCreateScheduleRejectsDuplicateName(t *testing.T) {
	tmp := t.TempDir()
	overrideSchedulePathsForTest(t, tmp)

	initial := "x-ark-schedules:\n  - name: nightly\n    type: backup\n    instance: asa1\n    enabled: true\n    cron: \"0 3 * * *\"\n"
	writeScheduleFixture(t, initial)

	withScheduleComposeDeps(t, emptyServices("asa1"), nil, func() {
		if err := runCreateScheduleBackup([]string{"--instance", "asa1", "--name", "nightly", "--cron", "0 4 * * *"}); err == nil {
			t.Fatal("expected duplicate-name error")
		}
	})
}

func TestShowUpdateRunSchedulePhase2(t *testing.T) {
	tmp := t.TempDir()
	overrideSchedulePathsForTest(t, tmp)

	initial := `x-ark-schedules:
  - name: nightly
    type: backup
    instance: asa1
    enabled: true
    cron: "0 3 * * *"
    format: tar.gz
    retain_count: 20
    retain_days: 7
    retain_size_gb: 5
`
	writeScheduleFixture(t, initial)

	showOut := captureStdout(t, func() {
		if err := RunShowSchedule(core.CurrentAppDeps(), []string{"--name", "nightly"}); err != nil {
			t.Fatalf("runShowSchedule: %v", err)
		}
	})
	for _, want := range []string{"name: nightly", "type: backup", "instance: asa1", "cron: 0 3 * * *"} {
		if !strings.Contains(showOut, want) {
			t.Fatalf("show output missing %q\n%s", want, showOut)
		}
	}

	withScheduleComposeDeps(t, emptyServices("asa1"), nil, func() {
		if err := RunUpdateSchedule(core.CurrentAppDeps(), []string{"--name", "nightly", "--cron", "0 4 * * *", "--disabled", "--retain-days", "14"}); err != nil {
			t.Fatalf("runUpdateSchedule: %v", err)
		}
	})
	updatedBytes, err := savedSchedulesYAML(t)
	if err != nil {
		t.Fatalf("read updated config: %v", err)
	}
	updated := string(updatedBytes)
	for _, want := range []string{"cron: 0 4 * * *", "enabled: false", "retain_days: 14"} {
		if !strings.Contains(updated, want) {
			t.Fatalf("updated config missing %q\n%s", want, updated)
		}
	}

	var gotName string
	var gotArgs []string
	withPatchedAppDeps(t, func(d *core.AppDeps) {
		d.RunBackupFromSchedule = func(name string, args []string) error {
			gotName = name
			gotArgs = append([]string(nil), args...)
			return nil
		}
	}, func() {
		if err := RunRunSchedule(core.CurrentAppDeps(), []string{"--name", "nightly"}); err != nil {
			t.Fatalf("runRunSchedule: %v", err)
		}
	})
	if gotName != "nightly" {
		t.Fatalf("unexpected run schedule name: %s", gotName)
	}
	if !containsArgPair(gotArgs, "--instance", "asa1") {
		t.Fatalf("unexpected run backup args: %#v", gotArgs)
	}
	if !containsArgPair(gotArgs, "--retain-days", "14") {
		t.Fatalf("expected updated retain-days in backup args: %#v", gotArgs)
	}

	showOut = captureStdout(t, func() {
		if err := RunShowSchedule(core.CurrentAppDeps(), []string{"--name", "nightly"}); err != nil {
			t.Fatalf("RunShowSchedule(core.CurrentAppDeps(), after run): %v", err)
		}
	})
	if !strings.Contains(showOut, "last_status: success") {
		t.Fatalf("expected last_status in show output:\n%s", showOut)
	}
	if !strings.Contains(showOut, "last_exit_code: 0") {
		t.Fatalf("expected last_exit_code in show output:\n%s", showOut)
	}
}

func TestValidateSchedulePhase3(t *testing.T) {
	tmp := t.TempDir()
	overrideSchedulePathsForTest(t, tmp)

	initial := `x-ark-schedules:
  - name: nightly
    type: backup
    instance: asa1
    enabled: true
    cron: "0 3 * * *"
    format: tar.gz
`
	writeScheduleFixture(t, initial)

	out := captureStdout(t, func() {
		withScheduleComposeDeps(t, emptyServices("asa1"), nil, func() {
			if err := RunValidateSchedule(core.CurrentAppDeps(), []string{"--name", "nightly"}); err != nil {
				t.Fatalf("runValidateSchedule: %v", err)
			}
		})
	})
	if !strings.Contains(out, "Validated schedule nightly") {
		t.Fatalf("unexpected output: %s", out)
	}

	if err := RunUpdateSchedule(core.CurrentAppDeps(), []string{"--name", "nightly", "--cron", "bad expr"}); err == nil {
		t.Fatal("expected invalid cron update error")
	}
}

func TestRunScheduleWritesLockLogAndStatus(t *testing.T) {
	tmp := t.TempDir()
	overrideSchedulePathsForTest(t, tmp)

	initial := `x-ark-schedules:
  - name: nightly
    type: backup
    instance: asa1
    enabled: true
    cron: "0 3 * * *"
    format: tar.gz
`
	writeScheduleFixture(t, initial)

	var gotArgs []string
	out := captureStdout(t, func() {
		withPatchedAppDeps(t, func(d *core.AppDeps) {
			d.RunBackupFromSchedule = func(_ string, args []string) error { gotArgs = args; return nil }
			d.ScheduleService.Now = func() time.Time { return time.Unix(1707000000, 0).UTC() }
		}, func() {
			if err := RunRunSchedule(core.CurrentAppDeps(), []string{"--name", "nightly"}); err != nil {
				t.Fatalf("runRunSchedule: %v", err)
			}
		})
	})
	for _, want := range []string{"2024-02-03T22:40:00Z START nightly", "2024-02-03T22:40:00Z SUCCESS nightly", "Ran schedule nightly"} {
		if !strings.Contains(out, want) {
			t.Fatalf("output missing %q\n%s", want, out)
		}
	}
	if n := len(gotArgs); n < 2 || gotArgs[n-2] != "--lock-timeout" || gotArgs[n-1] != "30m" {
		t.Fatalf("backup args must end with --lock-timeout 30m: %#v", gotArgs)
	}

	logBytes, err := os.ReadFile(core.CurrentAppDeps().ScheduleService.LogPath("nightly"))
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	logText := string(logBytes)
	for _, want := range []string{"START nightly", "SUCCESS nightly"} {
		if !strings.Contains(logText, want) {
			t.Fatalf("log missing %q\n%s", want, logText)
		}
	}

	stateBytes, err := os.ReadFile(core.CurrentAppDeps().ScheduleService.StatePath("nightly"))
	if err != nil {
		t.Fatalf("read state: %v", err)
	}
	var state schedule.RunState
	if err := json.Unmarshal(stateBytes, &state); err != nil {
		t.Fatalf("unmarshal state: %v", err)
	}
	if state.LastStatus != "success" || state.LastExitCode != 0 {
		t.Fatalf("unexpected state: %+v", state)
	}

	if core.CurrentAppDeps().ScheduleService.LockHeld("nightly") {
		t.Fatal("expected the schedule lock to be released")
	}
}

func TestRunScheduleLockHeldFails(t *testing.T) {
	tmp := t.TempDir()
	overrideSchedulePathsForTest(t, tmp)
	initial := `x-ark-schedules:
  - name: nightly
    type: backup
    instance: asa1
    enabled: true
    cron: "0 3 * * *"
`
	writeScheduleFixture(t, initial)
	release, err := core.CurrentAppDeps().ScheduleService.AcquireLock("nightly")
	if err != nil {
		t.Fatalf("hold lock: %v", err)
	}
	defer release()

	withPatchedAppDeps(t, func(d *core.AppDeps) {
		d.RunBackupFromSchedule = func(_ string, _ []string) error { return errors.New("should not run") }
	}, func() {
		if err := RunRunSchedule(core.CurrentAppDeps(), []string{"--name", "nightly"}); err == nil {
			t.Fatal("expected lock-held error")
		}
	})
}

func TestListSchedulesIncludesLastRunColumns(t *testing.T) {
	tmp := t.TempDir()
	overrideSchedulePathsForTest(t, tmp)

	initial := `x-ark-schedules:
  - name: nightly
    type: backup
    instance: asa1
    enabled: true
    cron: "0 3 * * *"
`
	writeScheduleFixture(t, initial)
	core.CurrentAppDeps().ScheduleService.WriteRunState(schedule.RunState{
		Name:           "nightly",
		LastStatus:     "success",
		LastExitCode:   0,
		LastFinishedAt: "2026-02-24T01:02:03Z",
	})

	out := captureStdout(t, func() {
		if err := RunListSchedules(core.CurrentAppDeps(), nil); err != nil {
			t.Fatalf("runListSchedules: %v", err)
		}
	})
	for _, want := range []string{"LAST_STATUS", "LAST_EXIT", "LAST_FINISHED", "success", "0", "2026-02-24T01:02:03Z"} {
		if !strings.Contains(out, want) {
			t.Fatalf("list output missing %q\n%s", want, out)
		}
	}
}

func TestShowScheduleIncludesCommandPreview(t *testing.T) {
	tmp := t.TempDir()
	overrideSchedulePathsForTest(t, tmp)

	initial := `x-ark-schedules:
  - name: nightly
    type: backup
    instance: asa1
    enabled: true
    cron: "0 3 * * *"
    format: tar.gz
    retain_count: 20
    retain_days: 14
    retain_size_gb: 5
`
	writeScheduleFixture(t, initial)

	out := captureStdout(t, func() {
		if err := RunShowSchedule(core.CurrentAppDeps(), []string{"--name", "nightly"}); err != nil {
			t.Fatalf("runShowSchedule: %v", err)
		}
	})
	for _, want := range []string{
		"command_preview: ark-cli create backup --instance asa1",
		"--retain-days 14",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("show output missing %q\n%s", want, out)
		}
	}
}

func TestRunScheduleFailureWritesFailedState(t *testing.T) {
	tmp := t.TempDir()
	overrideSchedulePathsForTest(t, tmp)
	writeScheduleFixture(t, `x-ark-schedules:
  - name: nightly
    type: backup
    instance: asa1
    enabled: true
    cron: "0 3 * * *"
`)
	out := captureStdout(t, func() {
		withPatchedAppDeps(t, func(d *core.AppDeps) {
			d.RunBackupFromSchedule = func(_ string, _ []string) error {
				return errors.New("disk full")
			}
		}, func() {
			if err := RunRunSchedule(core.CurrentAppDeps(), []string{"--name", "nightly"}); err == nil {
				t.Fatal("expected error from failed backup")
			}
		})
	})
	if !strings.Contains(out, " FAIL nightly: disk full") || strings.Contains(out, "Ran schedule") {
		t.Fatalf("unexpected output: %s", out)
	}

	stateBytes, err := os.ReadFile(core.CurrentAppDeps().ScheduleService.StatePath("nightly"))
	if err != nil {
		t.Fatalf("read state: %v", err)
	}
	var state schedule.RunState
	if err := json.Unmarshal(stateBytes, &state); err != nil {
		t.Fatalf("unmarshal state: %v", err)
	}
	if state.LastStatus != "failed" || state.LastExitCode != 1 {
		t.Fatalf("expected failed state, got: %+v", state)
	}
	if state.LastError == "" {
		t.Fatal("expected LastError to be set on failure")
	}
}

func TestUpdateScheduleNoChangesErrors(t *testing.T) {
	tmp := t.TempDir()
	overrideSchedulePathsForTest(t, tmp)
	writeScheduleFixture(t, `x-ark-schedules:
  - name: nightly
    type: backup
    instance: asa1
    enabled: true
    cron: "0 3 * * *"
`)
	if err := RunUpdateSchedule(core.CurrentAppDeps(), []string{"--name", "nightly"}); err == nil {
		t.Fatal("expected error when no changes specified")
	}
}

func TestUpdateScheduleConflictingEnabledDisabledErrors(t *testing.T) {
	tmp := t.TempDir()
	overrideSchedulePathsForTest(t, tmp)
	writeScheduleFixture(t, `x-ark-schedules:
  - name: nightly
    type: backup
    instance: asa1
    enabled: true
    cron: "0 3 * * *"
`)
	if err := RunUpdateSchedule(core.CurrentAppDeps(), []string{"--name", "nightly", "--enabled", "--disabled"}); err == nil {
		t.Fatal("expected error when both --enabled and --disabled are specified")
	}
}

func TestListSchedulesEmpty(t *testing.T) {
	tmp := t.TempDir()
	overrideSchedulePathsForTest(t, tmp)
	writeScheduleFixture(t, "x-ark-schedules: []\n")

	out := captureStdout(t, func() {
		if err := RunListSchedules(core.CurrentAppDeps(), []string{}); err != nil {
			t.Fatalf("runListSchedules: %v", err)
		}
	})
	if !strings.Contains(out, "No schedules configured.") {
		t.Fatalf("expected empty message, got:\n%s", out)
	}
}

func TestShowScheduleNotFound(t *testing.T) {
	tmp := t.TempDir()
	overrideSchedulePathsForTest(t, tmp)
	writeScheduleFixture(t, `x-ark-schedules:
  - name: nightly
    type: backup
    instance: asa1
    enabled: true
    cron: "0 3 * * *"
`)
	if err := RunShowSchedule(core.CurrentAppDeps(), []string{"--name", "nonexistent"}); err == nil {
		t.Fatal("expected not-found error")
	}
}

func TestRemoveScheduleNotFound(t *testing.T) {
	tmp := t.TempDir()
	overrideSchedulePathsForTest(t, tmp)
	writeScheduleFixture(t, `x-ark-schedules:
  - name: nightly
    type: backup
    instance: asa1
    enabled: true
    cron: "0 3 * * *"
`)
	if err := RunRemoveSchedule(core.CurrentAppDeps(), []string{"--name", "nonexistent"}); err == nil {
		t.Fatal("expected not-found error")
	}
}

func TestCreateScheduleDispatcherRequiresSubcommand(t *testing.T) {
	if err := RunCreateSchedule(core.CurrentAppDeps(), []string{}); err == nil {
		t.Fatal("expected error for missing subcommand")
	}
}

func TestCreateScheduleDispatcherRejectsUnknownSubcommand(t *testing.T) {
	if err := RunCreateSchedule(core.CurrentAppDeps(), []string{"monthly"}); err == nil {
		t.Fatal("expected error for unknown subcommand")
	}
}

func TestCreateScheduleBackupWithDisabledFlag(t *testing.T) {
	tmp := t.TempDir()
	overrideSchedulePathsForTest(t, tmp)
	setEnv(t, "COMPOSE_FILE", filepath.Join(tmp, "docker-compose.yaml"))
	if err := os.WriteFile(filepath.Join(tmp, "docker-compose.yaml"), []byte("services:\n  asa1:\n    labels: {ark.port.asa: '27015'}\n    image: ark-server:latest\n"), 0o644); err != nil {
		t.Fatalf("write compose: %v", err)
	}

	withScheduleComposeDeps(t, emptyServices("asa1"), nil, func() {
		if err := runCreateScheduleBackup([]string{"--instance", "asa1", "--name", "nightly", "--cron", "0 3 * * *", "--disabled"}); err != nil {
			t.Fatalf("runCreateScheduleBackup: %v", err)
		}
	})

	configBytes, err := savedSchedulesYAML(t)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	if !strings.Contains(string(configBytes), "enabled: false") {
		t.Fatalf("expected enabled: false in config:\n%s", string(configBytes))
	}
}

func TestValidateScheduleNoSchedulesConfigured(t *testing.T) {
	tmp := t.TempDir()
	overrideSchedulePathsForTest(t, tmp)
	writeScheduleFixture(t, "x-ark-schedules: []\n")

	if err := RunValidateSchedule(core.CurrentAppDeps(), []string{}); err == nil {
		t.Fatal("expected error for empty schedule list")
	}
}

func TestRunEnableScheduleFlipsEnabled(t *testing.T) {
	tmp := t.TempDir()
	overrideSchedulePathsForTest(t, tmp)
	writeScheduleFixture(t, `x-ark-schedules:
  - name: nightly
    type: backup
    instance: asa1
    enabled: false
    cron: "0 3 * * *"
`)

	out := captureStdout(t, func() {
		if err := RunEnableSchedule(core.CurrentAppDeps(), []string{"--name", "nightly"}); err != nil {
			t.Fatalf("RunEnableSchedule: %v", err)
		}
	})
	if !strings.Contains(out, "Enabled schedule nightly") {
		t.Fatalf("expected enable output, got:\n%s", out)
	}

	configBytes, err := savedSchedulesYAML(t)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	if !strings.Contains(string(configBytes), "enabled: true") {
		t.Fatalf("expected enabled: true in config:\n%s", string(configBytes))
	}
}

func TestRunEnableScheduleAlreadyEnabled(t *testing.T) {
	tmp := t.TempDir()
	overrideSchedulePathsForTest(t, tmp)
	writeScheduleFixture(t, `x-ark-schedules:
  - name: nightly
    type: backup
    instance: asa1
    enabled: true
    cron: "0 3 * * *"
`)

	out := captureStdout(t, func() {
		if err := RunEnableSchedule(core.CurrentAppDeps(), []string{"--name", "nightly"}); err != nil {
			t.Fatalf("RunEnableSchedule: %v", err)
		}
	})
	if !strings.Contains(out, "Enabled schedule nightly") {
		t.Fatalf("expected enable output, got:\n%s", out)
	}
}

func TestRunEnableScheduleNotFound(t *testing.T) {
	tmp := t.TempDir()
	overrideSchedulePathsForTest(t, tmp)
	writeScheduleFixture(t, `x-ark-schedules:
  - name: nightly
    type: backup
    instance: asa1
    enabled: false
    cron: "0 3 * * *"
`)

	if err := RunEnableSchedule(core.CurrentAppDeps(), []string{"--name", "nonexistent"}); err == nil {
		t.Fatal("expected not-found error")
	}
}

func TestRunDisableScheduleFlipsDisabled(t *testing.T) {
	tmp := t.TempDir()
	overrideSchedulePathsForTest(t, tmp)
	writeScheduleFixture(t, `x-ark-schedules:
  - name: nightly
    type: backup
    instance: asa1
    enabled: true
    cron: "0 3 * * *"
`)

	out := captureStdout(t, func() {
		if err := RunDisableSchedule(core.CurrentAppDeps(), []string{"--name", "nightly"}); err != nil {
			t.Fatalf("RunDisableSchedule: %v", err)
		}
	})
	if !strings.Contains(out, "Disabled schedule nightly") {
		t.Fatalf("expected disable output, got:\n%s", out)
	}

	configBytes, err := savedSchedulesYAML(t)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	if !strings.Contains(string(configBytes), "enabled: false") {
		t.Fatalf("expected enabled: false in config:\n%s", string(configBytes))
	}
}

func TestRunDisableScheduleAlreadyDisabled(t *testing.T) {
	tmp := t.TempDir()
	overrideSchedulePathsForTest(t, tmp)
	writeScheduleFixture(t, `x-ark-schedules:
  - name: nightly
    type: backup
    instance: asa1
    enabled: false
    cron: "0 3 * * *"
`)

	out := captureStdout(t, func() {
		if err := RunDisableSchedule(core.CurrentAppDeps(), []string{"--name", "nightly"}); err != nil {
			t.Fatalf("RunDisableSchedule: %v", err)
		}
	})
	if !strings.Contains(out, "Disabled schedule nightly") {
		t.Fatalf("expected disable output, got:\n%s", out)
	}
}

func TestRunDisableScheduleNotFound(t *testing.T) {
	tmp := t.TempDir()
	overrideSchedulePathsForTest(t, tmp)
	writeScheduleFixture(t, `x-ark-schedules:
  - name: nightly
    type: backup
    instance: asa1
    enabled: true
    cron: "0 3 * * *"
`)

	if err := RunDisableSchedule(core.CurrentAppDeps(), []string{"--name", "nonexistent"}); err == nil {
		t.Fatal("expected not-found error")
	}
}

func TestRunCheckScheduleShowsStatus(t *testing.T) {
	tmp := t.TempDir()
	overrideSchedulePathsForTest(t, tmp)
	writeScheduleFixture(t, `x-ark-schedules:
  - name: nightly
    type: backup
    instance: asa1
    enabled: true
    cron: "0 3 * * *"
`)
	core.CurrentAppDeps().ScheduleService.WriteRunState(schedule.RunState{
		Name:       "nightly",
		LastStatus: "success",
	})

	out := captureStdout(t, func() {
		if err := RunCheckSchedule(core.CurrentAppDeps(), []string{"--name", "nightly"}); err != nil {
			t.Fatalf("RunCheckSchedule: %v", err)
		}
	})
	for _, want := range []string{"name:", "enabled:", "running:", "last_status:", "result:   ok"} {
		if !strings.Contains(out, want) {
			t.Fatalf("output missing %q\n%s", want, out)
		}
	}
}

func TestRunCheckScheduleRunningState(t *testing.T) {
	tmp := t.TempDir()
	overrideSchedulePathsForTest(t, tmp)
	writeScheduleFixture(t, `x-ark-schedules:
  - name: nightly
    type: backup
    instance: asa1
    enabled: true
    cron: "0 3 * * *"
`)

	release, err := core.CurrentAppDeps().ScheduleService.AcquireLock("nightly")
	if err != nil {
		t.Fatalf("hold lock: %v", err)
	}
	defer release()

	out := captureStdout(t, func() {
		if err := RunCheckSchedule(core.CurrentAppDeps(), []string{"--name", "nightly"}); err != nil {
			t.Fatalf("RunCheckSchedule: %v", err)
		}
	})
	for _, want := range []string{"running:  yes", "result:   running"} {
		if !strings.Contains(out, want) {
			t.Fatalf("output missing %q\n%s", want, out)
		}
	}
}

func TestRunCheckScheduleFailedLastRun(t *testing.T) {
	tmp := t.TempDir()
	overrideSchedulePathsForTest(t, tmp)
	writeScheduleFixture(t, `x-ark-schedules:
  - name: nightly
    type: backup
    instance: asa1
    enabled: true
    cron: "0 3 * * *"
`)
	core.CurrentAppDeps().ScheduleService.WriteRunState(schedule.RunState{
		Name:       "nightly",
		LastStatus: "failed",
	})

	err := RunCheckSchedule(core.CurrentAppDeps(), []string{"--name", "nightly"})
	if err == nil || !strings.Contains(err.Error(), "failed") {
		t.Fatalf("expected error containing 'failed', got: %v", err)
	}
}

func TestRunCheckScheduleNotFound(t *testing.T) {
	tmp := t.TempDir()
	overrideSchedulePathsForTest(t, tmp)
	writeScheduleFixture(t, `x-ark-schedules:
  - name: nightly
    type: backup
    instance: asa1
    enabled: true
    cron: "0 3 * * *"
`)

	if err := RunCheckSchedule(core.CurrentAppDeps(), []string{"--name", "nonexistent"}); err == nil {
		t.Fatal("expected not-found error")
	}
}

func TestRunCheckScheduleNoRunState(t *testing.T) {
	tmp := t.TempDir()
	overrideSchedulePathsForTest(t, tmp)
	writeScheduleFixture(t, `x-ark-schedules:
  - name: nightly
    type: backup
    instance: asa1
    enabled: true
    cron: "0 3 * * *"
`)

	out := captureStdout(t, func() {
		if err := RunCheckSchedule(core.CurrentAppDeps(), []string{"--name", "nightly"}); err != nil {
			t.Fatalf("RunCheckSchedule: %v", err)
		}
	})
	if !strings.Contains(out, "last_status:      -") {
		t.Fatalf("expected last_status: - in output, got:\n%s", out)
	}
}

func containsArgPair(args []string, key, value string) bool {
	for i := 0; i < len(args)-1; i++ {
		if args[i] == key && args[i+1] == value {
			return true
		}
	}
	return false
}

// testStore is the test's in-memory compose project; it holds the schedules across dep patches.
var testStore *arktest.MockCompose

func testComposeFile() string { return os.Getenv("COMPOSE_FILE") }

// writeScheduleFixture seeds the store with raw, compose YAML holding an x-ark-schedules list.
func writeScheduleFixture(t *testing.T, raw string) {
	t.Helper()
	var doc map[string][]map[string]any
	if err := yaml.Unmarshal([]byte(raw), &doc); err != nil {
		t.Fatalf("parse fixture: %v", err)
	}
	items := []any{}
	for _, entry := range doc[ark.SchedulesExtensionKey] {
		for key, value := range entry {
			if n, ok := value.(uint64); ok { // LoadProject returns int
				entry[key] = int(n)
			}
		}
		items = append(items, entry)
	}
	project := fakeProject(nil)
	project.Project.Extensions = types.Extensions{ark.SchedulesExtensionKey: items}
	if err := testStore.SaveProject(context.Background(), testComposeFile(), project); err != nil {
		t.Fatalf("seed schedules: %v", err)
	}
}

// savedSchedulesYAML is the store's x-ark-schedules value as YAML text.
func savedSchedulesYAML(t *testing.T) ([]byte, error) {
	t.Helper()
	project, err := testStore.LoadProject(context.Background(), testComposeFile())
	if err != nil {
		return nil, err
	}
	return yaml.Marshal(project.Project.Extensions[ark.SchedulesExtensionKey])
}

// withScheduleComposeDeps runs with a compose project of the given services over the test's schedule store.
func withScheduleComposeDeps(
	t *testing.T,
	services types.Services,
	mutate func(*core.AppDeps),
	run func(),
) {
	t.Helper()
	withPatchedAppDeps(t, func(d *core.AppDeps) {
		mock := *testStore
		mock.LoadProjectFn = func(ctx context.Context, composeFile string) (*ark.ComposeProject, error) {
			project, err := testStore.LoadProject(ctx, composeFile)
			if err != nil {
				return nil, err
			}
			project.Project.Services = services
			return project, nil
		}
		d.ComposeService = &mock
		if mutate != nil {
			mutate(d)
		}
	}, run)
}

// overrideSchedulePathsForTest gives the test an empty schedule store and points COMPOSE_FILE at
// <root>/docker-compose.yaml, so run state, logs and locks land under root.
func overrideSchedulePathsForTest(t *testing.T, root string) {
	t.Helper()
	setEnv(t, "COMPOSE_FILE", filepath.Join(root, "docker-compose.yaml"))
	testStore = arktest.NewMemoryCompose(fakeProject(nil))
	patchAppDepsForTest(t, func(d *core.AppDeps) {
		d.ScheduleService = schedule.New(root, nil)
		d.ComposeService = testStore
	})
}

func TestCreateScheduleBackupForCluster(t *testing.T) {
	tmp := t.TempDir()
	overrideSchedulePathsForTest(t, tmp)
	setEnv(t, "COMPOSE_FILE", filepath.Join(tmp, "docker-compose.yaml"))
	if err := os.WriteFile(filepath.Join(tmp, "docker-compose.yaml"), []byte("services: {}\n"), 0o644); err != nil {
		t.Fatalf("write compose: %v", err)
	}
	withCluster := types.Services{"asa1": {Name: "asa1", Labels: types.Labels{ark.PortASALabel: "27015"}, Volumes: []types.ServiceVolumeConfig{
		{Type: "volume", Source: "ark-cluster-1", Target: backup.ClusterMountPath},
	}}}
	captureStdout(t, func() {
		withScheduleComposeDeps(t, withCluster, func(*core.AppDeps) {}, func() {
			if err := runCreateScheduleBackup([]string{"--instance", "cluster", "--name", "cluster-hourly", "--cron", "0 * * * *"}); err != nil {
				t.Fatalf("runCreateScheduleBackup: %v", err)
			}
		})
	})
	configBytes, err := savedSchedulesYAML(t)
	if err != nil {
		t.Fatalf("read schedule config: %v", err)
	}
	if !strings.Contains(string(configBytes), "instance: cluster") {
		t.Fatalf("schedule config:\n%s", configBytes)
	}

	withScheduleComposeDeps(t, emptyServices("asa1"), func(*core.AppDeps) {}, func() {
		err := runCreateScheduleBackup([]string{"--instance", "cluster", "--name", "no-cluster", "--cron", "0 * * * *"})
		if !errors.Is(err, backup.ErrNoClusterVolume) {
			t.Fatalf("got %v", err)
		}
	})
}

// TestScheduleCommandsWarnWithoutScheduler pins the warnings schedule commands print after saving: no scheduler
// service in the schedule's project.
func TestScheduleCommandsWarnWithoutScheduler(t *testing.T) {
	const notEnabled = "the scheduler service is not enabled"
	withScheduler := types.Services{ark.SchedulerService: {Name: ark.SchedulerService, Labels: types.Labels{ark.RoleLabel: ark.SchedulerRole}}}
	for _, tc := range []struct {
		name      string
		services  types.Services
		wantWarns []string
		noWarns   []string
	}{
		{name: "no scheduler", services: emptyServices("asa1"), wantWarns: []string{notEnabled}},
		{name: "scheduler enabled", services: withScheduler, noWarns: []string{notEnabled}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tmp := t.TempDir()
			overrideSchedulePathsForTest(t, tmp)
			composeFile := filepath.Join(tmp, "docker-compose.yaml")
			setEnv(t, "COMPOSE_FILE", composeFile)
			if err := os.WriteFile(composeFile, []byte("services: {}\n"), 0o644); err != nil {
				t.Fatalf("write compose: %v", err)
			}
			var buf strings.Builder
			origWarnf := core.Warnf
			core.Warnf = func(format string, args ...any) { fmt.Fprintf(&buf, format, args...) }
			t.Cleanup(func() { core.Warnf = origWarnf })
			withScheduleComposeDeps(t, tc.services, nil, func() {
				if err := runCreateScheduleBackup([]string{"--instance", "all", "--name", "nightly", "--cron", "0 3 * * *"}); err != nil {
					t.Fatalf("create: %v", err)
				}
				if err := RunRemoveSchedule(core.CurrentAppDeps(), []string{"--name", "nightly"}); err != nil {
					t.Fatalf("remove: %v", err)
				}
			})
			warnings := buf.String()
			for _, want := range tc.wantWarns {
				if strings.Count(warnings, want) != 2 {
					t.Fatalf("want %q after create and remove, got:\n%s", want, warnings)
				}
			}
			for _, not := range tc.noWarns {
				if strings.Contains(warnings, not) {
					t.Fatalf("unexpected %q, got:\n%s", not, warnings)
				}
			}
		})
	}
}

// actionRun is what one action schedule run did, for the action contract tests.
type actionRun struct {
	err      error
	stdout   string
	stderr   string
	log      string
	state    schedule.RunState
	execs    [][]string // "<service> <last argv element>" per exec
	restarts []string   // "stop <svc>", "start <svc>"
}

// runActionSchedule runs schedule "act" from entry with asa1 and asa2 in the project, the given
// services running, and execFn answering each exec (nil: exit 0 with reply "ok").
func runActionSchedule(t *testing.T, entry string, running []string, execFn func(service string, argv []string) ark.ExecResult, stopErr error) actionRun {
	t.Helper()
	tmp := t.TempDir()
	overrideSchedulePathsForTest(t, tmp)
	writeScheduleFixture(t, "x-ark-schedules:\n  - name: act\n    enabled: true\n    cron: \"0 * * * *\"\n"+entry)
	got := actionRun{}
	runningSet := map[string]struct{}{}
	for _, s := range running {
		runningSet[s] = struct{}{}
	}
	got.stderr = testutil.CaptureStderr(t, func() {
		got.stdout = captureStdout(t, func() {
			withScheduleComposeDeps(t, emptyServices("asa1", "asa2"), func(d *core.AppDeps) {
				mock := d.ComposeService.(*arktest.MockCompose)
				mock.RunningServicesFn = func(context.Context, string) (map[string]struct{}, error) { return runningSet, nil }
				mock.ExecFn = func(_ context.Context, _, service string, argv []string, _ ark.ExecOptions) (ark.ExecResult, error) {
					got.execs = append(got.execs, []string{service, argv[len(argv)-1]})
					if execFn != nil {
						return execFn(service, argv), nil
					}
					return ark.ExecResult{Stdout: "ok\n"}, nil
				}
				d.RestartStopInstance = func(req runtime.StopCommandRequest) error {
					got.restarts = append(got.restarts, "stop "+req.Instance)
					return stopErr
				}
				d.RestartStartInstance = func(req runtime.StartCommandRequest) error {
					got.restarts = append(got.restarts, "start "+req.Instance)
					return nil
				}
			}, func() {
				got.err = RunScheduleOnce(core.CurrentAppDeps(), testComposeFile(), mustLoadSchedule(t, "act"))
			})
		})
	})
	logBytes, _ := os.ReadFile(core.CurrentAppDeps().ScheduleService.LogPath("act"))
	got.log = string(logBytes)
	stateBytes, err := os.ReadFile(core.CurrentAppDeps().ScheduleService.StatePath("act"))
	if err != nil {
		t.Fatalf("read state: %v", err)
	}
	if err := json.Unmarshal(stateBytes, &got.state); err != nil {
		t.Fatalf("unmarshal state: %v", err)
	}
	return got
}

func mustLoadSchedule(t *testing.T, name string) schedule.BackupSchedule {
	t.Helper()
	_, s, _, err := core.CurrentAppDeps().ScheduleService.LoadScheduleByName(context.Background(), testStore, testComposeFile(), name)
	if err != nil {
		t.Fatalf("load schedule: %v", err)
	}
	return s
}

func TestRunScheduleActionsOnRunningInstances(t *testing.T) {
	cases := []struct {
		name, entry, wantExecs, wantRestarts string
	}{
		{"rcon", "    type: rcon\n    instance: asa1\n    command: ServerChat hi\n", "[[asa1 ServerChat hi]]", "[]"},
		{"save", "    type: save\n    instance: asa1\n", "[[asa1 saveworld]]", "[]"},
		{"restart", "    type: restart\n    instance: asa1\n", "[[asa1 scheduled restart]]", "[stop asa1 start asa1]"},
		{"all in name order", "    type: rcon\n    instance: all\n    command: ServerChat hi\n", "[[asa1 ServerChat hi] [asa2 ServerChat hi]]", "[]"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := runActionSchedule(t, tc.entry, []string{"asa1", "asa2"}, nil, nil)
			if got.err != nil {
				t.Fatalf("RunScheduleOnce: %v\n%s", got.err, got.stderr)
			}
			if s := fmt.Sprint(got.execs); s != tc.wantExecs {
				t.Fatalf("execs = %s, want %s", s, tc.wantExecs)
			}
			if s := fmt.Sprint(got.restarts); s != tc.wantRestarts {
				t.Fatalf("restarts = %s, want %s", s, tc.wantRestarts)
			}
			if got.state.LastStatus != "success" || !strings.Contains(got.log, "SUCCESS act") {
				t.Fatalf("state %+v, log:\n%s", got.state, got.log)
			}
			if tc.name == "rcon" && !strings.Contains(got.log, "asa1: ok") {
				t.Fatalf("log missing the rcon reply:\n%s", got.log)
			}
		})
	}
}

func TestRunScheduleActionsSkipStoppedTargets(t *testing.T) {
	t.Run("only target stopped", func(t *testing.T) {
		got := runActionSchedule(t, "    type: restart\n    instance: asa2\n", []string{"asa1"}, nil, nil)
		warning := "Warning: asa2 is stopped; restart command unsuccessful"
		if got.err != nil || got.state.LastStatus != "skipped" || len(got.execs) != 0 || len(got.restarts) != 0 {
			t.Fatalf("err %v, state %+v, execs %v, restarts %v", got.err, got.state, got.execs, got.restarts)
		}
		if !strings.Contains(got.stderr, warning) || !strings.Contains(got.log, warning) || !strings.Contains(got.log, "SKIPPED act") {
			t.Fatalf("stderr:\n%s\nlog:\n%s", got.stderr, got.log)
		}
	})
	t.Run("one running one stopped", func(t *testing.T) {
		got := runActionSchedule(t, "    type: save\n    instance: all\n", []string{"asa1"}, nil, nil)
		if got.err != nil || got.state.LastStatus != "success" || fmt.Sprint(got.execs) != "[[asa1 saveworld]]" {
			t.Fatalf("err %v, state %+v, execs %v", got.err, got.state, got.execs)
		}
		if !strings.Contains(got.log, "Warning: asa2 is stopped; save command unsuccessful") {
			t.Fatalf("log missing the warning:\n%s", got.log)
		}
	})
	t.Run("rcon failure keeps going", func(t *testing.T) {
		failAsa1 := func(service string, _ []string) ark.ExecResult {
			if service == "asa1" {
				return ark.ExecResult{ExitCode: 1, Stderr: "refused"}
			}
			return ark.ExecResult{}
		}
		got := runActionSchedule(t, "    type: rcon\n    instance: all\n    command: ServerChat hi\n", []string{"asa1", "asa2"}, failAsa1, nil)
		if got.err == nil || got.state.LastStatus != "failed" || !strings.Contains(got.state.LastError, "asa1: rcon exit code 1: refused") {
			t.Fatalf("err %v, state %+v", got.err, got.state)
		}
		if len(got.execs) != 2 {
			t.Fatalf("asa2 must still run: execs %v", got.execs)
		}
	})
	t.Run("restart failure", func(t *testing.T) {
		got := runActionSchedule(t, "    type: restart\n    instance: asa1\n", []string{"asa1"}, nil, errors.New("stop timed out"))
		if got.err == nil || got.state.LastStatus != "failed" || !strings.Contains(got.state.LastError, "asa1: stop timed out") {
			t.Fatalf("err %v, state %+v", got.err, got.state)
		}
	})
}

func TestCreateScheduleActionTypes(t *testing.T) {
	tmp := t.TempDir()
	overrideSchedulePathsForTest(t, tmp)
	setEnv(t, "COMPOSE_FILE", filepath.Join(tmp, "docker-compose.yaml"))
	if err := os.WriteFile(filepath.Join(tmp, "docker-compose.yaml"), []byte("services: {}\n"), 0o644); err != nil {
		t.Fatalf("write compose: %v", err)
	}
	cron := []string{"--cron", "0 * * * *"}
	withScheduleComposeDeps(t, emptyServices("asa1"), nil, func() {
		out := captureStdout(t, func() {
			for _, args := range [][]string{
				append([]string{"rcon", "--instance", "asa1", "--name", "chat", "--command", "ServerChat hi"}, cron...),
				append([]string{"save", "--instance", "all", "--name", "saver"}, cron...),
				append([]string{"restart", "--instance", "asa1", "--name", "reboot", "--disabled"}, cron...),
			} {
				if err := RunCreateSchedule(core.CurrentAppDeps(), args); err != nil {
					t.Fatalf("create %v: %v", args, err)
				}
			}
		})
		if !strings.Contains(out, "Created schedule chat") {
			t.Fatalf("output:\n%s", out)
		}
		for _, tc := range []struct {
			args []string
			want string
		}{
			{[]string{"rcon", "--instance", "asa1", "--name", "x"}, "Error: --command is required."},
			{[]string{"rcon", "--instance", "asa1", "--name", "x", "--command", "  "}, "Error: --command is required."},
			{[]string{"save", "--instance", "cluster", "--name", "x"}, "backup-only"},
			{[]string{"restart", "--instance", "cluster", "--name", "x"}, "backup-only"},
			{[]string{"rcon", "--instance", "cluster", "--name", "x", "--command", "c"}, "backup-only"},
			{[]string{"save", "--instance", "nope", "--name", "x"}, "nope"},
		} {
			err := RunCreateSchedule(core.CurrentAppDeps(), append(tc.args, cron...))
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("create %v: got %v, want %q", tc.args, err, tc.want)
			}
		}
	})
	file, err := core.CurrentAppDeps().ScheduleService.LoadScheduleFile(context.Background(), testStore, testComposeFile())
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	want := []schedule.BackupSchedule{
		{Name: "chat", Type: schedule.TypeRCON, Instance: "asa1", Enabled: true, Cron: "0 * * * *", Command: "ServerChat hi"},
		{Name: "reboot", Type: schedule.TypeRestart, Instance: "asa1", Enabled: false, Cron: "0 * * * *"},
		{Name: "saver", Type: schedule.TypeSave, Instance: "all", Enabled: true, Cron: "0 * * * *"},
	} // the store keeps schedules sorted by name
	if fmt.Sprint(file.Schedules) != fmt.Sprint(want) {
		t.Fatalf("saved schedules:\n got %+v\nwant %+v", file.Schedules, want)
	}
}

func TestUpdateScheduleTypeRules(t *testing.T) {
	tmp := t.TempDir()
	overrideSchedulePathsForTest(t, tmp)
	initial := `x-ark-schedules:
  - name: chat
    type: rcon
    instance: asa1
    enabled: true
    cron: "0 * * * *"
    command: ServerChat hi
  - name: saver
    type: save
    instance: asa1
    enabled: true
    cron: "0 * * * *"
  - name: nightly
    type: backup
    instance: asa1
    enabled: true
    cron: "0 3 * * *"
`
	writeScheduleFixture(t, initial)
	seeded, _ := savedSchedulesYAML(t)

	showOut := captureStdout(t, func() {
		if err := RunShowSchedule(core.CurrentAppDeps(), []string{"--name", "chat"}); err != nil {
			t.Fatalf("show: %v", err)
		}
	})
	if !strings.Contains(showOut, "command: ServerChat hi") || !strings.Contains(showOut, "retain_count: 0") {
		t.Fatalf("show output:\n%s", showOut)
	}

	for _, tc := range []struct {
		args []string
		want string
	}{
		{[]string{"--name", "saver", "--command", "x"}, "--command applies only to rcon"},
		{[]string{"--name", "nightly", "--command", "x"}, "--command applies only to rcon"},
		{[]string{"--name", "saver", "--format", "tar"}, "--format is backup-only"},
		{[]string{"--name", "saver", "--retain-days", "3"}, "--retain-days is backup-only"},
		{[]string{"--name", "chat", "--instance", "cluster"}, "backup-only"},
		{[]string{"--name", "chat", "--command", " "}, "--command must not be empty"},
	} {
		err := RunUpdateSchedule(core.CurrentAppDeps(), tc.args)
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Fatalf("update %v: got %v, want %q", tc.args, err, tc.want)
		}
	}
	unchanged, err := savedSchedulesYAML(t)
	if err != nil || string(unchanged) != string(seeded) {
		t.Fatalf("refused updates changed the file (%v):\n%s", err, unchanged)
	}

	if err := RunUpdateSchedule(core.CurrentAppDeps(), []string{"--name", "chat", "--command", "Broadcast hi"}); err != nil {
		t.Fatalf("update command: %v", err)
	}
	jsonOut := captureStdout(t, func() {
		if err := RunShowSchedule(core.CurrentAppDeps(), []string{"--name", "chat", "--json"}); err != nil {
			t.Fatalf("show --json: %v", err)
		}
	})
	var detail core.ScheduleDetailJSON
	if err := json.Unmarshal([]byte(jsonOut), &detail); err != nil || detail.Command != "Broadcast hi" {
		t.Fatalf("show --json (%v):\n%s", err, jsonOut)
	}
}

func TestValidateScheduleActionTypes(t *testing.T) {
	entry := func(name, typ, instance, command string) schedule.BackupSchedule {
		return schedule.BackupSchedule{Name: name, Type: typ, Instance: instance, Enabled: true, Cron: "0 * * * *", Command: command}
	}
	withScheduleComposeDeps(t, emptyServices("asa1"), nil, func() {
		valid := schedule.ScheduleFile{Schedules: []schedule.BackupSchedule{
			entry("chat", schedule.TypeRCON, "asa1", "ServerChat hi"),
			entry("saver", schedule.TypeSave, "all", ""),
			entry("reboot", schedule.TypeRestart, "asa1", ""),
			entry("nightly", schedule.TypeBackup, "asa1", ""),
		}}
		captureStdout(t, func() {
			if n, err := validateScheduleFile(valid, "", "docker-compose.yaml"); err != nil || n != 4 {
				t.Fatalf("validate valid entries: n=%d err=%v", n, err)
			}
		})
		for _, bad := range []schedule.BackupSchedule{
			entry("chat", schedule.TypeRCON, "asa1", ""),
			entry("saver", schedule.TypeSave, "cluster", ""),
			entry("reboot", schedule.TypeRestart, "cluster", ""),
			entry("weird", "monthly", "asa1", ""),
		} {
			if _, err := validateScheduleFile(schedule.ScheduleFile{Schedules: []schedule.BackupSchedule{bad}}, "", "docker-compose.yaml"); err == nil {
				t.Fatalf("expected validation failure for %+v", bad)
			}
		}
	})
}

// TestScheduleCommandsUseComposeFileFlag runs every schedule command from an unrelated directory with
// --compose-file naming a second compose file: each reads (and writes) only that file, and run state lands
// under its .ark-manage.
func TestScheduleCommandsUseComposeFileFlag(t *testing.T) {
	tmp := t.TempDir()
	overrideSchedulePathsForTest(t, tmp) // COMPOSE_FILE: the default file, which no command may touch
	other := filepath.Join(tmp, "other", "other.yaml")
	if err := os.MkdirAll(filepath.Dir(other), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(other, []byte("services: {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Chdir(t.TempDir())
	writeScheduleFixture(t, "x-ark-schedules:\n  - name: nightly\n    enabled: true\n    type: backup\n    instance: all\n    cron: \"0 3 * * *\"\n")
	var wrongFiles []string
	var backupArgs []string
	onlyOther := func(file string) error {
		if file != other {
			wrongFiles = append(wrongFiles, file)
			return fmt.Errorf("wrong compose file %s", file)
		}
		return nil
	}
	cf := []string{"--compose-file", other}
	commands := []struct {
		name string
		run  func(d *core.AppDeps) error
	}{
		{"create", func(*core.AppDeps) error {
			return runCreateScheduleBackup(append([]string{"--instance", "all", "--name", "extra", "--cron", "0 4 * * *"}, cf...))
		}},
		{"update", func(d *core.AppDeps) error {
			return RunUpdateSchedule(d, append([]string{"--name", "nightly", "--cron", "0 5 * * *"}, cf...))
		}},
		{"list", func(d *core.AppDeps) error { return RunListSchedules(d, cf) }},
		{"show", func(d *core.AppDeps) error { return RunShowSchedule(d, append([]string{"--name", "nightly"}, cf...)) }},
		{"run", func(d *core.AppDeps) error { return RunRunSchedule(d, append([]string{"--name", "nightly"}, cf...)) }},
		{"check", func(d *core.AppDeps) error { return RunCheckSchedule(d, append([]string{"--name", "nightly"}, cf...)) }},
		{"disable", func(d *core.AppDeps) error {
			return RunDisableSchedule(d, append([]string{"--name", "nightly"}, cf...))
		}},
		{"enable", func(d *core.AppDeps) error { return RunEnableSchedule(d, append([]string{"--name", "nightly"}, cf...)) }},
		{"validate", func(d *core.AppDeps) error { return RunValidateSchedule(d, cf) }},
		{"remove", func(d *core.AppDeps) error { return RunRemoveSchedule(d, append([]string{"--name", "extra"}, cf...)) }},
	}
	outputs := map[string]string{}
	withScheduleComposeDeps(t, emptyServices("asa1"), func(d *core.AppDeps) {
		mock := d.ComposeService.(*arktest.MockCompose)
		load, save := mock.LoadProjectFn, mock.SaveProjectFn
		mock.LoadProjectFn = func(ctx context.Context, file string) (*ark.ComposeProject, error) {
			if err := onlyOther(file); err != nil {
				return nil, err
			}
			return load(ctx, file)
		}
		mock.SaveProjectFn = func(ctx context.Context, file string, project *ark.ComposeProject) error {
			if err := onlyOther(file); err != nil {
				return err
			}
			return save(ctx, file, project)
		}
		d.RunBackupFromSchedule = func(_ string, args []string) error {
			backupArgs = args
			return nil
		}
	}, func() {
		for _, c := range commands {
			var err error
			outputs[c.name] = captureStdout(t, func() { err = c.run(core.CurrentAppDeps()) })
			if err != nil {
				t.Fatalf("%s: %v", c.name, err)
			}
		}
	})
	if len(wrongFiles) != 0 {
		t.Fatalf("commands touched other compose files: %v", wrongFiles)
	}
	if !strings.Contains(outputs["list"], "nightly") || !strings.Contains(outputs["list"], "extra") {
		t.Fatalf("list did not show other.yaml's schedules:\n%s", outputs["list"])
	}
	if !strings.Contains(outputs["show"], "compose_file: "+other) || strings.Contains(outputs["show"], "config_path") {
		t.Fatalf("show output:\n%s", outputs["show"])
	}
	if !strings.Contains(strings.Join(backupArgs, " "), "--compose-file "+other) {
		t.Fatalf("backup args = %v, want --compose-file %s", backupArgs, other)
	}
	if !strings.Contains(outputs["check"], "last_status:      success") {
		t.Fatalf("check did not see the run's state:\n%s", outputs["check"])
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(other), ".ark-manage")); err != nil {
		t.Fatalf("run state not under the compose file's .ark-manage: %v", err)
	}
}
