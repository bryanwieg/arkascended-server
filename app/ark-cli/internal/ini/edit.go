package ini

import (
	"ark-cli/internal/compose"
	"ark-cli/internal/core"
	"errors"
	"fmt"
	"strings"

	"pkg/ark"
	"pkg/ark/runtime"
)

const (
	savedDataContainerPath = "/home/ark-user/ark-server/ShooterGame/Saved"
	iniConfigSubdir        = "Config/WindowsServer"
)

// IniFileSpec identifies which INI file to operate on.
type IniFileSpec struct {
	Key      string
	Filename string
}

// RunEditIni opens an instance INI file inside the container (defaults to nano).
func RunEditIni(d *core.AppDeps, args []string) error {
	fs := core.NewCommandFlagSet(
		"edit ini",
		[]string{"edit ini --instance <instance> --file <game|gameusersettings> [options]"},
		"Open an instance INI file inside the container (defaults to nano).",
	)
	instanceFlag := fs.String("instance", "", "Instance name.")
	fileFlag := fs.String("file", "", "INI file to edit: game or gameusersettings.")
	editorFlag := fs.String("editor", "", "Editor command override (default: nano).")
	force := fs.Bool("force", false, "Allow editing while the instance is running (restart required for changes to take effect).")
	restartNow := fs.Bool("restart", false, "Restart the instance after saving changes (only applies when running).")
	composeFileFlag := fs.String("compose-file", "", "Path to docker-compose.yaml (default: ./docker-compose.yaml or COMPOSE_FILE).")
	if err := core.ParseNoPositionalArgs(fs, args); err != nil {
		return err
	}
	if *instanceFlag == "" {
		return errors.New("Error: --instance is required.")
	}
	instance := *instanceFlag

	fileSpec, err := ParseIniFileSpec(*fileFlag)
	if err != nil {
		return err
	}
	editorName := ResolveIniEditor(*editorFlag)

	composeFile, _, err := compose.ResolveRequiredServiceConfig(core.CurrentAppDeps(), *composeFileFlag, instance, compose.ResolveRequiredDockerComposeConfig)
	if err != nil {
		return err
	}
	running, err := d.RuntimeHelper.ServiceRunningStatus(composeFile, instance)
	if err != nil {
		return err
	}
	if running && !*force {
		return errors.New("Error: cannot edit config while service is running. Use --force to override.")
	}

	if running {
		if err := runIniEditorInRunningContainer(composeFile, instance, fileSpec, editorName); err != nil {
			return err
		}
		core.Infof("Changes to %s saved.\n", fileSpec.Filename)
		if *restartNow {
			core.Infof("Restarting %s...\n", instance)
			if err := compose.DoRestartStopInstance(d, runtime.StopCommandRequest{
				Instance:        instance,
				ComposeFileFlag: *composeFileFlag,
				Timeout:         -1, // default grace so the server saves
			}); err != nil {
				return err
			}
			if err := compose.DoRestartStartInstance(d, runtime.StartCommandRequest{
				Instance:        instance,
				ComposeFileFlag: *composeFileFlag,
				WaitTimeout:     compose.DefaultStartReadyWaitTimeout,
			}); err != nil {
				return err
			}
		} else {
			core.Infof("You must restart the server for changes to take effect.\n")
		}
		return nil
	}

	if err := runIniEditorInEphemeralContainer(composeFile, instance, fileSpec, editorName); err != nil {
		return err
	}
	core.Infof("Changes to %s saved successfully.\n", fileSpec.Filename)
	return nil
}

// ParseIniFileSpec parses the CLI --file flag value into an IniFileSpec.
func ParseIniFileSpec(raw string) (IniFileSpec, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "game":
		return IniFileSpec{Key: "game", Filename: "Game.ini"}, nil
	case "gameusersettings", "gus":
		return IniFileSpec{Key: "gameusersettings", Filename: "GameUserSettings.ini"}, nil
	case "":
		return IniFileSpec{}, errors.New("Error: --file is required (use 'game' or 'gameusersettings').")
	default:
		return IniFileSpec{}, fmt.Errorf("Error: invalid --file value: %s (use 'game' or 'gameusersettings')", raw)
	}
}

// ResolveIniEditor returns the editor command, defaulting to "nano" when empty.
func ResolveIniEditor(override string) string {
	override = strings.TrimSpace(override)
	if override != "" {
		return override
	}
	return "nano"
}

func runIniEditorInRunningContainer(composeFile, instance string, fileSpec IniFileSpec, editorName string) error {
	return core.CurrentAppDeps().ComposeService.ExecInteractive(core.CurrentAppDeps().Context(), composeFile, instance,
		[]string{"/bin/sh", "-lc", IniEditorShellScript(fileSpec, editorName)},
	)
}

func runIniEditorInEphemeralContainer(composeFile, instance string, fileSpec IniFileSpec, editorName string) error {
	return core.CurrentAppDeps().ComposeService.RunOneOffInteractive(core.CurrentAppDeps().Context(), composeFile, instance,
		[]string{"-lc", IniEditorShellScript(fileSpec, editorName)},
		ark.RunOptions{Remove: true, NoDeps: true, Entrypoint: "/bin/sh"},
	)
}

func iniContainerPath(fileSpec IniFileSpec) string {
	return savedDataContainerPath + "/" + strings.TrimPrefix(iniConfigSubdir, "/") + "/" + fileSpec.Filename
}

// missingIniMessage explains a missing INI file. GameUserSettings.ini is installed from the image's
// default on the instance's first start, so it must not be created by hand: a file made here would
// stop the default from ever being installed. ARK never creates Game.ini, so editing creates it.
func missingIniMessage(fileSpec IniFileSpec) string {
	if fileSpec.Key == "gameusersettings" {
		return "Error: " + fileSpec.Filename + " doesn't exist yet. Start the instance once to install the defaults, then edit it."
	}
	return "File not found: " + fileSpec.Filename + " (not yet created — use 'edit ini' to create it)"
}

func shellSingleQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", `'"'"'`) + "'"
}

// IniEditorShellScript returns a shell script that opens an editor for the INI file.
func IniEditorShellScript(fileSpec IniFileSpec, editorName string) string {
	containerPath := iniContainerPath(fileSpec)
	configDir := strings.TrimSuffix(containerPath, "/"+fileSpec.Filename)
	qEditor := shellSingleQuote(editorName)
	qPath := shellSingleQuote(containerPath)
	qDir := shellSingleQuote(configDir)
	missingMsg := "Error: editor not found: " + editorName + ". Install nano, or rerun with --editor <command> (for example: --editor vi)."
	if strings.TrimSpace(editorName) != "nano" {
		missingMsg = "Error: editor not found in container: " + editorName
	}
	qMissingMsg := shellSingleQuote(missingMsg)
	prepare := []string{"mkdir -p " + qDir, "touch " + qPath}
	if fileSpec.Key == "gameusersettings" {
		prepare = []string{
			"if [ ! -f " + qPath + " ]; then",
			"  echo " + shellSingleQuote(missingIniMessage(fileSpec)) + " >&2",
			"  exit 1",
			"fi",
		}
	}
	lines := []string{
		"set -e",
		"if ! command -v " + qEditor + " >/dev/null 2>&1; then",
		"  echo " + qMissingMsg + " >&2",
		"  exit 127",
		"fi",
	}
	lines = append(lines, prepare...)
	return strings.Join(append(lines, "exec "+qEditor+" "+qPath), "\n")
}
