package reconfigure

// Tests: enable/disable remote-rcon flip only the RCON binding's host IP and keep port, label, and other bindings.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ark-cli/internal/core"
	"ark-cli/internal/testutil"
)

func writeRemoteRCONCompose(t *testing.T, rconLine string) string {
	t.Helper()
	composePath := filepath.Join(t.TempDir(), "docker-compose.yaml")
	seed := "name: test\nservices:\n  asa1:\n    image: ark-server:latest\n    ports:\n" +
		"      - \"27015:27015/udp\"\n" + rconLine +
		"    labels:\n      ark.port.rcon: \"28020\"\n"
	if err := os.WriteFile(composePath, []byte(seed), 0o644); err != nil {
		t.Fatalf("write compose: %v", err)
	}
	return composePath
}

func remoteRCONPorts(t *testing.T, composePath string) string {
	t.Helper()
	project, err := testutil.LoadComposeFromPathForTest(composePath)
	if err != nil {
		t.Fatalf("LoadComposeFromPathForTest: %v", err)
	}
	svc := project.Services["asa1"]
	if svc.Labels["ark.port.rcon"] != "28020" {
		t.Fatalf("expected ark.port.rcon label preserved, got %q", svc.Labels["ark.port.rcon"])
	}
	return strings.Join(portConfigsToStrings(svc.Ports), "\n")
}

func TestDisableRemoteRCONBindsLoopback(t *testing.T) {
	composePath := writeRemoteRCONCompose(t, "      - \"28020:27020/tcp\"\n")
	withPatchedAppDeps(t, nil, func() {
		if err := RunDisableRemoteRCON(core.CurrentAppDeps(), []string{"--compose-file", composePath, "--instance", "asa1"}); err != nil {
			t.Fatalf("disable remote-rcon: %v", err)
		}
	})
	joined := remoteRCONPorts(t, composePath)
	if !strings.Contains(joined, "127.0.0.1:28020:27020/tcp") {
		t.Fatalf("expected loopback RCON binding, got:\n%s", joined)
	}
	if !strings.Contains(joined, "27015:27015/udp") || strings.Contains(joined, "127.0.0.1:27015") {
		t.Fatalf("expected ASA binding untouched, got:\n%s", joined)
	}
}

func TestEnableRemoteRCONClearsHostIP(t *testing.T) {
	composePath := writeRemoteRCONCompose(t, "      - \"127.0.0.1:28020:27020/tcp\"\n")
	withPatchedAppDeps(t, nil, func() {
		if err := RunEnableRemoteRCON(core.CurrentAppDeps(), []string{"--compose-file", composePath, "--instance", "asa1"}); err != nil {
			t.Fatalf("enable remote-rcon: %v", err)
		}
	})
	joined := remoteRCONPorts(t, composePath)
	if strings.Contains(joined, "127.0.0.1:28020") || strings.Contains(joined, "0.0.0.0") {
		t.Fatalf("expected RCON published on all interfaces (no host IP), got:\n%s", joined)
	}
	if !strings.Contains(joined, "28020:27020/tcp") {
		t.Fatalf("expected RCON port kept, got:\n%s", joined)
	}
}

func TestRemoteRCONNoChangeWhenAlreadySet(t *testing.T) {
	composePath := writeRemoteRCONCompose(t, "      - \"127.0.0.1:28020:27020/tcp\"\n")
	var out string
	withPatchedAppDeps(t, nil, func() {
		out = captureStdout(t, func() {
			if err := RunDisableRemoteRCON(core.CurrentAppDeps(), []string{"--compose-file", composePath, "--instance", "asa1"}); err != nil {
				t.Fatalf("disable remote-rcon: %v", err)
			}
		})
	})
	if !strings.Contains(out, "No compose setting changes were necessary") {
		t.Fatalf("expected no-change message, got:\n%s", out)
	}
}

func TestRemoteRCONErrorsWithoutRCONBinding(t *testing.T) {
	composePath := filepath.Join(t.TempDir(), "docker-compose.yaml")
	seed := "name: test\nservices:\n  asa1:\n    image: ark-server:latest\n    ports:\n      - \"27015:27015/udp\"\n"
	if err := os.WriteFile(composePath, []byte(seed), 0o644); err != nil {
		t.Fatalf("write compose: %v", err)
	}
	withPatchedAppDeps(t, nil, func() {
		err := RunEnableRemoteRCON(core.CurrentAppDeps(), []string{"--compose-file", composePath, "--instance", "asa1"})
		if err == nil || !strings.Contains(err.Error(), "no RCON port binding") {
			t.Fatalf("expected missing-binding error, got %v", err)
		}
	})
}

func TestRemoteRCONRequiresInstance(t *testing.T) {
	withPatchedAppDeps(t, nil, func() {
		if err := RunEnableRemoteRCON(core.CurrentAppDeps(), nil); err == nil || !strings.Contains(err.Error(), "--instance is required") {
			t.Fatalf("expected --instance error, got %v", err)
		}
	})
}
