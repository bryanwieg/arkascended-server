package runtime

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/user"
	"strings"

	"pkg/ark"
)

// RunWithPrivs executes a command, escalating to sudo if not root.
func (h *RuntimeHelper) RunWithPrivs(command []string) error {
	if len(command) == 0 {
		return errors.New("Error: command is required.")
	}
	if os.Geteuid() == 0 {
		result, err := h.Runtime.RunCommand(command, ark.CommandOptions{})
		if err != nil {
			return err
		}
		if result.ExitCode != 0 {
			return fmt.Errorf("Error: command failed: %s", strings.Join(command, " "))
		}
		return nil
	}
	if _, err := h.Runtime.LookPath("sudo"); err == nil {
		cmd := append([]string{"sudo"}, command...)
		result, err := h.Runtime.RunCommand(cmd, ark.CommandOptions{})
		if err != nil {
			return err
		}
		if result.ExitCode != 0 {
			return fmt.Errorf("Error: command failed: %s", strings.Join(cmd, " "))
		}
		return nil
	}
	return errors.New("Error: need root or sudo to fix ownership.")
}

// EnsureDockerGroup adds the current user to the docker group if needed.
// It applies only to the rootful daemon socket; rootless Docker and Podman skip it.
func (h *RuntimeHelper) EnsureDockerGroup(stderr io.Writer) error {
	if h.Runtime.ContainerRuntime == RuntimePodman {
		return nil
	}
	// The docker group grants the rootful socket (root-equivalent). Rootless Docker needs DOCKER_HOST
	// pointing at the user's own socket, so any other endpoint needs no group.
	if host := os.Getenv("DOCKER_HOST"); host != "" && host != "unix:///var/run/docker.sock" {
		return nil
	}

	// Root reaches the socket without the group. Check the account running ark-cli, not SUDO_USER:
	// under `sudo -iu ark` that is the admin, not ark.
	if os.Geteuid() == 0 {
		return nil
	}
	userInfo, err := user.Current()
	if err != nil {
		return nil
	}
	targetUser := userInfo.Username

	dockerGroup, err := user.LookupGroup("docker")
	if err != nil {
		fmt.Fprintln(stderr, "Warning: docker group not found; skipping group membership check.")
		return nil
	}

	groups, err := userInfo.GroupIds()
	if err != nil {
		return nil
	}
	for _, gid := range groups {
		if gid == dockerGroup.Gid {
			return nil
		}
	}

	if err := h.RunWithPrivs([]string{"usermod", "-aG", "docker", targetUser}); err != nil {
		return err
	}
	fmt.Fprintf(stderr, "Added %s to docker group. Re-login may be required.\n", targetUser)
	return nil
}
