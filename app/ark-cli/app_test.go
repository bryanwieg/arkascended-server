package main

// Tests: CLI routing, dependency injection seam routing, and app initialization

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
	"unsafe"

	"ark-cli/internal/compose"
	"ark-cli/internal/core"
	"ark-cli/internal/reachability"
	"pkg/ark"
	"pkg/ark/arktest"
	"pkg/ark/host"
)

// TestUsageListsCommands checks the usage output lists all supported verbs.
// - Expect: usage output contains supported verbs and object forms (no cron placeholder).
func TestUsageListsCommands(t *testing.T) {
	out := captureStderr(t, func() { usage() })
	for _, cmd := range []string{"create", "edit", "reconfigure", "update", "apply", "start", "stop", "restart", "save", "logs", "show", "restore", "list", "check", "wait", "validate", "prune", "remove", "run"} {
		if !strings.Contains(out, cmd) {
			t.Fatalf("usage missing %s", cmd)
		}
	}
	if strings.Contains(out, " cron ") || strings.Contains(out, "\n  cron") {
		t.Fatalf("usage should not list cron placeholder anymore:\n%s", out)
	}
	for _, syntax := range []string{"create instance", "create schedule", "edit ini", "reconfigure instance", "update instance", "update schedule", "apply schedules", "restart instance", "save instance", "logs instance", "show instance", "show backup", "show compose", "show volume-paths", "show schedule", "run schedule", "list instances", "list schedules", "create backup", "restore backup", "check reachability", "listen port", "check instance", "wait instance", "validate compose", "validate schedule", "validate backup", "prune backups", "remove schedule"} {
		if !strings.Contains(out, syntax) {
			t.Fatalf("usage missing syntax %q", syntax)
		}
	}
}

func TestCLIRunTopLevelHelpFlagShowsUsage(t *testing.T) {
	cli := NewCLI(nil)
	var err error
	out := captureStderr(t, func() {
		err = cli.Run([]string{"-h"})
	})
	if err != nil {
		t.Fatalf("CLI.Run(-h): %v", err)
	}
	if !strings.Contains(out, "Usage: ark-cli") {
		t.Fatalf("expected usage output, got:\n%s", out)
	}
	if !strings.Contains(out, "Commands:") {
		t.Fatalf("expected commands list, got:\n%s", out)
	}
}

