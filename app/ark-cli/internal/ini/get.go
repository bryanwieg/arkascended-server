package ini

import (
	"ark-cli/internal/compose"
	"ark-cli/internal/core"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
)

// RunGetIni prints every line for one INI key as a table (one row unless the key repeats), with
// where each came from. Read-only, so it works whether the instance is running or not.
func RunGetIni(d *core.AppDeps, args []string) error {
	fs := core.NewCommandFlagSet(
		"get ini",
		[]string{"get ini --instance <instance> --file <game|gameusersettings> --section <section> --key <key> [options]"},
		"Print one INI key as a table: every line for it, with whether ARK wrote it.",
	)
	instanceFlag := fs.String("instance", "", "Instance name.")
	fileFlag := fs.String("file", "", "INI file: game or gameusersettings.")
	sectionFlag := fs.String("section", "", "Section name, with or without brackets (e.g. ServerSettings).")
	keyFlag := fs.String("key", "", "Key name.")
	jsonFlag := fs.Bool("json", false, "Output as JSON (every line, with origins); exits 0 even when the key isn't set.")
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
	composeFile, _, err := compose.ResolveRequiredServiceConfig(d, *composeFileFlag, *instanceFlag, compose.ResolveRequiredDockerComposeConfig)
	if err != nil {
		return err
	}
	running, err := d.RuntimeHelper.ServiceRunningStatus(composeFile, *instanceFlag)
	if err != nil {
		return err
	}
	out, err := runIniScript(d, composeFile, *instanceFlag, running,
		"get", "--file", fileSpec.Key, "--section", *sectionFlag, "--key", *keyFlag)
	if err != nil {
		return err
	}
	var res core.IniKeyJSON
	if err := json.Unmarshal([]byte(out), &res); err != nil {
		return fmt.Errorf("Error: unexpected reply from ark-ini.py: %w", err)
	}
	if *jsonFlag {
		return core.PrintJSONOutput(res)
	}
	if len(res.Lines) == 0 {
		return fmt.Errorf("Error: %s is not set in [%s] of %s.", res.Key, res.Section, res.File)
	}
	rows := [][]string{{"LINE", "KEY", "VALUE", "ORIGIN"}}
	for _, l := range res.Lines {
		rows = append(rows, []string{strconv.Itoa(l.Line), res.Key, l.Value, l.Origin})
	}
	core.PrintTable(rows)
	return nil
}
