package lifecycle

import "time"

const (
	// DefaultStartWaitTimeout is the default maximum time to wait for services
	// to become ready after start.
	DefaultStartWaitTimeout = 5 * time.Minute

	// DefaultStartWaitInterval is the default polling interval for readiness
	// checks during start.
	DefaultStartWaitInterval = 2 * time.Second

	// DefaultStopTimeout is the default graceful shutdown timeout.
	DefaultStopTimeout = 15 * time.Minute
)

// StartRequest describes a start operation.
type StartRequest struct {
	ComposeFile  string
	Services     []string
	PullPolicy   string        // "always", "missing", or "never"
	WaitForReady bool          // if true, poll until services report ready
	WaitTimeout  time.Duration // max wait time (0 = DefaultStartWaitTimeout)
}

// StartResult reports what happened during a start operation.
type StartResult struct {
	Started []string // services that were started
}

// StopRequest describes a stop operation.
type StopRequest struct {
	ComposeFile string
	Services    []string
	Timeout     time.Duration // graceful shutdown timeout (0 = default)
	Kill        bool          // if true, send SIGKILL immediately
}

// DownRequest describes a down operation.
type DownRequest struct {
	ComposeFile string
	Service     string        // if empty, down the entire project
	Timeout     time.Duration // graceful shutdown timeout for running services
	Kill        bool          // if true, kill before removing
}
