package remove

import (
	"ark-cli/internal/compose"
	"ark-cli/internal/core"
	"bufio"
	"errors"
	"fmt"
	"os"
	"strings"
)

func RunTeardown(d *core.AppDeps, args []string) error {
	fs := core.NewCommandFlagSet(
		"teardown instances",
		[]string{"teardown instances [options]"},
		"Stop all services and remove networks and volumes for this compose project (compose down).",
	)
	composeFileFlag := fs.String("compose-file", "", "Path to docker-compose.yaml (default: ./docker-compose.yaml or COMPOSE_FILE).")
	skipConfirm := fs.Bool("skip-confirm", false, "Skip interactive confirmation prompt.")
	if err := core.ParseNoPositionalArgs(fs, args); err != nil {
		return err
	}

	if !*skipConfirm {
		core.Warnf("Warning: this will stop all services and remove networks and volumes for this compose project. This cannot be undone.\n")
		if core.IsInteractiveTerminal() {
			if !promptTeardownConfirm(os.Stdin) {
				core.Infof("Teardown cancelled.\n")
				return nil
			}
		} else {
			return errors.New("Error: teardown requires --skip-confirm when stdin is not a TTY.")
		}
	}

	return compose.RunDownRequest(compose.DownCommandRequest{
		Instance:        "all",
		ComposeFileFlag: *composeFileFlag,
		Timeout:         -1, // default grace so each server saves
	})
}

func promptTeardownConfirm(r *os.File) bool {
	fmt.Fprint(os.Stdout, "Continue? [y/N]: ")
	scanner := bufio.NewScanner(r)
	if !scanner.Scan() {
		return false
	}
	answer := strings.TrimSpace(scanner.Text())
	return strings.EqualFold(answer, "y") || strings.EqualFold(answer, "yes")
}
