package schedule

import (
	"ark-cli/internal/backup"
	"ark-cli/internal/core"
	"errors"
	"fmt"
	"time"

	"pkg/ark/schedule"
)

func runBackupFromSchedule(name string, args []string) error {
	if dep := core.CurrentAppDeps().RunBackupFromSchedule; dep != nil {
		return dep(name, args)
	}
	return backup.RunBackupFromScheduleImpl(name, args)
}

func RunRunSchedule(d *core.AppDeps, args []string) error {
	fs := core.NewCommandFlagSet("run schedule", []string{"run schedule --name <name>"}, "Run a configured schedule immediately.")
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
	if s.Type != "backup" {
		return schedule.ErrUnsupportedType(s.Type)
	}
	release, err := core.CurrentAppDeps().ScheduleService.AcquireLock(name)
	if err != nil {
		return err
	}
	defer release()

	startedAt := core.CurrentAppDeps().ScheduleService.Now()
	core.CurrentAppDeps().ScheduleService.AppendScheduleLog(name, fmt.Sprintf("%s START %s", startedAt.Format(time.RFC3339), name))
	state := schedule.RunState{
		Name:          name,
		LastStartedAt: startedAt.Format(time.RFC3339),
		LastStatus:    "running",
	}
	core.CurrentAppDeps().ScheduleService.WriteRunState(state)

	argsForBackup := []string{"--instance", s.Instance}
	argsForBackup = append(argsForBackup, schedule.BackupFlagArgs(s)...)
	if err := runBackupFromSchedule(name, argsForBackup); err != nil {
		finishedAt := core.CurrentAppDeps().ScheduleService.Now()
		state.LastFinishedAt = finishedAt.Format(time.RFC3339)
		state.LastStatus = "failed"
		state.LastExitCode = 1
		state.LastError = err.Error()
		core.CurrentAppDeps().ScheduleService.AppendScheduleLog(name, fmt.Sprintf("%s FAIL %s: %s", finishedAt.Format(time.RFC3339), name, err))
		core.CurrentAppDeps().ScheduleService.WriteRunState(state)
		return err
	}
	finishedAt := core.CurrentAppDeps().ScheduleService.Now()
	state.LastFinishedAt = finishedAt.Format(time.RFC3339)
	state.LastStatus = "success"
	state.LastExitCode = 0
	state.LastError = ""
	core.CurrentAppDeps().ScheduleService.AppendScheduleLog(name, fmt.Sprintf("%s SUCCESS %s", finishedAt.Format(time.RFC3339), name))
	core.CurrentAppDeps().ScheduleService.WriteRunState(state)
	core.Infof("Ran schedule %s\n", name)
	return nil
}
