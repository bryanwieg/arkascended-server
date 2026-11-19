package ini

// Tests: set-ini command: script call and reply, running refusal, stop-apply-start with --restart

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"ark-cli/internal/core"

	"pkg/ark"
	"pkg/ark/arktest"
	"pkg/ark/runtime"
)

func TestRunSetIniStoppedRunsScriptAndReportsChange(t *testing.T) {
	setEnv(t, "COMPOSE_FILE", "compose.yaml")
	var gotCommand []string
	var gotOpts ark.RunOptions
	out := captureStdout(t, func() {
		withPatchedAppDeps(t, func(d *core.AppDeps) {
			d.ComposeService = &arktest.MockCompose{
				LoadProjectFn:     editIniMockCompose(false).LoadProjectFn,
				RunningServicesFn: editIniMockCompose(false).RunningServicesFn,
				RunOneOffFn: func(_ context.Context, _, _ string, command []string, opts ark.RunOptions) (ark.ExecResult, error) {
					gotCommand, gotOpts = command, opts
					return ark.ExecResult{Stdout: `{"file":"GameUserSettings.ini","section":"ServerSettings","key":"MaxPlayers","value":"20","old":["70","80"],"changed":true,"removed":1,"backup":"/x/GameUserSettings.ini.bak"}`}, nil
				},
			}
		}, func() {
			err := RunSetIni(core.CurrentAppDeps(), []string{"--instance", "asa1", "--file", "gus",
				"--section", "ServerSettings", "--key", "MaxPlayers", "--value", "20", "--replace-all"})
			if err != nil {
				t.Fatalf("RunSetIni: %v", err)
			}
		})
	})
	want := []string{iniScriptPath, "set", "--file", "gameusersettings", "--section", "ServerSettings",
		"--key", "MaxPlayers", "--value", "20", "--replace-all"}
	if !reflect.DeepEqual(gotCommand, want) || gotOpts.Entrypoint != "/usr/bin/python3.14" || !gotOpts.Remove {
		t.Fatalf("command = %#v, opts = %+v", gotCommand, gotOpts)
	}
	for _, s := range []string{
		`GameUserSettings.ini [ServerSettings] MaxPlayers: "70", "80" -> "20"`,
		"Removed 1 repeated MaxPlayers line(s); the previous file is saved as /x/GameUserSettings.ini.bak.",
	} {
		if !strings.Contains(out, s) {
			t.Fatalf("missing %q in:\n%s", s, out)
		}
	}
}

func TestRunSetIniPassesScriptRefusalThrough(t *testing.T) {
	setEnv(t, "COMPOSE_FILE", "compose.yaml")
	var err error
	withPatchedAppDeps(t, func(d *core.AppDeps) {
		d.ComposeService = &arktest.MockCompose{
			LoadProjectFn:     editIniMockCompose(false).LoadProjectFn,
			RunningServicesFn: editIniMockCompose(false).RunningServicesFn,
			RunOneOffFn: func(context.Context, string, string, []string, ark.RunOptions) (ark.ExecResult, error) {
				return ark.ExecResult{ExitCode: 1, Stderr: "Error: K appears 2 times in [S] of Game.ini:\n  line 2: K=1\n"}, nil
			},
		}
	}, func() {
		err = RunSetIni(core.CurrentAppDeps(), []string{"--instance", "asa1", "--file", "game", "--section", "S", "--key", "K", "--value", "v"})
	})
	if err == nil || !strings.HasPrefix(err.Error(), "Error: K appears 2 times") || !strings.Contains(err.Error(), "line 2: K=1") {
		t.Fatalf("err = %v", err)
	}
}

func TestRunSetIniRunningWithoutRestartRefusesWithoutTouchingFile(t *testing.T) {
	setEnv(t, "COMPOSE_FILE", "compose.yaml")
	var err error
	withPatchedAppDeps(t, func(d *core.AppDeps) {
		d.ComposeService = editIniMockCompose(true) // no Exec/RunOneOff: any script call would fail the test
	}, func() {
		err = RunSetIni(core.CurrentAppDeps(), []string{"--instance", "asa1", "--file", "game", "--section", "S", "--key", "K", "--value", "v"})
	})
	if err == nil || !strings.Contains(err.Error(), "--restart") {
		t.Fatalf("err = %v", err)
	}
}

