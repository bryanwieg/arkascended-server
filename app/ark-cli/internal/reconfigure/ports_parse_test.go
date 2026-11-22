package reconfigure

// Tests: reconfigure command port updates: short/long syntax, label-based 1:1 scheme, appending missing role ports, conflicting port rejection

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ark-cli/internal/core"
	"ark-cli/internal/testutil"

	"pkg/ark/composeutil"
	"pkg/ark/runtime"

	"github.com/compose-spec/compose-go/v2/types"
)

// portConfigsToStrings renders []types.ServicePortConfig into human-readable strings
// matching the compose short syntax: "host_ip:published:target/protocol".
func portConfigsToStrings(ports []types.ServicePortConfig) []string {
	var out []string
	for _, p := range ports {
		pub, _ := composeutil.PublishedPort(p)
		s := fmt.Sprintf("%d:%d/%s", pub, p.Target, p.Protocol)
		if p.HostIP != "" {
			s = fmt.Sprintf("%s:%d:%d/%s", p.HostIP, pub, p.Target, p.Protocol)
		}
		out = append(out, s)
	}
	return out
}

func TestRunReconfigureUpdatesShortSyntaxPorts(t *testing.T) {
	composePath := filepath.Join(t.TempDir(), "docker-compose.yaml")
	seed := "" +
		"name: test\n" +
		"services:\n" +
		"  asa1:\n" +
		"    image: ark-server:latest\n" +
		"    labels: {ark.port.asa: \"27015\"}\n" +
		"    ports:\n" +
		"      - \"127.0.0.1:27015:27015/udp\"\n" +
		"      - \"27020:27020/tcp\"\n"
	if err := os.WriteFile(composePath, []byte(seed), 0o644); err != nil {
		t.Fatalf("write compose: %v", err)
	}

	withPatchedAppDeps(t, nil, func() {
		if err := RunReconfigure(core.CurrentAppDeps(), []string{
			"--compose-file", composePath,
			"--instance", "asa1",
			"--asa-port", "28015",
			"--rcon-port", "28020",
		}); err != nil {
			t.Fatalf("runReconfigure: %v", err)
		}
	})

	project, err := testutil.LoadComposeFromPathForTest(composePath)
	if err != nil {
		t.Fatalf("LoadComposeFromPathForTest: %v", err)
	}
	svc := project.Services["asa1"]
	text := portConfigsToStrings(svc.Ports)
	joined := strings.Join(text, "\n")
	if !strings.Contains(joined, "127.0.0.1:28015:28015/udp") {
		t.Fatalf("expected updated ark port mapping, got:\n%s", joined)
	}
	if !strings.Contains(joined, "28020:27020/tcp") {
		t.Fatalf("expected updated rcon port mapping, got:\n%s", joined)
	}
}

// A custom RCON_PORT moves RCON inside the container; --rcon-port must update
// the binding to that port, not add a dead one for 27020.
func TestRunReconfigureRCONPortFollowsRCONPortEnv(t *testing.T) {
	composePath := filepath.Join(t.TempDir(), "docker-compose.yaml")
	seed := "name: test\nservices:\n  asa1:\n    image: ark-server:latest\n" +
		"    labels: {ark.port.asa: \"27015\"}\n" +
		"    environment:\n      - RCON_PORT=27030\n" +
		"    ports:\n      - \"27015:27015/udp\"\n      - \"127.0.0.1:27030:27030/tcp\"\n"
	if err := os.WriteFile(composePath, []byte(seed), 0o644); err != nil {
		t.Fatalf("write compose: %v", err)
	}
	withPatchedAppDeps(t, nil, func() {
		if err := RunReconfigure(core.CurrentAppDeps(), []string{
			"--compose-file", composePath, "--instance", "asa1", "--rcon-port", "28020",
		}); err != nil {
			t.Fatalf("runReconfigure: %v", err)
		}
	})
	project, err := testutil.LoadComposeFromPathForTest(composePath)
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(portConfigsToStrings(project.Services["asa1"].Ports), "\n")
	if !strings.Contains(joined, "127.0.0.1:28020:27030/tcp") || strings.Contains(joined, "27020/tcp") {
		t.Fatalf("expected RCON binding 28020->27030, got:\n%s", joined)
	}
}

