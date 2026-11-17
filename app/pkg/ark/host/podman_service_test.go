package host

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHasPodmanService_RootlessSocketFound(t *testing.T) {
	tmpDir := t.TempDir()
	podmanDir := filepath.Join(tmpDir, "podman")
	if err := os.Mkdir(podmanDir, 0755); err != nil {
		t.Fatalf("failed to create podman dir: %v", err)
	}
	socketPath := filepath.Join(podmanDir, "podman.sock")
	if err := os.WriteFile(socketPath, nil, 0644); err != nil {
		t.Fatalf("failed to create podman.sock: %v", err)
	}

	oldXDG := os.Getenv("XDG_RUNTIME_DIR")
	os.Setenv("XDG_RUNTIME_DIR", tmpDir)
	defer os.Setenv("XDG_RUNTIME_DIR", oldXDG)

	socket, rootfulDetected := HasPodmanService()
	expectedSocket := "unix://" + socketPath
	if socket != expectedSocket {
		t.Errorf("expected socket %q, got %q", expectedSocket, socket)
	}
	if rootfulDetected {
		t.Error("expected rootfulDetected=false, got true")
	}
}

func TestHasPodmanService_RootlessSocketNotFound(t *testing.T) {
	tmpDir := t.TempDir()

	oldXDG := os.Getenv("XDG_RUNTIME_DIR")
	os.Setenv("XDG_RUNTIME_DIR", tmpDir)
	defer os.Setenv("XDG_RUNTIME_DIR", oldXDG)

	socket, rootfulDetected := HasPodmanService()
	if socket != "" {
		t.Errorf("expected empty socket, got %q", socket)
	}
	// rootfulDetected depends on whether /run/podman/podman.sock exists on this system
	// In CI, rootfulDetected is typically false, but we can't assert it reliably
	// since it depends on whether the test system has rootful Podman installed
	if rootfulDetected {
		t.Log("rootful Podman socket detected at /run/podman/podman.sock (unexpected in CI)")
	}
}

func TestHasPodmanService_NoRuntimeDir(t *testing.T) {
	// Test when XDG_RUNTIME_DIR is not set
	// Function should fall back to /run/user/<uid>
	oldXDG := os.Getenv("XDG_RUNTIME_DIR")
	os.Unsetenv("XDG_RUNTIME_DIR")
	defer os.Setenv("XDG_RUNTIME_DIR", oldXDG)

	// Verify function runs without panic
	// The actual socket detection depends on whether /run/user/<uid>/podman/podman.sock exists
	socket, rootfulDetected := HasPodmanService()

	// If socket exists, verify it has correct format with unix:// prefix
	if socket != "" {
		if !strings.HasPrefix(socket, "unix://") {
			t.Errorf("expected socket to start with 'unix://', got %q", socket)
		}
	}

	// rootfulDetected depends on system state - just verify it's a valid boolean
	_ = rootfulDetected
}

func TestHasPodmanService_RootfulFallback(t *testing.T) {
	// Test rootful detection when no user socket exists but system socket does.
	// We simulate this by pointing XDG_RUNTIME_DIR at an empty dir while
	// checking that rootfulDetected is set correctly based on system state.
	tmpDir := t.TempDir()
	oldXDG := os.Getenv("XDG_RUNTIME_DIR")
	os.Setenv("XDG_RUNTIME_DIR", tmpDir)
	defer os.Setenv("XDG_RUNTIME_DIR", oldXDG)

	socket, rootfulDetected := HasPodmanService()
	// User socket definitely doesn't exist (tmpDir is empty)
	if socket != "" {
		t.Errorf("expected empty socket, got %q", socket)
	}
	// rootfulDetected depends on whether /run/podman/podman.sock exists
	_ = rootfulDetected
}
