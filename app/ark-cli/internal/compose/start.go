package compose

import (
	"ark-cli/internal/core"
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"pkg/ark/backup"
	"pkg/ark/health"
	"pkg/ark/host"
	"pkg/ark/lifecycle"
)

func RunStart(d *core.AppDeps, args []string) error {
	fs := core.NewCommandFlagSet(
		"start",
		[]string{"start instance --instance <instance|all> [options]"},
		"Start one or more compose services.",
	)
	composeFileFlag := fs.String("compose-file", "", "Path to docker-compose.yaml (default: ./docker-compose.yaml or COMPOSE_FILE).")
	noWait := fs.Bool("no-wait", false, "Return after starting compose service(s) without waiting for ready status.")
	waitTimeout := fs.Duration("wait-timeout", resolveStartWaitTimeout(), "Maximum time to wait for ready status (default: 5m or ARK_START_WAIT_TIMEOUT).")
	instanceFlag := fs.String("instance", "", "Target instance name or 'all'.")
	if err := core.ParseNoPositionalArgs(fs, args); err != nil {
		return err
	}
	instance := *instanceFlag
	if instance == "" {
		return errors.New("Error: --instance is required.")
	}
	return withTargetLocks(d, *composeFileFlag, instance, startNoServicesErr, func() error {
		return RunStartRequest(d, startCommandRequest{
			Instance:        instance,
			ComposeFileFlag: *composeFileFlag,
			NoWait:          *noWait,
			WaitTimeout:     *waitTimeout,
		})
	})
}

const startNoServicesErr = "Error: compose config has no services to start."

func RunStartRequest(d *core.AppDeps, req startCommandRequest) error {
	plan, err := buildStartExecutionPlan(d, req)
	if err != nil {
		return err
	}

	// Use lifecycle for the core compose up.
	_, err = d.Lifecycle.Start(d.Context(), lifecycle.StartRequest{
		ComposeFile:  plan.ComposeFile,
		Services:     plan.Services,
		PullPolicy:   plan.PullPolicy,
		WaitForReady: false, // CLI handles readiness waiting with progress display
	})
	if err != nil {
		return err
	}

	if plan.NoWait {
		core.Infof(
			"Started %d service(s): %s (not waiting for ready status)\n",
			len(plan.Services),
			strings.Join(plan.Services, ", "),
		)
	} else {
		if err := waitForStartedServicesReady(d, plan); err != nil {
			return err
		}
		core.Infof("Started %d service(s) and confirmed ready: %s\n", len(plan.Services), strings.Join(plan.Services, ", "))
	}
	// Starting recreates containers on their current image, so superseded images may now be unused.
	RemoveOldImages(d, plan.ComposeFile)
	return nil
}

func buildStartExecutionPlan(d *core.AppDeps, req startCommandRequest) (StartExecutionPlan, error) {
	composeFile, config, services, err := ResolveDockerComposeServiceTargets(d,
		req.ComposeFileFlag,
		req.Instance,
		startNoServicesErr,
	)
	if err != nil {
		return StartExecutionPlan{}, err
	}
	// Images are built on this host, never pulled.
	if err := d.RuntimeHelper.EnsureLocalImages(composeFile, config, services); err != nil {
		return StartExecutionPlan{}, err
	}
	if err := ensureStartPreflight(d, composeFile, services); err != nil {
		return StartExecutionPlan{}, err
	}
	if err := validateStartWaitTimeout(req.NoWait, req.WaitTimeout); err != nil {
		return StartExecutionPlan{}, err
	}

	return StartExecutionPlan{
		ComposeFile:        composeFile,
		Services:           services,
		PullPolicy:         "never",
		NoWait:             req.NoWait,
		WaitTimeout:        normalizeStartWaitTimeout(req.WaitTimeout),
		PreRunningSnapshot: captureStartPreRunningSnapshot(d, req.NoWait, composeFile),
		StartLogSince:      time.Now().UTC().Add(-1 * time.Second),
	}, nil
}

