package ini

import (
	"ark-cli/internal/compose"
	"ark-cli/internal/core"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"strings"

	"pkg/ark/runtime"
)

// setIniResult is ark-ini.py set's JSON reply.
type setIniResult struct {
	File    string   `json:"file"`
	Section string   `json:"section"`
	Key     string   `json:"key"`
	Value   string   `json:"value"`
	Old     []string `json:"old"`
	Changed bool     `json:"changed"`
	Removed int      `json:"removed"`
	Backup  *string  `json:"backup"`
}

// RunSetIni sets one key in an instance INI file through ark-ini.py, which enforces the safety rules.
func RunSetIni(d *core.AppDeps, args []string) error {
	fs := core.NewCommandFlagSet(
		"set ini",
		[]string{"set ini --instance <instance> --file <game|gameusersettings> --section <section> --key <key> --value <value> [options]"},
		"Set one INI key, adding it (and its section) if missing. A key that appears more than once is refused unless --replace-all.",
	)
	instanceFlag := fs.String("instance", "", "Instance name.")
	fileFlag := fs.String("file", "", "INI file: game or gameusersettings.")
	sectionFlag := fs.String("section", "", "Section name, with or without brackets (e.g. ServerSettings).")
	keyFlag := fs.String("key", "", "Key name.")
	valueFlag := fs.String("value", "", "New value (\"\" for an empty value).")
	replaceAll := fs.Bool("replace-all", false, "Replace every line for a repeated key with one line (keeps a .bak of the file).")
	restartNow := fs.Bool("restart", false, "If the instance is running: stop it (saving the world), apply the change, start it again.")
	composeFileFlag := fs.String("compose-file", "", "Path to docker-compose.yaml (default: ./docker-compose.yaml or COMPOSE_FILE).")
	if err := core.ParseNoPositionalArgs(fs, args); err != nil {
		return err
	}
	valueSet := false
	fs.Visit(func(f *flag.Flag) { valueSet = valueSet || f.Name == "value" })
	switch {
	case *instanceFlag == "":
		return errors.New("Error: --instance is required.")
	case *sectionFlag == "" || *keyFlag == "":
		return errors.New("Error: --section and --key are required.")
	case !valueSet:
		return errors.New("Error: --value is required (use --value \"\" for an empty value).")
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

	scriptArgs := []string{"set", "--file", fileSpec.Key, "--section", *sectionFlag, "--key", *keyFlag, "--value", *valueFlag}
	if *replaceAll {
		scriptArgs = append(scriptArgs, "--replace-all")
	}
	if !running {
		return applySetIni(d, composeFile, instance, scriptArgs)
	}
	return withInstanceStopped(d, instance, *composeFileFlag, func() error {
		return applySetIni(d, composeFile, instance, scriptArgs)
	})
}

// withInstanceStopped stops a running instance with the normal grace (so the world saves), runs fn,
// then starts it again even when fn failed, so a refused change never leaves the server down.
// The instance lock is held from the stop to the start.
func withInstanceStopped(d *core.AppDeps, instance, composeFileFlag string, fn func() error) error {
	return compose.WithInstanceLocks(d, compose.ResolveComposeFile(composeFileFlag), []string{instance}, func() error {
		core.Infof("Stopping %s...\n", instance)
		if err := compose.DoRestartStopInstance(d, runtime.StopCommandRequest{
			Instance: instance, ComposeFileFlag: composeFileFlag, Timeout: -1, // default grace so the server saves
		}); err != nil {
			return err
		}
		fnErr := fn()
		core.Infof("Starting %s...\n", instance)
		startErr := compose.DoRestartStartInstance(d, runtime.StartCommandRequest{
			Instance: instance, ComposeFileFlag: composeFileFlag, WaitTimeout: compose.DefaultStartReadyWaitTimeout,
		})
		if fnErr != nil && startErr != nil {
			return fmt.Errorf("%w\n%w", fnErr, startErr)
		}
		if fnErr != nil {
			return fnErr
		}
		return startErr
	})
}

func applySetIni(d *core.AppDeps, composeFile, instance string, scriptArgs []string) error {
	out, err := runIniScript(d, composeFile, instance, false, scriptArgs...)
	if err != nil {
		return err
	}
	var res setIniResult
	if err := json.Unmarshal([]byte(out), &res); err != nil {
		return fmt.Errorf("Error: unexpected reply from ark-ini.py: %w", err)
	}
	label := fmt.Sprintf("%s [%s] %s", res.File, res.Section, res.Key)
	if !res.Changed {
		core.Infof("%s is already %q; no change.\n", label, res.Value)
		return nil
	}
	was := "not set"
	if len(res.Old) > 0 {
		quoted := make([]string, len(res.Old))
		for i, v := range res.Old {
			quoted[i] = fmt.Sprintf("%q", v)
		}
		was = strings.Join(quoted, ", ")
	}
	core.Infof("%s: %s -> %q\n", label, was, res.Value)
	if res.Removed > 0 && res.Backup != nil {
		core.Infof("Removed %d repeated %s line(s); the previous file is saved as %s.\n", res.Removed, res.Key, *res.Backup)
	}
	return nil
}
