package host

import (
	"os"
)

// HasDockerService checks if the Docker system service socket is available.
// Returns the socket path and true if found, empty string and false otherwise.
// A non-empty DOCKER_HOST wins (the Docker SDK reads it too); otherwise
// checks /var/run/docker.sock (standard Docker socket location).
func HasDockerService() (string, bool) {
	if dockerHost := os.Getenv("DOCKER_HOST"); dockerHost != "" {
		return dockerHost, true
	}
	dockerSocket := "/var/run/docker.sock"
	if _, err := os.Stat(dockerSocket); err == nil {
		return "unix://" + dockerSocket, true
	}
	return "", false
}
