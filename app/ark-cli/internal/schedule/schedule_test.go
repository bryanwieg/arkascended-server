package schedule

// Tests: schedule lifecycle: create/list/remove/show/update/run/validate, lock and status tracking, compose config caching

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

	configBytes, err := os.ReadFile(core.CurrentAppDeps().ScheduleService.ConfigPath)
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

	configBytes, err = os.ReadFile(core.CurrentAppDeps().ScheduleService.ConfigPath)
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

	initial := "version: 1\nschedules:\n  - name: nightly\n    type: backup\n    instance: asa1\n    enabled: true\n    cron: \"0 3 * * *\"\n"
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

	initial := `version: 1
schedules:
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
	updatedBytes, err := os.ReadFile(core.CurrentAppDeps().ScheduleService.ConfigPath)
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

	initial := `version: 1
schedules:
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

func TestValidateScheduleCachesComposeConfigByFile(t *testing.T) {
	tmp := t.TempDir()
	overrideSchedulePathsForTest(t, tmp)

	composePath := filepath.Join(tmp, "docker-compose.yaml")
	initial := `version: 1
schedules:
  - name: nightly
    type: backup
    instance: asa1
    enabled: true
    cron: "0 3 * * *"
    compose_file: ` + composePath + `
  - name: hourly
    type: backup
    instance: asa2
    enabled: true
    cron: "0 * * * *"
    compose_file: ` + composePath + `
`
	writeScheduleFixture(t, initial)

	// Create the compose file so requireCompose (os.Stat) succeeds.
	if err := os.WriteFile(composePath, []byte("services:\n  asa1: {}\n  asa2: {}\n"), 0o644); err != nil {
		t.Fatalf("write compose: %v", err)
	}

	loadCalls := 0
	withPatchedAppDeps(t, func(d *core.AppDeps) {
		d.ComposeService = &arktest.MockCompose{
			LoadProjectFn: func(_ context.Context, path string) (*ark.ComposeProject, error) {
				loadCalls++
				if path != composePath {
					t.Fatalf("unexpected compose path: %s", path)
				}
				return fakeProject(emptyServices("asa1", "asa2")), nil
			},
		}
	}, func() {
		if err := RunValidateSchedule(core.CurrentAppDeps(), []string{}); err != nil {
			t.Fatalf("runValidateSchedule: %v", err)
		}
	})

	if loadCalls != 1 {
		t.Fatalf("loadComposeConfig calls=%d want 1", loadCalls)
	}
}

func TestRunScheduleWritesLockLogAndStatus(t *testing.T) {
	tmp := t.TempDir()
	overrideSchedulePathsForTest(t, tmp)

	initial := `version: 1
schedules:
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
	initial := `version: 1
schedules:
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

	initial := `version: 1
schedules:
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

	initial := `version: 1
schedules:
  - name: nightly
    type: backup
    instance: asa1
    enabled: true
    cron: "0 3 * * *"
    format: tar.gz
    retain_count: 20
    retain_days: 14
    retain_size_gb: 5
    compose_file: docker-compose.alt.yaml
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
		"--compose-file docker-compose.alt.yaml",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("show output missing %q\n%s", want, out)
		}
	}
}

