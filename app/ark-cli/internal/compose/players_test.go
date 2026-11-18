package compose

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"ark-cli/internal/core"

	"pkg/ark"
	"pkg/ark/arktest"
)

func TestParseListPlayers(t *testing.T) {
	got := parseListPlayers("0. Alice, 0002aaaa\n1. Bob, the Builder, 0002bbbb\n\n")
	if len(got) != 2 || got[0] != (core.PlayerJSON{Name: "Alice", ID: "0002aaaa"}) ||
		got[1] != (core.PlayerJSON{Name: "Bob, the Builder", ID: "0002bbbb"}) {
		t.Fatalf("got %+v", got)
	}
	if got := parseListPlayers("No Players Connected\n"); len(got) != 0 {
		t.Fatalf("empty server: got %+v", got)
	}
}

func TestRunListPlayersAll(t *testing.T) {
	var cmds []string
	var out string
	var err error
	withPatchedAppDeps(t, func(d *core.AppDeps) {
		d.ComposeService = &arktest.MockCompose{
			LoadProjectFn: func(context.Context, string) (*ark.ComposeProject, error) {
				return fakeProject(emptyServices("a", "b", "c")), nil
			},
			RunningServicesFn: func(context.Context, string) (map[string]struct{}, error) {
				return map[string]struct{}{"a": {}, "b": {}}, nil
			},
			ExecFn: func(_ context.Context, _, service string, command []string, _ ark.ExecOptions) (ark.ExecResult, error) {
				cmds = append(cmds, service+" "+strings.Join(command[1:], " "))
				if service == "a" {
					return ark.ExecResult{Stdout: "0. Alice, 0002aaaa\n"}, nil
				}
				return ark.ExecResult{Stdout: "No Players Connected\n"}, nil
			},
		}
	}, func() {
		out = captureStdout(t, func() {
			err = RunListPlayers(core.CurrentAppDeps(), []string{"--instance", "all", "--json"})
		})
	})
	if err != nil {
		t.Fatalf("RunListPlayers: %v", err) // a stopped instance is not a failure for "all"
	}
	if len(cmds) != 2 || !strings.HasSuffix(cmds[0], "ark-rcon.py ListPlayers") {
		t.Fatalf("exec calls = %v (stopped c must be skipped)", cmds)
	}
	var got []core.InstancePlayersJSON
	if jerr := json.Unmarshal([]byte(out), &got); jerr != nil {
		t.Fatalf("json: %v\n%s", jerr, out)
	}
	if len(got) != 3 || len(got[0].Players) != 1 || got[0].Players[0].Name != "Alice" ||
		len(got[1].Players) != 0 || got[2].Error != "not running" {
		t.Fatalf("got %+v", got)
	}
}

func TestRunListPlayersStoppedSingleInstanceFails(t *testing.T) {
	withPatchedAppDeps(t, func(d *core.AppDeps) {
		d.ComposeService = &arktest.MockCompose{
			LoadProjectFn: func(context.Context, string) (*ark.ComposeProject, error) {
				return fakeProject(emptyServices("a")), nil
			},
		}
	}, func() {
		captureStdout(t, func() {
			if err := RunListPlayers(core.CurrentAppDeps(), []string{"--instance", "a"}); err == nil {
				t.Fatal("expected an error for a stopped instance")
			}
		})
	})
}