// --restart: stop with the default grace, apply in a one-off container, start again; the start
// happens even when the script refuses, so a refused change never leaves the server down.
func TestRunSetIniRestartStopsAppliesAndStartsEvenOnRefusal(t *testing.T) {
	for _, refuse := range []bool{false, true} {
		setEnv(t, "COMPOSE_FILE", "compose.yaml")
		var steps []string
		var err error
		withPatchedAppDeps(t, func(d *core.AppDeps) {
			d.ComposeService = &arktest.MockCompose{
				LoadProjectFn:     editIniMockCompose(true).LoadProjectFn,
				RunningServicesFn: editIniMockCompose(true).RunningServicesFn,
				RunOneOffFn: func(context.Context, string, string, []string, ark.RunOptions) (ark.ExecResult, error) {
					steps = append(steps, "apply")
					if refuse {
						return ark.ExecResult{ExitCode: 1, Stderr: "Error: refused"}, nil
					}
					return ark.ExecResult{Stdout: `{"file":"Game.ini","section":"S","key":"K","value":"v","old":[],"changed":true,"removed":0,"backup":null}`}, nil
				},
			}
			d.RestartStopInstance = func(req runtime.StopCommandRequest) error {
				if req.Timeout != -1 {
					t.Fatalf("stop timeout = %d, want -1 (default grace)", req.Timeout)
				}
				steps = append(steps, "stop")
				return nil
			}
			d.RestartStartInstance = func(runtime.StartCommandRequest) error {
				steps = append(steps, "start")
				return nil
			}
		}, func() {
			err = RunSetIni(core.CurrentAppDeps(), []string{"--instance", "asa1", "--file", "game",
				"--section", "S", "--key", "K", "--value", "v", "--restart"})
		})
		if !reflect.DeepEqual(steps, []string{"stop", "apply", "start"}) {
			t.Fatalf("refuse=%v: steps = %v", refuse, steps)
		}
		if refuse != (err != nil) {
			t.Fatalf("refuse=%v: err = %v", refuse, err)
		}
	}
}

func TestRunSetIniRequiresValueFlag(t *testing.T) {
	err := RunSetIni(core.CurrentAppDeps(), []string{"--instance", "asa1", "--file", "game", "--section", "S", "--key", "K"})
	if err == nil || !strings.Contains(err.Error(), "--value is required") {
		t.Fatalf("err = %v", err)
	}
}

// A failed stop must not touch the file or start anything: the instance may still be running.
func TestRunSetIniRestartStopFailureSkipsApplyAndStart(t *testing.T) {
	setEnv(t, "COMPOSE_FILE", "compose.yaml")
	var steps []string
	var err error
	withPatchedAppDeps(t, func(d *core.AppDeps) {
		d.ComposeService = &arktest.MockCompose{
			LoadProjectFn:     editIniMockCompose(true).LoadProjectFn,
			RunningServicesFn: editIniMockCompose(true).RunningServicesFn,
			RunOneOffFn: func(context.Context, string, string, []string, ark.RunOptions) (ark.ExecResult, error) {
				steps = append(steps, "apply")
				return ark.ExecResult{}, nil
			},
		}
		d.RestartStopInstance = func(runtime.StopCommandRequest) error { return errors.New("Error: stop failed") }
		d.RestartStartInstance = func(runtime.StartCommandRequest) error { steps = append(steps, "start"); return nil }
	}, func() {
		err = RunSetIni(core.CurrentAppDeps(), []string{"--instance", "asa1", "--file", "game",
			"--section", "S", "--key", "K", "--value", "v", "--restart"})
	})
	if err == nil || err.Error() != "Error: stop failed" || len(steps) != 0 {
		t.Fatalf("err = %v, steps = %v", err, steps)
	}
}

