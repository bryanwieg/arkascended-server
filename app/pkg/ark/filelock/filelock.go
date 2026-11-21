// Package filelock wraps Linux open-file-description (OFD) locks. OFD locks are
// released by the kernel when the holder exits, work across PID namespaces on a
// shared mount, and can be tested with F_OFD_GETLK without taking them.
package filelock

import (
	"errors"
	"os"
	"strconv"
	"time"

	"golang.org/x/sys/unix"
)

// TryLock opens path (created 0o644, never removed) and takes a whole-file
// write lock without waiting. held is true when another open file description
// holds it; then nothing stays open. On success the file holds a "pid=…
// started=…" line for humans, and release unlocks and closes it.
func TryLock(path string) (release func(), held bool, err error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, false, err
	}
	lock := unix.Flock_t{Type: unix.F_WRLCK}
	if err := unix.FcntlFlock(f.Fd(), unix.F_OFD_SETLK, &lock); err != nil {
		_ = f.Close()
		if errors.Is(err, unix.EAGAIN) || errors.Is(err, unix.EACCES) {
			return nil, true, nil
		}
		return nil, false, err
	}
	release = func() {
		unlock := unix.Flock_t{Type: unix.F_UNLCK}
		_ = unix.FcntlFlock(f.Fd(), unix.F_OFD_SETLK, &unlock)
		_ = f.Close()
	}
	line := "pid=" + strconv.Itoa(os.Getpid()) + " started=" + time.Now().UTC().Format(time.RFC3339) + "\n"
	if err := f.Truncate(0); err != nil {
		release()
		return nil, false, err
	}
	if _, err := f.WriteString(line); err != nil {
		release()
		return nil, false, err
	}
	return release, false, nil
}

// Held reports whether another open file description holds a lock on path,
// without taking it. A missing file means not held.
func Held(path string) (bool, error) {
	f, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	defer f.Close()
	// Pid must be 0 for OFD commands.
	probe := unix.Flock_t{Type: unix.F_WRLCK}
	if err := unix.FcntlFlock(f.Fd(), unix.F_OFD_GETLK, &probe); err != nil {
		return false, err
	}
	return probe.Type != unix.F_UNLCK, nil
}
