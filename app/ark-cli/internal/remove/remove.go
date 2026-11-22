package remove

import (
	"ark-cli/internal/backup"
	"ark-cli/internal/compose"
	"ark-cli/internal/core"
	"errors"
	"fmt"
	"os"
	"time"

	"pkg/ark"
	"pkg/ark/lifecycle"
	"pkg/ark/runtime"
)

type removeCommandService struct {
	deps *core.AppDeps
}

func newRemoveCommandService(deps *core.AppDeps) removeCommandService {
	return removeCommandService{deps: deps}
}

func runPreRemoveBackup(instance, composeFileFlag string) error {
	req := backup.BackupCreateCommandRequest{
		Target:            instance,
		Format:            "tar.gz",
		RetainCount:       20,
		RetainDays:        7,
		RetainSizeGB:      5,
		LockTimeout:       2 * time.Minute,
		BackupWaitTimeout: 10 * time.Minute,
		ComposeFileFlag:   composeFileFlag,
	}
	return backup.RunBackupCreateCommand(req)
}

func RunRemove(d *core.AppDeps, args []string) error {
	return newRemoveCommandService(d).run(args)
}

func (s removeCommandService) run(args []string) error {
	return core.WithAppDepsErr(s.deps, func() error {
		return runRemoveImpl(args)
	})
}

func runRemoveImpl(args []string) error {
	fs := core.NewCommandFlagSet(
		"remove",
		[]string{"remove instance --instance <instance> [options]"},
		"Remove one instance from compose and delete its saved volume (with backup by default). Use 'teardown instances' for the whole project.",
	)
	instanceFlag := fs.String("instance", "", "Instance name.")
	kill := fs.Bool("kill", false, "Kill the running service immediately before removal.")
	skipBackup := fs.Bool("skip-backup", false, "Skip the pre-remove backup step.")
	dryRun := fs.Bool("dry-run", false, "Print what would be done without making any changes.")
	composeFileFlag := fs.String("compose-file", "", "Path to docker-compose.yaml (default: ./docker-compose.yaml or COMPOSE_FILE).")

	if err := core.ParseNoPositionalArgs(fs, args); err != nil {
		return err
	}
	instance := *instanceFlag
	if instance == "" {
		return errors.New("Error: --instance is required.")
	}
	if instance == "all" {
		return errors.New("Error: use 'teardown instances' to tear down the full compose project.")
	}

	d := core.CurrentAppDeps()
	composeFile, _, err := compose.ResolveRequiredServiceConfig(d, *composeFileFlag, instance, compose.ResolveRequiredDockerComposeConfig)
	if err != nil {
		return err
	}

	if *dryRun {
		isRunning, _ := d.RuntimeHelper.ServiceRunningStatus(composeFile, instance)
		core.Infof("would remove instance: %s\n", instance)
		core.Infof("would remove volume: %s-saved\n", instance)
		if !*skipBackup {
			core.Infof("would create pre-remove backup for: %s\n", instance)
		}
		if isRunning {
			if *kill {
				core.Infof("would kill running service: %s\n", instance)
			} else {
				core.Infof("note: service is running; pass --kill to kill before removal\n")
			}
		}
		return nil
	}

	if !*skipBackup {
		if err := runPreRemoveBackup(instance, *composeFileFlag); err != nil {
			return err
		}
	}

	isRunning, err := d.RuntimeHelper.ServiceRunningStatus(composeFile, instance)
	if err != nil {
		return err
	}
	if isRunning {
		if !*kill {
			return errors.New("Error: service is running. Stop it first or pass --kill.")
		}
		if err := core.CurrentAppDeps().Lifecycle.Stop(core.CurrentAppDeps().Context(), lifecycle.StopRequest{
			ComposeFile: composeFile, Services: []string{instance}, Kill: true,
		}); err != nil {
			return err
		}
	}

	composeText, err := os.ReadFile(composeFile)
	if err != nil {
		return err
	}
	updated, removed := runtime.RemoveServiceBlock(string(composeText), instance)
	if !removed {
		return fmt.Errorf("Error: service not found in compose file: %s", instance)
	}
	updated, _ = runtime.RemoveVolumeEntry(updated, fmt.Sprintf("%s-saved", instance))
	if err := compose.WriteComposeText(d, composeFile, updated, nil); err != nil {
		return fmt.Errorf("Error: %s left unchanged, the edited file does not load: %w", composeFile, err)
	}

	if err := removeVolume(fmt.Sprintf("%s-saved", instance), composeFile); err != nil {
		return err
	}
	core.Infof("Removed instance %s from %s\n", instance, composeFile)
	return nil
}

func StopServiceImpl(composeFile, service string) error {
	return core.CurrentAppDeps().Lifecycle.Stop(core.CurrentAppDeps().Context(), lifecycle.StopRequest{
		ComposeFile: composeFile,
		Services:    []string{service},
		Timeout:     -1,
	})
}

func stopService(composeFile, service string) error {
	if dep := core.CurrentAppDeps().StopService; dep != nil {
		return dep(composeFile, service)
	}
	return StopServiceImpl(composeFile, service)
}

func RemoveVolumeImpl(volumeName, composeFile string) error {
	d := core.CurrentAppDeps()
	if d.Engine == nil {
		return fmt.Errorf("engine not initialized")
	}
	project := runtime.ComposeProjectName(composeFile)
	candidates := []string{fmt.Sprintf("%s_%s", project, volumeName), volumeName}
	for _, candidate := range candidates {
		err := d.Engine.RemoveVolume(d.Context(), candidate)
		if err == nil {
			return nil
		}
		if errors.Is(err, ark.ErrNotFound) {
			continue
		}
		return err
	}
	return fmt.Errorf("Error: failed to remove volume: %s", volumeName)
}

func removeVolume(volumeName, composeFile string) error {
	if dep := core.CurrentAppDeps().RemoveVolume; dep != nil {
		return dep(volumeName, composeFile)
	}
	return RemoveVolumeImpl(volumeName, composeFile)
}
