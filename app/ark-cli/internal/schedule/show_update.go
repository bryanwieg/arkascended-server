package schedule

import (
	"ark-cli/internal/compose"
	"ark-cli/internal/core"
	"errors"
	"fmt"
	"os"
	"strings"

	"pkg/ark/backup"
	"pkg/ark/runtime"
	"pkg/ark/schedule"
)

func RunCheckSchedule(d *core.AppDeps, args []string) error {
	fs := core.NewCommandFlagSet("check schedule", []string{"check schedule --name <name>"}, "Check schedule running status and last result.")
	nameFlag := fs.String("name", "", "Schedule name.")
	if err := core.ParseNoPositionalArgs(fs, args); err != nil {
		return err
	}
	if *nameFlag == "" {
		return errors.New("Error: --name is required.")
	}
	name := *nameFlag
	_, s, _, err := core.CurrentAppDeps().ScheduleService.LoadScheduleByName(name)
	if err != nil {
		return err
	}

	lockPath := core.CurrentAppDeps().ScheduleService.LockPath(name)
	_, lockErr := os.Stat(lockPath)
	running := lockErr == nil

	result := "ok"
	if running {
		result = "running"
	}

	core.Infof("name:     %s\n", s.Name)
	core.Infof("enabled:  %s\n", core.YesNo(s.Enabled))
	core.Infof("running:  %s\n", core.YesNo(running))

	if state, ok := core.CurrentAppDeps().ScheduleService.LoadRunState(name); ok {
		if !running && state.LastStatus == "failed" {
			result = "failed"
		}
		core.Infof("last_status:      %s\n", runtime.FallbackDash(state.LastStatus))
		core.Infof("last_started_at:  %s\n", runtime.FallbackDash(state.LastStartedAt))
		core.Infof("last_finished_at: %s\n", runtime.FallbackDash(state.LastFinishedAt))
	} else {
		core.Infof("last_status:      -\n")
		core.Infof("last_started_at:  -\n")
		core.Infof("last_finished_at: -\n")
	}
	core.Infof("result:   %s\n", result)

	if result == "failed" {
		return fmt.Errorf("Error: schedule last run failed: %s", name)
	}
	return nil
}

func RunShowSchedule(d *core.AppDeps, args []string) error {
	fs := core.NewCommandFlagSet("show schedule", []string{"show schedule --name <name>"}, "Show schedule configuration details.")
	jsonFlag := fs.Bool("json", false, "Output as JSON.")
	nameFlag := fs.String("name", "", "Schedule name.")
	if err := core.ParseNoPositionalArgs(fs, args); err != nil {
		return err
	}
	if *nameFlag == "" {
		return errors.New("Error: --name is required.")
	}
	name := *nameFlag
	_, s, _, err := core.CurrentAppDeps().ScheduleService.LoadScheduleByName(name)
	if err != nil {
		return err
	}

	if *jsonFlag {
		out := core.ScheduleDetailJSON{
			Name:         s.Name,
			Enabled:      s.Enabled,
			Type:         s.Type,
			Instance:     s.Instance,
			Cron:         s.Cron,
			Format:       s.Format,
			RetainCount:  s.RetainCount,
			RetainDays:   s.RetainDays,
			RetainSizeGB: s.RetainSizeGB,
			ComposeFile:  s.ComposeFile,
		}
		if state, ok := core.CurrentAppDeps().ScheduleService.LoadRunState(s.Name); ok {
			out.LastStatus = state.LastStatus
			out.LastExitCode = state.LastExitCode
			out.LastStartedAt = state.LastStartedAt
			out.LastFinishedAt = state.LastFinishedAt
			out.LastError = state.LastError
		}
		return core.PrintJSONOutput(out)
	}

	core.Infof("name: %s\n", s.Name)
	core.Infof("enabled: %s\n", core.YesNo(s.Enabled))
	core.Infof("type: %s\n", s.Type)
	core.Infof("instance: %s\n", s.Instance)
	core.Infof("cron: %s\n", s.Cron)
	core.Infof("format: %s\n", runtime.FallbackDash(s.Format))
	core.Infof("retain_count: %d\n", s.RetainCount)
	core.Infof("retain_days: %d\n", s.RetainDays)
	core.Infof("retain_size_gb: %d\n", s.RetainSizeGB)
	core.Infof("compose_file: %s\n", runtime.FallbackDash(s.ComposeFile))
	if s.Type == "backup" {
		preview, _ := core.CurrentAppDeps().ScheduleService.BackupCommandParts(s)
		core.Infof("command_preview: %s\n", strings.Join(preview, " "))
	}
	if state, ok := core.CurrentAppDeps().ScheduleService.LoadRunState(s.Name); ok {
		core.Infof("last_status: %s\n", runtime.FallbackDash(state.LastStatus))
		core.Infof("last_exit_code: %d\n", state.LastExitCode)
		core.Infof("last_started_at: %s\n", runtime.FallbackDash(state.LastStartedAt))
		core.Infof("last_finished_at: %s\n", runtime.FallbackDash(state.LastFinishedAt))
		if strings.TrimSpace(state.LastError) != "" {
			core.Infof("last_error: %s\n", state.LastError)
		}
	} else {
		core.Infof("last_status: -\n")
		core.Infof("last_exit_code: -\n")
		core.Infof("last_started_at: -\n")
		core.Infof("last_finished_at: -\n")
	}
	core.Infof("config_path: %s\n", core.CurrentAppDeps().ScheduleService.ConfigPath)
	return nil
}

