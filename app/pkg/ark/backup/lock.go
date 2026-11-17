package backup

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// WithInstanceLock acquires an exclusive file lock for backup operations on the
// given instance. The lock is released (file removed) when fn returns.
// If a stale lock file is found (the locking process no longer exists),
// it is removed automatically and the lock is reattempted.
func WithInstanceLock(lockDir, instance string, timeout time.Duration, fn func() error) error {
	if err := os.MkdirAll(lockDir, 0o755); err != nil {
		return err
	}
	lockPath := filepath.Join(lockDir, fmt.Sprintf(".%s-backup.lock", instance))
	deadline := time.Now().Add(timeout)
	for {
		handle, err := os.OpenFile(lockPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if err == nil {
			_, _ = fmt.Fprintf(handle, "pid=%d started=%s\n", os.Getpid(), time.Now().UTC().Format(time.RFC3339))
			_ = handle.Close()
			defer os.Remove(lockPath)
			return fn()
		}
		if !os.IsExist(err) {
			return err
		}
		// Lock file exists — check if the holder process is dead.
		if isStaleLock(lockPath) {
			_ = os.Remove(lockPath)
			continue
		}
		if timeout == 0 || time.Now().After(deadline) {
			return fmt.Errorf("Error: backup lock is held for %s: %s", instance, lockPath)
		}
		time.Sleep(2 * time.Second)
	}
}

// isStaleLock returns true if the lock file exists but the process that created
// it is no longer running. Returns false if the lock file can't be read or the
// PID can't be parsed, to avoid removing a potentially valid lock.
func isStaleLock(lockPath string) bool {
	data, err := os.ReadFile(lockPath)
	if err != nil {
		return false
	}
	pid := parsePID(string(data))
	if pid <= 0 {
		return false
	}
	return syscall.Kill(pid, 0) == syscall.ESRCH
}

// parsePID extracts the PID from a lock file line like "pid=12345 started=...".
func parsePID(content string) int {
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "pid=") {
			pidStr := strings.TrimPrefix(line, "pid=")
			if idx := strings.IndexByte(pidStr, ' '); idx >= 0 {
				pidStr = pidStr[:idx]
			}
			pid, err := strconv.Atoi(pidStr)
			if err == nil {
				return pid
			}
		}
	}
	return 0
}