func TestParseCommandRouteEnforcesVerbObjectSyntax(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		in       []string
		wantVerb string
		wantArgs []string
		wantErr  string
	}{
		{
			name:     "create instance",
			in:       []string{"create", "instance", "asa1", "--admin-password", "a", "--server-password", "b"},
			wantVerb: "create",
			wantArgs: []string{"asa1", "--admin-password", "a", "--server-password", "b"},
		},
		{
			name:     "create backup",
			in:       []string{"create", "backup", "asa1"},
			wantVerb: "backup",
			wantArgs: []string{"asa1"},
		},
		{
			name:     "create schedule backup",
			in:       []string{"create", "schedule", "backup", "asa1", "--name", "nightly", "--cron", "0 3 * * *"},
			wantVerb: "create_schedule",
			wantArgs: []string{"backup", "asa1", "--name", "nightly", "--cron", "0 3 * * *"},
		},
		{
			name:     "edit ini",
			in:       []string{"edit", "ini", "asa1", "--file", "game"},
			wantVerb: "edit_ini",
			wantArgs: []string{"asa1", "--file", "game"},
		},
		{
			name:     "restore backup",
			in:       []string{"restore", "backup", "asa1", "latest"},
			wantVerb: "restore",
			wantArgs: []string{"asa1", "latest"},
		},
		{
			name:     "start instance",
			in:       []string{"start", "instance", "asa1"},
			wantVerb: "start",
			wantArgs: []string{"asa1"},
		},
		{
			name:     "stop instance",
			in:       []string{"stop", "instance", "all"},
			wantVerb: "stop",
			wantArgs: []string{"all"},
		},
		{
			name:     "remove instance",
			in:       []string{"remove", "instance", "asa1", "--kill"},
			wantVerb: "remove",
			wantArgs: []string{"asa1", "--kill"},
		},
		{
			name:     "remove schedule",
			in:       []string{"remove", "schedule", "nightly"},
			wantVerb: "remove_schedule",
			wantArgs: []string{"nightly"},
		},
		{
			name:     "update instance",
			in:       []string{"update", "instance", "asa1", "--check"},
			wantVerb: "update",
			wantArgs: []string{"asa1", "--check"},
		},
		{
			name:     "reconfigure instance",
			in:       []string{"reconfigure", "instance", "asa1", "--map-name", "TheCenter"},
			wantVerb: "reconfigure",
			wantArgs: []string{"asa1", "--map-name", "TheCenter"},
		},
		{
			name:     "update schedule",
			in:       []string{"update", "schedule", "nightly", "--cron", "0 4 * * *"},
			wantVerb: "update_schedule",
			wantArgs: []string{"nightly", "--cron", "0 4 * * *"},
		},
		{
			name:     "apply schedules",
			in:       []string{"apply", "schedules"},
			wantVerb: "apply_schedules",
			wantArgs: []string{},
		},
		{
			name:     "restart instance",
			in:       []string{"restart", "instance", "all"},
			wantVerb: "restart",
			wantArgs: []string{"all"},
		},
		{
			name:     "save instance",
			in:       []string{"save", "instance", "asa1"},
			wantVerb: "save",
			wantArgs: []string{"asa1"},
		},
		{
			name:     "logs instance",
			in:       []string{"logs", "instance", "asa1", "--tail", "50"},
			wantVerb: "logs",
			wantArgs: []string{"asa1", "--tail", "50"},
		},
		{
			name:     "check instance",
			in:       []string{"check", "instance", "asa1"},
			wantVerb: "check_instance",
			wantArgs: []string{"asa1"},
		},
		{
			name:     "wait instance",
			in:       []string{"wait", "instance", "asa1", "--for", "ready"},
			wantVerb: "wait",
			wantArgs: []string{"asa1", "--for", "ready"},
		},
		{
			name:     "validate compose",
			in:       []string{"validate", "compose"},
			wantVerb: "validate_compose",
			wantArgs: []string{},
		},
		{
			name:     "validate schedule",
			in:       []string{"validate", "schedule", "nightly"},
			wantVerb: "validate_schedule",
			wantArgs: []string{"nightly"},
		},
		{
			name:     "validate backup",
			in:       []string{"validate", "backup", "asa1", "latest"},
			wantVerb: "validate_backup",
			wantArgs: []string{"asa1", "latest"},
		},
		{
			name:     "prune backups",
			in:       []string{"prune", "backups", "all"},
			wantVerb: "prune_backups",
			wantArgs: []string{"all"},
		},
		{
			name:     "check reachability port",
			in:       []string{"check", "reachability", "port", "27015"},
			wantVerb: "check_reachability",
			wantArgs: []string{"port", "27015"},
		},
		{
			name:     "listen port",
			in:       []string{"listen", "port", "27015"},
			wantVerb: "listen_port",
			wantArgs: []string{"27015"},
		},
		{
			name:     "list backups",
			in:       []string{"list", "backups", "asa1"},
			wantVerb: "list_backups",
			wantArgs: []string{"asa1"},
		},
		{
			name:     "list instances",
			in:       []string{"list", "instances"},
			wantVerb: "list_instances",
			wantArgs: []string{},
		},
		{
			name:     "list schedules",
			in:       []string{"list", "schedules"},
			wantVerb: "list_schedules",
			wantArgs: []string{},
		},
		{
			name:     "show instance",
			in:       []string{"show", "instance", "asa1"},
			wantVerb: "show_instance",
			wantArgs: []string{"asa1"},
		},
		{
			name:     "show backup",
			in:       []string{"show", "backup", "asa1", "latest"},
			wantVerb: "show_backup",
			wantArgs: []string{"asa1", "latest"},
		},
		{
			name:     "show compose",
			in:       []string{"show", "compose"},
			wantVerb: "show_compose",
			wantArgs: []string{},
		},
		{
			name:     "show volume-paths",
			in:       []string{"show", "volume-paths", "asa1"},
			wantVerb: "show_volume_paths",
			wantArgs: []string{"asa1"},
		},
		{
			name:     "show vol-paths (compat alias)",
			in:       []string{"show", "vol-paths", "asa1"},
			wantVerb: "show_vol_paths_compat",
			wantArgs: []string{"asa1"},
		},
		{
			name:     "show schedule",
			in:       []string{"show", "schedule", "nightly"},
			wantVerb: "show_schedule",
			wantArgs: []string{"nightly"},
		},
		{
			name:     "run schedule",
			in:       []string{"run", "schedule", "nightly"},
			wantVerb: "run_schedule",
			wantArgs: []string{"nightly"},
		},
		{
			name:     "run scheduler",
			in:       []string{"run", "scheduler"},
			wantVerb: "run_scheduler",
			wantArgs: []string{},
		},
		{
			name:    "legacy backup rejected",
			in:      []string{"backup", "asa1"},
			wantErr: "unknown command verb",
		},
		{
			name:    "edit wrong object rejected",
			in:      []string{"edit", "instance", "asa1"},
			wantErr: "edit expected object",
		},
		{
			name:    "down removed",
			in:      []string{"down", "instance", "asa1"},
			wantErr: "unknown command verb",
		},
		{
			name:     "check reachability without subcommand",
			in:       []string{"check", "reachability"},
			wantVerb: "check_reachability",
			wantArgs: []string{},
		},
		{
			name:    "cron removed",
			in:      []string{"cron"},
			wantErr: "unknown command verb",
		},
		{
			name:    "missing object rejected",
			in:      []string{"start", "asa1"},
			wantErr: "expected object",
		},
		{
			name:    "create schedule wrong object rejected",
			in:      []string{"create", "service", "backup"},
			wantErr: "create expected object",
		},
		{
			name:    "update wrong object rejected",
			in:      []string{"update", "backup", "asa1"},
			wantErr: "update expected object",
		},
		{
			name:    "reconfigure wrong object rejected",
			in:      []string{"reconfigure", "backup", "asa1"},
			wantErr: "reconfigure expected object",
		},
		{
			name:    "run wrong object rejected",
			in:      []string{"run", "backup", "x"},
			wantErr: "run expected object",
		},
		{
			name:    "apply wrong object rejected",
			in:      []string{"apply", "schedule"},
			wantErr: "apply expected object",
		},
		{
			name:    "list wrong schedule object rejected",
			in:      []string{"list", "schedule"},
			wantErr: "list expected object",
		},
		{
			name:    "save wrong object rejected",
			in:      []string{"save", "backup", "asa1"},
			wantErr: "save expected object",
		},
		{
			name:    "remove wrong object rejected",
			in:      []string{"remove", "backup", "x"},
			wantErr: "remove expected object",
		},
		{
			name:    "check wrong object rejected",
			in:      []string{"check", "backup", "x"},
			wantErr: "check expected object",
		},
		{
			name:    "wait wrong object rejected",
			in:      []string{"wait", "compose"},
			wantErr: "wait expected object",
		},
		{
			name:    "validate wrong object rejected",
			in:      []string{"validate", "instance"},
			wantErr: "validate expected object",
		},
		{
			name:    "verify removed",
			in:      []string{"verify", "backup", "asa1"},
			wantErr: "unknown command verb",
		},
		{
			name:    "prune wrong object rejected",
			in:      []string{"prune", "backup", "asa1"},
			wantErr: "prune expected object",
		},
		{
			name:    "show wrong object rejected",
			in:      []string{"show", "service", "x"},
			wantErr: "show expected object",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			verb, gotArgs, err := parseCommandRoute(tt.in)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(strings.ToLower(err.Error()), strings.ToLower(tt.wantErr)) {
					t.Fatalf("parseCommandRoute(%#v) err=%v, want substring %q", tt.in, err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseCommandRoute(%#v): %v", tt.in, err)
			}
			if verb != tt.wantVerb {
				t.Fatalf("verb=%q want %q", verb, tt.wantVerb)
			}
			if !reflect.DeepEqual(gotArgs, tt.wantArgs) {
				t.Fatalf("args=%#v want %#v", gotArgs, tt.wantArgs)
			}
		})
	}
}