func TestRunScheduleFailureWritesFailedState(t *testing.T) {
	tmp := t.TempDir()
	overrideSchedulePathsForTest(t, tmp)
	writeScheduleFixture(t, `version: 1
schedules:
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
	writeScheduleFixture(t, `version: 1
schedules:
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
	writeScheduleFixture(t, `version: 1
schedules:
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
	writeScheduleFixture(t, "version: 1\nschedules: []\n")

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
	writeScheduleFixture(t, `version: 1
schedules:
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
	writeScheduleFixture(t, `version: 1
schedules:
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

	configBytes, err := os.ReadFile(core.CurrentAppDeps().ScheduleService.ConfigPath)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	if !strings.Contains(string(configBytes), "enabled: false") {
		t.Fatalf("expected enabled: false in config:\n%s", string(configBytes))
	}
}

func TestCreateScheduleBackupWithComposeFile(t *testing.T) {
	tmp := t.TempDir()
	overrideSchedulePathsForTest(t, tmp)
	composePath := filepath.Join(tmp, "custom-compose.yaml")
	if err := os.WriteFile(composePath, []byte("services:\n  asa1:\n    labels: {ark.port.asa: '27015'}\n    image: ark-server:latest\n"), 0o644); err != nil {
		t.Fatalf("write compose: %v", err)
	}

	withScheduleComposeDeps(t, emptyServices("asa1"), nil, func() {
		if err := runCreateScheduleBackup([]string{
			"--instance", "asa1", "--name", "nightly", "--cron", "0 3 * * *",
			"--compose-file", composePath,
		}); err != nil {
			t.Fatalf("runCreateScheduleBackup: %v", err)
		}
	})

	configBytes, err := os.ReadFile(core.CurrentAppDeps().ScheduleService.ConfigPath)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	if !strings.Contains(string(configBytes), composePath) {
		t.Fatalf("expected compose_file in config:\n%s", string(configBytes))
	}
}

func TestValidateScheduleNoSchedulesConfigured(t *testing.T) {
	tmp := t.TempDir()
	overrideSchedulePathsForTest(t, tmp)
	writeScheduleFixture(t, "version: 1\nschedules: []\n")

	if err := RunValidateSchedule(core.CurrentAppDeps(), []string{}); err == nil {
		t.Fatal("expected error for empty schedule list")
	}
}

func TestRunEnableScheduleFlipsEnabled(t *testing.T) {
	tmp := t.TempDir()
	overrideSchedulePathsForTest(t, tmp)
	writeScheduleFixture(t, `version: 1
schedules:
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

	configBytes, err := os.ReadFile(core.CurrentAppDeps().ScheduleService.ConfigPath)
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
	writeScheduleFixture(t, `version: 1
schedules:
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
	writeScheduleFixture(t, `version: 1
schedules:
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
	writeScheduleFixture(t, `version: 1
schedules:
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

	configBytes, err := os.ReadFile(core.CurrentAppDeps().ScheduleService.ConfigPath)
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
	writeScheduleFixture(t, `version: 1
schedules:
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
	writeScheduleFixture(t, `version: 1
schedules:
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
	writeScheduleFixture(t, `version: 1
schedules:
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
	writeScheduleFixture(t, `version: 1
schedules:
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
	writeScheduleFixture(t, `version: 1
schedules:
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
	writeScheduleFixture(t, `version: 1
schedules:
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
	writeScheduleFixture(t, `version: 1
schedules:
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

func writeScheduleFixture(t *testing.T, raw string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(core.CurrentAppDeps().ScheduleService.ConfigPath), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(core.CurrentAppDeps().ScheduleService.ConfigPath, []byte(raw), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
}

func withScheduleComposeDeps(
	t *testing.T,
	services types.Services,
	mutate func(*core.AppDeps),
	run func(),
) {
	t.Helper()
	withPatchedAppDeps(t, func(d *core.AppDeps) {
		d.ComposeService = &arktest.MockCompose{
			LoadProjectFn: func(_ context.Context, _ string) (*ark.ComposeProject, error) {
				return fakeProject(services), nil
			},
		}
		if mutate != nil {
			mutate(d)
		}
	}, run)
}

func overrideSchedulePathsForTest(t *testing.T, root string) {
	t.Helper()
	patchAppDepsForTest(t, func(d *core.AppDeps) {
		d.ScheduleService = schedule.New(root, nil)
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
	configBytes, err := os.ReadFile(core.CurrentAppDeps().ScheduleService.ConfigPath)
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
// service in the schedule's project, and a working directory other than the compose directory.
func TestScheduleCommandsWarnWithoutScheduler(t *testing.T) {
	const notEnabled = "the scheduler service is not enabled"
	const wrongDir = "a scheduler only reads"
	withScheduler := types.Services{ark.SchedulerService: {Name: ark.SchedulerService, Labels: types.Labels{ark.RoleLabel: ark.SchedulerRole}}}
	for _, tc := range []struct {
		name      string
		services  types.Services
		otherCwd  bool
		wantWarns []string
		noWarns   []string
	}{
		{name: "no scheduler", services: emptyServices("asa1"), wantWarns: []string{notEnabled}, noWarns: []string{wrongDir}},
		{name: "scheduler enabled", services: withScheduler, noWarns: []string{notEnabled, wrongDir}},
		{name: "other working dir", services: withScheduler, otherCwd: true, wantWarns: []string{wrongDir}, noWarns: []string{notEnabled}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tmp := t.TempDir()
			overrideSchedulePathsForTest(t, tmp)
			composeFile := filepath.Join(tmp, "docker-compose.yaml")
			setEnv(t, "COMPOSE_FILE", composeFile)
			if err := os.WriteFile(composeFile, []byte("services: {}\n"), 0o644); err != nil {
				t.Fatalf("write compose: %v", err)
			}
			if tc.otherCwd {
				t.Chdir(t.TempDir())
			} else {
				t.Chdir(tmp)
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
	writeScheduleFixture(t, "version: 1\nschedules:\n  - name: act\n    enabled: true\n    cron: \"0 * * * *\"\n    compose_file: "+filepath.Join(tmp, "docker-compose.yaml")+"\n"+entry)
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
				got.err = RunScheduleOnce(core.CurrentAppDeps(), mustLoadSchedule(t, "act"))
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
	_, s, _, err := core.CurrentAppDeps().ScheduleService.LoadScheduleByName(name)
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
