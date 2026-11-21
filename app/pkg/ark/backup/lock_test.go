package backup

import (
	"errors"
	"path/filepath"
	"testing"
	"time"
)

func TestAcquireInstanceLockHeldReturnsErrLockHeld(t *testing.T) {
	dir := t.TempDir()
	release, err := AcquireInstanceLock(dir, "asa1", 0)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	if held, err := InstanceLockHeld(dir, "asa1"); err != nil || !held {
		t.Fatalf("InstanceLockHeld = %v, %v", held, err)
	}
	_, err = AcquireInstanceLock(dir, "asa1", 0)
	if !errors.Is(err, ErrLockHeld) {
		t.Fatalf("err = %v, want ErrLockHeld", err)
	}
	want := "Error: backup lock is held for asa1: " + filepath.Join(dir, ".asa1-backup.lock")
	if err.Error() != want {
		t.Fatalf("err text = %q, want %q", err.Error(), want)
	}
}

func TestAcquireInstanceLockWaitsForRelease(t *testing.T) {
	dir := t.TempDir()
	release, err := AcquireInstanceLock(dir, "asa1", 0)
	if err != nil {
		t.Fatal(err)
	}
	time.AfterFunc(100*time.Millisecond, release)
	release2, err := AcquireInstanceLock(dir, "asa1", 5*time.Second)
	if err != nil {
		t.Fatalf("acquire after holder released: %v", err)
	}
	release2()
	if held, err := InstanceLockHeld(dir, "asa1"); err != nil || held {
		t.Fatalf("after release: held=%v err=%v", held, err)
	}
}
