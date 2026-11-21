package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"ark-cli/internal/core"
	"pkg/ark"
)

// binarylessDeps fails the test if ark-cli looks for or runs a runtime binary.
func binarylessDeps(t *testing.T) *core.AppDeps {
	return &core.AppDeps{Runtime: core.RuntimeDeps{
		LookPath: func(name string) (string, error) {
			t.Fatalf("LookPath(%q) called", name)
			return "", nil
		},
		RunCommand: func(cmd []string, _ ark.CommandOptions) (ark.ExecResult, error) {
			t.Fatalf("RunCommand(%v) called", cmd)
			return ark.ExecResult{}, nil
		},
	}}
}

// TestCLIRunRuntimeFromEnvNeedsNoBinary checks that with ARK_CONTAINER_RUNTIME
// and DOCKER_HOST set, Podman uses DOCKER_HOST (no host socket probe) and
// startup neither looks up nor runs a runtime binary (no `compose version`).
func TestCLIRunRuntimeFromEnvNeedsNoBinary(t *testing.T) {
	daemon := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Api-Version", "1.47")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"Version":"5.4.0","ApiVersion":"1.47"}`))
	}))
	defer daemon.Close()
	dockerHost := "tcp://" + strings.TrimPrefix(daemon.URL, "http://")
	t.Setenv("ARK_CONTAINER_RUNTIME", "podman")
	t.Setenv("DOCKER_HOST", dockerHost)
	t.Setenv("DOCKER_TLS_VERIFY", "") // the fake daemon speaks plain HTTP
	t.Setenv("DOCKER_CERT_PATH", "")
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir()) // no podman.sock: a socket probe would fail

	var err error
	out := captureStderr(t, func() { err = NewCLI(binarylessDeps(t)).Run(nil) })
	if err != nil {
		t.Fatalf("CLI.Run: %v", err)
	}
	if !strings.Contains(out, "Usage: ark-cli") {
		t.Fatalf("expected usage output, got:\n%s", out)
	}
	if got := os.Getenv("DOCKER_HOST"); got != dockerHost {
		t.Fatalf("DOCKER_HOST changed to %q", got)
	}
}

// TestCLIRunRuntimePodmanWithoutDockerHostProbesSocket checks that with only
// ARK_CONTAINER_RUNTIME set, Podman still requires the host's rootless socket.
func TestCLIRunRuntimePodmanWithoutDockerHostProbesSocket(t *testing.T) {
	t.Setenv("ARK_CONTAINER_RUNTIME", "podman")
	t.Setenv("DOCKER_HOST", "")
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())
	deps := &core.AppDeps{Runtime: core.RuntimeDeps{
		LookPath: func(name string) (string, error) { return "/usr/bin/" + name, nil },
	}}
	err := NewCLI(deps).Run(nil)
	if err == nil || !strings.Contains(err.Error(), "podman") {
		t.Fatalf("expected podman socket error, got %v", err)
	}
}

func TestParseGlobalOptionsRuntimeFlag(t *testing.T) {
	opts, remaining, err := parseGlobalOptions([]string{"--runtime", "podman", "create", "instance"})
	if err != nil {
		t.Fatalf("parseGlobalOptions: %v", err)
	}
	if opts.runtime != "podman" {
		t.Fatalf("expected runtime=podman, got %q", opts.runtime)
	}
	if len(remaining) != 2 || remaining[0] != "create" {
		t.Fatalf("unexpected remaining: %#v", remaining)
	}
}

func TestParseGlobalOptionsRuntimeEquals(t *testing.T) {
	opts, remaining, err := parseGlobalOptions([]string{"--runtime=docker", "start", "asa1"})
	if err != nil {
		t.Fatalf("parseGlobalOptions: %v", err)
	}
	if opts.runtime != "docker" {
		t.Fatalf("expected runtime=docker, got %q", opts.runtime)
	}
	if len(remaining) != 2 || remaining[0] != "start" {
		t.Fatalf("unexpected remaining: %#v", remaining)
	}
}

func TestParseGlobalOptionsRuntimeMissingValue(t *testing.T) {
	_, _, err := parseGlobalOptions([]string{"--runtime"})
	if err == nil {
		t.Fatal("expected error for --runtime without value")
	}
}
func TestParseGlobalOptionsVersion(t *testing.T) {
	opts, remaining, err := parseGlobalOptions([]string{"--version", "start", "instance"})
	if err != nil {
		t.Fatalf("parseGlobalOptions: %v", err)
	}
	if !opts.version || remaining != nil {
		t.Fatalf("expected version=true and no remaining args, got %v %#v", opts.version, remaining)
	}
}