func TestParseCommandRouteShowMissingObjectListsSchedule(t *testing.T) {
	_, _, err := parseCommandRoute([]string{"show"})
	if err == nil {
		t.Fatal("expected error")
	}
	want := "Error: show expected object: ini, instance, backup, compose, volume-paths, vol-paths, or schedule."
	if err.Error() != want {
		t.Fatalf("err=%q want %q", err.Error(), want)
	}
}

func TestMainSupportsCreateInstance(t *testing.T) {
	tempDir := t.TempDir()
	composePath := filepath.Join(tempDir, "docker-compose.yaml")
	setEnv(t, "COMPOSE_FILE", composePath)

	// Main() calls core.SetCurrentAppDeps — save/restore to avoid polluting other tests.
	prevDeps := core.CurrentAppDeps()
	t.Cleanup(func() { core.SetCurrentAppDeps(prevDeps) })

	oldArgs := os.Args
	os.Args = []string{
		"ark-cli",
		"create",
		"instance",
		"--name", "asa1",
		"--admin-password",
		"adminpass",
		"--server-password",
		"serverpass",
	}
	t.Cleanup(func() { os.Args = oldArgs })

	out := captureStdout(t, func() { Main() })
	if !strings.Contains(out, composePath) {
		t.Fatalf("expected output to mention compose path, got %q", out)
	}
	if _, err := os.Stat(composePath); err != nil {
		t.Fatalf("expected compose file to be created: %v", err)
	}
}

