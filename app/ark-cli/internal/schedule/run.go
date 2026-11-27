package schedule

import (
	"ark-cli/internal/backup"
	"ark-cli/internal/compose"
	"ark-cli/internal/core"
	"ark-cli/internal/update"
	"errors"
	"fmt"
	"strings"
	"time"

	"pkg/ark/schedule"
)

func runBackupFromSchedule(d *core.AppDeps, name string, args []string) error {
	if dep := d.RunBackupFromSchedule; dep != nil {
		return dep(name, args)
	}
	return backup.RunBackupFromScheduleImpl(name, args)
}

func runUpdateFromSchedule(d *core.AppDeps, args []string) error {
	if dep := d.RunUpdateFromSchedule; dep != nil {
		return dep(args)
	}
	return update.RunUpdate(d, args)
}

func RunRunSchedule(d *core.AppDeps, args []string) error {
	fs := core.NewCommandFlagSet("run schedule", []string{"run schedule --name <name>"}, "Run a configured schedule immediately.")
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
	_, s, _, err := svc.LoadScheduleByName(d.Context(), d.ComposeService, composeFile, name)
	if err != nil {
		return err
	}
	if err := RunScheduleOnce(d, composeFile, s); err != nil {
		return err
	}
	core.Infof("Ran schedule %s\n", name)
	return nil
}

// RunScheduleOnce runs one schedule of composeFile on that compose file: lock, state "running", the backup or
// action, state "success"/"failed"/"skipped", log file lines and matching output lines. Returns the lock
// error or the run error.
func RunScheduleOnce(d *core.AppDeps, composeFile string, s schedule.BackupSchedule) error {
	if !schedule.KnownType(s.Type) {
		return schedule.ErrUnsupportedType(s.Type)
	}
	svc := d.ScheduleService.ForComposeFile(composeFile)
	release, err := svc.AcquireLock(s.Name)
	if err != nil {
		return err
	}
	defer release()

	logLine := func(line string) {
		svc.AppendScheduleLog(s.Name, line)
		core.Infof("%s\n", line)
	}
	logWarn := func(line string) {
		svc.AppendScheduleLog(s.Name, line)
		core.Warnf("%s\n", line)
	}
	startedAt := svc.Now().Format(time.RFC3339)
	logLine(fmt.Sprintf("%s START %s", startedAt, s.Name))
	svc.WriteRunState(schedule.RunState{Name: s.Name, LastStartedAt: startedAt, LastStatus: "running"})

	status, runErr := runSchedule(d, composeFile, s, logLine, logWarn)
	finishedAt := svc.Now().Format(time.RFC3339)
	if runErr != nil {
		logLine(fmt.Sprintf("%s FAIL %s: %s", finishedAt, s.Name, runErr))
		svc.WriteRunState(schedule.RunState{Name: s.Name, LastStartedAt: startedAt, LastFinishedAt: finishedAt, LastStatus: "failed", LastExitCode: 1, LastError: runErr.Error()})
		return runErr
	}
	logLine(fmt.Sprintf("%s %s %s", finishedAt, strings.ToUpper(status), s.Name))
	svc.WriteRunState(schedule.RunState{Name: s.Name, LastStartedAt: startedAt, LastFinishedAt: finishedAt, LastStatus: status})
	return nil
}

// runSchedule runs the backup or the action of s and returns the status to record on success.
func runSchedule(d *core.AppDeps, composeFile string, s schedule.BackupSchedule, logLine, logWarn func(string)) (string, error) {
	if s.Type == schedule.TypeBackup {
		// A stop or update rollout holds the instance lock for minutes; wait for it instead of failing.
		argsForBackup := append(append([]string{"--instance", s.Instance}, schedule.BackupFlagArgs(s)...), "--compose-file", composeFile, "--lock-timeout", "30m")
		return "success", runBackupFromSchedule(d, s.Name, argsForBackup)
	}
	_, _, services, err := compose.ResolveDockerComposeServiceTargets(d, composeFile, s.Instance,
		"Error: compose config has no services to run the schedule on.")
	if err != nil {
		return "", err
	}
	running, err := d.ComposeService.RunningServices(d.Context(), composeFile)
	if err != nil {
		return "", err
	}
	// Targets run one after another in name order; a failure does not stop the remaining ones,
	// except for update, where the first failure or rollback ends the run.
	var errs []error
	ran := 0
	for i, service := range services {
		if _, ok := running[service]; !ok {
			logWarn(fmt.Sprintf("Warning: %s is stopped; %s command unsuccessful", service, s.Type))
			continue
		}
		ran++
		if err := runActionOn(d, s, composeFile, service, logLine, logWarn); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", service, err))
			if s.Type == schedule.TypeUpdate {
				if rest := services[i+1:]; len(rest) > 0 {
					logWarn(fmt.Sprintf("Warning: skipped update of %s after %s failed", strings.Join(rest, ", "), service))
				}
				break
			}
		}
	}
	if ran == 0 {
		return "skipped", nil
	}
	return "success", errors.Join(errs...)
}

// runActionOn sends the rcon command or saveworld to a running service, restarts it the way
// `restart instance --instance <service>` does, or updates it like `update instance --rollout`.
func runActionOn(d *core.AppDeps, s schedule.BackupSchedule, composeFile, service string, logLine, logWarn func(string)) error {
	if s.Type == schedule.TypeUpdate {
		return runUpdateFromSchedule(d, []string{"--instance", service, "--rollout", "--compose-file", composeFile})
	}
	if s.Type == schedule.TypeRestart {
		if err := compose.SetStopReason(d, composeFile, service, "scheduled restart"); err != nil {
			logWarn(fmt.Sprintf("Warning: could not set the stop reason for %s: %v", service, err))
		}
		return compose.RunRestart(d, []string{"--instance", service, "--compose-file", composeFile})
	}
	command := s.Command
	if s.Type == schedule.TypeSave {
		command = "saveworld"
	}
	reply, err := compose.RCONCommand(d, composeFile, service, command)
	if reply != "" {
		logLine(fmt.Sprintf("%s: %s", service, reply))
	}
	return err
}
