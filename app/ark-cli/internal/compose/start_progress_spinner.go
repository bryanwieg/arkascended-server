package compose

import (
	"ark-cli/internal/core"
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"time"
)

type spinnerTerminalRenderState struct {
	mu          sync.Mutex
	active      bool
	lineVisible bool
}

var globalSpinnerTerminalState spinnerTerminalRenderState

func startProgressSpinnerEnabled() bool {
	if core.VerboseEnabled() {
		return false
	}
	if os.Getenv("TERM") == "dumb" {
		return false
	}
	info, err := os.Stdout.Stat()
	if err != nil {
		return false
	}
	return (info.Mode() & os.ModeCharDevice) != 0
}

func SpinnerAwareStdoutWrite(message string) {
	SpinnerAwareStreamWrite(os.Stdout, message)
}

func SpinnerAwareStderrWrite(message string) {
	SpinnerAwareStreamWrite(os.Stderr, message)
}

func SpinnerAwareStreamWrite(stream *os.File, message string) {
	state := &globalSpinnerTerminalState
	state.mu.Lock()
	defer state.mu.Unlock()
	spinnerPrepareLocked(state, stream)
	fmt.Fprint(stream, message)
}

func spinnerPrepareLocked(state *spinnerTerminalRenderState, stream *os.File) {
	if !state.active || !state.lineVisible {
		return
	}
	fmt.Fprint(stream, "\n")
	state.lineVisible = false
}

func spinnerTerminalBegin() {
	state := &globalSpinnerTerminalState
	state.mu.Lock()
	defer state.mu.Unlock()
	state.active = true
	state.lineVisible = false
}

func spinnerTerminalFrame(format string, args ...any) {
	state := &globalSpinnerTerminalState
	state.mu.Lock()
	defer state.mu.Unlock()
	fmt.Fprintf(os.Stdout, format, args...)
	state.lineVisible = true
}

func spinnerTerminalEndLine(format string, args ...any) {
	state := &globalSpinnerTerminalState
	state.mu.Lock()
	defer state.mu.Unlock()
	fmt.Fprintf(os.Stdout, format, args...)
	state.lineVisible = false
	state.active = false
}

func runSpinnerProgress(spinFormat, successLine, failLine string, fn func() error) error {
	done := make(chan error, 1)
	go func() {
		done <- fn()
	}()

	frames := []rune{'|', '/', '-', '\\'}
	ticker := time.NewTicker(120 * time.Millisecond)
	defer ticker.Stop()

	frame := 0
	spinnerTerminalBegin()
	for {
		select {
		case err := <-done:
			if err != nil {
				spinnerTerminalEndLine("\r%s\n", failLine)
				return err
			}
			spinnerTerminalEndLine("\r%s\n", successLine)
			return nil
		case <-ticker.C:
			spinnerTerminalFrame("\r"+spinFormat, frames[frame%len(frames)])
			frame++
		}
	}
}

func StartBackupLockSpinner(instance string) func() {
	label := fmt.Sprintf("Waiting for backup lock: %s", instance)
	if !startProgressSpinnerEnabled() {
		core.Infof("%s\n", label)
		return func() {}
	}
	done := make(chan struct{})
	frames := []rune{'|', '/', '-', '\\'}
	spinnerTerminalBegin()
	go func() {
		frame := 0
		ticker := time.NewTicker(120 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				spinnerTerminalFrame("\r%c %s", frames[frame%len(frames)], label)
				frame++
			}
		}
	}()
	return func() {
		close(done)
		spinnerTerminalEndLine("\r  %s ... done\n", label)
	}
}

// startPhase tracks a service through two-phase startup progression.
type startPhase int

const (
	phaseStarting    startPhase = iota // polling for "has successfully started!"
	phaseWorking                       // started; polling for "advertising for join"
	phaseAdvertising                   // complete
	phaseFailed                        // error
)

// startedColWidth is the fixed width of the STARTED column, sized for "Starting... /".
const startedColWidth = 13

type phaseUpdate struct {
	idx   int
	phase startPhase
	err   error
}

func launchTwoPhaseWorker(ctx context.Context, idx int, task TwoPhaseStartTask, ch chan<- phaseUpdate) {
	if err := task.WaitStarted(ctx); err != nil {
		ch <- phaseUpdate{idx, phaseFailed, err}
		return
	}
	ch <- phaseUpdate{idx, phaseWorking, nil}
	if err := task.WaitReady(ctx); err != nil {
		ch <- phaseUpdate{idx, phaseFailed, err}
		return
	}
	ch <- phaseUpdate{idx, phaseAdvertising, nil}
}

// applyPhaseUpdate records upd into phases and increments terminals for terminal states.
// Returns firstErr (updated on failure), and calls cancel on first real failure.
func applyPhaseUpdate(upd phaseUpdate, phases []startPhase, terminals *int, firstErr *error, cancel func()) {
	phases[upd.idx] = upd.phase
	switch upd.phase {
	case phaseAdvertising:
		*terminals++
	case phaseFailed:
		*terminals++
		if !errors.Is(upd.err, context.Canceled) && *firstErr == nil {
			*firstErr = upd.err
			cancel()
		}
	}
}

