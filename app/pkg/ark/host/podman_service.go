package host

import (
	"fmt"
	"os"
	"path/filepath"
)

// HasPodmanService checks if the Podman system service socket is available.
//
// Returns:
//   - socketURI: The socket path prefixed with "unix://" for use with DOCKER_HOST,
//     or empty string if no usable socket is found.
//   - rootfulDetected: true if a rootful (system) socket was found at /run/podman/podman.sock
//     but no rootless socket was available. This indicates rootful Podman is installed
//     but rootless Podman (required) is not running.
//
// Only rootless Podman is supported. If rootfulDetected is true, display a message
// directing users to install and configure rootless Podman.
func HasPodmanService() (string, bool) {
	// Rootless socket path: $XDG_RUNTIME_DIR/podman/podman.sock
	xdgRuntimeDir := os.Getenv("XDG_RUNTIME_DIR")
	if xdgRuntimeDir == "" {
		uid := os.Getuid()
		xdgRuntimeDir = fmt.Sprintf("/run/user/%d", uid)
	}
	rootlessSocket := filepath.Join(xdgRuntimeDir, "podman", "podman.sock")
	if _, err := os.Stat(rootlessSocket); err == nil {
		return "unix://" + rootlessSocket, false
	}

	// Check for system socket (rootful) — not supported, but provide helpful message
	systemSocket := "/run/podman/podman.sock"
	if _, err := os.Stat(systemSocket); err == nil {
		// User socket not found, but system socket exists — rootful Podman detected
		return "", true
	}

	return "", false
}
