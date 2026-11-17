// Package cmdrunner provides a generic command execution interface and default
// implementation using os/exec. Used by the backup service for non-container
// commands like tar and sqlite3.
package cmdrunner

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"strings"
)

// RunOptions controls how a command is executed.
type RunOptions struct {
	// Capture stdout/stderr into RunResult instead of streaming to os.Stdout/os.Stderr.
	Capture bool
}

// RunResult captures the output and exit code of an executed command.
type RunResult struct {
	Stdout   string
	Stderr   string
	ExitCode int
}

// Runner abstracts command execution so that tests can inject a mock
// without spawning real processes.
type Runner interface {
	// Run executes a command and returns its result. Exit errors are captured
	// in RunResult.ExitCode (not returned as error); only execution failures
	// (e.g. binary not found) are returned as error.
	Run(ctx context.Context, cmd []string, opts RunOptions) (RunResult, error)

	// RunInteractive executes a command with stdin/stdout/stderr attached to
	// the terminal. Returns an error if the command exits non-zero.
	RunInteractive(ctx context.Context, cmd []string) error
}

// DefaultRunner implements Runner via exec.CommandContext.
type DefaultRunner struct {
	logger *slog.Logger
}

// NewDefaultRunner creates a Runner that executes real processes.
func NewDefaultRunner(logger *slog.Logger) *DefaultRunner {
	return &DefaultRunner{logger: logger}
}

func (r *DefaultRunner) Run(ctx context.Context, cmd []string, opts RunOptions) (RunResult, error) {
	if len(cmd) == 0 {
		return RunResult{}, errors.New("command is required")
	}
	r.logger.Debug("exec", "cmd", strings.Join(cmd, " "))

	command := exec.CommandContext(ctx, cmd[0], cmd[1:]...)
	var stdout, stderr bytes.Buffer
	if opts.Capture {
		command.Stdout = &stdout
		command.Stderr = &stderr
	} else {
		command.Stdout = os.Stdout
		command.Stderr = os.Stderr
	}

	err := command.Run()
	result := RunResult{
		Stdout:   stdout.String(),
		Stderr:   stderr.String(),
		ExitCode: exitCode(err),
	}
	if result.ExitCode != 0 {
		r.logger.Debug("exit", "code", result.ExitCode)
	}
	if err != nil {
		if isExitError(err) {
			return result, nil
		}
		return result, err
	}
	return result, nil
}

func (r *DefaultRunner) RunInteractive(ctx context.Context, cmd []string) error {
	if len(cmd) == 0 {
		return errors.New("command is required")
	}
	r.logger.Debug("exec-interactive", "cmd", strings.Join(cmd, " "))

	command := exec.CommandContext(ctx, cmd[0], cmd[1:]...)
	command.Stdin = os.Stdin
	command.Stdout = os.Stdout
	command.Stderr = os.Stderr

	if err := command.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return fmt.Errorf("command exited with code %d: %s", exitErr.ExitCode(), strings.Join(cmd, " "))
		}
		return err
	}
	return nil
}

func exitCode(err error) int {
	if err == nil {
		return 0
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode()
	}
	return 1
}

func isExitError(err error) bool {
	var exitErr *exec.ExitError
	return errors.As(err, &exitErr)
}
