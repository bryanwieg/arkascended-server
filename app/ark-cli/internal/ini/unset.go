package ini

import (
	"ark-cli/internal/compose"
	"ark-cli/internal/core"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// unsetIniResult is ark-ini.py unset's JSON reply.
type unsetIniResult struct {
	File    string   `json:"file"`
	Section string   `json:"section"`
	Key     string   `json:"key"`
	Old     []string `json:"old"`
	Changed bool     `json:"changed"`
}

// RunUnsetIni removes one key from an instance INI file through ark-ini.py, which enforces the safety rules.
func RunUnsetIni(d *core.AppDeps, args []string) error {
	fs := core.NewCommandFlagSet(
		"unset ini",
		[]string{"unset ini --instance <instance> --file <game|gameusersettings> --section <section> --key <key> [options]"},
		"Remove one INI key; the INI history keeps the previous file. A key that appears more than once is refused unless --all.",
	)
	instanceFlag := fs.String("instance", "", "Instance name.")
	fileFlag := fs.String("file", "", "INI file: game or gameusersettings.")
	sectionFlag := fs.String("section", "", "Section name, with or without brackets (e.g. ServerSettings).")
	keyFlag := fs.String("key", "", "Key name.")
	removeAll := fs.Bool("all", false, "Remove every line for a repeated key.")
	restartNow := fs.Bool("restart", false, "If the instance is running: stop it (saving the world), apply the change, start it again.")
	composeFileFlag := fs.String("compose-file", "", "Path to docker-compose.yaml (default: ./docker-compose.yaml or COMPOSE_FILE).")
	if err := core.ParseNoPositionalArgs(fs, args); err != nil {
		return err
	}
	switch {
	case *instanceFlag == "":
		return errors.New("Error: --instance is required.")
	case *sectionFlag == "" || *keyFlag == "":
		return errors.New("Error: --section and --key are required.")
	}
	fileSpec, err := ParseIniFileSpec(*fileFlag)
	if err != nil {
		return err
	}
	instance := *instanceFlag

	composeFile, _, err := compose.ResolveRequiredServiceConfig(d, *composeFileFlag, instance, compose.ResolveRequiredDockerComposeConfig)
	if err != nil {
		return err
	}
	running, err := d.RuntimeHelper.ServiceRunningStatus(composeFile, instance)
	if err != nil {
		return err
	}
	if running && !*restartNow {
		return fmt.Errorf("Error: %s is running, and ARK rewrites its INI files when it stops. Rerun with --restart to stop it, apply the change and start it again.", instance)
	}

	scriptArgs := []string{"unset", "--file", fileSpec.Key, "--section", *sectionFlag, "--key", *keyFlag}
	if *removeAll {
		scriptArgs = append(scriptArgs, "--all")
	}
	if !running {
		return applyUnsetIni(d, composeFile, instance, scriptArgs)
	}
	return withInstanceStopped(d, instance, *composeFileFlag, func() error {
		return applyUnsetIni(d, composeFile, instance, scriptArgs)
	})
}

func applyUnsetIni(d *core.AppDeps, composeFile, instance string, scriptArgs []string) error {
	out, err := runIniScript(d, composeFile, instance, false, scriptArgs...)
	if err != nil {
		return err
	}
	var res unsetIniResult
	if err := json.Unmarshal([]byte(out), &res); err != nil {
		return fmt.Errorf("Error: unexpected reply from ark-ini.py: %w", err)
	}
	label := fmt.Sprintf("%s [%s] %s", res.File, res.Section, res.Key)
	if !res.Changed {
		core.Infof("%s is not set; no change.\n", label)
		return nil
	}
	quoted := make([]string, len(res.Old))
	for i, v := range res.Old {
		quoted[i] = fmt.Sprintf("%q", v)
	}
	core.Infof("%s: removed %s\n", label, strings.Join(quoted, ", "))
	return nil
}
