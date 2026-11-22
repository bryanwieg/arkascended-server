package filelock

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestTryLockExcludesSecondDescriptorUntilRelease(t *testing.T) {
	path := filepath.Join(t.TempDir(), "x.lock")
	release, held, err := TryLock(path)
	if err != nil || held {
		t.Fatalf("first TryLock: held=%v err=%v", held, err)
	}
	data, _ := os.ReadFile(path)
	if !strings.HasPrefix(string(data), "pid=") {
		t.Fatalf("lock file content = %q", data)
	}
	if _, held, err := TryLock(path); err != nil || !held {
		t.Fatalf("second TryLock while held: held=%v err=%v", held, err)
	}
	release()
	release2, held, err := TryLock(path)
	if err != nil || held {
		t.Fatalf("TryLock after release: held=%v err=%v", held, err)
	}
	release2()
}

func TestHeld(t *testing.T) {
	path := filepath.Join(t.TempDir(), "x.lock")
	if held, err := Held(path); err != nil || held {
		t.Fatalf("missing file: held=%v err=%v", held, err)
	}
	release, _, err := TryLock(path)
	if err != nil {
		t.Fatal(err)
	}
	if held, err := Held(path); err != nil || !held {
		t.Fatalf("while held: held=%v err=%v", held, err)
	}
	release()
	if held, err := Held(path); err != nil || held {
		t.Fatalf("after release: held=%v err=%v", held, err)
	}
}

func TestLeftoverFileDoesNotBlock(t *testing.T) {
	path := filepath.Join(t.TempDir(), "x.lock")
	if err := os.WriteFile(path, []byte("pid=1 started=2026-01-01T00:00:00Z\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if held, err := Held(path); err != nil || held {
		t.Fatalf("Held on leftover file: held=%v err=%v", held, err)
	}
	release, held, err := TryLock(path)
	if err != nil || held {
		t.Fatalf("TryLock on leftover file: held=%v err=%v", held, err)
	}
	release()
}

func TestHeldNeverBlocksConcurrentTryLock(t *testing.T) {
	path := filepath.Join(t.TempDir(), "x.lock")
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for range 1000 {
			if _, err := Held(path); err != nil {
				t.Errorf("Held: %v", err)
				return
			}
		}
	}()
	go func() {
		defer wg.Done()
		for range 1000 {
			release, held, err := TryLock(path)
			if err != nil || held {
				t.Errorf("TryLock during Held probes: held=%v err=%v", held, err)
				return
			}
			release()
		}
	}()
	wg.Wait()
}
