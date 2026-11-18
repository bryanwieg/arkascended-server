package core

import (
	"fmt"
	"log/slog"
	"os"
)

// NewCLILogger creates a *slog.Logger for CLI use. Library packages under
// pkg/ark/ receive this logger via constructor injection and never reference
// os.Stderr directly. ark-web builds its own logger.
func NewCLILogger(verbose bool) *slog.Logger {
	level := slog.LevelInfo
	if verbose {
		level = slog.LevelDebug
	}
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level}))
}

// Infof prints an info message to stdout (spinner-aware in the CLI).
// Set during CLI initialization to the verbose-aware printer.
// Default: fmt.Printf (safe for tests and non-CLI contexts).
var Infof = func(format string, args ...any) { fmt.Printf(format, args...) }

// Infoln prints an info line to stdout (spinner-aware in the CLI).
// Set during CLI initialization to the verbose-aware printer.
var Infoln = func(args ...any) { fmt.Println(args...) }

// Warnf prints a warning message to stderr.
var Warnf = func(format string, args ...any) { fmt.Fprintf(os.Stderr, format, args...) }

// VerboseLogf prints a verbose/debug message.
var VerboseLogf = func(format string, args ...any) {}

// VerboseEnabled reports whether verbose mode is active.
var VerboseEnabled = func() bool { return false }

// VerboseWriter returns the io.Writer for verbose log output.
var VerboseWriter = func() any { return os.Stderr }

// SpinnerAwareStdoutWrite writes a message to stdout respecting spinner state.
var SpinnerAwareStdoutWrite = func(message string) { fmt.Print(message) }

// SpinnerAwareStderrWrite writes a message to stderr respecting spinner state.
var SpinnerAwareStderrWrite = func(message string) { fmt.Fprint(os.Stderr, message) }

// Promptf writes an interactive prompt to stderr. Prompts go on stderr so
// that stdout stays clean for machine consumers when the operator is
// reading a value out of a piped invocation. The message is written as-is
// (no trailing newline, no spinner coordination); callers include any
// trailing colon-space themselves.
var Promptf = func(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format, args...)
}
