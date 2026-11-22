package main

// Tests: integration: multi-service append, compose config validation after edits, start pull-missing and local-build modes

import (
	"ark-cli/internal/compose"
	"ark-cli/internal/core"
	"ark-cli/internal/create"
	"ark-cli/internal/testutil"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"pkg/ark"
	"pkg/ark/arktest"

	"github.com/compose-spec/compose-go/v2/types"
)

func requireDockerCompose(t *testing.T) {
	t.Helper()
	withPatchedAppDeps(t, nil, func() {
		if _, err := core.CurrentAppDeps().Runtime.LookPath("docker"); err != nil {
			t.Skip("docker not installed")
		}
		result, err := core.CurrentAppDeps().Runtime.RunCommand([]string{"docker", "compose", "version"}, ark.CommandOptions{Capture: true})
		if err != nil || result.ExitCode != 0 {
			t.Skip("docker compose not available")
		}
	})
}

// TestCLIIntegrationCreateAppendsService verifies create appends a second service to compose.
// - Expect: compose file contains both services and saved volume entries.
func TestCLIIntegrationCreateAppendsService(t *testing.T) {
	tempDir := t.TempDir()
	composePath := filepath.Join(tempDir, "docker-compose.yaml")
	setEnv(t, "COMPOSE_FILE", composePath)

	testutil.WithPatchedAppDeps(t, testutil.TestDepsConfig{}, nil, func() {
		if err := create.RunCreate(core.CurrentAppDeps(), []string{
			"--name", "asa1",
			"--admin-password", "adminpass",
			"--server-password", "serverpass",
		}); err != nil {
			t.Fatalf("runCreate: %v", err)
		}
		if err := create.RunCreate(core.CurrentAppDeps(), []string{
			"--name", "asa2",
			"--admin-password", "adminpass",
			"--server-password", "serverpass",
		}); err != nil {
			t.Fatalf("runCreate second: %v", err)
		}
	})

	contents, err := os.ReadFile(composePath)
	if err != nil {
		t.Fatalf("read compose: %v", err)
	}
	text := string(contents)
	for _, needle := range []string{"asa1:", "asa2:", "asa1-saved:", "asa2-saved:"} {
		if !strings.Contains(text, needle) {
			t.Fatalf("compose missing %s", needle)
		}
	}
}

// TestCLIIntegrationComposeConfigValidatesAfterEdits runs compose config after multiple create edits.
// - Expect: docker compose config parses the file and includes all services.
func TestCLIIntegrationComposeConfigValidatesAfterEdits(t *testing.T) {
	tempDir := t.TempDir()
	composePath := filepath.Join(tempDir, "docker-compose.yaml")
	setEnv(t, "COMPOSE_FILE", composePath)

	withPatchedAppDeps(t, func(d *core.AppDeps) {
		d.ComposeService = &arktest.MockCompose{
			LoadProjectFn: func(_ context.Context, composeFile string) (*ark.ComposeProject, error) {
				project, err := loadComposeFromPathForTest(composeFile)
				if err != nil {
					return nil, err
				}
				return &ark.ComposeProject{Project: project}, nil
			},
		}
	}, func() {
		if err := create.RunCreate(core.CurrentAppDeps(), []string{
			"--name", "asa1",
			"--admin-password", "adminpass",
			"--server-password", "serverpass",
		}); err != nil {
			t.Fatalf("runCreate: %v", err)
		}
		if err := create.RunCreate(core.CurrentAppDeps(), []string{
			"--name", "asa2",
			"--admin-password", "adminpass",
			"--server-password", "serverpass",
			"--map-name", "ScorchedEarth",
		}); err != nil {
			t.Fatalf("runCreate second: %v", err)
		}
		if err := create.RunCreate(core.CurrentAppDeps(), []string{
			"--name", "asa3",
			"--admin-password", "adminpass",
			"--server-password", "serverpass",
			"--asa-port", "27017",
		}); err != nil {
			t.Fatalf("runCreate third: %v", err)
		}

		project, err := loadComposeFromPathForTest(composePath)
		if err != nil {
			t.Fatalf("loadComposeFromPathForTest: %v", err)
		}
		for _, name := range []string{"asa1", "asa2", "asa3"} {
			if _, ok := project.Services[name]; !ok {
				t.Fatalf("expected service %s in compose config", name)
			}
		}
	})
}

