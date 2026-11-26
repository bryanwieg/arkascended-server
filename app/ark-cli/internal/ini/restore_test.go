package ini

// Tests: restore-ini command: script call and reply, running refusal, revision check and stop-apply-start with --restart

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

func TestRunRestoreIni(t *testing.T) {
	setEnv(t, "COMPOSE_FILE", "compose.yaml")
	base := []string{"--instance", "asa1", "--file", "gus", "--revision", "3f2a91c"}
	// run calls RunRestoreIni on an instance whose history (read by exec while running) lists
	// revision 3f2a91c and whose restore answers with reply; it returns the restore command, the
	// check/stop/apply/start steps, stdout and the error.
	run := func(t *testing.T, running bool, reply ark.ExecResult, args ...string) ([]string, []string, string, error) {
		var command, steps []string
		var err error
		out := captureStdout(t, func() {
			withPatchedAppDeps(t, func(d *core.AppDeps) {
				d.ComposeService = &arktest.MockCompose{
					LoadProjectFn:     editIniMockCompose(running).LoadProjectFn,
					RunningServicesFn: editIniMockCompose(running).RunningServicesFn,
					ExecFn: func(_ context.Context, _, _ string, cmd []string, _ ark.ExecOptions) (ark.ExecResult, error) {
						steps = append(steps, strings.Join(cmd[2:], " "))
						return ark.ExecResult{Stdout: `{"revisions":[{"revision":"3f2a91c"}],"more":false}`}, nil
					},
					RunOneOffFn: func(_ context.Context, _, _ string, cmd []string, _ ark.RunOptions) (ark.ExecResult, error) {
						command, steps = cmd, append(steps, "apply")
						return reply, nil
					},
				}
				d.RestartStopInstance = func(runtime.StopCommandRequest) error { steps = append(steps, "stop"); return nil }
				d.RestartStartInstance = func(runtime.StartCommandRequest) error { steps = append(steps, "start"); return nil }
			}, func() { err = RunRestoreIni(core.CurrentAppDeps(), args) })
		})
		return command, steps, out, err
	}
	restored := ark.ExecResult{Stdout: `{"file":"GameUserSettings.ini","revision":"3f2a91c","changed":true}`}
	restartSteps := []string{"history --limit 0", "stop", "apply", "start"}

	t.Run("stopped: runs the script and prints the restore", func(t *testing.T) {
		command, steps, out, err := run(t, false, restored, base...)
		want := []string{iniScriptPath, "restore", "--file", "gameusersettings", "--revision", "3f2a91c"}
		if err != nil || !reflect.DeepEqual(command, want) || !reflect.DeepEqual(steps, []string{"apply"}) {
			t.Fatalf("err = %v, command = %#v, steps = %v", err, command, steps)
		}
		if out != "GameUserSettings.ini restored from 3f2a91c." {
			t.Fatalf("output = %q", out)
		}
	})

	t.Run("already matching: no change", func(t *testing.T) {
		reply := ark.ExecResult{Stdout: `{"file":"GameUserSettings.ini","revision":"3f2a91c","changed":false}`}
		_, _, out, err := run(t, false, reply, base...)
		if err != nil || out != "GameUserSettings.ini already matches 3f2a91c; no change." {
			t.Fatalf("err = %v, output = %q", err, out)
		}
	})

	t.Run("running without --restart: refused, script never runs", func(t *testing.T) {
		_, steps, _, err := run(t, true, restored, base...)
		if err == nil || !strings.Contains(err.Error(), "--restart") || len(steps) != 0 {
			t.Fatalf("err = %v, steps = %v", err, steps)
		}
	})

	t.Run("running with --restart: check, stop, apply, start, even on refusal", func(t *testing.T) {
		for _, reply := range []ark.ExecResult{restored, {ExitCode: 1, Stderr: "Error: refused\n"}} {
			_, steps, _, err := run(t, true, reply, append(base, "--restart")...)
			if !reflect.DeepEqual(steps, restartSteps) {
				t.Fatalf("steps = %v", steps)
			}
			if (reply.ExitCode != 0) != (err != nil) || (err != nil && err.Error() != "Error: refused") {
				t.Fatalf("exit %d: err = %v", reply.ExitCode, err)
			}
		}
	})

	t.Run("--restart: a longer or shorter hash of a listed revision matches", func(t *testing.T) {
		for _, revision := range []string{"3F2A", "3f2a91c0ffee"} {
			_, steps, _, err := run(t, true, restored, "--instance", "asa1", "--file", "gus", "--restart", "--revision", revision)
			if err != nil || !reflect.DeepEqual(steps, restartSteps) {
				t.Fatalf("%s: err = %v, steps = %v", revision, err, steps)
			}
		}
	})

	t.Run("--restart with an unknown revision: fails without stopping", func(t *testing.T) {
		_, steps, _, err := run(t, true, restored, "--instance", "asa1", "--file", "gus", "--restart", "--revision", "deadbeef")
		want := "Error: revision deadbeef is not in the INI history of asa1."
		if err == nil || err.Error() != want || !reflect.DeepEqual(steps, restartSteps[:1]) {
			t.Fatalf("err = %v, steps = %v", err, steps)
		}
	})

	t.Run("flag errors, before the script runs", func(t *testing.T) {
		const badRevision = "Error: --revision must be a revision from list ini-history."
		for _, tc := range []struct {
			args []string
			want string
		}{
			{[]string{"--file", "gus", "--revision", "3f2a91c"}, "Error: --instance is required."},
			{[]string{"--instance", "asa1", "--revision", "3f2a91c"}, "Error: --file is required (use 'game' or 'gameusersettings')."},
			{[]string{"--instance", "asa1", "--file", "gus"}, badRevision},
			{[]string{"--instance", "asa1", "--file", "gus", "--revision", "abc"}, badRevision},
			{[]string{"--instance", "asa1", "--file", "gus", "--revision", "HEAD~1"}, badRevision},
		} {
			_, steps, _, err := run(t, true, restored, append(tc.args, "--restart")...)
			if err == nil || err.Error() != tc.want || len(steps) != 0 {
				t.Fatalf("%v: err = %v, steps = %v", tc.args, err, steps)
			}
		}
	})

	t.Run("bad reply", func(t *testing.T) {
		_, _, _, err := run(t, false, ark.ExecResult{Stdout: "not json"}, base...)
		if err == nil || !strings.HasPrefix(err.Error(), "Error: unexpected reply from ark-ini.py:") {
			t.Fatalf("err = %v", err)
		}
	})
}