func TestMainSupportsCheckReachability(t *testing.T) {
	out := captureStdout(t, func() {
		withPatchedAppDeps(t, func(d *core.AppDeps) {
			d.HostDeps.FetchText = func(url string) (string, error) {
				switch {
				case strings.HasSuffix(url, "/api/me"):
					return "203.0.113.10", nil
				case strings.Contains(url, "starrupture-utilities.com"):
					return `{"success":true,"result":{"tcp":{"is_open":true,"status":"open"},"udp":{"is_open":true,"status":"open"}}}`, nil
				case strings.Contains(url, "portchecker.io/api/me/"):
					return "open", nil
				default:
					return "", nil
				}
			}
		}, func() {
			if err := reachability.RunCheckReachability(core.CurrentAppDeps(), []string{"--port", "27015"}); err != nil {
				t.Fatalf("RunCheckReachability: %v", err)
			}
		})
	})
	if !strings.Contains(out, "203.0.113.10") {
		t.Fatalf("unexpected check reachability output: %q", out)
	}
	if !strings.Contains(strings.ToUpper(out), "OPEN") {
		t.Fatalf("expected OPEN result in output: %q", out)
	}
}

func TestWithPatchedAppDepsRoutesSeamCalls(t *testing.T) {
	called := false
	withPatchedAppDeps(t, func(d *core.AppDeps) {
		d.Runtime.LookPath = func(file string) (string, error) {
			called = true
			if file != "docker" {
				t.Fatalf("unexpected lookup target: %s", file)
			}
			return "", nil
		}
	}, func() {
		_, _ = core.CurrentAppDeps().Runtime.LookPath("docker")
	})
	if !called {
		t.Fatal("expected patched dependency to be called")
	}
}

func TestWithPatchedAppDepsRoutesComposeServiceLoadProject(t *testing.T) {
	calledLoad := false
	withPatchedAppDeps(t, func(d *core.AppDeps) {
		d.ComposeService = &arktest.MockCompose{
			LoadProjectFn: func(_ context.Context, path string) (*ark.ComposeProject, error) {
				calledLoad = true
				if path != "docker-compose.yaml" {
					t.Fatalf("unexpected compose path in load: %s", path)
				}
				return fakeProject(emptyServices()), nil
			},
		}
	}, func() {
		_, _, err := compose.ResolveRequiredComposeConfig(core.CurrentAppDeps(), "")
		if err != nil {
			t.Fatalf("ResolveRequiredComposeConfig returned error: %v", err)
		}
	})
	if !calledLoad {
		t.Fatalf("expected LoadProject seam to be called")
	}
}

func TestWithPatchedAppDepsRoutesFetchText(t *testing.T) {
	withPatchedAppDeps(t, func(d *core.AppDeps) {
		d.HostDeps.FetchText = func(url string) (string, error) {
			if url != "https://example.test" {
				t.Fatalf("unexpected url: %s", url)
			}
			return "ok", nil
		}
	}, func() {
		got, err := host.FetchText(core.CurrentAppDeps().HostDeps, "https://example.test")
		if err != nil {
			t.Fatalf("fetchText err=%v", err)
		}
		if got != "ok" {
			t.Fatalf("fetchText=%q want ok", got)
		}
	})
}

func TestWithPatchedAppDepsRoutesScheduleNow(t *testing.T) {
	want := time.Date(2026, time.February, 25, 12, 0, 0, 0, time.UTC)
	withPatchedAppDeps(t, func(d *core.AppDeps) {
		d.ScheduleService.Now = func() time.Time { return want }
	}, func() {
		if got := core.CurrentAppDeps().ScheduleService.Now(); !got.Equal(want) {
			t.Fatalf("scheduleNow=%v want %v", got, want)
		}
	})
}

