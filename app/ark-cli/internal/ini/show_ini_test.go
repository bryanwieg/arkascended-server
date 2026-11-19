package ini

// Tests: show-ini command: stopped vs running modes, compose-file flag, script errors

import (
	"context"
	"reflect"
	"testing"

	"ark-cli/internal/core"

	"pkg/ark"
	"pkg/ark/arktest"
)

func showIniMockCompose(running bool) *arktest.MockCompose {
	return &arktest.MockCompose{
		LoadProjectFn: func(_ context.Context, _ string) (*ark.ComposeProject, error) {
			return fakeProject(emptyServices("asa1")), nil
		},
		RunningServicesFn: func(_ context.Context, _ string) (map[string]struct{}, error) {
			if running {
				return map[string]struct{}{"asa1": {}}, nil
			}
			return map[string]struct{}{}, nil
		},
	}
}

func TestRunShowIniStoppedRunsScriptInOneOffAndPrintsIt(t *testing.T) {
	setEnv(t, "COMPOSE_FILE", "compose.yaml")
	var gotService string
	var gotCommand []string
	var gotOpts ark.RunOptions
	out := captureStdout(t, func() {
		withPatchedAppDeps(t, func(d *core.AppDeps) {
			d.ComposeService = &arktest.MockCompose{
				LoadProjectFn:     showIniMockCompose(false).LoadProjectFn,
				RunningServicesFn: showIniMockCompose(false).RunningServicesFn,
				RunOneOffFn: func(_ context.Context, _, service string, command []string, opts ark.RunOptions) (ark.ExecResult, error) {
					gotService, gotCommand, gotOpts = service, command, opts
					return ark.ExecResult{Stdout: "[S]\nK=v"}, nil
				},
			}
		}, func() {
			if err := RunShowIni(core.CurrentAppDeps(), []string{"--instance", "asa1", "--file", "game"}); err != nil {
				t.Fatalf("runShowIni: %v", err)
			}
		})
	})
	if gotService != "asa1" || !gotOpts.Remove || !gotOpts.NoDeps || !gotOpts.DisableTTY || gotOpts.Entrypoint != "/usr/bin/python3.14" {
		t.Fatalf("unexpected service/options: %s %+v", gotService, gotOpts)
	}
	if want := []string{iniScriptPath, "get", "--file", "game"}; !reflect.DeepEqual(gotCommand, want) {
		t.Fatalf("command = %#v, want %#v", gotCommand, want)
	}
	if out != "[S]\nK=v" {
		t.Fatalf("output = %q", out)
	}
}

func TestRunShowIniRunningExecsScript(t *testing.T) {
	setEnv(t, "COMPOSE_FILE", "compose.yaml")
	var gotCommand []string
	withPatchedAppDeps(t, func(d *core.AppDeps) {
		d.ComposeService = &arktest.MockCompose{
			LoadProjectFn:     showIniMockCompose(true).LoadProjectFn,
			RunningServicesFn: showIniMockCompose(true).RunningServicesFn,
			ExecFn: func(_ context.Context, _, _ string, command []string, _ ark.ExecOptions) (ark.ExecResult, error) {
				gotCommand = command
				return ark.ExecResult{}, nil
			},
		}
	}, func() {
		if err := RunShowIni(core.CurrentAppDeps(), []string{"--instance", "asa1", "--file", "gus"}); err != nil {
			t.Fatalf("runShowIni: %v", err)
		}
	})
	if want := []string{"/usr/bin/python3.14", iniScriptPath, "get", "--file", "gameusersettings"}; !reflect.DeepEqual(gotCommand, want) {
		t.Fatalf("command = %#v, want %#v", gotCommand, want)
	}
}

func TestRunShowIniReturnsScriptError(t *testing.T) {
	setEnv(t, "COMPOSE_FILE", "compose.yaml")
	var err error
	withPatchedAppDeps(t, func(d *core.AppDeps) {
		d.ComposeService = &arktest.MockCompose{
			LoadProjectFn:     showIniMockCompose(false).LoadProjectFn,
			RunningServicesFn: showIniMockCompose(false).RunningServicesFn,
			RunOneOffFn: func(context.Context, string, string, []string, ark.RunOptions) (ark.ExecResult, error) {
				return ark.ExecResult{ExitCode: 1, Stderr: "Error: Game.ini doesn't exist yet.\n"}, nil
			},
		}
	}, func() {
		err = RunShowIni(core.CurrentAppDeps(), []string{"--instance", "asa1", "--file", "game"})
	})
	if err == nil || err.Error() != "Error: Game.ini doesn't exist yet." {
		t.Fatalf("err = %v", err)
	}
}

func TestRunShowIniUsesComposeFileFlag(t *testing.T) {
	var gotComposeFile string
	withPatchedAppDeps(t, func(d *core.AppDeps) {
		d.ComposeService = &arktest.MockCompose{
			LoadProjectFn:     showIniMockCompose(false).LoadProjectFn,
			RunningServicesFn: showIniMockCompose(false).RunningServicesFn,
			RunOneOffFn: func(_ context.Context, composeFile, _ string, _ []string, _ ark.RunOptions) (ark.ExecResult, error) {
				gotComposeFile = composeFile
				return ark.ExecResult{}, nil
			},
		}
	}, func() {
		if err := RunShowIni(core.CurrentAppDeps(), []string{"--instance", "asa1", "--file", "game", "--compose-file", "docker-compose.dev.yaml"}); err != nil {
			t.Fatalf("runShowIni: %v", err)
		}
	})
	if gotComposeFile != "docker-compose.dev.yaml" {
		t.Fatalf("expected compose-file flag to be honored, got: %s", gotComposeFile)
	}
}

// Script failures reach the user as one "Error:" message: the script's own, a wrapped one for any
// other stderr, or how to rebuild an image that predates ark-ini.py.
func TestRunShowIniScriptFailureMessages(t *testing.T) {
	for _, tc := range []struct{ stderr, want string }{
		{"Error: Game.ini doesn't exist yet.\n", "Error: Game.ini doesn't exist yet."},
		{"Traceback: boom\n", "Error: ark-ini.py exited 1 in asa1: Traceback: boom"},
		{"/usr/bin/python3.14: can't open file '/home/ark-user/scripts/runtime/ark-ini.py': [Errno 2] No such file or directory",
			"Error: asa1 runs an image built before ark-cli managed its INI files; rebuild it with: ark-cli update instance --instance asa1"},
	} {
		setEnv(t, "COMPOSE_FILE", "compose.yaml")
		var err error
		withPatchedAppDeps(t, func(d *core.AppDeps) {
			d.ComposeService = &arktest.MockCompose{
				LoadProjectFn:     showIniMockCompose(false).LoadProjectFn,
				RunningServicesFn: showIniMockCompose(false).RunningServicesFn,
				RunOneOffFn: func(context.Context, string, string, []string, ark.RunOptions) (ark.ExecResult, error) {
					return ark.ExecResult{ExitCode: 1, Stderr: tc.stderr}, nil
				},
			}
		}, func() {
			err = RunShowIni(core.CurrentAppDeps(), []string{"--instance", "asa1", "--file", "game"})
		})
		if err == nil || err.Error() != tc.want {
			t.Fatalf("stderr %q: err = %v, want %q", tc.stderr, err, tc.want)
		}
	}
}
