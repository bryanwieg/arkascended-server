package schedule

import (
	"ark-cli/internal/compose"
	"ark-cli/internal/core"
	"errors"
	"fmt"
	"strings"

	backup "pkg/ark/backup"
	"pkg/ark/runtime"
	"pkg/ark/schedule"
)

func RunCreateSchedule(d *core.AppDeps, args []string) error {
	if len(args) == 0 {
		return errors.New("Error: create schedule command is required.")
	}
	switch args[0] {
	case schedule.TypeBackup:
		return runCreateScheduleBackup(args[1:])
	case schedule.TypeRCON, schedule.TypeSave, schedule.TypeRestart:
		return runCreateScheduleAction(args[0], args[1:])
	default:
		return fmt.Errorf("Error: unknown create schedule command: %s", args[0])
	}
}

func runCreateScheduleBackup(args []string) error {
	fs := core.NewCommandFlagSet(
		"create schedule backup",
		[]string{"create schedule backup --instance <instance> --name <name> --cron \"m h dom mon dow\" [options]"},
		"Create a managed backup schedule entry in the compose file.",
	)
	instanceFlag := fs.String("instance", "", "Backup target: instance name, 'cluster' or 'all'.")
	name := fs.String("name", "", "Unique schedule name.")
	cronExpr := fs.String("cron", "", "Cron expression (5 fields).")
	composeFileFlag := fs.String("compose-file", "", "Compose file that holds the schedule.")
	format := fs.String("format", "tar.gz", "Backup format: tar, tar.gz, or tar.zst.")
	retainCount := fs.Int("retain-count", 20, "Retention count for scheduled backups.")
	retainDays := fs.Int("retain-days", 7, "Retention days for scheduled backups.")
	retainSizeGB := fs.Int("retain-size-gb", 5, "Retention size GiB for scheduled backups.")
	disabled := fs.Bool("disabled", false, "Create the schedule disabled.")
	if err := core.ParseNoPositionalArgs(fs, args); err != nil {
		return err
	}
	if *instanceFlag == "" {
		return errors.New("Error: --instance is required.")
	}
	instance := *instanceFlag
	if *name == "" {
		return errors.New("Error: --name is required.")
	}
	if *cronExpr == "" {
		return errors.New("Error: --cron is required.")
	}
	if err := runtime.ValidateComposeName(*name, "schedule name"); err != nil {
		return err
	}
	if err := schedule.ValidateCronExpr(*cronExpr); err != nil {
		return err
	}
	if err := backup.ValidateFormat(*format); err != nil {
		return err
	}
	if *retainCount < 0 || *retainDays < 0 || *retainSizeGB < 0 {
		return errors.New(schedule.ErrRetentionValuesNonNegative)
	}

	if err := validateScheduleComposeTarget(*composeFileFlag, instance); err != nil {
		return err
	}
	return addSchedule(*composeFileFlag, schedule.BackupSchedule{
		Name:         *name,
		Type:         schedule.TypeBackup,
		Instance:     instance,
		Enabled:      !*disabled,
		Cron:         *cronExpr,
		Format:       *format,
		RetainCount:  *retainCount,
		RetainDays:   *retainDays,
		RetainSizeGB: *retainSizeGB,
	})
}

func runCreateScheduleAction(scheduleType string, args []string) error {
	usage := "create schedule " + scheduleType + " --instance <instance> --name <name> --cron \"m h dom mon dow\""
	if scheduleType == schedule.TypeRCON {
		usage += " --command <text>"
	}
	fs := core.NewCommandFlagSet(
		"create schedule "+scheduleType,
		[]string{usage + " [options]"},
		"Create a managed "+scheduleType+" schedule entry in the compose file.",
	)
	instanceFlag := fs.String("instance", "", "Target: instance name or 'all'.")
	name := fs.String("name", "", "Unique schedule name.")
	cronExpr := fs.String("cron", "", "Cron expression (5 fields).")
	composeFileFlag := fs.String("compose-file", "", "Compose file that holds the schedule.")
	disabled := fs.Bool("disabled", false, "Create the schedule disabled.")
	command := new(string)
	if scheduleType == schedule.TypeRCON {
		command = fs.String("command", "", "RCON command to send.")
	}
	if err := core.ParseNoPositionalArgs(fs, args); err != nil {
		return err
	}
	if *instanceFlag == "" {
		return errors.New("Error: --instance is required.")
	}
	if *name == "" {
		return errors.New("Error: --name is required.")
	}
	if *cronExpr == "" {
		return errors.New("Error: --cron is required.")
	}
	if scheduleType == schedule.TypeRCON && strings.TrimSpace(*command) == "" {
		return errors.New("Error: --command is required.")
	}
	if *instanceFlag == backup.ClusterTarget {
		return errClusterBackupOnly(scheduleType)
	}
	if err := runtime.ValidateComposeName(*name, "schedule name"); err != nil {
		return err
	}
	if err := schedule.ValidateCronExpr(*cronExpr); err != nil {
		return err
	}
	if err := validateScheduleComposeTarget(*composeFileFlag, *instanceFlag); err != nil {
		return err
	}
	return addSchedule(*composeFileFlag, schedule.BackupSchedule{
		Name:     *name,
		Type:     scheduleType,
		Instance: *instanceFlag,
		Enabled:  !*disabled,
		Cron:     *cronExpr,
		Command:  *command,
	})
}

