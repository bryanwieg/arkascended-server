package create

// Tests: instance create command: next-port selection, name/scalar/password validation

import (
	"ark-cli/internal/core"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"pkg/ark/runtime"
)

// TestBuildSettingsPicksNextPorts verifies port auto-selection when defaults are already used.
// - Expect: ASA port increments to 27016 and RCON host port increments to 27021.
func TestBuildSettingsPicksNextPorts(t *testing.T) {
	composeText := "services:\n  asa-server-0:\n    ports:\n      - \"27015:27015/udp\"\n      - \"27020:27020/tcp\"\n"
	settings, err := buildCreateSettings("asa-server-1", createParams{adminPassword: "AdminPass123", serverPassword: "ServerPass123"}, composeText, "")
	if err != nil {
		t.Fatalf("build settings: %v", err)
	}
	if settings.asaPort != 27016 {
		t.Fatalf("expected asa port 27016, got %d", settings.asaPort)
	}
	if settings.rconHostPort != 27021 {
		t.Fatalf("expected rcon port 27021, got %d", settings.rconHostPort)
	}
}

// TestValidateComposeNameRejectsBad checks invalid compose names are rejected.
// - Expect: runtime.ValidateComposeName returns an error for names with spaces.
func TestValidateComposeNameRejectsBad(t *testing.T) {
	if err := runtime.ValidateComposeName("bad name", "instance name"); err == nil {
		t.Fatal("expected error for invalid compose name")
	}
}

// TestValidateYAMLScalarRejectsNewline ensures YAML scalars cannot contain newlines.
// - Expect: runtime.ValidateYAMLScalar returns an error when a newline is present.
func TestValidateYAMLScalarRejectsNewline(t *testing.T) {
	if err := runtime.ValidateYAMLScalar("line1\nline2", "value"); err == nil {
		t.Fatal("expected error for newline")
	}
}

// TestValidateYAMLScalarRejectsColonSpace ensures YAML scalars cannot contain ': '.
// - Expect: runtime.ValidateYAMLScalar returns an error when colon+space is present.
func TestValidateYAMLScalarRejectsColonSpace(t *testing.T) {
	if err := runtime.ValidateYAMLScalar("bad: value", "value"); err == nil {
		t.Fatal("expected error for colon-space")
	}
}

// TestValidatePasswordRejectsNonAlnum ensures passwords reject non-alphanumeric chars.
// - Expect: runtime.ValidatePassword returns an error for symbols or spaces.
func TestValidatePasswordRejectsNonAlnum(t *testing.T) {
	if err := runtime.ValidatePassword("bad pass", "password"); err == nil {
		t.Fatal("expected error for space")
	}
	if err := runtime.ValidatePassword("bad!", "password"); err == nil {
		t.Fatal("expected error for symbol")
	}
}

// TestBuildSettingsRejectsInvalidPasswords ensures create rejects invalid passwords.
// - Expect: buildCreateSettings returns an error for non-alphanumeric passwords.
func TestBuildSettingsRejectsInvalidPasswords(t *testing.T) {
	if _, err := buildCreateSettings("asa-server-1", createParams{adminPassword: "Admin!", serverPassword: "ServerPass123"}, "", ""); err == nil {
		t.Fatal("expected error for admin password")
	}
	if _, err := buildCreateSettings("asa-server-1", createParams{adminPassword: "AdminPass123", serverPassword: "Server Pass"}, "", ""); err == nil {
		t.Fatal("expected error for server password")
	}
}

// TestRunCreateEnsuresClusterAndSaveVolume verifies volumes are always present.
// - Expect: compose file includes instance save and cluster volumes.
func TestRunCreateEnsuresClusterAndSaveVolume(t *testing.T) {
	tempDir := t.TempDir()
	composePath := filepath.Join(tempDir, "docker-compose.yaml")
	setEnv(t, "COMPOSE_FILE", composePath)

	seed := "services:\n  asa0:\n    image: ark-server:latest\n"
	if err := os.WriteFile(composePath, []byte(seed), 0o644); err != nil {
		t.Fatalf("write compose: %v", err)
	}

	if err := RunCreate(core.CurrentAppDeps(), []string{
		"--name", "asa1",
		"--admin-password", "adminpass",
		"--server-password", "serverpass",
	}); err != nil {
		t.Fatalf("runCreate: %v", err)
	}

	contents, err := os.ReadFile(composePath)
	if err != nil {
		t.Fatalf("read compose: %v", err)
	}
	text := string(contents)
	if !strings.Contains(text, "asa1-saved:") {
		t.Fatal("expected instance save volume entry")
	}
	if !strings.Contains(text, "ark-cluster-1:") {
		t.Fatal("expected cluster volume entry")
	}
}

// TestBuildSettingsNoServerPassword verifies --no-server-password allows empty server password.
// - Expect: buildCreateSettings succeeds with empty server password when noServerPassword is set.
func TestBuildSettingsNoServerPassword(t *testing.T) {
	settings, err := buildCreateSettings("asa-server-1", createParams{adminPassword: "AdminPass123", noServerPassword: true}, "", "")
	if err != nil {
		t.Fatalf("build settings: %v", err)
	}
	if settings.serverPassword != "" {
		t.Fatalf("expected empty server password, got %q", settings.serverPassword)
	}
}

