package compose

import (
	"ark-cli/internal/core"
	"errors"
	"fmt"
	"strings"
	"time"

	"pkg/ark/lifecycle"
)

func RunStop(d *core.AppDeps, args []string) error {
	fs := core.NewCommandFlagSet(
		"stop",
		[]string{"stop instance --instance <instance|all> [options]"},
		"Gracefully stop one or more compose services.",
	)
	timeoutFlag := fs.String("timeout", "", "Time to allow for graceful shutdown before SIGKILL (e.g. 8m, 480s). Default: the compose stop_grace_period.")
	kill := fs.Bool("kill", false, "Kill the service immediately (bypasses graceful shutdown).")
	composeFileFlag := fs.String("compose-file", "", "Path to docker-compose.yaml (default: ./docker-compose.yaml or COMPOSE_FILE).")
	instanceFlag := fs.String("instance", "", "Target instance name or 'all'.")
	if err := core.ParseNoPositionalArgs(fs, args); err != nil {
		return err
	}
	instance := *instanceFlag
	if instance == "" {
		return errors.New("Error: --instance is required.")
	}
	timeout, err := parseTimeoutFlag(*timeoutFlag, *kill)
	if err != nil {
		return err
	}
	stop := func() error {
		return runStopRequest(stopCommandRequest{
			Instance:        instance,
			ComposeFileFlag: *composeFileFlag,
			Timeout:         timeout,
			Kill:            *kill,
		})
	}
	// --kill is the way out of a hung stop, rollout or backup: it must not wait behind their lock.
	if *kill {
		return stop()
	}
	return withTargetLocks(d, *composeFileFlag, instance, stopNoServicesErr, stop)
}

const stopNoServicesErr = "Error: compose config has no services to stop."

func runStopRequest(req stopCommandRequest) error {
	return NewComposeLifecycleCommandService(core.CurrentAppDeps()).runStopRequest(req)
}

func (s ComposeLifecycleCommandService) runStopRequest(req stopCommandRequest) error {
	return s.runWithDeps(func() error {
		composeFile, _, services, err := ResolveDockerComposeServiceTargets(core.CurrentAppDeps(),
			req.ComposeFileFlag,
			req.Instance,
			stopNoServicesErr,
		)
		if err != nil {
			return err
		}
		return runStopServices(composeFile, services, req.Timeout, req.Kill)
	})
}

func validateWaitModeFlags(kill bool, waitValue string) error {
	if kill && waitValue != "" {
		return fmt.Errorf("Error: use either --kill or --timeout, not both")
	}
	return nil
}

func parseTimeoutFlag(timeoutFlag string, kill bool) (time.Duration, error) {
	var timeout time.Duration = -1
	if timeoutFlag != "" {
		dur, err := time.ParseDuration(timeoutFlag)
		if err != nil {
			return -1, fmt.Errorf("Error: invalid --timeout value: %s", timeoutFlag)
		}
		if dur < 0 {
			return -1, fmt.Errorf("Error: --timeout must be >= 0")
		}
		timeout = dur
	}
	if err := validateWaitModeFlags(kill, timeoutFlag); err != nil {
		return -1, err
	}
	return timeout, nil
}

func runStopServices(composeFile string, services []string, timeout time.Duration, kill bool) error {
	d := core.CurrentAppDeps()

	if kill {
		core.Infof("Kill requested; bypassing graceful shutdown.\n")
	} else if timeout < 0 {
		core.Infof("Graceful stop requested (timeout: compose stop_grace_period).\n")
	} else {
		core.Infof("Graceful stop requested (timeout: %ds).\n", int(timeout.Seconds()))
	}

	failures, err := stopServicesWithProgress(services, func(service string) (bool, error) {
		stopErr := d.Lifecycle.Stop(d.Context(), lifecycle.StopRequest{
			ComposeFile: composeFile,
			Services:    []string{service},
			Timeout:     timeout,
			Kill:        kill,
		})
		return stopErr != nil, nil
	})
	if err != nil {
		return err
	}
	return finalizeStopResults(services, failures)
}

func stopServicesWithProgress(
	services []string,
	stopFn func(service string) (bool, error),
) (int, error) {
	failures := 0
	for _, service := range services {
		serviceFailed := false
		err := runStopServiceWithProgress(service, func() error {
			var stopErr error
			serviceFailed, stopErr = stopFn(service)
			return stopErr
		})
		if err != nil {
			return 0, err
		}
		if serviceFailed {
			failures++
		}
	}
	return failures, nil
}

func finalizeStopResults(services []string, failures int) error {
	if failures > 0 {
		return fmt.Errorf("Error: failed to stop %d service(s)", failures)
	}
	core.Infof("Stopped %d service(s): %s\n", len(services), strings.Join(services, ", "))
	return nil
}
