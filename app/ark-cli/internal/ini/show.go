package ini

import (
	"ark-cli/internal/compose"
	"ark-cli/internal/core"
	"errors"
	"fmt"
	"strings"

	"pkg/ark"
)

// RunShowIni prints the contents of an instance INI config file to the terminal.
func RunShowIni(d *core.AppDeps, args []string) error {
	fs := core.NewCommandFlagSet(
		"show ini",
		[]string{"show ini --instance <instance> --file <game|gameusersettings> [options]"},
		"Print the contents of an instance INI config file to the terminal.",
	)
	instanceFlag := fs.String("instance", "", "Instance name.")
	fileFlag := fs.String("file", "", "INI file to show: game or gameusersettings.")
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

	composeFile, _, err := compose.ResolveRequiredServiceConfig(core.CurrentAppDeps(), *composeFileFlag, instance, compose.ResolveRequiredDockerComposeConfig)
	if err != nil {
		return err
	}
	running, err := d.RuntimeHelper.ServiceRunningStatus(composeFile, instance)
	if err != nil {
		return err
	}

	content, err := ReadIni(d, composeFile, instance, running, fileSpec)
	if err != nil {
		return err
	}
	if content != "" && !strings.HasSuffix(content, "\n") {
		content += "\n"
	}
	core.Infof("%s", content)
	return nil
}

const iniScriptPath = "/home/ark-user/scripts/runtime/ark-ini.py"

// ReadIni returns an instance INI file's content via ark-ini.py get.
func ReadIni(d *core.AppDeps, composeFile, instance string, running bool, fileSpec IniFileSpec) (string, error) {
	return runIniScript(d, composeFile, instance, running, "get", "--file", fileSpec.Key)
}

// runIniScript runs ark-ini.py in the running container, or in a one-off container when the
// instance is stopped, and returns its stdout. A non-zero exit returns the script's message.
func runIniScript(d *core.AppDeps, composeFile, instance string, running bool, args ...string) (string, error) {
	var result ark.ExecResult
	var err error
	if running {
		result, err = d.ComposeService.Exec(d.Context(), composeFile, instance,
			append([]string{"/usr/bin/python3.14", iniScriptPath}, args...), ark.ExecOptions{DisableTTY: true})
	} else {
		result, err = d.ComposeService.RunOneOff(d.Context(), composeFile, instance,
			append([]string{iniScriptPath}, args...),
			ark.RunOptions{Remove: true, NoDeps: true, DisableTTY: true, Entrypoint: "/usr/bin/python3.14"})
	}
	if err != nil {
		return "", err
	}
	if result.ExitCode != 0 {
		msg := strings.TrimSpace(result.Stderr)
		// Images built by an older ark-cli don't have the script; update instance rebuilds them.
		if strings.Contains(msg, "can't open file") && strings.Contains(msg, "ark-ini.py") {
			return "", fmt.Errorf("Error: %s runs an image built before ark-cli managed its INI files; rebuild it with: ark-cli update instance --instance %s", instance, instance)
		}
		// An older image's script doesn't know a newer subcommand (argparse: invalid choice).
		if len(args) > 0 && strings.Contains(msg, "invalid choice: '"+args[0]+"'") {
			return "", fmt.Errorf("Error: %s runs an image built before this ark-cli command existed; rebuild it with: ark-cli update instance --instance %s", instance, instance)
		}
		if !strings.HasPrefix(msg, "Error:") {
			msg = fmt.Sprintf("Error: ark-ini.py exited %d in %s: %s", result.ExitCode, instance, msg)
		}
		return "", errors.New(msg)
	}
	return result.Stdout, nil
}
