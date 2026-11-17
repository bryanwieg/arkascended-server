package health

import (
	"errors"
	"fmt"
	"time"

	"ark-cli/internal/compose"
	"ark-cli/internal/core"
	health "pkg/ark/health"
	"pkg/ark/runtime"
	"pkg/ark/update"
)

// RunCheckInstance checks instance health (running + startup ready).
func RunCheckInstance(d *core.AppDeps, args []string) error {
	fs := core.NewCommandFlagSet(
		"check instance",
		[]string{"check instance --instance <instance> [options]"},
		"Check instance health (running + startup ready).",
	)
	instanceFlag := fs.String("instance", "", "Instance name.")
	composeFileFlag := fs.String("compose-file", "", "Path to docker-compose.yaml (default: ./docker-compose.yaml or COMPOSE_FILE).")
	if err := core.ParseNoPositionalArgs(fs, args); err != nil {
		return err
	}
	if *instanceFlag == "" {
		return errors.New("Error: --instance is required.")
	}
	instance := *instanceFlag

	composeFile, config, err := compose.ResolveRequiredServiceConfig(d, *composeFileFlag, instance, compose.ResolveRequiredComposeConfig)
	if err != nil {
		return err
	}

	service := config.Project.Services[instance]
	summary := d.RuntimeHelper.BuildInstanceSummary(composeFile, instance, service)

	ready := false
	if summary.Status == "running" {
		isReady, _ := d.HealthChecker.IsReady(d.Context(), composeFile, instance, nil)
		ready = isReady
	}

	result := "unhealthy"
	if ready {
		result = "healthy"
	} else if summary.Status == "running" {
		result = "starting"
	} else if summary.Status == "stopped" {
		result = "stopped"
	}

	core.Infof("name: %s\n", summary.Name)
	core.Infof("status: %s\n", summary.Status)
	core.Infof("ready: %s\n", core.YesNo(ready))
	core.Infof("image: %s\n", runtime.FallbackDash(summary.Image))
	core.Infof("map: %s\n", runtime.FallbackDash(summary.Map))
	core.Infof("asa_port: %s\n", runtime.IntOrDash(summary.ASAPort))
	core.Infof("result: %s\n", result)
	printLastUpdate(composeFile, instance)

	if result == "healthy" || result == "stopped" {
		return nil
	}
	return fmt.Errorf("Error: instance is not healthy: %s (%s)", instance, result)
}

// printLastUpdate shows the instance's last update attempt, so a rolled-back
// server never looks like a plain healthy one. It never changes the exit code.
func printLastUpdate(composeFile, instance string) {
	st, err := update.LoadState(update.StateDir(composeFile), instance)
	if err != nil {
		core.Warnf("Warning: could not read update state: %v\n", err)
		return
	}
	a := st.LastAttempt
	if a == nil {
		return
	}
	say := core.Warnf
	if a.Result == update.ResultSucceeded {
		say = core.Infof
	}
	say("last_update: %s at %s (%s -> %s)\n", a.Result, a.Time.Format(time.RFC3339), a.From, runtime.FallbackDash(a.To))
	if a.Message != "" {
		say("last_update_message: %s\n", a.Message)
	}
}

// RunWait waits for an instance lifecycle state.
func RunWait(d *core.AppDeps, args []string) error {
	fs := core.NewCommandFlagSet(
		"wait",
		[]string{"wait instance --instance <instance> --for started|advertising|ready|stopped [options]"},
		"Wait for an instance lifecycle state.",
	)
	instanceFlag := fs.String("instance", "", "Instance name.")
	composeFileFlag := fs.String("compose-file", "", "Path to docker-compose.yaml (default: ./docker-compose.yaml or COMPOSE_FILE).")
	waitFor := fs.String("for", "ready", "Target state: started, advertising (alias: ready), or stopped.")
	timeout := fs.Duration("timeout", 10*time.Minute, "Maximum time to wait.")
	interval := fs.Duration("interval", 2*time.Second, "Polling interval.")
	if err := core.ParseNoPositionalArgs(fs, args); err != nil {
		return err
	}
	if *instanceFlag == "" {
		return errors.New("Error: --instance is required.")
	}
	instance := *instanceFlag
	if *timeout <= 0 {
		return errors.New("Error: timeout must be > 0.")
	}
	if *interval <= 0 {
		return errors.New("Error: interval must be > 0.")
	}

	composeFile, _, err := compose.ResolveRequiredServiceConfig(d, *composeFileFlag, instance, compose.ResolveRequiredComposeConfig)
	if err != nil {
		return err
	}

	opts := health.WaitOptions{Timeout: *timeout, Interval: *interval}
	ctx := d.Context()

	var (
		waitFn     func() error
		resultText string
	)
	switch *waitFor {
	case "started":
		waitFn = func() error {
			return d.HealthChecker.WaitForStarted(ctx, composeFile, instance, opts)
		}
		resultText = "started"
	case "advertising", "ready":
		waitFn = func() error {
			return d.HealthChecker.WaitForReady(ctx, composeFile, instance, opts)
		}
		resultText = "advertising"
	case "stopped":
		waitFn = func() error {
			return d.HealthChecker.WaitForStopped(ctx, composeFile, instance, opts)
		}
		resultText = "stopped"
	default:
		return fmt.Errorf("Error: --for must be 'started', 'advertising', 'ready', or 'stopped' (got %s)", *waitFor)
	}
	if err := waitFn(); err != nil {
		return err
	}
	core.Infof("Instance %s is %s.\n", instance, resultText)
	return nil
}

// RunValidateCompose validates compose configuration and summarizes service count.
func RunValidateCompose(d *core.AppDeps, args []string) error {
	fs := core.NewCommandFlagSet(
		"validate compose",
		[]string{"validate compose [options]"},
		"Validate compose configuration and summarize service count.",
	)
	composeFileFlag := fs.String("compose-file", "", "Path to docker-compose.yaml (default: ./docker-compose.yaml or COMPOSE_FILE).")
	if err := core.ParseNoPositionalArgs(fs, args); err != nil {
		return err
	}

	composeFile, config, err := compose.ResolveRequiredComposeConfig(d, *composeFileFlag)
	if err != nil {
		return err
	}
	serviceCount := len(config.ServiceNames())
	core.Infof("Compose config valid: %s (%d service(s))\n", composeFile, serviceCount)
	return nil
}