func RunEnableSchedule(d *core.AppDeps, args []string) error {
	return setScheduleEnabled(d, args, "enable schedule", true)
}

func RunDisableSchedule(d *core.AppDeps, args []string) error {
	return setScheduleEnabled(d, args, "disable schedule", false)
}

func setScheduleEnabled(d *core.AppDeps, args []string, command string, enabled bool) error {
	verb := "Enabled"
	if !enabled {
		verb = "Disabled"
	}
	fs := core.NewCommandFlagSet(command, []string{command + " --name <name>"}, verb+" a managed schedule.")
	nameFlag := fs.String("name", "", "Schedule name.")
	if err := core.ParseNoPositionalArgs(fs, args); err != nil {
		return err
	}
	if *nameFlag == "" {
		return errors.New("Error: --name is required.")
	}
	name := *nameFlag
	file, s, idx, err := core.CurrentAppDeps().ScheduleService.LoadScheduleByName(name)
	if err != nil {
		return err
	}
	s.Enabled = enabled
	file.Schedules[idx] = s
	if err := core.CurrentAppDeps().ScheduleService.SaveScheduleAndCrontab(file); err != nil {
		return err
	}
	core.Infof("%s schedule %s\n", verb, s.Name)
	return nil
}

func RunUpdateSchedule(d *core.AppDeps, args []string) error {
	fs := core.NewCommandFlagSet(
		"update schedule",
		[]string{"update schedule --name <name> [options]"},
		"Update a configured schedule and regenerate the managed crontab file.",
	)
	nameFlag := fs.String("name", "", "Schedule name.")
	cronExpr := fs.String("cron", "", "Cron expression (5 fields).")
	instance := fs.String("instance", "", "Backup instance target (`all` allowed).")
	composeFileFlag := fs.String("compose-file", "", "Compose file to use when schedule runs.")
	format := fs.String("format", "", "Backup format: tar, tar.gz, or tar.zst.")
	retainCount := fs.Int("retain-count", -1, "Retention count.")
	retainDays := fs.Int("retain-days", -1, "Retention days.")
	retainSizeGB := fs.Int("retain-size-gb", -1, "Retention size GiB.")
	enable := fs.Bool("enabled", false, "Enable the schedule.")
	disable := fs.Bool("disabled", false, "Disable the schedule.")
	if err := core.ParseNoPositionalArgs(fs, args); err != nil {
		return err
	}
	if *nameFlag == "" {
		return errors.New("Error: --name is required.")
	}
	name := *nameFlag
	if *enable && *disable {
		return errors.New("Error: cannot pass both --enabled and --disabled.")
	}

	changed := false
	if *cronExpr != "" {
		if err := schedule.ValidateCronExpr(*cronExpr); err != nil {
			return err
		}
		changed = true
	}
	if *instance != "" {
		if err := runtime.ValidateComposeName(*instance, "instance"); err != nil && *instance != "all" {
			return err
		}
		changed = true
	}
	if *composeFileFlag != "" {
		changed = true
	}
	if *format != "" {
		if err := backup.ValidateFormat(*format); err != nil {
			return err
		}
		changed = true
	}
	if *retainCount >= 0 || *retainDays >= 0 || *retainSizeGB >= 0 || *enable || *disable {
		changed = true
	}
	if !changed {
		return errors.New("Error: no changes specified.")
	}

	file, s, idx, err := core.CurrentAppDeps().ScheduleService.LoadScheduleByName(name)
	if err != nil {
		return err
	}
	if s.Type != "backup" {
		return fmt.Errorf("Error: unsupported schedule type for update: %s", s.Type)
	}

	composeFile := compose.ResolveComposeFile(s.ComposeFile)
	if *composeFileFlag != "" {
		composeFile = *composeFileFlag
	}
	if *instance != "" {
		if err := validateScheduleComposeTarget(composeFile, *instance); err != nil {
			return err
		}
		s.Instance = *instance
	}

	if *cronExpr != "" {
		s.Cron = *cronExpr
	}
	if *composeFileFlag != "" {
		s.ComposeFile = *composeFileFlag
	}
	if *format != "" {
		s.Format = *format
	}
	if *retainCount >= 0 {
		s.RetainCount = *retainCount
	}
	if *retainDays >= 0 {
		s.RetainDays = *retainDays
	}
	if *retainSizeGB >= 0 {
		s.RetainSizeGB = *retainSizeGB
	}
	if *enable {
		s.Enabled = true
	}
	if *disable {
		s.Enabled = false
	}

	file.Schedules[idx] = s
	if err := core.CurrentAppDeps().ScheduleService.SaveScheduleAndCrontab(file); err != nil {
		return err
	}
	core.Infof("Updated schedule %s\n", s.Name)
	core.Infof("Managed crontab file: %s\n", core.CurrentAppDeps().ScheduleService.ManagedCrontabPath)
	return nil
}
