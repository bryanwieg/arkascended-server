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
// Rootless Podman skips this check.
func (h *RuntimeHelper) EnsureDockerGroup(stderr io.Writer) error {
	// Rootless Podman needs no group membership.
	if h.Runtime.ContainerRuntime == RuntimePodman {
		return nil
	}

	targetUser := os.Getenv("SUDO_USER")
	if targetUser == "" {
		targetUser = os.Getenv("USER")
	}
	if targetUser == "" {
		return nil
	}

	dockerGroup, err := user.LookupGroup("docker")
	if err != nil {
		fmt.Fprintln(stderr, "Warning: docker group not found; skipping group membership check.")
		return nil
	}

	userInfo, err := user.Lookup(targetUser)
	if err != nil {
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
