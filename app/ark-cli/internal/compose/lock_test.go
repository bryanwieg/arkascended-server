package compose

// Tests: WithInstanceLocks: held during fn and released after, waits for another holder,
// times out, sorted lock order, fn never retried.

import (
	"ark-cli/internal/core"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"pkg/ark/backup"
)

func setLockWaitTimeout(t *testing.T, d time.Duration) {
	t.Helper()
	orig := lockWaitTimeout
	lockWaitTimeout = d
	t.Cleanup(func() { lockWaitTimeout = orig })
}

// lockFixture points the backup root at a temp dir and returns a probe and a holder for instance locks.
func lockFixture(t *testing.T) (held func(inst string) bool, hold func(inst string) func()) {
	t.Helper()
	root := t.TempDir()
	t.Setenv("ARK_BACKUP_DIR", root)
	held = func(inst string) bool {
		h, err := backup.InstanceLockHeld(backup.InstanceLockDir(root, inst), inst)
		if err != nil {
			t.Fatal(err)
		}
		return h
	}
	hold = func(inst string) func() {
		release, err := backup.AcquireInstanceLock(backup.InstanceLockDir(root, inst), inst, 0)
		if err != nil {
			t.Fatal(err)
		}
		return release
	}
	return held, hold
}

func TestWithInstanceLocksHoldsDuringFnAndReleasesAfter(t *testing.T) {
	held, _ := lockFixture(t)
	for _, fnErr := range []error{nil, errors.New("boom")} {
		var heldInFn []bool
		err := WithInstanceLocks(core.CurrentAppDeps(), "compose.yaml", []string{"asa1", "asa2"}, func() error {
			heldInFn = []bool{held("asa1"), held("asa2")}
			return fnErr
		})
		if !errors.Is(err, fnErr) {
			t.Fatalf("err = %v, want %v", err, fnErr)
		}
		if fmt.Sprint(heldInFn) != "[true true]" {
			t.Fatalf("locks held in fn: %v", heldInFn)
		}
		if held("asa1") || held("asa2") {
			t.Fatalf("locks still held after fn (fn error %v)", fnErr)
		}
	}
}

func TestWithInstanceLocksWaitsForHolder(t *testing.T) {
	setLockWaitTimeout(t, 10*time.Second)
	_, hold := lockFixture(t)
	release := hold("asa1")
	time.AfterFunc(100*time.Millisecond, release)
	calls := 0
	var err error
	out := captureStdout(t, func() {
		err = WithInstanceLocks(core.CurrentAppDeps(), "compose.yaml", []string{"asa1"}, func() error { calls++; return nil })
	})
	if err != nil || calls != 1 {
		t.Fatalf("err = %v, calls = %d", err, calls)
	}
	if n := strings.Count(out, "Waiting for backup lock: asa1"); n != 1 {
		t.Fatalf("waiting line printed %d times:\n%s", n, out)
	}
}

func TestWithInstanceLocksTimesOut(t *testing.T) {
	setLockWaitTimeout(t, 200*time.Millisecond)
	_, hold := lockFixture(t)
	defer hold("asa1")()
	called := false
	var err error
	captureStdout(t, func() {
		err = WithInstanceLocks(core.CurrentAppDeps(), "compose.yaml", []string{"asa1"}, func() error { called = true; return nil })
	})
	if !errors.Is(err, backup.ErrLockHeld) || !strings.Contains(err.Error(), "backup lock is held for asa1") {
		t.Fatalf("err = %v, want the lock-held error", err)
	}
	if called {
		t.Fatal("fn ran without the lock")
	}
}

// Sorted order: with b held elsewhere and input [b a], a is taken before waiting on b.
func TestWithInstanceLocksTakesSortedOrder(t *testing.T) {
	setLockWaitTimeout(t, 10*time.Second)
	held, hold := lockFixture(t)
	releaseB := hold("b")
	done := make(chan error, 1)
	go func() {
		done <- WithInstanceLocks(core.CurrentAppDeps(), "compose.yaml", []string{"b", "a"}, func() error { return nil })
	}()
	deadline := time.Now().Add(time.Second)
	for !held("a") {
		if time.Now().After(deadline) {
			releaseB()
			t.Fatal("a was not locked while waiting for b: locks are not taken in sorted order")
		}
		time.Sleep(10 * time.Millisecond)
	}
	releaseB()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

// An fn error that mentions a lock is the fn's own (a rollout's inner step): returned once, never retried.
func TestWithInstanceLocksNeverRetriesFn(t *testing.T) {
	lockFixture(t)
	calls := 0
	inner := fmt.Errorf("step failed: %w", backup.ErrLockHeld)
	err := WithInstanceLocks(core.CurrentAppDeps(), "compose.yaml", []string{"asa1"}, func() error { calls++; return inner })
	if !errors.Is(err, inner) || calls != 1 {
		t.Fatalf("err = %v, calls = %d; want the fn error once", err, calls)
	}
}
