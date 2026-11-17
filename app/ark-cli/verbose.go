package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"ark-cli/internal/core"
)

type verboseRuntimeState struct {
	enabled bool
	out     io.Writer
	file    *os.File
}

func newVerboseRuntimeState() *verboseRuntimeState {
	return &verboseRuntimeState{out: os.Stderr}
}

var globalVerboseState = newVerboseRuntimeState()

func currentVerboseState() *verboseRuntimeState {
	return globalVerboseState
}

func configureVerbose(opts globalOptions) error {
	return currentVerboseState().configure(opts)
}

func (s *verboseRuntimeState) configure(opts globalOptions) error {
	s.enabled = opts.verbose
	s.out = os.Stderr
	if !opts.verbose {
		return nil
	}
	if opts.logFile == "" {
		return nil
	}
	absPath, err := filepath.Abs(opts.logFile)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(absPath), 0o755); err != nil {
		return err
	}
	file, err := os.OpenFile(absPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	s.file = file
	s.out = io.MultiWriter(os.Stderr, file)
	return nil
}

func closeVerbose() {
	currentVerboseState().close()
}

func (s *verboseRuntimeState) close() {
	if s.file == nil {
		return
	}
	_ = s.file.Close()
	s.file = nil
}

func verboseLogf(format string, args ...any) {
	currentVerboseState().logf(format, args...)
}

func (s *verboseRuntimeState) logf(format string, args ...any) {
	if !s.enabled {
		return
	}
	fmt.Fprintf(s.out, "[%s] ", time.Now().Format("15:04:05.000"))
	fmt.Fprintf(s.out, format, args...)
}

func mirrorCLIToVerboseFile(tag, message string) {
	currentVerboseState().mirrorCLI(tag, message)
}

func (s *verboseRuntimeState) mirrorCLI(tag, message string) {
	if !s.enabled || s.file == nil || message == "" {
		return
	}
	if strings.HasSuffix(message, "\n") {
		fmt.Fprintf(s.file, "[%s] %s", tag, message)
		return
	}
	fmt.Fprintf(s.file, "[%s] %s\n", tag, message)
}

func infof(format string, args ...any) {
	message := fmt.Sprintf(format, args...)
	core.SpinnerAwareStdoutWrite(message)
	mirrorCLIToVerboseFile("cli-stdout", message)
}

func infoln(args ...any) {
	message := fmt.Sprintln(args...)
	core.SpinnerAwareStdoutWrite(message)
	mirrorCLIToVerboseFile("cli-stdout", message)
}

func warnf(format string, args ...any) {
	message := fmt.Sprintf(format, args...)
	core.SpinnerAwareStderrWrite(message)
	mirrorCLIToVerboseFile("cli-stderr", message)
}

func warnln(args ...any) {
	message := fmt.Sprintln(args...)
	core.SpinnerAwareStderrWrite(message)
	mirrorCLIToVerboseFile("cli-stderr", message)
}

func mirrorStderrf(format string, args ...any) {
	warnf(format, args...)
}
