package scheduler

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"ark-cli/internal/compose"
	"ark-cli/internal/core"

	"pkg/ark"
	"pkg/ark/lifecycle"
	"pkg/ark/runtime"
	"pkg/ark/schedule"
)

// executablePath is the running ark-cli binary; tests point it at a fixture.
var executablePath = os.Executable

// RunEnableScheduler builds the scheduler image from this binary, writes the ark-scheduler service into the
// compose file (replacing an existing one) and starts it.
func RunEnableScheduler(d *core.AppDeps, args []string) error {
	fs := core.NewCommandFlagSet("enable scheduler", []string{"enable scheduler [--health-port <port>]"}, "Build the scheduler image from this ark-cli and add, refresh and start the ark-scheduler service.")
	composeFileFlag := fs.String("compose-file", "", "Path to docker-compose.yaml (default: ./docker-compose.yaml or COMPOSE_FILE).")
	healthPort := fs.Int("health-port", HealthContainerPort, "Host port publishing the health endpoint (0 = not published; default: the current one, else 8099).")
	if err := core.ParseNoPositionalArgs(fs, args); err != nil {
		return err
	}
	if *healthPort < 0 || *healthPort > 65535 {
		return errors.New("Error: --health-port must be between 0 and 65535")
	}
	composeFile := compose.ResolveComposeFile(*composeFileFlag)
	if err := compose.RequireComposeFile(composeFile); err != nil {
		return err
	}
	composeDir, err := requireComposeDirIsCwd(composeFile)
	if err != nil {
		return err
	}
	ref, err := buildSchedulerImage(d, composeDir)
	if err != nil {
		return err
	}
	project, err := d.RuntimeHelper.LoadComposeProject(composeFile)
	if err != nil {
		return err
	}
	settings, err := DetectServiceSettings(d, composeFile, project)
	if err != nil {
		return err
	}
	settings.Image = ref
	settings.HealthPort = *healthPort
	if !flagGiven(fs, "health-port") && project.HasScheduler() {
		settings.HealthPort = publishedHealthPort(project)
	}
	if err := writeSchedulerService(d, composeFile, settings); err != nil {
		return err
	}
	return startScheduler(d, composeFile, composeDir, ref, settings.HealthPort)
}

// RunDisableScheduler stops and removes the ark-scheduler service and its block in the compose file.
func RunDisableScheduler(d *core.AppDeps, args []string) error {
	fs := core.NewCommandFlagSet("disable scheduler", []string{"disable scheduler"}, "Stop and remove the ark-scheduler service (its images stay).")
	composeFileFlag := fs.String("compose-file", "", "Path to docker-compose.yaml (default: ./docker-compose.yaml or COMPOSE_FILE).")
	if err := core.ParseNoPositionalArgs(fs, args); err != nil {
		return err
	}
	composeFile := compose.ResolveComposeFile(*composeFileFlag)
	if err := compose.RequireComposeFile(composeFile); err != nil {
		return err
	}
	project, err := d.RuntimeHelper.LoadComposeProject(composeFile)
	if err != nil {
		return err
	}
	if !project.HasScheduler() {
		core.Infoln("Scheduler is not enabled")
		return nil
	}
	// Timeout -1 = the service's stop_grace_period, so a running backup can finish.
	if err := d.Lifecycle.Down(d.Context(), lifecycle.DownRequest{ComposeFile: composeFile, Service: ark.SchedulerService, Timeout: -1}); err != nil {
		return err
	}
	text, err := os.ReadFile(composeFile)
	if err != nil {
		return err
	}
	without, _ := runtime.RemoveServiceBlock(string(text), ark.SchedulerService)
	if err := replaceComposeChecked(d, composeFile, without, false); err != nil {
		return err
	}
	core.Infoln("Scheduler removed")
	return nil
}

// requireComposeDirIsCwd returns the compose dir, failing unless it is the working directory, or when the
// compose file is a symlink to a file outside it (the scheduler mounts only the compose dir).
func requireComposeDirIsCwd(composeFile string) (string, error) {
	composeDir, cwd, same, err := compose.ComposeDirIsCwd(composeFile)
	if err != nil {
		return "", err
	}
	if !same {
		return "", fmt.Errorf("Error: run 'ark-cli enable scheduler' from %s: schedules are read from %s, and the scheduler reads them from %s",
			composeDir, filepath.Join(cwd, ".ark-manage"), filepath.Join(composeDir, ".ark-manage"))
	}
	if target, err := filepath.EvalSymlinks(filepath.Join(composeDir, filepath.Base(composeFile))); err == nil {
		realDir, dirErr := filepath.EvalSymlinks(composeDir)
		if dirErr == nil && filepath.Dir(target) != realDir {
			return "", fmt.Errorf("Error: %s links to %s, outside %s; the scheduler mounts only the compose dir. Move the file into %s",
				composeFile, target, composeDir, composeDir)
		}
	}
	return composeDir, nil
}

