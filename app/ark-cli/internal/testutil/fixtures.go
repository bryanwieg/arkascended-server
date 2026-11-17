package testutil

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"pkg/ark"

	"github.com/compose-spec/compose-go/v2/loader"
	"github.com/compose-spec/compose-go/v2/types"
	"github.com/goccy/go-yaml"
)

// FakeProject builds an *ark.ComposeProject with the given services map.
func FakeProject(services types.Services) *ark.ComposeProject {
	return &ark.ComposeProject{
		Project: &types.Project{
			Services: services,
		},
	}
}

// FakeProjectWithVolumes builds an *ark.ComposeProject with services and volumes.
func FakeProjectWithVolumes(services types.Services, volumes types.Volumes) *ark.ComposeProject {
	return &ark.ComposeProject{
		Project: &types.Project{
			Services: services,
			Volumes:  volumes,
		},
	}
}

// EmptyServices builds a types.Services map with only service names (empty configs).
func EmptyServices(names ...string) types.Services {
	svcs := types.Services{}
	for _, name := range names {
		svcs[name] = types.ServiceConfig{Name: name}
	}
	return svcs
}

// StrPtr returns a pointer to the given string (for types.MappingWithEquals).
func StrPtr(s string) *string { return &s }

// TestEnv builds a types.MappingWithEquals from key=value pairs.
func TestEnv(pairs ...string) types.MappingWithEquals {
	env := types.MappingWithEquals{}
	for _, pair := range pairs {
		parts := strings.SplitN(pair, "=", 2)
		if len(parts) == 2 {
			v := parts[1]
			env[parts[0]] = &v
		}
	}
	return env
}

// TestPorts builds []types.ServicePortConfig from "published:target/protocol" strings.
func TestPorts(specs ...string) []types.ServicePortConfig {
	var ports []types.ServicePortConfig
	for _, spec := range specs {
		p := ParseTestPortSpec(spec)
		ports = append(ports, p)
	}
	return ports
}

// ParseTestPortSpec parses a port spec string like "27015:27015/udp".
func ParseTestPortSpec(spec string) types.ServicePortConfig {
	protocol := "tcp"
	if idx := strings.LastIndex(spec, "/"); idx != -1 {
		protocol = spec[idx+1:]
		spec = spec[:idx]
	}

	parts := strings.Split(spec, ":")
	var hostIP, published, target string
	switch len(parts) {
	case 2:
		published, target = parts[0], parts[1]
	case 3:
		hostIP, published, target = parts[0], parts[1], parts[2]
	default:
		published = parts[0]
		target = parts[0]
	}

	var targetInt uint32
	if _, err := fmt.Sscanf(target, "%d", &targetInt); err != nil {
		targetInt = 0
	}

	return types.ServicePortConfig{
		Target:    targetInt,
		Published: published,
		Protocol:  protocol,
		HostIP:    hostIP,
	}
}

// CheckTableRow verifies that a table row contains exactly the expected fields.
func CheckTableRow(t *testing.T, line string, want []string) {
	t.Helper()
	got := strings.Fields(line)
	if len(got) != len(want) {
		t.Fatalf("row field count: got %d %q, want %d %q", len(got), got, len(want), want)
	}
	for i, w := range want {
		if got[i] != w {
			t.Fatalf("row field[%d]: got %q, want %q (line: %q)", i, got[i], w, line)
		}
	}
}

// LoadComposeFromPathForTest loads a compose file for test assertions.
func LoadComposeFromPathForTest(path string) (*types.Project, error) {
	dir := filepath.Dir(path)
	projectName := filepath.Base(dir)
	return loader.LoadWithContext(context.Background(), types.ConfigDetails{
		ConfigFiles: []types.ConfigFile{{Filename: path}},
		WorkingDir:  dir,
	}, func(o *loader.Options) {
		o.SetProjectName(projectName, true)
	})
}

// SaveComposeForTest marshals a compose project to YAML and writes it to the
// given path. This is the test-side counterpart to LoadComposeFromPathForTest,
// enabling round-trip (load → mutate → save) assertions.
func SaveComposeForTest(path string, project *types.Project) error {
	data, err := yaml.Marshal(project)
	if err != nil {
		return fmt.Errorf("failed to marshal compose project: %w", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("failed to write compose file %s: %w", path, err)
	}
	return nil
}

// CreateBackupArchive creates a test backup archive file in the given directory.
func CreateBackupArchive(t *testing.T, backupsDir, filename string) string {
	t.Helper()
	archivePath := filepath.Join(backupsDir, filename)
	if err := os.WriteFile(archivePath, []byte("data"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	return archivePath
}

// WritePreRestoreArtifact simulates the restore container creating
// a pre-restore backup file.
func WritePreRestoreArtifact(t *testing.T, command []string, opts ark.RunOptions) {
	t.Helper()

	hostPreRestoreDir := ""
	for _, v := range opts.Volumes {
		if strings.HasSuffix(v, ":/home/ark-user/maintenance/output") {
			hostPreRestoreDir = strings.TrimSuffix(v, ":/home/ark-user/maintenance/output")
			break
		}
	}

	for i := 0; i < len(command)-1; i++ {
		if command[i] != "--pre-restore" {
			continue
		}
		target := command[i+1]
		if hostPreRestoreDir != "" {
			target = filepath.Join(hostPreRestoreDir, filepath.Base(command[i+1]))
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			t.Fatalf("mkdir pre-restore parent: %v", err)
		}
		if err := os.WriteFile(target, []byte("pre-restore"), 0o644); err != nil {
			t.Fatalf("write pre-restore file: %v", err)
		}
		return
	}
}