// TestBuildSettingsRejectsMissingServerPasswordWithoutFlag ensures server password is still required without the flag.
// - Expect: buildCreateSettings returns an error when server password is missing and noServerPassword is false.
func TestBuildSettingsRejectsMissingServerPasswordWithoutFlag(t *testing.T) {
	if _, err := buildCreateSettings("asa-server-1", createParams{adminPassword: "AdminPass123"}, "", ""); err == nil {
		t.Fatal("expected error for missing server password without --no-server-password")
	}
}

// TestRunCreateNoServerPassword verifies --no-server-password produces compose with empty SERVER_PASSWORD.
// - Expect: compose file contains SERVER_PASSWORD= (empty value).
func TestRunCreateNoServerPassword(t *testing.T) {
	tempDir := t.TempDir()
	composePath := filepath.Join(tempDir, "docker-compose.yaml")
	setEnv(t, "COMPOSE_FILE", composePath)

	if err := os.WriteFile(composePath, []byte("services:\n  asa0:\n    image: ark-server:latest\n"), 0o644); err != nil {
		t.Fatalf("write compose: %v", err)
	}

	if err := RunCreate(core.CurrentAppDeps(), []string{
		"--name", "asa1",
		"--admin-password", "adminpass",
		"--no-server-password",
	}); err != nil {
		t.Fatalf("runCreate: %v", err)
	}

	contents, err := os.ReadFile(composePath)
	if err != nil {
		t.Fatalf("read compose: %v", err)
	}
	text := string(contents)
	if !strings.Contains(text, "SERVER_PASSWORD=") {
		t.Fatal("expected SERVER_PASSWORD= with empty value in compose file")
	}
}

// TestRunCreateRejectsBothServerAndNoPasswordFlags ensures --server-password and --no-server-password are mutually exclusive.
func TestRunCreateRejectsBothServerAndNoPasswordFlags(t *testing.T) {
	tempDir := t.TempDir()
	composePath := filepath.Join(tempDir, "docker-compose.yaml")
	setEnv(t, "COMPOSE_FILE", composePath)

	if err := os.WriteFile(composePath, []byte("services:\n  asa0:\n    image: ark-server:latest\n"), 0o644); err != nil {
		t.Fatalf("write compose: %v", err)
	}

	err := RunCreate(core.CurrentAppDeps(), []string{
		"--name", "asa1",
		"--admin-password", "adminpass",
		"--server-password", "serverpass",
		"--no-server-password",
	})
	if err == nil {
		t.Fatal("expected error for mutually exclusive flags")
	}
}

// TestRunCreateRejectsBothServerAndNoPasswordWithInteractive ensures mutual exclusion also applies with --interactive.
func TestRunCreateRejectsBothServerAndNoPasswordWithInteractive(t *testing.T) {
	tempDir := t.TempDir()
	composePath := filepath.Join(tempDir, "docker-compose.yaml")
	setEnv(t, "COMPOSE_FILE", composePath)

	if err := os.WriteFile(composePath, []byte("services:\n  asa0:\n    image: ark-server:latest\n"), 0o644); err != nil {
		t.Fatalf("write compose: %v", err)
	}

	err := RunCreate(core.CurrentAppDeps(), []string{
		"--name", "asa1",
		"--admin-password", "adminpass",
		"--server-password", "serverpass",
		"--interactive",
		"--no-server-password",
	})
	if err == nil {
		t.Fatal("expected error for mutually exclusive flags")
	}
}

func TestBuildSettingsWarnsForMapNameMissingWP(t *testing.T) {
	var warnBuf strings.Builder
	origWarnf := core.Warnf
	core.Warnf = func(format string, args ...any) { warnBuf.WriteString(fmt.Sprintf(format, args...)) }
	t.Cleanup(func() { core.Warnf = origWarnf })

	settings, err := buildCreateSettings("asa-server-1", createParams{adminPassword: "AdminPass123", serverPassword: "ServerPass123", mapName: "TheIsland"}, "", "")
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if settings.mapName != "TheIsland" {
		t.Fatalf("expected mapName TheIsland, got %q", settings.mapName)
	}
	if !strings.Contains(warnBuf.String(), "_WP") {
		t.Fatal("expected _WP warning for map name without suffix")
	}
}

func TestBuildSettingsNoWarningForMapNameWithWP(t *testing.T) {
	var warnBuf strings.Builder
	origWarnf := core.Warnf
	core.Warnf = func(format string, args ...any) { warnBuf.WriteString(fmt.Sprintf(format, args...)) }
	t.Cleanup(func() { core.Warnf = origWarnf })

	settings, err := buildCreateSettings("asa-server-1", createParams{adminPassword: "AdminPass123", serverPassword: "ServerPass123", mapName: "TheIsland_WP"}, "", "")
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if settings.mapName != "TheIsland_WP" {
		t.Fatalf("expected mapName TheIsland_WP, got %q", settings.mapName)
	}
	if strings.Contains(warnBuf.String(), "WARNING") {
		t.Fatal("expected no warning for map name with _WP suffix")
	}
}

func setEnv(t *testing.T, key, value string) {
	t.Helper()
	old, ok := os.LookupEnv(key)
	if err := os.Setenv(key, value); err != nil {
		t.Fatalf("set env: %v", err)
	}
	t.Cleanup(func() {
		if ok {
			_ = os.Setenv(key, old)
		} else {
			_ = os.Unsetenv(key)
		}
	})
}