// buildSchedulerImage builds ark-scheduler:<hash> from the running (static) binary unless it exists.
func buildSchedulerImage(d *core.AppDeps, composeDir string) (string, error) {
	exe, err := executablePath()
	if err != nil {
		return "", err
	}
	binary, err := filepath.EvalSymlinks(exe)
	if err != nil {
		return "", err
	}
	if err := schedule.CheckStatic(binary); err != nil {
		return "", err
	}
	ref, built, err := schedule.BuildImage(d.Context(), d.Engine, binary, filepath.Join(composeDir, ".ark-manage", "build", "scheduler"))
	if err != nil {
		return "", err
	}
	if built {
		core.Infof("Built scheduler image %s\n", ref)
	} else {
		core.Infof("Scheduler image %s is up to date\n", ref)
	}
	return ref, nil
}

func flagGiven(fs *flag.FlagSet, name string) bool {
	given := false
	fs.Visit(func(f *flag.Flag) { given = given || f.Name == name })
	return given
}

// publishedHealthPort returns the host port the current scheduler publishes for the health endpoint, or 0.
func publishedHealthPort(project *ark.ComposeProject) int {
	for _, p := range project.Project.Services[ark.SchedulerService].Ports {
		if p.Target == HealthContainerPort {
			if port, err := strconv.Atoi(p.Published); err == nil {
				return port
			}
		}
	}
	return 0
}

// writeSchedulerService replaces any ark-scheduler block in the compose file with the rendered one.
func writeSchedulerService(d *core.AppDeps, composeFile string, s ServiceSettings) error {
	block, err := runtime.ExtractServiceBlock(RenderService(s))
	if err != nil {
		return err
	}
	text, err := os.ReadFile(composeFile)
	if err != nil {
		return err
	}
	without, _ := runtime.RemoveServiceBlock(string(text), ark.SchedulerService)
	return replaceComposeChecked(d, composeFile, runtime.InsertService(without, block), true)
}

// replaceComposeChecked writes text next to composeFile, renames it over the file only when it loads and
// has (or lacks) the scheduler as wanted, and leaves the file unchanged otherwise.
func replaceComposeChecked(d *core.AppDeps, composeFile, text string, wantScheduler bool) error {
	// Write through a symlinked compose file; renaming over the link would replace it with a copy.
	if target, err := filepath.EvalSymlinks(composeFile); err == nil {
		composeFile = target
	}
	info, err := os.Stat(composeFile)
	if err != nil {
		return err
	}
	tmp := filepath.Join(filepath.Dir(composeFile), "."+filepath.Base(composeFile)+".ark-save.tmp")
	err = os.WriteFile(tmp, []byte(text), info.Mode().Perm()) //nolint:gosec // G703: tmp sits next to the user's own compose file
	if err == nil {
		err = checkSchedulerLoads(d, tmp, wantScheduler)
	}
	if err == nil {
		err = os.Rename(tmp, composeFile)
	}
	if err != nil {
		_ = os.Remove(tmp)
		if wantScheduler {
			return fmt.Errorf("Error: the scheduler service did not load, %s left unchanged: %w", composeFile, err)
		}
		return fmt.Errorf("Error: %s did not load without the scheduler service, left unchanged: %w", composeFile, err)
	}
	return nil
}

func checkSchedulerLoads(d *core.AppDeps, file string, want bool) error {
	project, err := d.ComposeService.LoadProject(d.Context(), file)
	if err != nil {
		return err
	}
	if project.HasScheduler() != want {
		return fmt.Errorf("scheduler service present = %v after the edit", !want)
	}
	return nil
}

// startScheduler creates the dirs the scratch image cannot, ups the service, then removes old images and
// ark-cli's old crontab block (both only warn on failure).
func startScheduler(d *core.AppDeps, composeFile, composeDir, ref string, healthPort int) error {
	for _, dir := range []string{".ark-manage", ".ark-tmp"} {
		if err := os.MkdirAll(filepath.Join(composeDir, dir), 0o755); err != nil {
			return err
		}
	}
	if err := d.ComposeService.Up(d.Context(), composeFile, []string{ark.SchedulerService}, ark.UpOptions{Detach: true}); err != nil {
		return err
	}
	core.Infof("Scheduler running (service %s)\n", ark.SchedulerService)
	if healthPort > 0 {
		hostname, _ := os.Hostname()
		core.Infof("Health endpoint: http://%s:%d/health\n", hostname, healthPort)
	}
	if _, err := schedule.RemoveOldImages(d.Context(), d.Engine, ref); err != nil {
		core.Warnf("Warning: could not remove old scheduler images: %v\n", err)
	}
	if err := RemoveManagedCrontab(d); err != nil {
		core.Warnf("Warning: could not remove ark-cli schedules from your crontab: %v\n", err)
	}
	return nil
}
