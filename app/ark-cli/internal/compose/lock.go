package compose

import (
	"ark-cli/internal/core"
	"errors"
	"slices"
	"time"

	"pkg/ark/backup"
)

// lockWaitTimeout is how long a command waits for an instance lock held by another process (a backup,
// another stop/start, an update). A package var so tests can shorten it.
var lockWaitTimeout = 15 * time.Minute

// WithInstanceLocks holds the lock of every instance while fn runs once. Locks are taken in sorted
// order so two processes never deadlock; fn's errors are returned as they are and never retried.
func WithInstanceLocks(d *core.AppDeps, composeFile string, instances []string, fn func() error) error {
	backupRoot := core.BackupRootDirForComposeFile(composeFile)
	for _, inst := range slices.Sorted(slices.Values(instances)) {
		lockDir := backup.InstanceLockDir(backupRoot, inst)
		release, err := backup.AcquireInstanceLock(lockDir, inst, 0)
		if errors.Is(err, backup.ErrLockHeld) {
			stop := StartBackupLockSpinner(inst)
			release, err = backup.AcquireInstanceLock(lockDir, inst, lockWaitTimeout)
			stop()
		}
		if err != nil {
			return err
		}
		defer release()
	}
	return fn()
}

// withTargetLocks resolves the target instances and runs fn holding their instance locks.
func withTargetLocks(d *core.AppDeps, composeFileFlag, instance, emptyAllErr string, fn func() error) error {
	composeFile, _, services, err := ResolveDockerComposeServiceTargets(d, composeFileFlag, instance, emptyAllErr)
	if err != nil {
		return err
	}
	return WithInstanceLocks(d, composeFile, services, fn)
}
