package ini

// Tests: list-ini-history command: script args per flag, table/patch/JSON output, note lines, flag errors

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"ark-cli/internal/core"

	"pkg/ark"
	"pkg/ark/arktest"
)

func TestRunListIniHistory(t *testing.T) {
	setEnv(t, "COMPOSE_FILE", "compose.yaml")
	const revisions = `{"revision":"3f2a91c","date":"2026-11-26 16:00:00 UTC","by":"user","how":"set with ark-cli","author":"ark-cli set ini","files":["GameUserSettings.ini","Game.ini"],"message":"[S] K=1","patch":"+K=1\n"},` +
		`{"revision":"1b2c3d4","date":"2026-11-25 10:00:00 UTC","by":"ARK","how":"written by ARK","author":"ARK server","files":["GameUserSettings.ini"],"message":"server stop","patch":"-K=0\n"}`
	history := func(more string) ark.ExecResult {
		return ark.ExecResult{Stdout: `{"revisions":[` + revisions + `],"more":` + more + "}\n"}
	}
	// run calls RunListIniHistory on an instance whose script answers with reply; it returns the
	// script command, whether it ran by exec (running) and stdout.
	run := func(t *testing.T, running bool, reply ark.ExecResult, args ...string) ([]string, bool, string, error) {
		var command []string
		var execed bool
		var err error
		out := captureStdout(t, func() {
			withPatchedAppDeps(t, func(d *core.AppDeps) {
				d.ComposeService = &arktest.MockCompose{
					LoadProjectFn:     editIniMockCompose(running).LoadProjectFn,
					RunningServicesFn: editIniMockCompose(running).RunningServicesFn,
					RunOneOffFn: func(_ context.Context, _, _ string, cmd []string, _ ark.RunOptions) (ark.ExecResult, error) {
						command = cmd[1:]
						return reply, nil
					},
					ExecFn: func(_ context.Context, _, _ string, cmd []string, _ ark.ExecOptions) (ark.ExecResult, error) {
						command, execed = cmd[2:], true
						return reply, nil
					},
				}
			}, func() {
				err = RunListIniHistory(core.CurrentAppDeps(), append([]string{"--instance", "asa1"}, args...))
			})
		})
		return command, execed, out, err
	}
	const note = "Showing all changes, including ARK's own writes; add --user-only to show user edits only."

	t.Run("script args per flag", func(t *testing.T) {
		for _, tc := range []struct {
			args []string
			want []string
		}{
			{nil, []string{"history", "--limit", "20"}},
			{[]string{"--file", "gus"}, []string{"history", "--file", "gameusersettings", "--limit", "20"}},
			{[]string{"--user-only", "--limit", "0", "--since", "2026-11-26", "--patch"},
				[]string{"history", "--user-only", "--limit", "0", "--since", "2026-11-26", "--patch"}},
		} {
			for _, running := range []bool{false, true} {
				command, execed, _, err := run(t, running, history("false"), tc.args...)
				if err != nil || !reflect.DeepEqual(command, tc.want) || execed != running {
					t.Fatalf("%v running=%v: err = %v, command = %#v, execed = %v", tc.args, running, err, command, execed)
				}
			}
		}
	})

	t.Run("table, newest first, then the note", func(t *testing.T) {
		_, _, out, err := run(t, false, history("false"))
		lines := strings.Split(strings.TrimSpace(out), "\n")
		if err != nil || len(lines) != 4 || lines[3] != note {
			t.Fatalf("err = %v, output:\n%s", err, out)
		}
		for i, want := range [][]string{
			{"REVISION", "DATE", "BY", "HOW", "FILES", "MESSAGE"},
			{"3f2a91c", "2026-11-26", "16:00:00", "UTC", "user", "set", "with", "ark-cli", "GameUserSettings.ini,Game.ini", "[S]", "K=1"},
			{"1b2c3d4", "2026-11-25", "10:00:00", "UTC", "ARK", "written", "by", "ARK", "GameUserSettings.ini", "server", "stop"},
		} {
			if got := strings.Fields(lines[i]); !reflect.DeepEqual(got, want) {
				t.Fatalf("line %d = %#v", i, got)
			}
		}
	})

	t.Run("--user-only: no note; more: older-revisions line", func(t *testing.T) {
		_, _, out, err := run(t, false, history("true"), "--user-only")
		last := strings.Split(strings.TrimSpace(out), "\n")
		if err != nil || strings.Contains(out, note) || !strings.Contains(last[len(last)-1], "--limit 0") || !strings.Contains(last[len(last)-1], "--since") {
			t.Fatalf("err = %v, output:\n%s", err, out)
		}
	})

	t.Run("empty history", func(t *testing.T) {
		_, _, out, err := run(t, false, ark.ExecResult{Stdout: `{"revisions":[],"more":false}`})
		if err != nil || out != "No INI history recorded for asa1." {
			t.Fatalf("err = %v, output = %q", err, out)
		}
	})

	t.Run("--patch: header and diff per revision, then the notes", func(t *testing.T) {
		_, _, out, err := run(t, false, history("true"), "--patch")
		want := "3f2a91c  2026-11-26 16:00:00 UTC  user  set with ark-cli  [S] K=1\n+K=1\n" +
			"1b2c3d4  2026-11-25 10:00:00 UTC  ARK  written by ARK  server stop\n-K=0\n" + note + "\nOlder revisions exist"
		if err != nil || !strings.HasPrefix(out, want) || strings.Contains(out, "REVISION") {
			t.Fatalf("err = %v, output:\n%s", err, out)
		}
	})

	t.Run("--json: the reply, no note", func(t *testing.T) {
		_, _, out, err := run(t, false, history("true"), "--json")
		for _, s := range []string{`"revision": "3f2a91c"`, `"author": "ARK server"`, `"more": true`, `"GameUserSettings.ini"`} {
			if err != nil || !strings.Contains(out, s) || strings.Contains(out, "Showing all") {
				t.Fatalf("err = %v, missing %q in:\n%s", err, s, out)
			}
		}
	})

	t.Run("flag errors, before the script runs", func(t *testing.T) {
		for _, tc := range []struct {
			args []string
			want string
		}{
			{[]string{"--since", "yesterday"}, "Error: --since must be a date like 2026-11-26."},
			{[]string{"--limit", "-1"}, "Error: --limit must be 0 or more."},
			{[]string{"--file", "nope"}, "Error: invalid --file value: nope (use 'game' or 'gameusersettings')"},
		} {
			command, _, _, err := run(t, false, history("false"), tc.args...)
			if err == nil || err.Error() != tc.want || command != nil {
				t.Fatalf("%v: err = %v, command = %v", tc.args, err, command)
			}
		}
		if err := RunListIniHistory(core.CurrentAppDeps(), nil); err == nil || err.Error() != "Error: --instance is required." {
			t.Fatalf("no instance: err = %v", err)
		}
	})

	t.Run("bad reply", func(t *testing.T) {
		_, _, _, err := run(t, false, ark.ExecResult{Stdout: "not json"})
		if err == nil || !strings.HasPrefix(err.Error(), "Error: unexpected reply from ark-ini.py:") {
			t.Fatalf("err = %v", err)
		}
	})

	t.Run("old image: rebuild hint", func(t *testing.T) {
		reply := ark.ExecResult{ExitCode: 2, Stderr: "usage: ark-ini.py [-h] {get,set,track} ...\nark-ini.py: error: argument command: invalid choice: 'history' (choose from get, set, track)\n"}
		_, _, _, err := run(t, false, reply)
		want := "Error: asa1 runs an image built before this ark-cli command existed; rebuild it with: ark-cli update instance --instance asa1"
		if err == nil || err.Error() != want {
			t.Fatalf("err = %v", err)
		}
	})
}
