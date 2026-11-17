package compose

import (
	"ark-cli/internal/core"
	"errors"
	"os"

	"pkg/ark"
)

type ComposeConfigResolver func(d *core.AppDeps, composeFileFlag string) (string, *ark.ComposeProject, error)

func ResolveRequiredServiceConfig(d *core.AppDeps,
	composeFileFlag,
	service string,
	configResolver ComposeConfigResolver,
) (string, *ark.ComposeProject, error) {
	composeFile, project, err := configResolver(d, composeFileFlag)
	if err != nil {
		return "", nil, err
	}
	if err := project.EnsureService(service); err != nil {
		return "", nil, err
	}
	return composeFile, project, nil
}

func ResolveRequiredDockerComposeConfig(d *core.AppDeps, composeFileFlag string) (string, *ark.ComposeProject, error) {
	composeFile := ResolveComposeFile(composeFileFlag)
	if err := core.CurrentAppDeps().RuntimeHelper.EnsureDockerGroup(os.Stderr); err != nil {
		return "", nil, err
	}
	project, err := d.RuntimeHelper.LoadComposeProject(composeFile)
	if err != nil {
		return "", nil, err
	}
	return composeFile, project, nil
}

func ResolveDockerComposeServiceTargets(d *core.AppDeps,
	composeFileFlag string,
	instance string,
	emptyAllErr string,
) (string, *ark.ComposeProject, []string, error) {
	composeFile, project, err := ResolveRequiredDockerComposeConfig(d, composeFileFlag)
	if err != nil {
		return "", nil, nil, err
	}
	services, err := ResolveComposeServiceTargets(d, project, instance, emptyAllErr)
	if err != nil {
		return "", nil, nil, err
	}
	return composeFile, project, services, nil
}

func ResolveComposeServiceTargets(d *core.AppDeps, project *ark.ComposeProject, instance string, emptyAllErr string) ([]string, error) {
	if instance == "all" {
		services := project.ServiceNames()
		if len(services) == 0 {
			return nil, errors.New(emptyAllErr)
		}
		return services, nil
	}
	if err := project.EnsureService(instance); err != nil {
		return nil, err
	}
	return []string{instance}, nil
}