func ensureStartPreflight(d *core.AppDeps, composeFile string, services []string) error {
	host.WarnBelowMinimum(d.HostDeps)
	backupRoot := core.BackupRootDirForComposeFile(composeFile)
	return ensureStartBackupHeadroom(d, backupRoot, services)
}

func ensureStartBackupHeadroom(d *core.AppDeps, backupRoot string, services []string) error {
	return backup.EnsureHeadroomForStart(backupRoot, services, d.DiskUsageForPath, d.Logger)
}

func resolveStartWaitTimeout() time.Duration {
	if v := EnvOrDefault("ARK_START_WAIT_TIMEOUT", ""); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			return d
		}
	}
	return DefaultStartReadyWaitTimeout
}

func validateStartWaitTimeout(noWait bool, timeout time.Duration) error {
	if noWait {
		return nil
	}
	if timeout <= 0 {
		return fmt.Errorf("Error: --wait-timeout must be > 0 (got %s)", timeout)
	}
	return nil
}

func normalizeStartWaitTimeout(waitTimeout time.Duration) time.Duration {
	if waitTimeout <= 0 {
		return DefaultStartReadyWaitTimeout
	}
	return waitTimeout
}

func captureStartPreRunningSnapshot(d *core.AppDeps, noWait bool, composeFile string) map[string]struct{} {
	if noWait {
		return nil
	}
	snapshot, err := DoRunningServiceSnapshot(d, composeFile)
	if err != nil {
		return nil
	}
	return snapshot
}

func waitForStartedServicesReady(d *core.AppDeps, plan StartExecutionPlan) error {
	if core.VerboseEnabled() {
		return verboseFollowServicesUntilReady(d, plan)
	}
	if len(plan.Services) == 1 {
		return waitForOneServiceReady(d, plan, plan.Services[0])
	}
	return waitForAllServicesReadyConcurrent(d, plan)
}

func waitForOneServiceReady(d *core.AppDeps, plan StartExecutionPlan, service string) error {
	if shouldUseFreshReadinessLogs(plan.PreRunningSnapshot, service) {
		return waitForInstanceReadySinceWithStartProgress(d,
			plan.ComposeFile, service, plan.StartLogSince, plan.WaitTimeout, defaultStartReadyWaitInterval,
		)
	}
	return waitForInstanceReadyWithStartProgress(d,
		plan.ComposeFile, service, plan.WaitTimeout, defaultStartReadyWaitInterval,
	)
}

func waitForAllServicesReadyConcurrent(d *core.AppDeps, plan StartExecutionPlan) error {
	tasks := make([]TwoPhaseStartTask, len(plan.Services))
	for i, service := range plan.Services {
		since := plan.StartLogSince
		useFresh := shouldUseFreshReadinessLogs(plan.PreRunningSnapshot, service)
		var sincePtr *time.Time
		if useFresh {
			sincePtr = &since
		}
		tasks[i] = TwoPhaseStartTask{
			Service: service,
			WaitStarted: func(ctx context.Context) error {
				return d.HealthChecker.WaitForStarted(ctx, plan.ComposeFile, service, health.WaitOptions{
					Timeout: plan.WaitTimeout, Interval: defaultStartReadyWaitInterval, Since: sincePtr,
				})
			},
			WaitReady: func(ctx context.Context) error {
				return d.HealthChecker.WaitForReady(ctx, plan.ComposeFile, service, health.WaitOptions{
					Timeout: plan.WaitTimeout, Interval: defaultStartReadyWaitInterval, Since: sincePtr,
				})
			},
		}
	}
	return runTwoPhaseStartProgress(tasks)
}

func shouldUseFreshReadinessLogs(preRunningSnapshot map[string]struct{}, service string) bool {
	if preRunningSnapshot == nil {
		return false
	}
	_, wasRunning := preRunningSnapshot[service]
	return !wasRunning
}
