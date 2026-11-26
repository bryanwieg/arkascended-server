package ini

import (
	"ark-cli/internal/compose"
	"ark-cli/internal/core"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// RunListIniHistory prints the recorded revisions of an instance's INI files, newest first.
// Read-only, so it works whether the instance is running or not.
func RunListIniHistory(d *core.AppDeps, args []string) error {
	fs := core.NewCommandFlagSet(
		"list ini-history",
		[]string{"list ini-history --instance <instance> [options]"},
		"List who changed the instance's INI files and when, newest first (ARK's own writes included).",
	)
	instanceFlag := fs.String("instance", "", "Instance name.")
	fileFlag := fs.String("file", "", "Only revisions that changed this INI file: game or gameusersettings.")
	userOnly := fs.Bool("user-only", false, "Hide the ARK server's own writes.")
	limitFlag := fs.Int("limit", 20, "How many revisions to show (0 for all).")
	sinceFlag := fs.String("since", "", "Only revisions on or after this UTC date (YYYY-MM-DD).")
	patchFlag := fs.Bool("patch", false, "Show each revision's diff instead of the table.")
	jsonFlag := fs.Bool("json", false, "Output as JSON.")
	composeFileFlag := fs.String("compose-file", "", "Path to docker-compose.yaml (default: ./docker-compose.yaml or COMPOSE_FILE).")
	if err := core.ParseNoPositionalArgs(fs, args); err != nil {
		return err
	}
	if *instanceFlag == "" {
		return errors.New("Error: --instance is required.")
	}
	if *limitFlag < 0 {
		return errors.New("Error: --limit must be 0 or more.")
	}
	scriptArgs := []string{"history"}
	if *fileFlag != "" {
		fileSpec, err := ParseIniFileSpec(*fileFlag)
		if err != nil {
			return err
		}
		scriptArgs = append(scriptArgs, "--file", fileSpec.Key)
	}
	if *userOnly {
		scriptArgs = append(scriptArgs, "--user-only")
	}
	scriptArgs = append(scriptArgs, "--limit", strconv.Itoa(*limitFlag))
	if *sinceFlag != "" {
		if _, err := time.Parse("2006-01-02", *sinceFlag); err != nil {
			return errors.New("Error: --since must be a date like 2026-11-26.")
		}
		scriptArgs = append(scriptArgs, "--since", *sinceFlag)
	}
	if *patchFlag {
		scriptArgs = append(scriptArgs, "--patch")
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
	res, err := readIniHistory(d, composeFile, instance, running, scriptArgs...)
	if err != nil {
		return err
	}
	if *jsonFlag {
		return core.PrintJSONOutput(res)
	}
	if len(res.Revisions) == 0 {
		core.Infof("No INI history recorded for %s.\n", instance)
		return nil
	}
	if *patchFlag {
		for _, r := range res.Revisions {
			core.Infof("%s  %s  %s  %s  %s\n%s\n", r.Revision, r.Date, r.By, r.How, r.Message, strings.TrimRight(r.Patch, "\n"))
		}
	} else {
		rows := [][]string{{"REVISION", "DATE", "BY", "HOW", "FILES", "MESSAGE"}}
		for _, r := range res.Revisions {
			rows = append(rows, []string{r.Revision, r.Date, r.By, r.How, strings.Join(r.Files, ","), r.Message})
		}
		core.PrintTable(rows)
	}
	if !*userOnly {
		core.Infoln("Showing all changes, including ARK's own writes; add --user-only to show user edits only.")
	}
	if res.More {
		core.Infoln("Older revisions exist; show them with --limit 0, or bound them with --since <YYYY-MM-DD>.")
	}
	return nil
}

// readIniHistory runs ark-ini.py history with scriptArgs and returns its reply.
func readIniHistory(d *core.AppDeps, composeFile, instance string, running bool, scriptArgs ...string) (core.IniHistoryJSON, error) {
	out, err := runIniScript(d, composeFile, instance, running, scriptArgs...)
	if err != nil {
		return core.IniHistoryJSON{}, err
	}
	var res core.IniHistoryJSON
	if err := json.Unmarshal([]byte(out), &res); err != nil {
		return core.IniHistoryJSON{}, fmt.Errorf("Error: unexpected reply from ark-ini.py: %w", err)
	}
	return res, nil
}
