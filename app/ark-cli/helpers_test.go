package main

// Tests: test infrastructure helpers: stdout/stderr capture, env var override, fake compose/schedule/backup deps

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"ark-cli/internal/core"

	"pkg/ark"
	"pkg/ark/health"
	"pkg/ark/runtime"

	"github.com/compose-spec/compose-go/v2/loader"
	"github.com/compose-spec/compose-go/v2/types"
)

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

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stdout = w
	defer func() {
		os.Stdout = old
	}()
	defer func() {
		if recovered := recover(); recovered != nil {
			_ = w.Close()
			panic(recovered)
		}
	}()

	fn()
	_ = w.Close()

	data, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("read stdout: %v", err)
	}
	return string(bytes.TrimSpace(data))
}

func captureStderr(t *testing.T, fn func()) string {
	t.Helper()
	old := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stderr = w
	defer func() {
		os.Stderr = old
	}()
	defer func() {
		if recovered := recover(); recovered != nil {
			_ = w.Close()
			panic(recovered)
		}
	}()

	fn()
	_ = w.Close()

	data, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("read stderr: %v", err)
	}
	return string(bytes.TrimSpace(data))
}

func cloneAppDepsForTest() *core.AppDeps {
	base := core.DefaultAppDeps()
	current := core.CurrentAppDeps()
	if current != nil {
		core.MergeNonZeroAppDepFields(base, current)
	}
	core.FillDefaults(base)
	return base
}

func withPatchedAppDeps(t *testing.T, patch func(*core.AppDeps), fn func()) {
	t.Helper()
	deps := cloneAppDepsForTest()
	// Save pre-patch pointers for services that depend on patched base fields
	// (Engine, ComposeService). If the test explicitly replaces them, preserve
	// the test's value; otherwise clear so FillDefaults rebuilds them.
	backupBefore := deps.BackupService
	updateBefore := deps.UpdateService
	if patch != nil {
		patch(deps)
	}
	if deps.BackupService == backupBefore {
		deps.BackupService = nil
	}
	if deps.UpdateService == updateBefore {
		deps.UpdateService = nil
	}
	// ScheduleService, HealthChecker, Lifecycle do NOT depend on
	// Engine/ComposeService, so never clear them.
	deps.RuntimeHelper = runtime.RuntimeHelper{}
	core.FillDefaults(deps)
	core.WithAppDeps(deps, fn)
}

func patchAppDepsForTest(t *testing.T, patch func(*core.AppDeps)) {
	t.Helper()
	prev := core.CurrentAppDeps()
	deps := cloneAppDepsForTest()
	backupBefore := deps.BackupService
	updateBefore := deps.UpdateService
	if patch != nil {
		patch(deps)
	}
	if deps.BackupService == backupBefore {
		deps.BackupService = nil
	}
	if deps.UpdateService == updateBefore {
		deps.UpdateService = nil
	}
	deps.RuntimeHelper = runtime.RuntimeHelper{}
	core.FillDefaults(deps)
	core.SetCurrentAppDeps(deps)
	t.Cleanup(func() {
		core.SetCurrentAppDeps(prev)
	})
}

// fakeProject builds an *ark.ComposeProject with the given services map.
// This replaces the old pattern of &ark.ComposeProject{Raw: map[string]interface{}{...}}.
func fakeProject(services types.Services) *ark.ComposeProject {
	return &ark.ComposeProject{
		Project: &types.Project{
			Services: services,
		},
	}
}

// fakeProjectWithVolumes builds an *ark.ComposeProject with services and volumes.
func fakeProjectWithVolumes(services types.Services, volumes types.Volumes) *ark.ComposeProject {
	return &ark.ComposeProject{
		Project: &types.Project{
			Services: services,
			Volumes:  volumes,
		},
	}
}

// emptyServices builds a types.Services map with only service names (empty configs).
func emptyServices(names ...string) types.Services {
	svcs := types.Services{}
	for _, name := range names {
		svcs[name] = types.ServiceConfig{Name: name}
	}
	return svcs
}

// strPtr returns a pointer to the given string (for types.MappingWithEquals).
func strPtr(s string) *string { return &s }

// testEnv builds a types.MappingWithEquals from key=value pairs.
func testEnv(pairs ...string) types.MappingWithEquals {
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

// testPorts builds []types.ServicePortConfig from "published:target/protocol" strings.
// Supports formats: "27015:27015/udp", "127.0.0.1:27015:27015/udp", "27015:27015"
func testPorts(specs ...string) []types.ServicePortConfig {
	var ports []types.ServicePortConfig
	for _, spec := range specs {
		p := parseTestPortSpec(spec)
		ports = append(ports, p)
	}
	return ports
}

func parseTestPortSpec(spec string) types.ServicePortConfig {
	// Handle host_ip prefix: "127.0.0.1:27015:27015/udp"
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

// mockHealthChecker implements health.InstanceHealthChecker for tests.
type mockHealthChecker struct {
	isStartedFn   func(ctx context.Context, composeFile, service string, since *time.Time) (bool, error)
	isReadyFn     func(ctx context.Context, composeFile, service string, since *time.Time) (bool, error)
	waitStartedFn func(ctx context.Context, composeFile, service string, opts health.WaitOptions) error
	waitReadyFn   func(ctx context.Context, composeFile, service string, opts health.WaitOptions) error
	waitStoppedFn func(ctx context.Context, composeFile, service string, opts health.WaitOptions) error
}

func (m *mockHealthChecker) IsStarted(ctx context.Context, composeFile, service string, since *time.Time) (bool, error) {
	if m.isStartedFn != nil {
		return m.isStartedFn(ctx, composeFile, service, since)
	}
	return true, nil
}

func (m *mockHealthChecker) IsReady(ctx context.Context, composeFile, service string, since *time.Time) (bool, error) {
	if m.isReadyFn != nil {
		return m.isReadyFn(ctx, composeFile, service, since)
	}
	return true, nil
}

func (m *mockHealthChecker) WaitForStarted(ctx context.Context, composeFile, service string, opts health.WaitOptions) error {
	if m.waitStartedFn != nil {
		return m.waitStartedFn(ctx, composeFile, service, opts)
	}
	return nil
}

func (m *mockHealthChecker) WaitForReady(ctx context.Context, composeFile, service string, opts health.WaitOptions) error {
	if m.waitReadyFn != nil {
		return m.waitReadyFn(ctx, composeFile, service, opts)
	}
	return nil
}

func (m *mockHealthChecker) WaitForStopped(ctx context.Context, composeFile, service string, opts health.WaitOptions) error {
	if m.waitStoppedFn != nil {
		return m.waitStoppedFn(ctx, composeFile, service, opts)
	}
	return nil
}

// checkTableRow verifies that a table row (space-aligned by tabwriter) contains
// exactly the expected fields in order when split on whitespace.
func checkTableRow(t *testing.T, line string, want []string) {
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

// loadComposeFromPathForTest loads a compose file for test assertions.
// It derives a project name from the directory when the file lacks an
// explicit "name:" field (e.g. files generated by runCreate).
func loadComposeFromPathForTest(path string) (*types.Project, error) {
	dir := filepath.Dir(path)
	projectName := filepath.Base(dir)
	return loader.LoadWithContext(context.Background(), types.ConfigDetails{
		ConfigFiles: []types.ConfigFile{{Filename: path}},
		WorkingDir:  dir,
	}, func(o *loader.Options) {
		o.SetProjectName(projectName, true)
	})
}
