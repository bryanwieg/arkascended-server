package update

import (
	"ark-cli/internal/compose"
	"ark-cli/internal/core"

	"pkg/ark/runtime"
)

type UpdateCommandRequest struct {
	Target          string
	ComposeFileFlag string
	CheckOnly       bool
	Restart         bool
	Rollout         bool
}

func RunUpdate(d *core.AppDeps, args []string) error {
	fs := core.NewCommandFlagSet(
		"update",
		[]string{"update instance [--instance <instance>] [options]"},
		"Build the image for ARK's current Steam build and, with --rollout, switch instances to it.",
	)
	instanceFlag := fs.String("instance", "all", "Target instance name or 'all'.")
	composeFileFlag := fs.String("compose-file", "", "Path to docker-compose.yaml (default: ./docker-compose.yaml or COMPOSE_FILE).")
	checkOnly := fs.Bool("check", false, "Check for updates and print a table without changing images or restarting.")
	restart := fs.Bool("restart", false, "Switch updated instances to the new image one at a time, rolling back on failure.")
	rollout := fs.Bool("rollout", false, "Alias for --restart.")

	if err := core.ParseNoPositionalArgs(fs, args); err != nil {
		return err
	}

	req, err := buildUpdateCommandRequest(
		*instanceFlag,
		*composeFileFlag,
		*checkOnly,
		*restart,
		*rollout,
	)
	if err != nil {
		return err
	}
	return runUpdateCommand(req)
}

func buildUpdateCommandRequest(
	target,
	composeFileFlag string,
	checkOnly,
	restart,
	rollout bool,
) (UpdateCommandRequest, error) {
	if target != "all" {
		if err := runtime.ValidateComposeName(target, "instance"); err != nil {
			return UpdateCommandRequest{}, err
		}
	}
	return UpdateCommandRequest{
		Target:          target,
		ComposeFileFlag: composeFileFlag,
		CheckOnly:       checkOnly,
		Restart:         restart,
		Rollout:         rollout,
	}, nil
}

func runUpdateCommand(req UpdateCommandRequest) error {
	composeFile := compose.ResolveComposeFile(req.ComposeFileFlag)
	if err := compose.RequireComposeFile(composeFile); err != nil {
		return err
	}
	composeProject, err := core.CurrentAppDeps().ComposeService.LoadProject(core.CurrentAppDeps().Context(), composeFile)
	if err != nil {
		return err
	}
	project := composeProject.Project
	services, err := resolveUpdateTargetServices(project, req.Target)
	if err != nil {
		return err
	}

	rows, updates, err := planAndApplyImageUpdates(project, services, composeFile, req)
	if err != nil {
		return err
	}
	printImageUpdateTable(rows)
	for _, u := range updates {
		core.Infof("%s\n", u.summary())
	}

	if req.CheckOnly {
		return nil
	}
	if req.Restart || req.Rollout {
		// Each restarted instance's start removes images that became unused.
		return rolloutUpdatedInstances(composeFile, updates)
	}
	for _, u := range updates {
		core.Infof("Built %s. Switch with: ark-cli update instance --instance %s --rollout\n", u.To, u.Instance)
	}
	compose.RemoveOldImages(core.CurrentAppDeps(), composeFile)
	return nil
}