func TestWithPatchedAppDepsRoutesStopService(t *testing.T) {
	want := errors.New("stop failed")
	withPatchedAppDeps(t, func(d *core.AppDeps) {
		d.StopService = func(composeFile, service string) error {
			if composeFile != "compose.yaml" || service != "asa1" {
				t.Fatalf("unexpected args: %s %s", composeFile, service)
			}
			return want
		}
	}, func() {
		err := core.CurrentAppDeps().StopService("compose.yaml", "asa1")
		if !errors.Is(err, want) {
			t.Fatalf("StopService err=%v want %v", err, want)
		}
	})
}

func assertCurrentAppDepsIs(t *testing.T, expected *core.AppDeps, context string) {
	t.Helper()
	// Compare core.AppDeps pointer identity (withAppDeps saves/restores *core.AppDeps).
	if core.CurrentAppDeps() != expected {
		t.Fatalf("expected app deps to match: %s", context)
	}
}

func assertCurrentAppDepsDiffers(t *testing.T, disallowed *core.AppDeps, context string) *core.AppDeps {
	t.Helper()
	if core.CurrentAppDeps() == disallowed {
		t.Fatalf("expected app deps to differ: %s", context)
	}
	return core.CurrentAppDeps()
}

func assertLookPathEquals(t *testing.T, want, context string) {
	t.Helper()
	got, err := core.CurrentAppDeps().Runtime.LookPath("docker")
	if err != nil || got != want {
		t.Fatalf("%s lookPath result=%q err=%v", context, got, err)
	}
}

func runNestedWithAppDepsAssertions(t *testing.T, original *core.AppDeps, inner *core.AppDeps) {
	t.Helper()
	outerActive := assertCurrentAppDepsDiffers(t, original, "outer app deps should be installed")
	assertLookPathEquals(t, "outer", "outer")

	core.WithAppDeps(inner, func() {
		assertCurrentAppDepsDiffers(t, outerActive, "nested app deps should replace outer deps")
		assertLookPathEquals(t, "inner", "inner")
	})

	assertCurrentAppDepsIs(t, outerActive, "outer app deps should be restored after nested withAppDeps")
	assertLookPathEquals(t, "outer", "outer after nested restore")
}

func TestWithAppDepsRestoresNestedState(t *testing.T) {
	original := core.CurrentAppDeps()

	outer := cloneAppDepsForTest()
	outer.Runtime.LookPath = func(_ string) (string, error) { return "outer", nil }
	inner := cloneAppDepsForTest()
	inner.Runtime.LookPath = func(_ string) (string, error) { return "inner", nil }

	core.WithAppDeps(outer, func() {
		runNestedWithAppDepsAssertions(t, original, inner)
	})

	assertCurrentAppDepsIs(t, original, "original app deps should be restored after withAppDeps")
}

func TestAppDepsWithDefaultsMergesNonZeroFields(t *testing.T) {
	override := &core.AppDeps{}
	overrideValue := reflect.ValueOf(override).Elem()

	for i := 0; i < overrideValue.NumField(); i++ {
		field := overrideValue.Field(i)
		if field.Kind() != reflect.Func {
			continue
		}
		funcType := field.Type()
		fn := reflect.MakeFunc(funcType, func(args []reflect.Value) []reflect.Value {
			out := make([]reflect.Value, funcType.NumOut())
			for j := 0; j < funcType.NumOut(); j++ {
				out[j] = reflect.Zero(funcType.Out(j))
			}
			return out
		})
		reflect.NewAt(field.Type(), unsafe.Pointer(field.UnsafeAddr())).Elem().Set(fn)
	}

	merged := cloneAppDepsForTest()
	core.MergeNonZeroAppDepFields(merged, override)
	mergedValue := reflect.ValueOf(merged).Elem()
	for i := 0; i < overrideValue.NumField(); i++ {
		overrideField := overrideValue.Field(i)
		if overrideField.Kind() != reflect.Func || overrideField.IsNil() {
			continue
		}
		mergedField := mergedValue.Field(i)
		if mergedField.IsNil() {
			t.Fatalf("merged field %q unexpectedly nil", overrideValue.Type().Field(i).Name)
		}
		if mergedField.Pointer() != overrideField.Pointer() {
			t.Fatalf("merged field %q did not preserve override function", overrideValue.Type().Field(i).Name)
		}
	}
}
