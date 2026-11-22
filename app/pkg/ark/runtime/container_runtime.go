package runtime

import (
	"fmt"
	"os"
	"strings"

	"pkg/ark"
)

// ResolveContainerRuntime determines which container runtime to use.
// Priority: flag > env var > PATH probe (prefer docker).
func ResolveContainerRuntime(flagValue string, lookPath func(string) (string, error)) (ContainerRuntime, error) {
	if flagValue != "" {
		rt, err := ParseContainerRuntime(flagValue)
		if err != nil {
			return "", err
		}
		if _, err := lookPath(string(rt)); err != nil {
			return "", fmt.Errorf("Error: %s not found on PATH", rt)
		}
		return rt, nil
	}

	if envVal := os.Getenv("ARK_CONTAINER_RUNTIME"); envVal != "" {
		rt, err := ParseContainerRuntime(envVal)
		if err != nil {
			return "", err
		}
		if EngineFromEnv() {
			return rt, nil
		}
		if _, err := lookPath(string(rt)); err != nil {
			return "", fmt.Errorf("Error: %s not found on PATH (set by ARK_CONTAINER_RUNTIME)", rt)
		}
		return rt, nil
	}

	if _, err := lookPath("docker"); err == nil {
		return RuntimeDocker, nil
	}
	if _, err := lookPath("podman"); err == nil {
		return RuntimePodman, nil
	}
	return "", fmt.Errorf("Error: no container runtime found. Install docker or podman.")
}

// EngineFromEnv reports whether the environment names both the runtime and its
// API endpoint (ARK_CONTAINER_RUNTIME and DOCKER_HOST), as inside the scheduler
// container: ark-cli then needs no docker/podman binary and trusts DOCKER_HOST.
func EngineFromEnv() bool {
	return os.Getenv("ARK_CONTAINER_RUNTIME") != "" && os.Getenv("DOCKER_HOST") != ""
}

// ParseContainerRuntime parses a container runtime name string.
func ParseContainerRuntime(value string) (ContainerRuntime, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "docker":
		return RuntimeDocker, nil
	case "podman":
		return RuntimePodman, nil
	default:
		return "", fmt.Errorf("Error: invalid container runtime %q (expected docker or podman)", value)
	}
}

// ValidateComposeProvider checks that `<runtime> compose version` succeeds.
func (h *RuntimeHelper) ValidateComposeProvider() error {
	rt := string(h.Runtime.ContainerRuntime)
	result, err := h.Runtime.RunCommand(
		[]string{rt, "compose", "version"},
		ark.CommandOptions{Capture: true},
	)
	if err != nil {
		return fmt.Errorf("Error: %s compose is not available: %w", rt, err)
	}
	if result.ExitCode != 0 {
		return fmt.Errorf("Error: %s compose is not available. Install a compose provider.", rt)
	}
	return nil
}
