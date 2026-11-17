package host

import (
	"os"
	"testing"
)

func TestHasDockerService_SocketFound(t *testing.T) {
	t.Setenv("DOCKER_HOST", "")
	// Check if Docker socket exists on the system
	dockerSocket := "/var/run/docker.sock"
	if _, err := os.Stat(dockerSocket); err != nil {
		if os.IsNotExist(err) {
			t.Skip("Docker socket not found (expected in CI or non-Docker environments)")
		}
		t.Fatalf("error checking Docker socket: %v", err)
	}

	socket, ok := HasDockerService()
	if !ok {
		t.Fatal("expected Docker socket to be found")
	}
	expectedSocket := "unix:///var/run/docker.sock"
	if socket != expectedSocket {
		t.Errorf("expected socket %q, got %q", expectedSocket, socket)
	}
}

func TestHasDockerService_SocketNotFound(t *testing.T) {
	t.Setenv("DOCKER_HOST", "")
	// Create a temp directory and temporarily override the socket path
	// by creating a mock environment where /var/run/docker.sock doesn't exist
	// We can't actually override the hardcoded path, so this test verifies
	// the function behavior when the socket doesn't exist
	// In CI, docker socket is unlikely present
	socket, ok := HasDockerService()
	if ok {
		t.Logf("Docker socket found at %s (unexpected in CI or non-Docker environments)", socket)
		return
	}
	if socket != "" {
		t.Errorf("expected empty socket when not found, got %q", socket)
	}
}

func TestHasDockerService_Functionality(t *testing.T) {
	t.Setenv("DOCKER_HOST", "")
	// This test verifies the function logic runs without panic
	// The actual socket detection depends on system state (/var/run/docker.sock)
	// In CI or non-Docker environments, the socket is unlikely to exist
	socket, ok := HasDockerService()

	// If socket exists, verify it has correct format
	if ok {
		expectedSocket := "unix:///var/run/docker.sock"
		if socket != expectedSocket {
			t.Errorf("expected socket %q, got %q", expectedSocket, socket)
		}
	} else {
		// If socket doesn't exist, verify empty return
		if socket != "" {
			t.Errorf("expected empty socket when not found, got %q", socket)
		}
	}
}

func TestHasDockerService_DockerHostWins(t *testing.T) {
	t.Setenv("DOCKER_HOST", "tcp://docker:2376")
	socket, ok := HasDockerService()
	if !ok || socket != "tcp://docker:2376" {
		t.Errorf("expected DOCKER_HOST to be used, got %q, %v", socket, ok)
	}
}
