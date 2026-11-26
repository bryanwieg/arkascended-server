package ini

// Tests: unset-ini command: script call and reply, running refusal, stop-apply-start with --restart

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"ark-cli/internal/core"

	"pkg/ark"
	"pkg/ark/arktest"
	"pkg/ark/runtime"
)

func TestRunUnsetIni(t *testing.T) {
	setEnv(t, "COMPOSE_FILE", "compose.yaml")
	base := []string{"--instance", "asa1", "--file", "gus", "--section", "ServerSettings", "--key", "MaxPlayers"}
	// run calls RunUnsetIni on an instance whose script answers with reply; it returns the script
	// command, the stop/apply/start steps, stdout and the error.
	run := func(t *testing.T, running bool, reply ark.ExecResult, args ...string) ([]string, []string, string, error) {
		var command, steps []string
		var err error
		out := captureStdout(t, func() {
			withPatchedAppDeps(t, func(d *core.AppDeps) {
				d.ComposeService = &arktest.MockCompose{
					LoadProjectFn:     editIniMockCompose(running).LoadProjectFn,
					RunningServicesFn: editIniMockCompose(running).RunningServicesFn,
					RunOneOffFn: func(_ context.Context, _, _ string, cmd []string, _ ark.RunOptions) (ark.ExecResult, error) {
						command, steps = cmd, append(steps, "apply")
						return reply, nil
					},
				}
				d.RestartStopInstance = func(runtime.StopCommandRequest) error { steps = append(steps, "stop"); return nil }
				d.RestartStartInstance = func(runtime.StartCommandRequest) error { steps = append(steps, "start"); return nil }
			}, func() { err = RunUnsetIni(core.CurrentAppDeps(), args) })
		})
		return command, steps, out, err
	}
	removed := ark.ExecResult{Stdout: `{"file":"GameUserSettings.ini","section":"ServerSettings","key":"MaxPlayers","old":["70"],"changed":true,"backup":"/x/GameUserSettings.ini.bak"}`}

	t.Run("stopped: runs the script and prints the removal", func(t *testing.T) {
		command, steps, out, err := run(t, false, removed, base...)
		want := []string{iniScriptPath, "unset", "--file", "gameusersettings", "--section", "ServerSettings", "--key", "MaxPlayers"}
		if err != nil || !reflect.DeepEqual(command, want) || !reflect.DeepEqual(steps, []string{"apply"}) {
			t.Fatalf("err = %v, command = %#v, steps = %v", err, command, steps)
		}
		for _, s := range []string{
			`GameUserSettings.ini [ServerSettings] MaxPlayers: removed "70"`,
			"The previous file is saved as /x/GameUserSettings.ini.bak.",
		} {
			if !strings.Contains(out, s) {
				t.Fatalf("missing %q in:\n%s", s, out)
			}
		}
	})

	t.Run("not set: no change", func(t *testing.T) {
		reply := ark.ExecResult{Stdout: `{"file":"GameUserSettings.ini","section":"ServerSettings","key":"MaxPlayers","old":[],"changed":false,"backup":null}`}
		_, _, out, err := run(t, false, reply, base...)
		if err != nil || out != "GameUserSettings.ini [ServerSettings] MaxPlayers is not set; no change." {
			t.Fatalf("err = %v, output = %q", err, out)
		}
	})

	t.Run("--all: passed on, every value printed", func(t *testing.T) {
		reply := ark.ExecResult{Stdout: `{"file":"Game.ini","section":"S","key":"K","old":["a","b"],"changed":true,"backup":"/x/Game.ini.bak"}`}
		command, _, out, err := run(t, false, reply, append(base, "--all")...)
		if err != nil || command[len(command)-1] != "--all" || !strings.Contains(out, `Game.ini [S] K: removed "a", "b"`) {
			t.Fatalf("err = %v, command = %#v, output = %q", err, command, out)
		}
	})

	t.Run("running without --restart: refused, script never runs", func(t *testing.T) {
		_, steps, _, err := run(t, true, removed, base...)
		if err == nil || !strings.Contains(err.Error(), "--restart") || len(steps) != 0 {
			t.Fatalf("err = %v, steps = %v", err, steps)
		}
	})

	t.Run("running with --restart: stop, apply, start, even on refusal", func(t *testing.T) {
		for _, reply := range []ark.ExecResult{removed, {ExitCode: 1, Stderr: "Error: refused\n  line 2: K=1\n"}} {
			_, steps, _, err := run(t, true, reply, append(base, "--restart")...)
			if !reflect.DeepEqual(steps, []string{"stop", "apply", "start"}) {
				t.Fatalf("steps = %v", steps)
			}
			if (reply.ExitCode != 0) != (err != nil) || (err != nil && err.Error() != "Error: refused\n  line 2: K=1") {
				t.Fatalf("exit %d: err = %v", reply.ExitCode, err)
			}
		}
	})

	t.Run("flag errors", func(t *testing.T) {
		for _, tc := range []struct {
			args []string
			want string
		}{
			{[]string{"--file", "game", "--section", "S", "--key", "K"}, "Error: --instance is required."},
			{[]string{"--instance", "asa1", "--file", "game", "--key", "K"}, "Error: --section and --key are required."},
			{[]string{"--instance", "asa1", "--file", "game", "--section", "S"}, "Error: --section and --key are required."},
		} {
			if err := RunUnsetIni(core.CurrentAppDeps(), tc.args); err == nil || err.Error() != tc.want {
				t.Fatalf("%v: err = %v", tc.args, err)
			}
		}
		positional := append(append([]string{}, base...), "extra")
		unsetErr := RunUnsetIni(core.CurrentAppDeps(), positional)
		setErr := RunSetIni(core.CurrentAppDeps(), append(positional[:len(positional)-1:len(positional)-1], "--value", "v", "extra"))
		if unsetErr == nil || setErr == nil || strings.ReplaceAll(setErr.Error(), "set ini", "unset ini") != unsetErr.Error() {
			t.Fatalf("positional: unset err = %v, set err = %v", unsetErr, setErr)
		}
	})

	t.Run("bad reply", func(t *testing.T) {
		_, _, _, err := run(t, false, ark.ExecResult{Stdout: "not json"}, base...)
		if err == nil || !strings.HasPrefix(err.Error(), "Error: unexpected reply from ark-ini.py:") {
			t.Fatalf("err = %v", err)
		}
	})

	t.Run("old image: rebuild hint", func(t *testing.T) {
		reply := ark.ExecResult{ExitCode: 2, Stderr: "usage: ark-ini.py [-h] {get,set,track} ...\nark-ini.py: error: argument command: invalid choice: 'unset' (choose from get, set, track)\n"}
		_, _, _, err := run(t, false, reply, base...)
		if err == nil || !strings.Contains(err.Error(), "ark-cli update instance --instance asa1") {
			t.Fatalf("err = %v", err)
		}
	})
}
