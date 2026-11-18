package compose

import (
	"errors"
	"fmt"
	"regexp"
	"strings"

	"ark-cli/internal/core"
)

// playerLinePattern matches one ListPlayers reply line: "0. Name, <EOS ID>".
// Names may contain commas, so the ID is whatever follows the last comma.
var playerLinePattern = regexp.MustCompile(`^\d+\.\s*(.*),\s*(\S+)$`)

// parseListPlayers returns the players in an RCON ListPlayers reply; an empty
// server replies "No Players Connected", which yields none.
func parseListPlayers(reply string) []core.PlayerJSON {
	players := []core.PlayerJSON{}
	for _, line := range strings.Split(reply, "\n") {
		if m := playerLinePattern.FindStringSubmatch(strings.TrimSpace(line)); m != nil {
			players = append(players, core.PlayerJSON{Name: m[1], ID: m[2]})
		}
	}
	return players
}

// RunListPlayers prints the players connected to each running target via RCON ListPlayers.
func RunListPlayers(d *core.AppDeps, args []string) error {
	fs := core.NewCommandFlagSet(
		"list players",
		[]string{"list players --instance <instance|all> [options]"},
		"List players connected to running instances (RCON ListPlayers).",
	)
	composeFileFlag := fs.String("compose-file", "", "Path to docker-compose.yaml (default: ./docker-compose.yaml or COMPOSE_FILE).")
	instanceFlag := fs.String("instance", "", "Target instance name or 'all'.")
	jsonFlag := fs.Bool("json", false, "Print JSON.")
	if err := core.ParseNoPositionalArgs(fs, args); err != nil {
		return err
	}
	if *instanceFlag == "" {
		return errors.New("Error: --instance is required.")
	}
	composeFile, _, services, err := ResolveDockerComposeServiceTargets(d, *composeFileFlag, *instanceFlag,
		"Error: compose config has no services to list players for.")
	if err != nil {
		return err
	}
	running, err := d.ComposeService.RunningServices(d.Context(), composeFile)
	if err != nil {
		return err
	}

	results := make([]core.InstancePlayersJSON, 0, len(services))
	var failed []string
	for _, service := range services {
		entry := core.InstancePlayersJSON{Instance: service, Players: []core.PlayerJSON{}}
		if _, ok := running[service]; !ok {
			entry.Error = "not running"
		} else if result, err := rconExec(d, composeFile, service, "ListPlayers"); err != nil {
			return err
		} else if result.ExitCode != 0 {
			entry.Error = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(result.Stderr), "Error:"))
			if entry.Error == "" {
				entry.Error = fmt.Sprintf("ark-rcon.py exited %d", result.ExitCode)
			}
		} else {
			entry.Players = parseListPlayers(result.Stdout)
		}
		if entry.Error != "" && (*instanceFlag != "all" || entry.Error != "not running") {
			failed = append(failed, service)
		}
		results = append(results, entry)
	}

	if *jsonFlag {
		if err := core.PrintJSONOutput(results); err != nil {
			return err
		}
	} else {
		for _, r := range results {
			switch {
			case r.Error != "":
				core.Infof("%s: %s\n", r.Instance, r.Error)
			case len(r.Players) == 0:
				core.Infof("%s: no players connected\n", r.Instance)
			default:
				core.Infof("%s: %d player(s)\n", r.Instance, len(r.Players))
				for _, p := range r.Players {
					core.Infof("  %s  (%s)\n", p.Name, p.ID)
				}
			}
		}
	}
	if len(failed) > 0 {
		return fmt.Errorf("Error: could not list players for: %s", strings.Join(failed, ", "))
	}
	return nil
}
