package compose

import (
	"ark-cli/internal/core"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"pkg/ark"
	"pkg/ark/runtime"
)

func DoRestartStartInstance(d *core.AppDeps, req runtime.StartCommandRequest) error {
	if dep := d.RestartStartInstance; dep != nil {
		return dep(req)
	}
	return RunStartRequest(d, startCommandRequest(req))
}

func DoRestartStopInstance(d *core.AppDeps, req runtime.StopCommandRequest) error {
	if dep := d.RestartStopInstance; dep != nil {
		return dep(req)
	}
	return runStopRequest(stopCommandRequest(req))
}

func RunRestart(d *core.AppDeps, args []string) error {
	fs := core.NewCommandFlagSet(
		"restart",
		[]string{"restart instance --instance <instance|all> [options]"},
		"Restart one or more compose services (stop then start).",
	)
	timeoutFlag := fs.String("timeout", "", "Time to allow for graceful shutdown (e.g. 5m, 30s). Forwarded to stop.")
	composeFileFlag := fs.String("compose-file", "", "Path to docker-compose.yaml (forwarded to stop/start).")
	instanceFlag := fs.String("instance", "", "Target instance name or 'all'.")
	if err := core.ParseNoPositionalArgs(fs, args); err != nil {
		return err
	}
	instance := *instanceFlag
	if instance == "" {
		return errors.New("Error: --instance is required.")
	}

	timeout, err := parseTimeoutFlag(*timeoutFlag, false)
	if err != nil {
		return err
	}
	// One lock span over stop and start, so the monitor never sees the gap between them.
	err = withTargetLocks(d, *composeFileFlag, instance, stopNoServicesErr, func() error {
		if err := DoRestartStopInstance(d, stopCommandRequest{
			Instance:        instance,
			ComposeFileFlag: *composeFileFlag,
			Timeout:         timeout,
		}); err != nil {
			return err
		}
		return DoRestartStartInstance(d, startCommandRequest{
			Instance:        instance,
			ComposeFileFlag: *composeFileFlag,
			// Restart uses the same readiness wait default as `start instance`.
			WaitTimeout: DefaultStartReadyWaitTimeout,
		})
	})
	if err != nil {
		return err
	}
	core.Infof("Restarted instance target %s\n", instance)
	return nil
}

func RunSave(d *core.AppDeps, args []string) error {
	fs := core.NewCommandFlagSet(
		"save",
		[]string{"save instance --instance <instance|all> [options]"},
		"Trigger an in-game saveworld via RCON for one or more running services.",
	)
	composeFileFlag := fs.String("compose-file", "", "Path to docker-compose.yaml (default: ./docker-compose.yaml or COMPOSE_FILE).")
	instanceFlag := fs.String("instance", "", "Target instance name or 'all'.")
	if err := core.ParseNoPositionalArgs(fs, args); err != nil {
		return err
	}
	instance := *instanceFlag
	if instance == "" {
		return errors.New("Error: --instance is required.")
	}

	composeFile, _, services, err := ResolveDockerComposeServiceTargets(d,
		*composeFileFlag,
		instance,
		"Error: compose config has no services to save.",
	)
	if err != nil {
		return err
	}

	failures := 0
	for _, service := range services {
		result, err := rconExec(d, composeFile, service, "saveworld")
		if err != nil {
			return err
		}
		if result.ExitCode != 0 {
			failures++
		}
	}
	if failures > 0 {
		return fmt.Errorf("Error: failed to save %d service(s)", failures)
	}
	core.Infof("Triggered saveworld for %d service(s): %s\n", len(services), strings.Join(services, ", "))
	return nil
}

// rconExec runs one RCON command in service through ark-rcon.py. The script reads
// SERVER_ADMIN_PASSWORD from the container env, so the password stays off argv;
// RCON_HOST/RCON_PORT on the host override the container's values.
func rconExec(d *core.AppDeps, composeFile, service string, command ...string) (ark.ExecResult, error) {
	var envVars []string
	for _, key := range []string{"RCON_HOST", "RCON_PORT"} {
		if v := os.Getenv(key); v != "" {
			envVars = append(envVars, fmt.Sprintf("%s=%s", key, v))
		}
	}
	argv := append([]string{"/usr/bin/python3.14", "/home/ark-user/scripts/runtime/ark-rcon.py"}, command...)
	return d.ComposeService.Exec(d.Context(), composeFile, service, argv, ark.ExecOptions{DisableTTY: true, Env: envVars})
}

// SetStopReason tells a running instance why its next stop happens ("updating to 98.9"), so the
// stop's player countdown can name it. Images older than ark-stop-reason.py fail the exec; the
// caller only warns, since the stop itself does not depend on it.
func SetStopReason(d *core.AppDeps, composeFile, service, reason string) error {
	argv := []string{"/usr/bin/python3.14", "/home/ark-user/scripts/runtime/ark-stop-reason.py", reason}
	result, err := d.ComposeService.Exec(d.Context(), composeFile, service, argv, ark.ExecOptions{DisableTTY: true})
	if err == nil && result.ExitCode != 0 {
		err = fmt.Errorf("exit code %d: %s", result.ExitCode, strings.TrimSpace(result.Stderr))
	}
	return err
}

func RunLogs(d *core.AppDeps, args []string) error {
	fs := core.NewCommandFlagSet(
		"logs",
		[]string{"logs instance --instance <instance> [options]"},
		"Show container logs for an instance.",
	)
	composeFileFlag := fs.String("compose-file", "", "Path to docker-compose.yaml (default: ./docker-compose.yaml or COMPOSE_FILE).")
	follow := fs.Bool("follow", false, "Follow log output.")
	timestamps := fs.Bool("timestamps", false, "Include timestamps.")
	tail := fs.Int("tail", 200, "Number of lines to show from the end of the logs.")
	instanceFlag := fs.String("instance", "", "Target instance name.")
	if err := core.ParseNoPositionalArgs(fs, args); err != nil {
		return err
	}
	instance := *instanceFlag
	if instance == "" {
		return errors.New("Error: --instance is required.")
	}
	if *tail < 0 {
		return errors.New("Error: tail must be >= 0.")
	}

	if err := core.CurrentAppDeps().RuntimeHelper.EnsureDockerGroup(os.Stderr); err != nil {
		return err
	}
	composeFile, _, err := ResolveRequiredServiceConfig(d, *composeFileFlag, instance, ResolveRequiredComposeConfig)
	if err != nil {
		return err
	}

	reader, err := d.ComposeService.Logs(d.Context(), composeFile, instance,
		ark.LogOptions{Tail: *tail, Follow: *follow, Timestamps: *timestamps})
	if err != nil {
		return err
	}
	defer reader.Close()
	_, err = io.Copy(os.Stdout, reader)
	return err
}
