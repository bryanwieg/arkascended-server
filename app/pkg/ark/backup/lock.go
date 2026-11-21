package backup

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"pkg/ark/filelock"
)

// ErrLockHeld marks an instance lock that another holder kept for the whole timeout.
var ErrLockHeld = errors.New("lock held")

// lockHeldError keeps the user-facing text while matching ErrLockHeld.
type lockHeldError struct{ msg string }

func (e lockHeldError) Error() string { return e.msg }
func (lockHeldError) Unwrap() error   { return ErrLockHeld }

func instanceLockPath(lockDir, instance string) string {
	return filepath.Join(lockDir, fmt.Sprintf(".%s-backup.lock", instance))
}

// AcquireInstanceLock takes the kernel lock for instance, retrying every 2 s
// until timeout (0 = one try). Still held → an error wrapping ErrLockHeld.
func AcquireInstanceLock(lockDir, instance string, timeout time.Duration) (func(), error) {
	if err := os.MkdirAll(lockDir, 0o755); err != nil {
		return nil, err
	}
	lockPath := instanceLockPath(lockDir, instance)
	deadline := time.Now().Add(timeout)
	for {
		release, held, err := filelock.TryLock(lockPath)
		if err != nil {
			return nil, err
		}
		if !held {
			return release, nil
		}
		if timeout == 0 || time.Now().After(deadline) {
			return nil, lockHeldError{fmt.Sprintf("Error: backup lock is held for %s: %s", instance, lockPath)}
		}
		time.Sleep(2 * time.Second)
	}
}

// WithInstanceLock runs fn while holding the instance lock.
func WithInstanceLock(lockDir, instance string, timeout time.Duration, fn func() error) error {
	release, err := AcquireInstanceLock(lockDir, instance, timeout)
	if err != nil {
		return err
	}
	defer release()
	return fn()
}

// InstanceLockHeld reports whether anyone holds the instance lock, without taking it.
func InstanceLockHeld(lockDir, instance string) (bool, error) {
	return filelock.Held(instanceLockPath(lockDir, instance))
}
