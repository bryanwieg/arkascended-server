package ini

// Tests: show-ini command: stopped vs running modes, compose-file flag, shell script content

import (
	"context"
	"strings"
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

func TestRunShowIniStoppedUsesComposeRun(t *testing.T) {
	setEnv(t, "COMPOSE_FILE", "compose.yaml")
	var gotService string
	var gotCommand []string
	var gotOpts ark.RunOptions
	withPatchedAppDeps(t, func(d *core.AppDeps) {
		d.ComposeService = &arktest.MockCompose{
			LoadProjectFn:     showIniMockCompose(false).LoadProjectFn,
			RunningServicesFn: showIniMockCompose(false).RunningServicesFn,
			RunOneOffInteractiveFn: func(_ context.Context, _, service string, command []string, opts ark.RunOptions) error {
				gotService = service
				gotCommand = command
				gotOpts = opts
				return nil
			},
		}
	}, func() {
		if err := RunShowIni(core.CurrentAppDeps(), []string{"--instance", "asa1", "--file", "game"}); err != nil {
			t.Fatalf("runShowIni: %v", err)
		}
	})

	if gotService != "asa1" {
		t.Fatalf("unexpected service: %s", gotService)
	}
	if !gotOpts.Remove || !gotOpts.NoDeps || gotOpts.Entrypoint != "/bin/sh" {
		t.Fatalf("unexpected options: %+v", gotOpts)
	}
	if len(gotCommand) < 2 || gotCommand[0] != "-lc" {
		t.Fatalf("unexpected command: %#v", gotCommand)
	}
	script := gotCommand[1]
	if !strings.Contains(script, "Game.ini") {
		t.Fatalf("expected Game.ini in script, got:\n%s", script)
	}
	if !strings.Contains(script, "cat ") {
		t.Fatalf("expected cat in script, got:\n%s", script)
	}
}

func TestRunShowIniRunningUsesComposeExec(t *testing.T) {
	setEnv(t, "COMPOSE_FILE", "compose.yaml")
	var gotService string
	var gotCommand []string
	withPatchedAppDeps(t, func(d *core.AppDeps) {
		d.ComposeService = &arktest.MockCompose{
			LoadProjectFn:     showIniMockCompose(true).LoadProjectFn,
			RunningServicesFn: showIniMockCompose(true).RunningServicesFn,
			ExecInteractiveFn: func(_ context.Context, _, service string, command []string) error {
				gotService = service
				gotCommand = command
				return nil
			},
		}
	}, func() {
		if err := RunShowIni(core.CurrentAppDeps(), []string{"--instance", "asa1", "--file", "gus"}); err != nil {
			t.Fatalf("runShowIni: %v", err)
		}
	})

	if gotService != "asa1" {
		t.Fatalf("unexpected service: %s", gotService)
	}
	if len(gotCommand) < 3 || gotCommand[0] != "/bin/sh" || gotCommand[1] != "-lc" {
		t.Fatalf("unexpected command: %#v", gotCommand)
	}
	script := gotCommand[2]
	if !strings.Contains(script, "GameUserSettings.ini") {
		t.Fatalf("expected GameUserSettings.ini in script, got:\n%s", script)
	}
}

func TestRunShowIniUsesComposeFileFlag(t *testing.T) {
	var gotComposeFile string
	withPatchedAppDeps(t, func(d *core.AppDeps) {
		d.ComposeService = &arktest.MockCompose{
			LoadProjectFn:     showIniMockCompose(false).LoadProjectFn,
			RunningServicesFn: showIniMockCompose(false).RunningServicesFn,
			RunOneOffInteractiveFn: func(_ context.Context, composeFile, _ string, _ []string, _ ark.RunOptions) error {
				gotComposeFile = composeFile
				return nil
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

func TestIniCatShellScriptIncludesNotFoundHint(t *testing.T) {
	gus := IniCatShellScript(IniFileSpec{Key: "gameusersettings", Filename: "GameUserSettings.ini"})
	if !strings.Contains(gus, "Start the instance once") || strings.Contains(gus, "edit ini") {
		t.Fatalf("GameUserSettings.ini hint should say to start the instance, got:\n%s", gus)
	}
	script := IniCatShellScript(IniFileSpec{Key: "game", Filename: "Game.ini"})
	if !strings.Contains(script, "edit ini") {
		t.Fatalf("expected 'edit ini' hint in not-found message, got:\n%s", script)
	}
	if !strings.Contains(script, "cat ") {
		t.Fatalf("expected cat in script, got:\n%s", script)
	}
}
