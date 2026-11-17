package compose

import (
	"ark-cli/internal/core"
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"pkg/ark"
	"pkg/ark/health"
)

func verboseFollowServicesUntilReady(d *core.AppDeps, plan StartExecutionPlan) error {
	ctx, cancel := context.WithTimeout(d.Context(), plan.WaitTimeout)
	defer cancel()

	type result struct{ err error }
	ch := make(chan result, len(plan.Services))
	for _, svc := range plan.Services {
		var since *time.Time
		if shouldUseFreshReadinessLogs(plan.PreRunningSnapshot, svc) {
			t := plan.StartLogSince
			since = &t
		}
		go func() {
			ch <- result{followOneServiceUntilReady(d, ctx, plan.ComposeFile, svc, since)}
		}()
	}

	var firstErr error
	for range plan.Services {
		r := <-ch
		if r.err == nil || errors.Is(r.err, context.Canceled) {
			continue
		}
		if firstErr == nil {
			firstErr = r.err
			cancel()
		}
	}
	return firstErr
}

func followOneServiceUntilReady(d *core.AppDeps,
	ctx context.Context,
	composeFile, service string,
	since *time.Time,
) error {
	out := core.VerboseWriter().(io.Writer)

	// Read service logs via the SDK ComposeService seam. Follow: true streams
	// log output as it arrives, replacing the old raw exec.Command approach.
	reader, err := d.ComposeService.Logs(ctx, composeFile, service, ark.LogOptions{
		Tail:   0,
		Since:  since,
		Follow: true,
	})
	if err != nil {
		return fmt.Errorf("Error: failed to read logs for %s: %w", service, err)
	}
	defer reader.Close()

	// Tee: stream log output to both the verbose log writer and the scanner.
	pr, pw := io.Pipe()
	defer pw.Close()
	defer pr.Close()
	tee := io.TeeReader(reader, pw)

	// Write streamed output to verbose log in the background.
	go func() {
		scanOut := bufio.NewScanner(pr)
		scanOut.Buffer(make([]byte, 1<<20), 1<<20)
		for scanOut.Scan() {
			fmt.Fprintln(out, scanOut.Text())
		}
	}()

	// Exit detector: polls compose status in background.
	exitCtx, cancelExit := context.WithCancel(ctx)
	defer cancelExit()
	crashCh := make(chan error, 1)
	go func() {
		if err := pollForPrematureExit(d, exitCtx, composeFile, service); err != nil {
			crashCh <- err
		}
	}()

	// Scan the log stream for the startup marker.
	scanReader := bufio.NewScanner(tee)
	scanReader.Buffer(make([]byte, 1<<20), 1<<20)
	markerFound := false
	for scanReader.Scan() {
		if strings.Contains(scanReader.Text(), health.ReadyMarker) {
			markerFound = true
			break
		}
	}
	// Close the follow-mode reader so the pipe terminates and the subsequent
	// drain does not block indefinitely. This is critical for Podman's SDK
	// backend which does not propagate context cancellation to log following
	// the same way Docker does.
	reader.Close()
	// Drain any remaining data to keep background goroutines happy.
	_, _ = io.Copy(io.Discard, tee)
	cancelExit()

	if markerFound {
		return nil
	}

	// Check if a crash was detected by the exit detector.
	select {
	case err := <-crashCh:
		return err
	default:
	}

	if err := scanReader.Err(); err != nil {
		return err
	}

	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return fmt.Errorf("Error: timed out waiting for instance to become ready: %s", service)
	}
	if ctx.Err() != nil {
		return context.Canceled
	}
	return fmt.Errorf("Error: service exited before becoming ready: %s", service)
}

// pollForPrematureExit polls compose status to detect when a service exits
// before becoming ready.
func pollForPrematureExit(d *core.AppDeps, ctx context.Context, composeFile, service string) error {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			services, err := d.ComposeService.ExitedServices(ctx, composeFile)
			if err != nil {
				continue
			}
			if _, ok := services[service]; ok {
				return fmt.Errorf("Error: service exited prematurely before becoming ready: %s", service)
			}
		}
	}
}
