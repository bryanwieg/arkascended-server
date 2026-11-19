package ini

// Tests: get-ini command: script call (running and stopped), one value, repeated lines, unset key, --json

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"ark-cli/internal/core"

	"pkg/ark"
	"pkg/ark/arktest"
)

const getReplyOne = `{"file":"GameUserSettings.ini","section":"ServerSettings","key":"MaxPlayers","lines":[{"line":3,"value":"20","origin":"written by ARK 2026-11-19 00:01:26 UTC"}],"history":"Last change to GameUserSettings.ini: written by ARK 2026-11-19 00:01:26 UTC (m)."}`

func runGetIni(t *testing.T, running bool, reply string, args ...string) (string, []string, error) {
	t.Helper()
	setEnv(t, "COMPOSE_FILE", "compose.yaml")
	var gotCommand []string
	var err error
	script := func(command []string) ark.ExecResult {
		gotCommand = command
		return ark.ExecResult{Stdout: reply}
	}
	out := captureStdout(t, func() {
		withPatchedAppDeps(t, func(d *core.AppDeps) {
			d.ComposeService = &arktest.MockCompose{
				LoadProjectFn:     editIniMockCompose(running).LoadProjectFn,
				RunningServicesFn: editIniMockCompose(running).RunningServicesFn,
				ExecFn: func(_ context.Context, _, _ string, command []string, _ ark.ExecOptions) (ark.ExecResult, error) {
					return script(command), nil
				},
				RunOneOffFn: func(_ context.Context, _, _ string, command []string, _ ark.RunOptions) (ark.ExecResult, error) {
					return script(command), nil
				},
			}
		}, func() {
			err = RunGetIni(core.CurrentAppDeps(), append([]string{"--instance", "asa1", "--file", "gus",
				"--section", "ServerSettings", "--key", "MaxPlayers"}, args...))
		})
	})
	return out, gotCommand, err
}

func TestRunGetIniPrintsTableFromRunningOrStoppedInstance(t *testing.T) {
	scriptArgs := []string{"get", "--file", "gameusersettings", "--section", "ServerSettings", "--key", "MaxPlayers"}
	for _, running := range []bool{true, false} {
		out, command, err := runGetIni(t, running, getReplyOne)
		if err != nil || !hasRow(out, "3", "MaxPlayers", "20", "written by ARK 2026-11-19 00:01:26 UTC") {
			t.Fatalf("running=%v: out = %q, err = %v", running, out, err)
		}
		want := append([]string{iniScriptPath}, scriptArgs...)
		if running {
			want = append([]string{"/usr/bin/python3.14"}, want...)
		}
		if !reflect.DeepEqual(command, want) {
			t.Fatalf("running=%v: command = %#v", running, command)
		}
	}
}

func TestRunGetIniTableListsRepeatedLinesWithOrigins(t *testing.T) {
	reply := `{"file":"GameUserSettings.ini","section":"ServerSettings","key":"K","lines":[{"line":2,"value":"a","origin":"written by ARK 2026-11-19 00:01:26 UTC"},{"line":5,"value":"b","origin":"not written by ARK (added after 2026-11-19 00:01:26 UTC)"}],"history":"h"}`
	out, _, err := runGetIni(t, false, reply)
	if err != nil || !hasRow(out, "LINE", "KEY", "VALUE", "ORIGIN") ||
		!hasRow(out, "2", "K", "a", "written by ARK 2026-11-19 00:01:26 UTC") ||
		!hasRow(out, "5", "K", "b", "not written by ARK (added after 2026-11-19 00:01:26 UTC)") ||
		strings.Count(out, "\n") != 2 { // header and two rows, nothing after the table
		t.Fatalf("out = %q, err = %v", out, err)
	}
}

func TestRunGetIniUnsetKeyFailsButJSONReportsIt(t *testing.T) {
	reply := `{"file":"Game.ini","section":"S","key":"K","lines":[],"history":"h"}`
	if _, _, err := runGetIni(t, false, reply); err == nil || err.Error() != "Error: K is not set in [S] of Game.ini." {
		t.Fatalf("err = %v", err)
	}
	out, _, err := runGetIni(t, false, reply, "--json")
	if err != nil || !strings.Contains(out, `"lines": []`) || !strings.Contains(out, `"key": "K"`) {
		t.Fatalf("out = %q, err = %v", out, err)
	}
}

// hasRow reports whether a PrintTable line holds exactly these cells, in order.
func hasRow(out string, cells ...string) bool {
	for _, line := range strings.Split(out, "\n") {
		if reflect.DeepEqual(strings.Fields(line), strings.Fields(strings.Join(cells, " "))) {
			return true
		}
	}
	return false
}

func TestRunGetIniRejectsMissingFlagsAndBadReplies(t *testing.T) {
	for _, tc := range []struct {
		args []string
		want string
	}{
		{[]string{"--file", "gus", "--section", "S", "--key", "K"}, "Error: --instance is required."},
		{[]string{"--instance", "asa1", "--file", "gus", "--key", "K"}, "Error: --section and --key are required."},
	} {
		if err := RunGetIni(core.CurrentAppDeps(), tc.args); err == nil || err.Error() != tc.want {
			t.Fatalf("%v: err = %v", tc.args, err)
		}
	}
	if _, _, err := runGetIni(t, false, "not json"); err == nil || !strings.HasPrefix(err.Error(), "Error: unexpected reply from ark-ini.py:") {
		t.Fatalf("bad reply: err = %v", err)
	}
}
