package ini

import (
	"ark-cli/internal/compose"
	"ark-cli/internal/core"
	"errors"
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

	script := IniCatShellScript(fileSpec)
	if running {
		return d.ComposeService.ExecInteractive(d.Context(), composeFile, instance,
			[]string{"/bin/sh", "-lc", script})
	}
	return d.ComposeService.RunOneOffInteractive(d.Context(), composeFile, instance,
		[]string{"-lc", script},
		ark.RunOptions{Remove: true, NoDeps: true, Entrypoint: "/bin/sh"})
}

// IniCatShellScript returns a shell script that cats an INI file inside the container.
func IniCatShellScript(fileSpec IniFileSpec) string {
	qPath := shellSingleQuote(iniContainerPath(fileSpec))
	qNotFoundMsg := shellSingleQuote(missingIniMessage(fileSpec))
	return strings.Join([]string{
		"set -e",
		"if [ ! -f " + qPath + " ]; then",
		"  echo " + qNotFoundMsg + " >&2",
		"  exit 1",
		"fi",
		"cat " + qPath,
	}, "\n")
}
