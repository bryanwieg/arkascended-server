package schedule

import (
	"ark-cli/internal/compose"
	"ark-cli/internal/core"
	"errors"
	"fmt"
	"strings"

	"pkg/ark"
	backup "pkg/ark/backup"
	"pkg/ark/runtime"
	"pkg/ark/schedule"
)

func InstallManagedSchedulesCrontabImpl(path string) error {
	result, err := core.CurrentAppDeps().Runtime.RunCommand([]string{"crontab", path}, ark.CommandOptions{Capture: true})
	if err != nil {
		return err
	}
	if result.ExitCode != 0 {
		stderr := strings.TrimSpace(result.Stderr)
		if stderr == "" {
			stderr = fmt.Sprintf("crontab exited with code %d", result.ExitCode)
		}
		return fmt.Errorf("Error: failed to install crontab: %s", stderr)
	}
	return nil
}

func installManagedSchedulesCrontab(path string) error {
	if dep := core.CurrentAppDeps().InstallManagedSchedulesCrontab; dep != nil {
		return dep(path)
	}
	return InstallManagedSchedulesCrontabImpl(path)
}

var validateScheduleRetention = schedule.ValidateRetention

func validateScheduleComposeTarget(composeFileFlag, instance string) error {
	composeFile := compose.ResolveComposeFile(composeFileFlag)
	if err := compose.RequireComposeFile(composeFile); err != nil {
		return err
	}
	if instance == "all" {
		return nil
	}
	project, err := core.CurrentAppDeps().RuntimeHelper.LoadComposeProject(composeFile)
	if err != nil {
		return err
	}
	return ensureBackupTarget(project, instance)
}

// ensureBackupTarget checks that a non-'all' schedule target exists: an instance, or a cluster volume for 'cluster'.
func ensureBackupTarget(project *ark.ComposeProject, instance string) error {
	if instance == backup.ClusterTarget {
		_, err := backup.ResolveClusterVolume(project)
		return err
	}
	return project.EnsureService(instance)
}

func validateScheduleEntry(s schedule.BackupSchedule, seen map[string]bool, composeConfigs map[string]*ark.ComposeProject) error {
	if seen[s.Name] {
		return fmt.Errorf("Error: duplicate schedule name: %s", s.Name)
	}
	seen[s.Name] = true
	if err := runtime.ValidateComposeName(s.Name, "schedule name"); err != nil {
		return err
	}
	if s.Type != "backup" {
		return schedule.ErrUnsupportedType(s.Type)
	}
	if err := schedule.ValidateCronExpr(s.Cron); err != nil {
		return err
	}
	if s.Format != "" && !backup.IsValidFormat(s.Format) {
		return fmt.Errorf("Error: invalid backup format: %s", s.Format)
	}
	if err := validateScheduleRetention(s); err != nil {
		return err
	}
	composeFile := compose.ResolveComposeFile(s.ComposeFile)
	if s.Instance == "all" {
		return nil
	}
	project, ok := composeConfigs[composeFile]
	if !ok {
		loadedProject, err := core.CurrentAppDeps().RuntimeHelper.LoadComposeProject(composeFile)
		if err != nil {
			return err
		}
		project = loadedProject
		composeConfigs[composeFile] = project
	}
	return ensureBackupTarget(project, s.Instance)
}

func validateScheduleFile(file schedule.ScheduleFile, target string) (int, error) {
	if len(file.Schedules) == 0 {
		return 0, errors.New("Error: no schedules configured.")
	}
	composeConfigs := map[string]*ark.ComposeProject{}
	seen := map[string]bool{}
	validated := 0
	for _, s := range file.Schedules {
		if target != "" && s.Name != target {
			continue
		}
		if err := validateScheduleEntry(s, seen, composeConfigs); err != nil {
			return 0, schedule.ErrValidationFailed(s.Name, err)
		}
		validated++
		if target != "" {
			core.Infof("Validated schedule %s\n", s.Name)
		}
	}
	if validated == 0 {
		return 0, schedule.ErrNotFound(target)
	}
	if target == "" {
		core.Infof("Validated %d schedule(s)\n", validated)
	}
	return validated, nil
}

func RunValidateSchedule(d *core.AppDeps, args []string) error {
	fs := core.NewCommandFlagSet(
		"validate schedule",
		[]string{"validate schedule [--name <name>]"},
		"Validate one schedule (or all schedules) and report configuration problems.",
	)
	nameFlag := fs.String("name", "", "Schedule name (optional; validates all if omitted).")
	if err := core.ParseNoPositionalArgs(fs, args); err != nil {
		return err
	}
	target := *nameFlag
	file, err := core.CurrentAppDeps().ScheduleService.LoadScheduleFile()
	if err != nil {
		return err
	}
	_, err = validateScheduleFile(file, target)
	return err
}

func RunApplySchedules(d *core.AppDeps, args []string) error {
	fs := core.NewCommandFlagSet(
		"apply schedules",
		[]string{"apply schedules"},
		"Validate schedules, regenerate the managed crontab file, and install it with crontab.",
	)
	skipValidate := fs.Bool("skip-validate", false, "Skip schedule validation before applying.")
	if err := core.ParseNoPositionalArgs(fs, args); err != nil {
		return err
	}

	file, err := core.CurrentAppDeps().ScheduleService.LoadScheduleFile()
	if err != nil {
		return err
	}
	if !*skipValidate {
		if _, err := validateScheduleFile(file, ""); err != nil {
			return err
		}
	}
	if err := core.CurrentAppDeps().ScheduleService.WriteManagedScheduleCrontab(file); err != nil {
		return err
	}
	cronPath := core.CurrentAppDeps().ScheduleService.ManagedCrontabPath
	if err := installManagedSchedulesCrontab(cronPath); err != nil {
		return err
	}
	core.Infof("Applied schedules to user crontab\n")
	core.Infof("Managed crontab file: %s\n", cronPath)
	return nil
}
