package ini

import (
	"ark-cli/internal/compose"
	"ark-cli/internal/core"
	"errors"
	"fmt"
	"strings"

	"pkg/ark"
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

// RunEditIni opens an instance INI file in a one-off container (defaults to nano); a running
// instance is stopped first and started again afterwards, as ARK rewrites the file when it stops.
func RunEditIni(d *core.AppDeps, args []string) error {
	fs := core.NewCommandFlagSet(
		"edit ini",
		[]string{"edit ini --instance <instance> --file <game|gameusersettings> [options]"},
		"Open an instance INI file inside the container (defaults to nano).",
	)
	instanceFlag := fs.String("instance", "", "Instance name.")
	fileFlag := fs.String("file", "", "INI file to edit: game or gameusersettings.")
	editorFlag := fs.String("editor", "", "Editor command override (default: nano).")
	restartNow := fs.Bool("restart", false, "If the instance is running: stop it (saving the world), open the editor, start it again.")
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
	if running && !*restartNow {
		return fmt.Errorf("Error: %s is running, and ARK rewrites its INI files when it stops. Rerun with --restart to stop it, edit the file and start it again.", instance)
	}
	edit := func() error {
		if err := runIniEditorInEphemeralContainer(composeFile, instance, fileSpec, editorName); err != nil {
			return err
		}
		core.Infof("Changes to %s saved.\n", fileSpec.Filename)
		return nil
	}
	if running {
		return withInstanceStopped(d, instance, *composeFileFlag, edit)
	}
	return edit()
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
	// History (ark-ini.py track): changes nobody recorded first, then the edit; never blocks editing.
	track := func(by string) string {
		return "/usr/bin/python3.14 " + iniScriptPath + " track --by " + by +
			" || echo 'Warning: could not record the INI history.' >&2"
	}
	lines = append(lines, track("outside"))
	lines = append(lines, prepare...)
	return strings.Join(append(lines, qEditor+" "+qPath, track("edit")), "\n")
}
