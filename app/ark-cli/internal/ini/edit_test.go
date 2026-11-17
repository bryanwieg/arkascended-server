package ini

// Tests: edit-ini command: stopped vs running modes, force flag, editor override, compose-file flag

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"strings"
	"testing"

	"ark-cli/internal/core"

	"pkg/ark"
	"pkg/ark/arktest"
)

func editIniMockCompose(running bool) *arktest.MockCompose {
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

func TestRunEditIniStoppedUsesComposeRunAndPrintsSuccess(t *testing.T) {
	setEnv(t, "COMPOSE_FILE", "compose.yaml")
	var gotService string
	var gotCommand []string
	var gotOpts ark.RunOptions
	out := captureStdout(t, func() {
		withPatchedAppDeps(t, func(d *core.AppDeps) {
			d.ComposeService = &arktest.MockCompose{
				LoadProjectFn:     editIniMockCompose(false).LoadProjectFn,
				RunningServicesFn: editIniMockCompose(false).RunningServicesFn,
				RunOneOffInteractiveFn: func(_ context.Context, _, service string, command []string, opts ark.RunOptions) error {
					gotService = service
					gotCommand = command
					gotOpts = opts
					return nil
				},
			}
		}, func() {
			if err := RunEditIni(core.CurrentAppDeps(), []string{"--instance", "asa1", "--file", "game"}); err != nil {
				t.Fatalf("runEditIni: %v", err)
			}
		})
	})

	if !strings.Contains(out, "Changes to Game.ini saved successfully.") {
		t.Fatalf("expected stopped success message, got:\n%s", out)
	}
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
	if !strings.Contains(script, "exec 'nano' ") {
		t.Fatalf("expected nano in script, got:\n%s", script)
	}
	if !strings.Contains(script, "Game.ini") {
		t.Fatalf("expected target filename in script, got:\n%s", script)
	}
}

func TestRunEditIniRunningRequiresForce(t *testing.T) {
	setEnv(t, "COMPOSE_FILE", "compose.yaml")
	withPatchedAppDeps(t, func(d *core.AppDeps) {
		d.ComposeService = editIniMockCompose(true)
	}, func() {
		err := RunEditIni(core.CurrentAppDeps(), []string{"--instance", "asa1", "--file", "game"})
		if err == nil {
			t.Fatal("expected error")
		}
		if err.Error() != "Error: cannot edit config while service is running. Use --force to override." {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}

func TestRunEditIniRunningForceUsesComposeExecAndRestartMessage(t *testing.T) {
	setEnv(t, "COMPOSE_FILE", "compose.yaml")
	var gotService string
	var gotCommand []string
	out := captureStdout(t, func() {
		withPatchedAppDeps(t, func(d *core.AppDeps) {
			d.ComposeService = &arktest.MockCompose{
				LoadProjectFn:     editIniMockCompose(true).LoadProjectFn,
				RunningServicesFn: editIniMockCompose(true).RunningServicesFn,
				ExecInteractiveFn: func(_ context.Context, _, service string, command []string) error {
					gotService = service
					gotCommand = command
					return nil
				},
			}
		}, func() {
			if err := RunEditIni(core.CurrentAppDeps(), []string{"--instance", "asa1", "--file", "gus", "--force"}); err != nil {
				t.Fatalf("runEditIni: %v", err)
			}
		})
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
	if !strings.Contains(out, "Changes to GameUserSettings.ini saved.") {
		t.Fatalf("missing running-save message, got:\n%s", out)
	}
	if !strings.Contains(out, "You must restart the server for changes to take effect.") {
		t.Fatalf("missing restart message, got:\n%s", out)
	}
}

func TestRunEditIniUsesEditorOverride(t *testing.T) {
	setEnv(t, "COMPOSE_FILE", "compose.yaml")
	var gotCommand []string
	withPatchedAppDeps(t, func(d *core.AppDeps) {
		d.ComposeService = &arktest.MockCompose{
			LoadProjectFn:     editIniMockCompose(false).LoadProjectFn,
			RunningServicesFn: editIniMockCompose(false).RunningServicesFn,
			RunOneOffInteractiveFn: func(_ context.Context, _, _ string, command []string, _ ark.RunOptions) error {
				gotCommand = command
				return nil
			},
		}
	}, func() {
		if err := RunEditIni(core.CurrentAppDeps(), []string{"--instance", "asa1", "--file", "gameusersettings", "--editor", "vi"}); err != nil {
			t.Fatalf("runEditIni: %v", err)
		}
	})

	script := gotCommand[len(gotCommand)-1]
	if !strings.Contains(script, "exec 'vi' ") {
		t.Fatalf("expected editor override in script, got:\n%s", script)
	}
}

func TestRunEditIniUsesComposeFileFlag(t *testing.T) {
	var gotComposeFile string
	withPatchedAppDeps(t, func(d *core.AppDeps) {
		d.ComposeService = &arktest.MockCompose{
			LoadProjectFn:     editIniMockCompose(false).LoadProjectFn,
			RunningServicesFn: editIniMockCompose(false).RunningServicesFn,
			RunOneOffInteractiveFn: func(_ context.Context, composeFile, _ string, _ []string, _ ark.RunOptions) error {
				gotComposeFile = composeFile
				return nil
			},
		}
	}, func() {
		if err := RunEditIni(core.CurrentAppDeps(), []string{"--instance", "asa1", "--file", "game", "--compose-file", "docker-compose.dev.yaml"}); err != nil {
			t.Fatalf("runEditIni: %v", err)
		}
	})
	if gotComposeFile != "docker-compose.dev.yaml" {
		t.Fatalf("expected compose-file flag to be honored, got: %s", gotComposeFile)
	}
}

func TestIniEditorShellScriptIncludesNanoMissingGuidance(t *testing.T) {
	script := IniEditorShellScript(IniFileSpec{Filename: "Game.ini"}, "nano")
	if !strings.Contains(script, "Install nano") || !strings.Contains(script, "--editor <command>") {
		t.Fatalf("missing nano guidance in script:\n%s", script)
	}
}

func TestParseIniFileSpecRejectsUnknownValue(t *testing.T) {
	if _, err := ParseIniFileSpec("unknown"); err == nil {
		t.Fatal("expected error")
	}
}

func TestResolveIniEditorDefaultsAndOverride(t *testing.T) {
	if got := ResolveIniEditor(""); got != "nano" {
		t.Fatalf("default editor=%q want nano", got)
	}
	if got := ResolveIniEditor("vi"); got != "vi" {
		t.Fatalf("override editor=%q want vi", got)
	}
}

func TestIniEditorShellScriptRefusesMissingGameUserSettings(t *testing.T) {
	// GameUserSettings.ini is installed from the image's default on first start; a hand-made file
	// would stop that, so the editor must not create it. Run the real script: the container path
	// doesn't exist here, which is exactly the never-started case.
	spec, err := ParseIniFileSpec("gameusersettings")
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("sh", "-c", IniEditorShellScript(spec, "true"))
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	runErr := cmd.Run()
	var exitErr *exec.ExitError
	if !errors.As(runErr, &exitErr) || exitErr.ExitCode() != 1 {
		t.Fatalf("expected exit 1, got %v (stderr: %s)", runErr, stderr.String())
	}
	if !strings.Contains(stderr.String(), "doesn't exist yet. Start the instance once") {
		t.Fatalf("unexpected message: %s", stderr.String())
	}
	if _, statErr := os.Stat(iniContainerPath(spec)); !os.IsNotExist(statErr) {
		t.Fatalf("script must not create %s", iniContainerPath(spec))
	}
}

func TestIniEditorShellScriptCreatesMissingGameIni(t *testing.T) {
	// ARK never writes Game.ini, so editing is how admins create it.
	spec, err := ParseIniFileSpec("game")
	if err != nil {
		t.Fatal(err)
	}
	script := IniEditorShellScript(spec, "nano")
	if !strings.Contains(script, "touch ") || strings.Contains(script, "doesn't exist yet") {
		t.Fatalf("Game.ini script should create the file:\n%s", script)
	}
}
