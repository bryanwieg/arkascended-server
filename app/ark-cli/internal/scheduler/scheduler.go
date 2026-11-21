// Package scheduler implements the foreground `run scheduler` service command.
package scheduler

import (
	"context"
	"time"

	"ark-cli/internal/core"
	clischedule "ark-cli/internal/schedule"

	"pkg/ark/schedule"
)

// RunRunScheduler runs every enabled backup schedule when due until SIGINT/SIGTERM.
func RunRunScheduler(d *core.AppDeps, args []string) error {
	fs := core.NewCommandFlagSet("run scheduler", []string{"run scheduler"}, "Run schedules in the foreground until stopped (scheduler service).")
	if err := core.ParseNoPositionalArgs(fs, args); err != nil {
		return err
	}
	sig := d.Context()
	// Backups read their context from AppDeps; a stop signal must not abort one midway.
	d.Ctx = context.WithoutCancel(sig)
	d.RuntimeHelper.Ctx = d.Ctx

	core.Infof("Scheduler started (TZ %s, schedules %s)\n", time.Local, d.ScheduleService.ConfigPath)
	runner := schedule.Runner{
		Service: d.ScheduleService,
		RunOne:  func(_ context.Context, s schedule.BackupSchedule) error { return clischedule.RunScheduleOnce(d, s) },
		Logf:    func(format string, args ...any) { core.Infof(format+"\n", args...) },
	}
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	runner.Run(sig, ticker.C)
	core.Infoln("Scheduler stopped")
	return nil
}