func TestRunReconfigureUpdatesLongSyntaxPorts(t *testing.T) {
	composePath := filepath.Join(t.TempDir(), "docker-compose.yaml")
	seed := "" +
		"name: test\n" +
		"services:\n" +
		"  asa1:\n" +
		"    image: ark-server:latest\n" +
		"    labels: {ark.port.asa: \"27015\"}\n" +
		"    ports:\n" +
		"      - target: 27015\n" +
		"        published: 27015\n" +
		"        protocol: udp\n" +
		"      - target: 27020\n" +
		"        published: 27020\n" +
		"        protocol: tcp\n"
	if err := os.WriteFile(composePath, []byte(seed), 0o644); err != nil {
		t.Fatalf("write compose: %v", err)
	}

	withPatchedAppDeps(t, nil, func() {
		if err := RunReconfigure(core.CurrentAppDeps(), []string{
			"--compose-file", composePath,
			"--instance", "asa1",
			"--asa-port", "28115",
			"--rcon-port", "28120",
		}); err != nil {
			t.Fatalf("runReconfigure: %v", err)
		}
	})

	project, err := testutil.LoadComposeFromPathForTest(composePath)
	if err != nil {
		t.Fatalf("LoadComposeFromPathForTest: %v", err)
	}
	svc := project.Services["asa1"]
	ports := svc.Ports
	if len(ports) != 2 {
		t.Fatalf("expected two ports, got %d", len(ports))
	}

	if ports[0].Target != 28115 {
		t.Fatalf("expected ark target to follow the published port (1:1), got %d", ports[0].Target)
	}
	if ports[0].Published != "28115" {
		t.Fatalf("expected ark published update, got %s", ports[0].Published)
	}
	if ports[1].Target != 27020 {
		t.Fatalf("expected rcon target unchanged, got %d", ports[1].Target)
	}
	if ports[1].Published != "28120" {
		t.Fatalf("expected rcon published update, got %s", ports[1].Published)
	}
}

func TestRunReconfigureAppendsMissingRolePortMapping(t *testing.T) {
	composePath := filepath.Join(t.TempDir(), "docker-compose.yaml")
	seed := "" +
		"name: test\n" +
		"services:\n" +
		"  asa1:\n" +
		"    image: ark-server:latest\n" +
		"    labels: {ark.port.asa: \"27015\"}\n" +
		"    ports:\n" +
		"      - \"27015:27015/udp\"\n"
	if err := os.WriteFile(composePath, []byte(seed), 0o644); err != nil {
		t.Fatalf("write compose: %v", err)
	}

	withPatchedAppDeps(t, nil, func() {
		if err := RunReconfigure(core.CurrentAppDeps(), []string{
			"--compose-file", composePath,
			"--instance", "asa1",
			"--rcon-port", "29020",
		}); err != nil {
			t.Fatalf("runReconfigure: %v", err)
		}
	})

	project, err := testutil.LoadComposeFromPathForTest(composePath)
	if err != nil {
		t.Fatalf("LoadComposeFromPathForTest: %v", err)
	}
	svc := project.Services["asa1"]
	text := portConfigsToStrings(svc.Ports)
	joined := strings.Join(text, "\n")
	if !strings.Contains(joined, "29020:27020/tcp") {
		t.Fatalf("expected appended rcon mapping, got:\n%s", joined)
	}
}

func TestRunReconfigureRejectsConflictingRolePort(t *testing.T) {
	composePath := filepath.Join(t.TempDir(), "docker-compose.yaml")
	seed := "" +
		"name: test\n" +
		"services:\n" +
		"  asa1:\n" +
		"    image: ark-server:latest\n" +
		"    labels: {ark.port.asa: \"27015\"}\n" +
		"    ports:\n" +
		"      - \"27015:27015/udp\"\n" +
		"      - \"27020:27020/tcp\"\n" +
		"  asa2:\n" +
		"    image: ark-server:latest\n" +
		"    labels: {ark.port.asa: \"29015\", ark.port.rcon: \"29020\"}\n" +
		"    ports:\n" +
		"      - \"29015:29015/udp\"\n" +
		"      - \"29020:27020/tcp\"\n"
	if err := os.WriteFile(composePath, []byte(seed), 0o644); err != nil {
		t.Fatalf("write compose: %v", err)
	}

	var err error
	withPatchedAppDeps(t, nil, func() {
		err = RunReconfigure(core.CurrentAppDeps(), []string{
			"--compose-file", composePath,
			"--instance", "asa1",
			"--rcon-port", "29020",
		})
	})
	if err == nil {
		t.Fatal("expected port conflict error")
	}
	if !strings.Contains(err.Error(), "instance asa2") {
		t.Fatalf("expected conflicting instance in error, got: %v", err)
	}
	if !strings.Contains(strings.ToLower(err.Error()), "rcon") {
		t.Fatalf("expected conflicting role in error, got: %v", err)
	}
}

