package schedule

import (
	"ark-cli/internal/backup"
	"ark-cli/internal/core"
	"errors"
	"fmt"
	"time"

	"pkg/ark/schedule"
)

func runBackupFromSchedule(d *core.AppDeps, name string, args []string) error {
	if dep := d.RunBackupFromSchedule; dep != nil {
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
	_, s, _, err := d.ScheduleService.LoadScheduleByName(name)
	if err != nil {
		return err
	}
	if err := RunScheduleOnce(d, s); err != nil {
		return err
	}
	core.Infof("Ran schedule %s\n", name)
	return nil
}

// RunScheduleOnce runs one backup schedule: lock, state "running", backup, state "success"/"failed",
// log file line and a matching stdout line. Returns the lock error or the backup error.
func RunScheduleOnce(d *core.AppDeps, s schedule.BackupSchedule) error {
	if s.Type != "backup" {
		return schedule.ErrUnsupportedType(s.Type)
	}
	svc := d.ScheduleService
	release, err := svc.AcquireLock(s.Name)
	if err != nil {
		return err
	}
	defer release()

	logLine := func(line string) {
		svc.AppendScheduleLog(s.Name, line)
		core.Infof("%s\n", line)
	}
	startedAt := svc.Now().Format(time.RFC3339)
	logLine(fmt.Sprintf("%s START %s", startedAt, s.Name))
	svc.WriteRunState(schedule.RunState{Name: s.Name, LastStartedAt: startedAt, LastStatus: "running"})

	// A stop or update rollout holds the instance lock for minutes; wait for it instead of failing.
	argsForBackup := append(append([]string{"--instance", s.Instance}, schedule.BackupFlagArgs(s)...), "--lock-timeout", "30m")
	backupErr := runBackupFromSchedule(d, s.Name, argsForBackup)
	finishedAt := svc.Now().Format(time.RFC3339)
	if backupErr != nil {
		logLine(fmt.Sprintf("%s FAIL %s: %s", finishedAt, s.Name, backupErr))
		svc.WriteRunState(schedule.RunState{Name: s.Name, LastStartedAt: startedAt, LastFinishedAt: finishedAt, LastStatus: "failed", LastExitCode: 1, LastError: backupErr.Error()})
		return backupErr
	}
	logLine(fmt.Sprintf("%s SUCCESS %s", finishedAt, s.Name))
	svc.WriteRunState(schedule.RunState{Name: s.Name, LastStartedAt: startedAt, LastFinishedAt: finishedAt, LastStatus: "success"})
	return nil
}
