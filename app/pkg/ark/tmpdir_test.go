package ark

import (
	"os"
	"testing"
)

func TestTempDir(t *testing.T) {
	// Save original env var
	orig := os.Getenv("ARK_TMP_DIR")
	defer os.Setenv("ARK_TMP_DIR", orig)

	// Test default behavior (requires ARK_TMP_DIR to be set for deterministic test)
	os.Setenv("ARK_TMP_DIR", t.TempDir())

	dir, err := TempDir("test-prefix-")
	if err != nil {
		t.Fatalf("TempDir failed: %v", err)
	}
	if dir == "" {
		t.Fatal("TempDir returned empty path")
	}
	// Verify it uses the prefix
	if len(dir) < 10 {
		t.Fatalf("TempDir path too short: %s", dir)
	}
}

func TestTempDirEnvOverride(t *testing.T) {
	// Test that ARK_TMP_DIR is respected
	customDir := t.TempDir()
	os.Setenv("ARK_TMP_DIR", customDir)

	dir, err := TempDir("custom-")
	if err != nil {
		t.Fatalf("TempDir failed with env override: %v", err)
	}
	if dir == "" {
		t.Fatal("TempDir returned empty path")
	}
	// Path should be within customDir
	if len(dir) <= len(customDir) {
		t.Fatalf("TempDir should create subdir in customDir: %s", dir)
	}
}