func TestRunReconfigureNewSchemeUpdatesPortLabelAndEnv(t *testing.T) {
	// New-scheme compose: labels present, port binding is 1:1.
	composePath := filepath.Join(t.TempDir(), "docker-compose.yaml")
	seed := "" +
		"name: test\n" +
		"services:\n" +
		"  asa1:\n" +
		"    image: ark-server:latest\n" +
		"    environment:\n" +
		"      - ASA_PORT=27015\n" +
		"    ports:\n" +
		"      - \"27015:27015/udp\"\n" +
		"      - \"27020:27020/tcp\"\n" +
		"    labels:\n" +
		"      ark.port.asa: \"27015\"\n" +
		"      ark.port.rcon: \"27020\"\n"
	if err := os.WriteFile(composePath, []byte(seed), 0o644); err != nil {
		t.Fatalf("write compose: %v", err)
	}

	withPatchedAppDeps(t, nil, func() {
		if err := RunReconfigure(core.CurrentAppDeps(), []string{
			"--compose-file", composePath,
			"--instance", "asa1",
			"--asa-port", "28015",
		}); err != nil {
			t.Fatalf("runReconfigure: %v", err)
		}
	})

	project, err := testutil.LoadComposeFromPathForTest(composePath)
	if err != nil {
		t.Fatalf("LoadComposeFromPathForTest: %v", err)
	}
	svc := project.Services["asa1"]

	// Port binding must be 1:1.
	text := portConfigsToStrings(svc.Ports)
	joined := strings.Join(text, "\n")
	if !strings.Contains(joined, "28015:28015/udp") {
		t.Fatalf("expected 1:1 port binding 28015:28015/udp, got:\n%s", joined)
	}

	// Label must be updated.
	if got := svc.Labels[runtime.LabelPortASA]; got != "28015" {
		t.Fatalf("expected ark.port.asa label=28015, got %q", got)
	}

	// ASA_PORT env var must be updated.
	if got := runtime.ServiceEnvValue(svc, "ASA_PORT"); got != "28015" {
		t.Fatalf("expected ASA_PORT=28015, got %q", got)
	}
}

func TestRunReconfigureNewSchemeConflictDetectedViaLabel(t *testing.T) {
	// Conflict detection uses labels when present (not port binding targets).
	composePath := filepath.Join(t.TempDir(), "docker-compose.yaml")
	seed := "" +
		"name: test\n" +
		"services:\n" +
		"  asa1:\n" +
		"    image: ark-server:latest\n" +
		"    ports:\n" +
		"      - \"27015:27015/udp\"\n" +
		"    labels:\n" +
		"      ark.port.asa: \"27015\"\n" +
		"  asa2:\n" +
		"    image: ark-server:latest\n" +
		"    ports:\n" +
		"      - \"28015:28015/udp\"\n" +
		"    labels:\n" +
		"      ark.port.asa: \"28015\"\n"
	if err := os.WriteFile(composePath, []byte(seed), 0o644); err != nil {
		t.Fatalf("write compose: %v", err)
	}

	var err error
	withPatchedAppDeps(t, nil, func() {
		err = RunReconfigure(core.CurrentAppDeps(), []string{
			"--compose-file", composePath,
			"--instance", "asa1",
			"--asa-port", "28015",
		})
	})
	if err == nil {
		t.Fatal("expected port conflict error")
	}
	if !strings.Contains(err.Error(), "instance asa2") {
		t.Fatalf("expected conflicting instance in error, got: %v", err)
	}
	if !strings.Contains(strings.ToLower(err.Error()), "ark") {
		t.Fatalf("expected conflicting role in error, got: %v", err)
	}
}

func TestRunReconfigureRejectsSchedulerService(t *testing.T) {
	composePath := filepath.Join(t.TempDir(), "docker-compose.yaml")
	seed := "" +
		"name: test\n" +
		"services:\n" +
		"  ark-scheduler:\n" +
		"    image: ark-scheduler:abc\n" +
		"    labels:\n" +
		"      ark.role: scheduler\n"
	if err := os.WriteFile(composePath, []byte(seed), 0o644); err != nil {
		t.Fatalf("write compose: %v", err)
	}
	withPatchedAppDeps(t, nil, func() {
		err := RunReconfigure(core.CurrentAppDeps(), []string{
			"--compose-file", composePath, "--instance", "ark-scheduler", "--asa-port", "28015",
		})
		if err == nil || !strings.Contains(err.Error(), "is the scheduler service") {
			t.Fatalf("err = %v, want the scheduler rejection", err)
		}
	})
	after, err := os.ReadFile(composePath)
	if err != nil || string(after) != seed {
		t.Fatalf("compose file changed: %v\n%s", err, after)
	}
}