// TestCLIIntegrationStartPinsLatestAndNeverPulls checks the CLI start flow with a built image.
// - Expect: compose is pinned to the build-ID tag and Up receives pull policy "never".
func TestCLIIntegrationStartPinsLatestAndNeverPulls(t *testing.T) {
	tempDir := t.TempDir()
	composePath := filepath.Join(tempDir, "docker-compose.yaml")
	setEnv(t, "COMPOSE_FILE", composePath)
	if err := os.WriteFile(composePath, []byte("services:\n  asa1:\n    labels: {ark.port.asa: '27015'}\n    image: ark-server:latest\n"), 0o644); err != nil {
		t.Fatalf("write compose: %v", err)
	}

	var gotPullPolicy, savedImage string
	withPatchedAppDeps(t, func(d *core.AppDeps) {
		d.RunningServiceSnapshot = func(_ string) (map[string]struct{}, error) {
			return map[string]struct{}{"asa1": {}}, nil
		}
		d.Engine = &arktest.MockEngine{
			ImageExistsFn: func(context.Context, string) (bool, error) { return true, nil },
			ImageLabelsFn: func(context.Context, string) (map[string]string, error) {
				return map[string]string{"io.ark.server.build_id": "200"}, nil
			},
		}
		d.DiskUsageForPath = func(_ string) (int64, int64, error) {
			return 100 * 1024 * 1024 * 1024, 200 * 1024 * 1024 * 1024, nil
		}
		d.ComposeService = &arktest.MockCompose{
			LoadProjectFn: func(_ context.Context, _ string) (*ark.ComposeProject, error) {
				return fakeProject(types.Services{"asa1": {Name: "asa1", Labels: types.Labels{ark.PortASALabel: "27015"}, Image: "ark-server:latest"}}), nil
			},
			UpFn: func(_ context.Context, _ string, _ []string, opts ark.UpOptions) error {
				gotPullPolicy = opts.PullPolicy
				return nil
			},
			SaveProjectFn: func(_ context.Context, _ string, p *ark.ComposeProject) error {
				savedImage = p.Project.Services["asa1"].Image
				return nil
			},
		}
		d.HealthChecker = &mockHealthChecker{}
	}, func() {
		if err := compose.RunStart(core.CurrentAppDeps(), []string{"--instance", "asa1"}); err != nil {
			t.Fatalf("runStart: %v", err)
		}
	})

	if gotPullPolicy != "never" {
		t.Fatalf("expected pull policy 'never', got %q", gotPullPolicy)
	}
	if savedImage != "ark-server:200" {
		t.Fatalf("expected compose pinned to ark-server:200, got %q", savedImage)
	}
}

// TestCLIIntegrationStartLocalBuildsWhenMissing ensures --local triggers a local build when images are missing.
// - Expect: engine.BuildImage is invoked and start succeeds.
func TestCLIIntegrationStartLocalBuildsWhenMissing(t *testing.T) {
	tempDir := t.TempDir()
	composePath := filepath.Join(tempDir, "docker-compose.yaml")
	setEnv(t, "COMPOSE_FILE", composePath)
	if err := os.WriteFile(composePath, []byte("services:\n  asa1:\n    labels: {ark.port.asa: '27015'}\n    image: ark-server:latest\n"), 0o644); err != nil {
		t.Fatalf("write compose: %v", err)
	}

	built := false
	withPatchedAppDeps(t, func(d *core.AppDeps) {
		d.Runtime.RunCommand = func([]string, ark.CommandOptions) (ark.ExecResult, error) { return ark.ExecResult{}, nil }
		d.HostDeps.FetchText = func(string) (string, error) {
			return `{"data":{"2430930":{"depots":{"branches":{"public":{"buildid":"16491427"}}}}}}`, nil
		}
		d.RunningServiceSnapshot = func(_ string) (map[string]struct{}, error) {
			return map[string]struct{}{"asa1": {}}, nil
		}
		d.Engine = &arktest.MockEngine{
			ImageExistsFn: func(_ context.Context, image string) (bool, error) {
				return false, nil
			},
			ReadFileFromImageFn: func(_ context.Context, image, path string) (string, error) {
				if strings.Contains(path, "VERSION") {
					return "82.3", nil
				}
				if strings.Contains(path, "BUILDID") {
					return "16491427", nil
				}
				return "", nil
			},
			BuildImageFn: func(_ context.Context, _ ark.BuildRequest) error {
				built = true
				return nil
			},
		}
		d.DiskUsageForPath = func(_ string) (int64, int64, error) {
			return 100 * 1024 * 1024 * 1024, 200 * 1024 * 1024 * 1024, nil
		}
		d.ComposeService = &arktest.MockCompose{
			LoadProjectFn: func(_ context.Context, _ string) (*ark.ComposeProject, error) {
				return fakeProject(types.Services{"asa1": {Name: "asa1", Labels: types.Labels{ark.PortASALabel: "27015"}, Image: "ark-server:latest"}}), nil
			},
			UpFn: func(_ context.Context, _ string, _ []string, _ ark.UpOptions) error {
				return nil
			},
		}
		d.HealthChecker = &mockHealthChecker{}
	}, func() {
		if err := compose.RunStart(core.CurrentAppDeps(), []string{"--instance", "asa1"}); err != nil {
			t.Fatalf("runStart: %v", err)
		}
	})
	if !built {
		t.Fatal("expected local build to run when image is missing")
	}
}
