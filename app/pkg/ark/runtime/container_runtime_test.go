package runtime

import (
	"os"
	"strings"
	"testing"
)

func TestResolveContainerRuntimeFlagDocker(t *testing.T) {
	rt, err := ResolveContainerRuntime("docker", func(name string) (string, error) {
		return "/usr/bin/" + name, nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rt != RuntimeDocker {
		t.Fatalf("expected docker, got %s", rt)
	}
}

func TestResolveContainerRuntimeFlagPodman(t *testing.T) {
	rt, err := ResolveContainerRuntime("podman", func(name string) (string, error) {
		return "/usr/bin/" + name, nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rt != RuntimePodman {
		t.Fatalf("expected podman, got %s", rt)
	}
}

func TestResolveContainerRuntimeFlagInvalid(t *testing.T) {
	_, err := ResolveContainerRuntime("invalid", func(string) (string, error) { return "", nil })
	if err == nil {
		t.Fatal("expected error for invalid runtime")
	}
}

func TestResolveContainerRuntimeEnvVar(t *testing.T) {
	t.Setenv("ARK_CONTAINER_RUNTIME", "docker")
	rt, err := ResolveContainerRuntime("", func(name string) (string, error) {
		return "/usr/bin/" + name, nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rt != RuntimeDocker {
		t.Fatalf("expected docker, got %s", rt)
	}
}

func TestResolveContainerRuntimeEnvEngineSkipsLookPath(t *testing.T) {
	t.Setenv("ARK_CONTAINER_RUNTIME", "podman")
	t.Setenv("DOCKER_HOST", "unix:///run/podman.sock")
	rt, err := ResolveContainerRuntime("", func(name string) (string, error) {
		t.Fatalf("lookPath(%q) called with ARK_CONTAINER_RUNTIME and DOCKER_HOST set", name)
		return "", nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rt != RuntimePodman {
		t.Fatalf("expected podman, got %s", rt)
	}
}

func TestResolveContainerRuntimeEnvVarWithoutDockerHostNeedsPath(t *testing.T) {
	t.Setenv("ARK_CONTAINER_RUNTIME", "podman")
	t.Setenv("DOCKER_HOST", "")
	_, err := ResolveContainerRuntime("", func(string) (string, error) { return "", os.ErrNotExist })
	if err == nil || !strings.Contains(err.Error(), "not found on PATH") {
		t.Fatalf("expected not-on-PATH error, got %v", err)
	}
}

func TestEngineFromEnv(t *testing.T) {
	tests := []struct {
		runtime, host string
		want          bool
	}{
		{"docker", "unix:///var/run/docker.sock", true},
		{"docker", "", false},
		{"", "unix:///var/run/docker.sock", false},
		{"", "", false},
	}
	for _, tc := range tests {
		t.Setenv("ARK_CONTAINER_RUNTIME", tc.runtime)
		t.Setenv("DOCKER_HOST", tc.host)
		if got := EngineFromEnv(); got != tc.want {
			t.Fatalf("EngineFromEnv() with runtime=%q host=%q = %v, want %v", tc.runtime, tc.host, got, tc.want)
		}
	}
}

func TestResolveContainerRuntimeAutoDetectPrefersDocker(t *testing.T) {
	t.Setenv("ARK_CONTAINER_RUNTIME", "")
	rt, err := ResolveContainerRuntime("", func(name string) (string, error) {
		return "/usr/bin/" + name, nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rt != RuntimeDocker {
		t.Fatalf("expected docker (preferred), got %s", rt)
	}
}

func TestResolveContainerRuntimeAutoDetectDockerOnly(t *testing.T) {
	t.Setenv("ARK_CONTAINER_RUNTIME", "")
	rt, err := ResolveContainerRuntime("", func(name string) (string, error) {
		if name == "docker" {
			return "/usr/bin/docker", nil
		}
		return "", os.ErrNotExist
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rt != RuntimeDocker {
		t.Fatalf("expected docker, got %s", rt)
	}
}

func TestResolveContainerRuntimeAutoDetectPodmanOnly(t *testing.T) {
	t.Setenv("ARK_CONTAINER_RUNTIME", "")
	rt, err := ResolveContainerRuntime("", func(name string) (string, error) {
		if name == "podman" {
			return "/usr/bin/podman", nil
		}
		return "", os.ErrNotExist
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rt != RuntimePodman {
		t.Fatalf("expected podman (fallback), got %s", rt)
	}
}

func TestResolveContainerRuntimeNoneAvailable(t *testing.T) {
	t.Setenv("ARK_CONTAINER_RUNTIME", "")
	_, err := ResolveContainerRuntime("", func(string) (string, error) { return "", os.ErrNotExist })
	if err == nil {
		t.Fatal("expected error when no runtime available")
	}
}

func TestResolveContainerRuntimeFlagNotOnPath(t *testing.T) {
	_, err := ResolveContainerRuntime("podman", func(string) (string, error) { return "", os.ErrNotExist })
	if err == nil {
		t.Fatal("expected error when flag runtime not on PATH")
	}
	if !strings.Contains(err.Error(), "not found on PATH") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestParseContainerRuntime(t *testing.T) {
	tests := []struct {
		input   string
		want    ContainerRuntime
		wantErr bool
	}{
		{"docker", RuntimeDocker, false},
		{"podman", RuntimePodman, false},
		{"DOCKER", RuntimeDocker, false},
		{" Podman ", RuntimePodman, false},
		{"invalid", "", true},
	}
	for _, tc := range tests {
		got, err := ParseContainerRuntime(tc.input)
		if tc.wantErr && err == nil {
			t.Fatalf("ParseContainerRuntime(%q): expected error", tc.input)
		}
		if !tc.wantErr && err != nil {
			t.Fatalf("ParseContainerRuntime(%q): %v", tc.input, err)
		}
		if got != tc.want {
			t.Fatalf("ParseContainerRuntime(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}
