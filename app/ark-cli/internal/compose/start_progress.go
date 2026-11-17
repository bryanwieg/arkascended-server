package compose

import (
	"ark-cli/internal/core"
	"context"
	"io"
	"time"

	"pkg/ark"
	"pkg/ark/health"
)

// TwoPhaseStartTask describes the two polling phases for one service startup:
// waitStarted resolves when the server process is up, waitReady resolves when
// it opens for player connections (advertising).
type TwoPhaseStartTask struct {
	Service     string
	WaitStarted func(ctx context.Context) error
	WaitReady   func(ctx context.Context) error
}

func waitForInstanceReadyWithStartProgress(d *core.AppDeps, composeFile, service string, timeout, interval time.Duration) error {
	return runTwoPhaseStartProgress([]TwoPhaseStartTask{{
		Service: service,
		WaitStarted: func(ctx context.Context) error {
			return d.HealthChecker.WaitForStarted(ctx, composeFile, service, health.WaitOptions{Timeout: timeout, Interval: interval})
		},
		WaitReady: func(ctx context.Context) error {
			return d.HealthChecker.WaitForReady(ctx, composeFile, service, health.WaitOptions{Timeout: timeout, Interval: interval})
		},
	}})
}

func waitForInstanceReadySinceWithStartProgress(d *core.AppDeps,
	composeFile, service string,
	since time.Time,
	timeout, interval time.Duration,
) error {
	sincePtr := &since
	return runTwoPhaseStartProgress([]TwoPhaseStartTask{{
		Service: service,
		WaitStarted: func(ctx context.Context) error {
			return d.HealthChecker.WaitForStarted(ctx, composeFile, service, health.WaitOptions{Timeout: timeout, Interval: interval, Since: sincePtr})
		},
		WaitReady: func(ctx context.Context) error {
			return d.HealthChecker.WaitForReady(ctx, composeFile, service, health.WaitOptions{Timeout: timeout, Interval: interval, Since: sincePtr})
		},
	}})
}

func runTwoPhaseStartProgress(tasks []TwoPhaseStartTask) error {
	if !startProgressSpinnerEnabled() {
		return runTwoPhaseNoSpinner(tasks)
	}
	return runTwoPhaseSpinner(tasks)
}

func fetchComposeServiceLogsQuiet(d *core.AppDeps, composeFile, service string, tail int, since *time.Time) (string, error) {
	reader, err := d.ComposeService.Logs(d.Context(), composeFile, service,
		ark.LogOptions{Tail: tail, Since: since})
	if err != nil {
		return "", err
	}
	defer reader.Close()
	data, err := io.ReadAll(reader)
	if err != nil {
		return "", err
	}
	text := string(data)
	return text, nil
}
