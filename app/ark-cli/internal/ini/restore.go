package ini

import (
	"ark-cli/internal/compose"
	"ark-cli/internal/core"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
)

var iniRevisionPattern = regexp.MustCompile(`^[0-9a-fA-F]{4,40}$`)

// RunRestoreIni brings one INI file back to a revision of its history through ark-ini.py, which
// records the restore as a new revision.
func RunRestoreIni(d *core.AppDeps, args []string) error {
	fs := core.NewCommandFlagSet(
		"restore ini",
		[]string{"restore ini --instance <instance> --file <game|gameusersettings> --revision <revision> [options]"},
		"Bring an INI file back to a revision from list ini-history. The restore is recorded as a new revision.",
	)
	instanceFlag := fs.String("instance", "", "Instance name.")
	fileFlag := fs.String("file", "", "INI file: game or gameusersettings.")
	revisionFlag := fs.String("revision", "", "Revision from list ini-history.")
	restartNow := fs.Bool("restart", false, "If the instance is running: stop it (saving the world), restore the file, start it again.")
	composeFileFlag := fs.String("compose-file", "", "Path to docker-compose.yaml (default: ./docker-compose.yaml or COMPOSE_FILE).")
	if err := core.ParseNoPositionalArgs(fs, args); err != nil {
		return err
	}
	if *instanceFlag == "" {
		return errors.New("Error: --instance is required.")
	}
	fileSpec, err := ParseIniFileSpec(*fileFlag)
	if err != nil {
		return err
	}
	if !iniRevisionPattern.MatchString(*revisionFlag) {
		return errors.New("Error: --revision must be a revision from list ini-history.")
	}
	instance, revision := *instanceFlag, *revisionFlag

	composeFile, _, err := compose.ResolveRequiredServiceConfig(d, *composeFileFlag, instance, compose.ResolveRequiredDockerComposeConfig)
	if err != nil {
		return err
	}
	running, err := d.RuntimeHelper.ServiceRunningStatus(composeFile, instance)
	if err != nil {
		return err
	}
	apply := func() error {
		out, err := runIniScript(d, composeFile, instance, false, "restore", "--file", fileSpec.Key, "--revision", revision)
		if err != nil {
			return err
		}
		var res struct {
			File    string `json:"file"`
			Changed bool   `json:"changed"`
		}
		if err := json.Unmarshal([]byte(out), &res); err != nil {
			return fmt.Errorf("Error: unexpected reply from ark-ini.py: %w", err)
		}
		if !res.Changed {
			core.Infof("%s already matches %s; no change.\n", res.File, revision)
			return nil
		}
		core.Infof("%s restored from %s.\n", res.File, revision)
		return nil
	}
	if !running {
		return apply()
	}
	if !*restartNow {
		return fmt.Errorf("Error: %s is running, and ARK rewrites its INI files when it stops. Rerun with --restart to stop it, restore the file and start it again.", instance)
	}
	// Checked while still running, so an unknown revision doesn't cost a restart.
	history, err := readIniHistory(d, composeFile, instance, true, "history", "--limit", "0")
	if err != nil {
		return err
	}
	if !historyHasRevision(history, revision) {
		return fmt.Errorf("Error: revision %s is not in the INI history of %s.", revision, instance)
	}
	return withInstanceStopped(d, instance, *composeFileFlag, apply)
}

// historyHasRevision reports whether revision names a listed revision: the listed hashes are
// short, so either may be a prefix of the other.
func historyHasRevision(history core.IniHistoryJSON, revision string) bool {
	wanted := strings.ToLower(revision)
	for _, r := range history.Revisions {
		listed := strings.ToLower(r.Revision)
		if strings.HasPrefix(listed, wanted) || strings.HasPrefix(wanted, listed) {
			return true
		}
	}
	return false
}