// When both the change and the start fail, both errors reach the user.
func TestRunSetIniRestartReportsApplyAndStartFailures(t *testing.T) {
	setEnv(t, "COMPOSE_FILE", "compose.yaml")
	var err error
	withPatchedAppDeps(t, func(d *core.AppDeps) {
		d.ComposeService = &arktest.MockCompose{
			LoadProjectFn:     editIniMockCompose(true).LoadProjectFn,
			RunningServicesFn: editIniMockCompose(true).RunningServicesFn,
			RunOneOffFn: func(context.Context, string, string, []string, ark.RunOptions) (ark.ExecResult, error) {
				return ark.ExecResult{ExitCode: 1, Stderr: "Error: refused"}, nil
			},
		}
		d.RestartStopInstance = func(runtime.StopCommandRequest) error { return nil }
		d.RestartStartInstance = func(runtime.StartCommandRequest) error { return errors.New("Error: start failed") }
	}, func() {
		err = RunSetIni(core.CurrentAppDeps(), []string{"--instance", "asa1", "--file", "game",
			"--section", "S", "--key", "K", "--value", "v", "--restart"})
	})
	if err == nil || err.Error() != "Error: refused\nError: start failed" {
		t.Fatalf("err = %v", err)
	}
}

func TestRunSetIniReportsNoChange(t *testing.T) {
	setEnv(t, "COMPOSE_FILE", "compose.yaml")
	out := captureStdout(t, func() {
		withPatchedAppDeps(t, func(d *core.AppDeps) {
			d.ComposeService = &arktest.MockCompose{
				LoadProjectFn:     editIniMockCompose(false).LoadProjectFn,
				RunningServicesFn: editIniMockCompose(false).RunningServicesFn,
				RunOneOffFn: func(context.Context, string, string, []string, ark.RunOptions) (ark.ExecResult, error) {
					return ark.ExecResult{Stdout: `{"file":"Game.ini","section":"S","key":"K","value":"v","old":["v"],"changed":false,"removed":0,"backup":null}`}, nil
				},
			}
		}, func() {
			if err := RunSetIni(core.CurrentAppDeps(), []string{"--instance", "asa1", "--file", "game", "--section", "S", "--key", "K", "--value", "v"}); err != nil {
				t.Fatalf("RunSetIni: %v", err)
			}
		})
	})
	if out != `Game.ini [S] K is already "v"; no change.` {
		t.Fatalf("output = %q", out)
	}
}

func TestRunSetIniRejectsMissingFlagsAndBadReplies(t *testing.T) {
	for _, tc := range []struct {
		args []string
		want string
	}{
		{[]string{"--file", "game", "--section", "S", "--key", "K", "--value", "v"}, "Error: --instance is required."},
		{[]string{"--instance", "asa1", "--file", "game", "--key", "K", "--value", "v"}, "Error: --section and --key are required."},
	} {
		if err := RunSetIni(core.CurrentAppDeps(), tc.args); err == nil || err.Error() != tc.want {
			t.Fatalf("%v: err = %v", tc.args, err)
		}
	}
	setEnv(t, "COMPOSE_FILE", "compose.yaml")
	var err error
	withPatchedAppDeps(t, func(d *core.AppDeps) {
		d.ComposeService = &arktest.MockCompose{
			LoadProjectFn:     editIniMockCompose(false).LoadProjectFn,
			RunningServicesFn: editIniMockCompose(false).RunningServicesFn,
			RunOneOffFn: func(context.Context, string, string, []string, ark.RunOptions) (ark.ExecResult, error) {
				return ark.ExecResult{Stdout: "not json"}, nil
			},
		}
	}, func() {
		err = RunSetIni(core.CurrentAppDeps(), []string{"--instance", "asa1", "--file", "game", "--section", "S", "--key", "K", "--value", "v"})
	})
	if err == nil || !strings.HasPrefix(err.Error(), "Error: unexpected reply from ark-ini.py:") {
		t.Fatalf("bad reply: err = %v", err)
	}
}
