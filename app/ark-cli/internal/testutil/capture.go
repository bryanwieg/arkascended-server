package testutil

import (
	"bytes"
	"io"
	"os"
	"testing"
)

// SetEnv sets an environment variable and restores it on test cleanup.
func SetEnv(t *testing.T, key, value string) {
	old, ok := os.LookupEnv(key)
	if err := os.Setenv(key, value); err != nil {
		t.Fatalf("set env: %v", err)
	}
	t.Cleanup(func() {
		if ok {
			_ = os.Setenv(key, old)
		} else {
			_ = os.Unsetenv(key)
		}
	})
}

// CaptureStdout captures stdout during fn and returns the trimmed output.
func CaptureStdout(t *testing.T, fn func()) string {
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stdout = w
	defer func() {
		os.Stdout = old
	}()
	defer func() {
		if recovered := recover(); recovered != nil {
			_ = w.Close()
			panic(recovered)
		}
	}()

	fn()
	_ = w.Close()

	data, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("read stdout: %v", err)
	}
	return string(bytes.TrimSpace(data))
}

// CaptureStderr captures stderr during fn and returns the trimmed output.
func CaptureStderr(t *testing.T, fn func()) string {
	old := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stderr = w
	defer func() {
		os.Stderr = old
	}()
	defer func() {
		if recovered := recover(); recovered != nil {
			_ = w.Close()
			panic(recovered)
		}
	}()

	fn()
	_ = w.Close()

	data, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("read stderr: %v", err)
	}
	return string(bytes.TrimSpace(data))
}
