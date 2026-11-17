package runtime

import (
	"bytes"
	"errors"
	"log/slog"
	"os"
	"os/exec"
	"strings"

	"pkg/ark"
)

// RunCommandImpl executes a command and returns the result.
// It uses the provided logger for debug output; pass nil to suppress logging.
func RunCommandImpl(cmd []string, opts ark.CommandOptions, logger *slog.Logger) (ark.ExecResult, error) {
	if len(cmd) == 0 {
		return ark.ExecResult{}, errors.New("Error: command is required.")
	}
	if logger != nil {
		logger.Debug("cmd", "args", strings.Join(cmd, " "))
	}
	command := exec.Command(cmd[0], cmd[1:]...)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if opts.Capture {
		command.Stdout = &stdout
		command.Stderr = &stderr
	} else {
		command.Stdout = os.Stdout
		command.Stderr = os.Stderr
	}

	err := command.Run()
	result := ark.ExecResult{
		Stdout:   stdout.String(),
		Stderr:   stderr.String(),
		ExitCode: exitCodeFromErr(err),
	}
	if opts.Capture && logger != nil {
		if strings.TrimSpace(result.Stdout) != "" {
			logger.Debug("stdout", "output", result.Stdout)
		}
		if strings.TrimSpace(result.Stderr) != "" {
			logger.Debug("stderr", "output", result.Stderr)
		}
	}
	if result.ExitCode != 0 && logger != nil {
		logger.Debug("exit", "code", result.ExitCode)
	}
	if err != nil {
		if _, ok := err.(*exec.ExitError); ok {
			return result, nil
		}
		return result, err
	}
	return result, nil
}

// LookPathImpl searches for an executable in the system PATH.
func LookPathImpl(file string) (string, error) {
	return exec.LookPath(file)
}

// exitCodeFromErr extracts the exit code from an error, returning 0 for nil
// and 1 for non-exit errors.
func exitCodeFromErr(err error) int {
	if err == nil {
		return 0
	}
	if exitErr, ok := err.(*exec.ExitError); ok {
		return exitErr.ExitCode()
	}
	return 1
}
