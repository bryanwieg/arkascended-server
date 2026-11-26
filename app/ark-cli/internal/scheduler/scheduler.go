// Package scheduler implements the foreground `run scheduler` service command.
package scheduler

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"path/filepath"
	"sync"
	"time"

	"ark-cli/internal/compose"
	"ark-cli/internal/core"
	clischedule "ark-cli/internal/schedule"

	"pkg/ark"
	"pkg/ark/backup"
	"pkg/ark/health"
	"pkg/ark/host"
	"pkg/ark/lifecycle"
	"pkg/ark/schedule"
	"pkg/ark/update"
)

// RestartStopTimeout bounds a monitor restart's stop: an empty healthy server stops in ~30 s; a hung one is
// SIGKILLed by the engine instead of running the 5-minute player countdown (design 13).
const RestartStopTimeout = 2 * time.Minute

// healthListenAddr is where the health endpoint listens; tests use 127.0.0.1:0.
var healthListenAddr = fmt.Sprintf(":%d", HealthContainerPort)

// RunRunScheduler runs every enabled backup schedule when due and the health monitor until SIGINT/SIGTERM.
func RunRunScheduler(d *core.AppDeps, args []string) error {
	fs := core.NewCommandFlagSet("run scheduler", []string{"run scheduler"}, "Run schedules and health checks in the foreground until stopped (scheduler service).")
	if err := core.ParseNoPositionalArgs(fs, args); err != nil {
		return err
	}
	composeFile := compose.ResolveComposeFile("")
	if err := compose.RequireComposeFile(composeFile); err != nil {
		return err
	}
	sig := d.Context()
	ln, err := (&net.ListenConfig{}).Listen(sig, "tcp", healthListenAddr)
	if err != nil {
		return fmt.Errorf("Error: health endpoint cannot listen on %s: %w", healthListenAddr, err)
	}
	// Backups read their context from core.CurrentAppDeps() (internal/backup/create_runtime.go), so d must be
	// the current deps (the registry passes it); a stop signal must not abort a backup midway.
	d.Ctx = context.WithoutCancel(sig)
	d.RuntimeHelper.Ctx = d.Ctx
	ctx, cancel := context.WithCancel(sig) // also ends the loops when the health endpoint fails
	defer cancel()

	core.Infof("Scheduler started (TZ %s, schedules %s)\n", time.Local, composeFile)
	logf := func(format string, args ...any) { core.Infof(format+"\n", args...) }
	runner := schedule.Runner{
		Service:     d.ScheduleService.ForComposeFile(composeFile),
		Compose:     d.ComposeService,
		ComposeFile: composeFile,
		RunOne: func(_ context.Context, s schedule.BackupSchedule) error {
			return clischedule.RunScheduleOnce(d, composeFile, s)
		},
		Logf: logf,
	}
	monitor := newMonitor(d, composeFile, time.Now(), logf)
	srv := &http.Server{Handler: health.Handler(monitor.Latest, time.Now), ReadHeaderTimeout: 5 * time.Second}
	served := make(chan error, 1)
	go func() { served <- srv.Serve(ln); cancel() }()

	var wg sync.WaitGroup
	ticker := time.NewTicker(time.Minute) // the monitor needs no alignment
	defer ticker.Stop()
	wg.Go(func() { runner.Run(ctx, minuteTicks(ctx)) })
	wg.Go(func() { monitor.Run(ctx, ticker.C) })
	wg.Wait()

	shutdownCtx, stop := context.WithTimeout(d.Ctx, 5*time.Second)
	defer stop()
	_ = srv.Shutdown(shutdownCtx) // a slow client only delays the exit by the timeout
	if err := <-served; !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("Error: health endpoint on %s stopped: %w", healthListenAddr, err)
	}
	core.Infoln("Scheduler stopped")
	return nil
}

// minuteTicks ticks just after each wall-clock minute starts, so a schedule due at hh:mm runs then; a
// time.Ticker counts from the scheduler's start and ran schedules up to a minute late. Like a Ticker, it
// drops a tick the runner is too busy to take.
func minuteTicks(ctx context.Context) <-chan time.Time {
	c := make(chan time.Time, 1)
	go func() {
		for {
			timer := time.NewTimer(untilNextMinute(time.Now()))
			select {
			case <-ctx.Done():
				timer.Stop()
				return
			case t := <-timer.C:
				select {
				case c <- t:
				default:
				}
			}
		}
	}()
	return c
}

// untilNextMinute is the wait from now to the start of the next minute (a full minute when now is on one).
func untilNextMinute(now time.Time) time.Duration {
	return now.Truncate(time.Minute).Add(time.Minute).Sub(now)
}

// newMonitor wires the health monitor for composeFile; started is the scheduler's start time.
func newMonitor(d *core.AppDeps, composeFile string, started time.Time, logf func(string, ...any)) *health.Monitor {
	backupRoot := core.BackupRootDirForComposeFile(composeFile)
	return &health.Monitor{
		Probe:       &health.Probe{Compose: d.ComposeService, Engine: d.Engine, Checker: d.HealthChecker, UDP: host.ProbeUE5UDP, Now: time.Now},
		ComposeFile: composeFile,
		LoadProject: func(ctx context.Context) (*ark.ComposeProject, error) {
			return d.ComposeService.LoadProject(ctx, composeFile)
		},
		Schedules: func(now time.Time) []health.CheckResult {
			var checks []health.CheckResult
			for _, s := range d.ScheduleService.ForComposeFile(composeFile).Health(d.Context(), d.ComposeService, composeFile, now, started) {
				checks = append(checks, health.CheckResult{Name: s.Name, OK: s.OK, Detail: s.Detail})
			}
			return checks
		},
		LockHeld: func(inst string) (bool, error) {
			return backup.InstanceLockHeld(backup.InstanceLockDir(backupRoot, inst), inst)
		},
		Restart: func(ctx context.Context, inst string) error {
			release, err := backup.AcquireInstanceLock(backup.InstanceLockDir(backupRoot, inst), inst, 0)
			if errors.Is(err, backup.ErrLockHeld) {
				return health.ErrInstanceBusy
			}
			if err != nil {
				return err
			}
			defer release()
			// A stop signal must not leave the instance stopped halfway (stop_grace_period 8m covers both).
			noCancel := context.WithoutCancel(ctx)
			services := []string{inst}
			// Start even when the stop failed: a half-stopped instance is worse than a failed start attempt.
			stopErr := d.Lifecycle.Stop(noCancel, lifecycle.StopRequest{ComposeFile: composeFile, Services: services, Timeout: RestartStopTimeout})
			_, startErr := d.Lifecycle.Start(noCancel, lifecycle.StartRequest{ComposeFile: composeFile, Services: services, PullPolicy: "never"})
			return errors.Join(stopErr, startErr)
		},
		HealthPath: healthPath(composeFile),
		Now:        time.Now,
		Logf:       logf,
	}
}

// healthPath is the monitor's report file for composeFile.
func healthPath(composeFile string) string {
	return filepath.Join(update.StateDir(composeFile), "state", "scheduler", "health.json")
}