func phaseToTableCells(phase startPhase, spinChar rune) (started, advertising string) {
	switch phase {
	case phaseStarting:
		return fmt.Sprintf("Starting... %c", spinChar), "-"
	case phaseWorking:
		return "Started", fmt.Sprintf("Working... %c", spinChar)
	case phaseAdvertising:
		return "Started", "Advertising"
	case phaseFailed:
		return "Failed", "-"
	default:
		return "-", "-"
	}
}

func renderTwoPhaseLines(names []string, phases []startPhase, frames []rune, frame int, firstPrint *bool, nameWidth int) {
	state := &globalSpinnerTerminalState
	state.mu.Lock()
	defer state.mu.Unlock()

	spinChar := frames[frame%len(frames)]
	totalLines := len(names) + 1 // header + one row per service

	if !*firstPrint {
		fmt.Fprintf(os.Stdout, "\033[%dA", totalLines)
	}
	*firstPrint = false

	fmt.Fprintf(os.Stdout, "\r\033[K%-*s  %-*s  %s\n", nameWidth, "INSTANCE", startedColWidth, "STARTED", "ADVERTISING")
	for i, name := range names {
		startedCell, advCell := phaseToTableCells(phases[i], spinChar)
		fmt.Fprintf(os.Stdout, "\r\033[K%-*s  %-*s  %s\n", nameWidth, name, startedColWidth, startedCell, advCell)
	}
	state.lineVisible = false
}

func runTwoPhaseNoSpinner(tasks []TwoPhaseStartTask) error {
	ctx, cancel := context.WithCancel(core.CurrentAppDeps().Context())
	defer cancel()

	ch := make(chan phaseUpdate, len(tasks)*2)
	for i, t := range tasks {
		core.Infof("Starting ark server: %s\n", t.Service)
		go launchTwoPhaseWorker(ctx, i, t, ch)
	}

	phases := make([]startPhase, len(tasks))
	var firstErr error
	done := 0
	for done < len(tasks) {
		upd := <-ch
		applyPhaseUpdate(upd, phases, &done, &firstErr, cancel)
		switch upd.phase {
		case phaseWorking:
			core.Infof("Starting ark server: %s started\n", tasks[upd.idx].Service)
		case phaseAdvertising:
			core.Infof("Starting ark server: %s advertising\n", tasks[upd.idx].Service)
		case phaseFailed:
			if !errors.Is(upd.err, context.Canceled) {
				core.Infof("Starting ark server: %s failed\n", tasks[upd.idx].Service)
			}
		}
	}
	return firstErr
}

func runTwoPhaseSpinner(tasks []TwoPhaseStartTask) error {
	n := len(tasks)
	ctx, cancel := context.WithCancel(core.CurrentAppDeps().Context())
	defer cancel()

	ch := make(chan phaseUpdate, n*2)
	phases := make([]startPhase, n)
	names := make([]string, n)

	nameWidth := len("INSTANCE")
	for i, t := range tasks {
		names[i] = t.Service
		if len(t.Service) > nameWidth {
			nameWidth = len(t.Service)
		}
	}

	for i, t := range tasks {
		go launchTwoPhaseWorker(ctx, i, t, ch)
	}

	frames := []rune{'|', '/', '-', '\\'}
	ticker := time.NewTicker(120 * time.Millisecond)
	defer ticker.Stop()

	frame := 0
	terminals := 0
	firstPrint := true
	var firstErr error

	spinnerTerminalBegin()
	for terminals < n {
		select {
		case upd := <-ch:
			applyPhaseUpdate(upd, phases, &terminals, &firstErr, cancel)
			renderTwoPhaseLines(names, phases, frames, frame, &firstPrint, nameWidth)
		case <-ticker.C:
			frame++
			renderTwoPhaseLines(names, phases, frames, frame, &firstPrint, nameWidth)
		}
	}

	// Final stable render (all phases are terminal, no spinner char matters).
	renderTwoPhaseLines(names, phases, frames, frame, &firstPrint, nameWidth)

	state := &globalSpinnerTerminalState
	state.mu.Lock()
	state.active = false
	state.lineVisible = false
	state.mu.Unlock()

	return firstErr
}

func runStopServiceWithProgress(service string, fn func() error) error {
	if !startProgressSpinnerEnabled() {
		core.Infof("Stopping ark server: %s\n", service)
		if err := fn(); err != nil {
			return err
		}
		core.Infof("Stopping ark server: %s done\n", service)
		return nil
	}

	return runSpinnerProgress(
		fmt.Sprintf("Stopping ark server: %%c %s", service),
		fmt.Sprintf("Stopping ark server: %s ... done ", service),
		fmt.Sprintf("Stopping ark server: %s ... failed", service),
		fn,
	)
}
