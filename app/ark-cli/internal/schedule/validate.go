package schedule

import (
	"ark-cli/internal/compose"
	"ark-cli/internal/core"
	"errors"
	"fmt"
	"strings"
	"sync"

	"pkg/ark"
	backup "pkg/ark/backup"
	"pkg/ark/runtime"
	"pkg/ark/schedule"
)

var validateScheduleRetention = schedule.ValidateRetention

// errClusterBackupOnly refuses the cluster target for a non-backup schedule type.
func errClusterBackupOnly(scheduleType string) error {
	return fmt.Errorf("Error: instance cluster is backup-only; %s schedules target an instance or 'all'.", scheduleType)
}

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

func validateScheduleEntry(s schedule.BackupSchedule, seen map[string]bool, loadProject func() (*ark.ComposeProject, error)) error {
	if seen[s.Name] {
		return fmt.Errorf("Error: duplicate schedule name: %s", s.Name)
	}
	seen[s.Name] = true
	if err := runtime.ValidateComposeName(s.Name, "schedule name"); err != nil {
		return err
	}
	if !schedule.KnownType(s.Type) {
		return schedule.ErrUnsupportedType(s.Type)
	}
	if s.Type == schedule.TypeRCON && strings.TrimSpace(s.Command) == "" {
		return errors.New("Error: rcon schedule requires a command.")
	}
	if s.Type != schedule.TypeBackup && s.Instance == backup.ClusterTarget {
		return errClusterBackupOnly(s.Type)
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
	if s.Instance == "all" {
		return nil
	}
	project, err := loadProject()
	if err != nil {
		return err
	}
	return ensureBackupTarget(project, s.Instance)
}

// validateScheduleFile validates the schedules of composeFile; the project is loaded once, when a target needs it.
func validateScheduleFile(file schedule.ScheduleFile, target, composeFile string) (int, error) {
	if len(file.Schedules) == 0 {
		return 0, errors.New("Error: no schedules configured.")
	}
	loadProject := sync.OnceValues(func() (*ark.ComposeProject, error) {
		return core.CurrentAppDeps().RuntimeHelper.LoadComposeProject(composeFile)
	})
	seen := map[string]bool{}
	validated := 0
	for _, s := range file.Schedules {
		if target != "" && s.Name != target {
			continue
		}
		if err := validateScheduleEntry(s, seen, loadProject); err != nil {
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
	composeFile, svc := scheduleStore(d, "")
	file, err := svc.LoadScheduleFile(d.Context(), d.ComposeService, composeFile)
	if err != nil {
		return err
	}
	_, err = validateScheduleFile(file, target, composeFile)
	return err
}
