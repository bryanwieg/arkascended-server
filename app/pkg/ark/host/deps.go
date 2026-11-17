package host

import (
	"io"
	"log/slog"

	"pkg/ark"
)

// HostDeps holds the injectable seams for host operations.
// All host functions that need side effects receive this (or a subset) explicitly.
type HostDeps struct {
	// Command execution
	RunCommand           func([]string, ark.CommandOptions) (ark.ExecResult, error)
	LookPath             func(string) (string, error)
	GetNofileHardLimit   func() (uint64, error)
	OpenUDPProbeListener func(int) (func(), error)

	// Kernel parameter reading
	ReadKernelParam func(path string) (int, error)

	// Network
	FetchText func(string) (string, error)

	// Logging
	Logger *slog.Logger

	// Output
	Stdout io.Writer
	Stderr io.Writer
}
