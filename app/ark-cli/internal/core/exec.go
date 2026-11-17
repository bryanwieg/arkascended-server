package core

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"pkg/ark"
)

// LiveCommandOutputWriters returns stdout and stderr writers that tee to the
// verbose log when verbosity is enabled.
func LiveCommandOutputWriters() (io.Writer, io.Writer) {
	if VerboseEnabled() {
		if w, ok := VerboseWriter().(io.Writer); ok && w != nil {
			return io.MultiWriter(os.Stdout, w), io.MultiWriter(os.Stderr, w)
		}
	}
	return os.Stdout, os.Stderr
}

// LogCapturedOutput writes a labeled block of captured command output to the
// verbose log, skipping empty content.
func LogCapturedOutput(label, output string) {
	if strings.TrimSpace(output) == "" {
		return
	}
	VerboseLogf("[%s]\n%s\n", label, output)
}

// RunCommandImpl executes cmd via os/exec and returns the result. When
// opts.Capture is set, stdout/stderr are captured; otherwise they stream to
// the terminal (teed to the verbose log when appropriate).
func RunCommandImpl(cmd []string, opts ark.CommandOptions) (ark.ExecResult, error) {
	if len(cmd) == 0 {
		return ark.ExecResult{}, errors.New("Error: command is required.")
	}
	VerboseLogf("[cmd] %s\n", strings.Join(cmd, " "))
	command := exec.Command(cmd[0], cmd[1:]...)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if opts.Capture {
		command.Stdout = &stdout
		command.Stderr = &stderr
	} else {
		command.Stdout, command.Stderr = LiveCommandOutputWriters()
	}

	err := command.Run()
	result := ark.ExecResult{
		Stdout: stdout.String(),
		Stderr: stderr.String(),
		ExitCode: func() int {
			if err == nil {
				return 0
			}
			if exitErr, ok := err.(*exec.ExitError); ok {
				return exitErr.ExitCode()
			}
			return 1
		}(),
	}
	if opts.Capture {
		LogCapturedOutput("stdout", result.Stdout)
		LogCapturedOutput("stderr", result.Stderr)
	}
	if result.ExitCode != 0 {
		VerboseLogf("[exit] %d\n", result.ExitCode)
	}
	if err != nil {
		if _, ok := err.(*exec.ExitError); ok {
			return result, nil
		}
		return result, err
	}
	return result, nil
}

// LookPathImpl searches for an executable named file using the host's PATH.
func LookPathImpl(file string) (string, error) {
	return exec.LookPath(file)
}

// RunCommandExpectZero runs a command via the deps runtime and returns an
// error if the exit code is non-zero. If failErr is non-nil it is returned
// instead of the generic "command failed" error.
func RunCommandExpectZero(d *AppDeps, cmd []string, opts ark.CommandOptions, failErr error) error {
	result, err := d.Runtime.RunCommand(cmd, opts)
	if err != nil {
		return err
	}
	if result.ExitCode != 0 {
		if failErr != nil {
			return failErr
		}
		return fmt.Errorf("Error: command failed: %s", strings.Join(cmd, " "))
	}
	return nil
}