// scheduleStore returns the compose file that holds the schedules and the service whose run state, logs
// and locks are under that file's directory.
func scheduleStore(d *core.AppDeps, composeFileFlag string) (string, *schedule.Service) {
	composeFile := compose.ResolveComposeFile(composeFileFlag)
	return composeFile, d.ScheduleService.ForComposeFile(composeFile)
}

// addSchedule appends a new, already validated entry to the compose file, refusing a duplicate name.
func addSchedule(composeFileFlag string, s schedule.BackupSchedule) error {
	d := core.CurrentAppDeps()
	composeFile, svc := scheduleStore(d, composeFileFlag)
	file, err := svc.LoadScheduleFile(d.Context(), d.ComposeService, composeFile)
	if err != nil {
		return err
	}
	for _, existing := range file.Schedules {
		if existing.Name == s.Name {
			return fmt.Errorf("Error: schedule already exists: %s", s.Name)
		}
	}
	file.Schedules = append(file.Schedules, s)
	if err := svc.SaveScheduleFile(d.Context(), d.ComposeService, composeFile, file); err != nil {
		return err
	}
	core.Infof("Created schedule %s\n", s.Name)
	core.Infof("Schedule config: %s\n", composeFile)
	warnIfSchedulerDisabled(d, composeFile)
	return nil
}

func RunListSchedules(d *core.AppDeps, args []string) error {
	fs := core.NewCommandFlagSet("list schedules", []string{"list schedules"}, "List configured schedules.")
	jsonFlag := fs.Bool("json", false, "Output as JSON.")
	composeFileFlag := fs.String("compose-file", "", "Compose file that holds the schedules (default: ./docker-compose.yaml or COMPOSE_FILE).")
	if err := core.ParseNoPositionalArgs(fs, args); err != nil {
		return err
	}
	composeFile, svc := scheduleStore(d, *composeFileFlag)
	file, err := svc.LoadScheduleFile(d.Context(), d.ComposeService, composeFile)
	if err != nil {
		return err
	}
	if len(file.Schedules) == 0 {
		if *jsonFlag {
			return core.PrintJSONOutput([]core.ScheduleEntryJSON{})
		}
		core.Infoln("No schedules configured.")
		return nil
	}

	if *jsonFlag {
		out := make([]core.ScheduleEntryJSON, 0, len(file.Schedules))
		for _, s := range file.Schedules {
			out = append(out, core.ScheduleEntryJSON{Name: s.Name, Enabled: s.Enabled, Type: s.Type, Instance: s.Instance, Cron: s.Cron})
		}
		return core.PrintJSONOutput(out)
	}

	rows := make([][]string, 0, len(file.Schedules)+1)
	rows = append(rows, []string{"NAME", "ENABLED", "TYPE", "INSTANCE", "CRON", "LAST_STATUS", "LAST_EXIT", "LAST_FINISHED"})
	for _, s := range file.Schedules {
		lastStatus := "-"
		lastExit := "-"
		lastFinished := "-"
		if state, ok := svc.LoadRunState(s.Name); ok && state.LastStatus != "" {
			lastStatus = state.LastStatus
			lastExit = fmt.Sprintf("%d", state.LastExitCode)
			if strings.TrimSpace(state.LastFinishedAt) != "" {
				lastFinished = state.LastFinishedAt
			}
		}
		rows = append(rows, []string{s.Name, core.YesNo(s.Enabled), s.Type, s.Instance, s.Cron, lastStatus, lastExit, lastFinished})
	}
	core.PrintTable(rows)
	return nil
}

func RunRemoveSchedule(d *core.AppDeps, args []string) error {
	fs := core.NewCommandFlagSet(
		"remove schedule",
		[]string{"remove schedule --name <name>"},
		"Remove a configured schedule from the compose file.",
	)
	nameFlag := fs.String("name", "", "Schedule name.")
	composeFileFlag := fs.String("compose-file", "", "Compose file that holds the schedules (default: ./docker-compose.yaml or COMPOSE_FILE).")
	if err := core.ParseNoPositionalArgs(fs, args); err != nil {
		return err
	}
	if *nameFlag == "" {
		return errors.New("Error: --name is required.")
	}
	name := *nameFlag
	composeFile, svc := scheduleStore(d, *composeFileFlag)
	file, err := svc.LoadScheduleFile(d.Context(), d.ComposeService, composeFile)
	if err != nil {
		return err
	}
	_, idx, ok := schedule.FindScheduleByName(file, name)
	if !ok {
		return schedule.ErrNotFound(name)
	}
	file.Schedules = append(file.Schedules[:idx:idx], file.Schedules[idx+1:]...)
	if err := svc.SaveScheduleFile(d.Context(), d.ComposeService, composeFile, file); err != nil {
		return err
	}
	core.Infof("Removed schedule %s\n", name)
	warnIfSchedulerDisabled(d, composeFile)
	return nil
}

// warnIfSchedulerDisabled warns when the schedules' compose project has no scheduler service.
func warnIfSchedulerDisabled(d *core.AppDeps, composeFile string) {
	// A load error is ignored: validation already ran, and this is only a hint.
	if project, err := d.RuntimeHelper.LoadComposeProject(composeFile); err == nil && !project.HasScheduler() {
		core.Warnf("Warning: the scheduler service is not enabled, so schedules will not run. Run 'ark-cli enable scheduler'.\n")
	}
}
