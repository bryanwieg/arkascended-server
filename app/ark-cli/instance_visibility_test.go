package main

// Tests: list instances and show instance commands: sorted output, running-snapshot integration, detail field display

import (
	"ark-cli/internal/core"
	"ark-cli/internal/show"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"pkg/ark"
	"pkg/ark/arktest"

	"github.com/compose-spec/compose-go/v2/types"
)

func TestRunListInstancesOutputsSortedSummary(t *testing.T) {
	out := captureStdout(t, func() {
		withPatchedAppDeps(t, func(d *core.AppDeps) {
			d.ComposeService = &arktest.MockCompose{
				LoadProjectFn: func(_ context.Context, _ string) (*ark.ComposeProject, error) {
					return fakeProject(types.Services{
						"b": {
							Name:        "b",
							Labels:      types.Labels{ark.PortASALabel: "27016"},
							Image:       "ark-server:latest",
							Ports:       testPorts("27016:27016/udp"),
							Environment: testEnv("MAP_NAME=ScorchedEarth"),
						},
						"a": {
							Name:        "a",
							Labels:      types.Labels{ark.PortASALabel: "27015"},
							Image:       "ark-server:latest",
							Ports:       testPorts("27015:27015/udp"),
							Environment: testEnv("MAP_NAME=TheIsland_WP"),
						},
					}), nil
				},
				RunningServicesFn: func(_ context.Context, _ string) (map[string]struct{}, error) {
					return map[string]struct{}{"a": {}}, nil
				},
			}
			d.RunningServiceSnapshot = func(_ string) (map[string]struct{}, error) { return nil, errors.New("snapshot unavailable") }
			d.Runtime.RunCommand = func(_ []string, _ ark.CommandOptions) (ark.ExecResult, error) {
				return ark.ExecResult{ExitCode: 1}, nil
			}
			d.HostDeps.FetchText = func(_ string) (string, error) {
				return "", errors.New("mock")
			}
		}, func() {
			if err := show.RunListInstances(core.CurrentAppDeps(), []string{}); err != nil {
				t.Fatalf("runListInstances: %v", err)
			}
		})
	})

	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 3 {
		t.Fatalf("expected 3 lines, got %d: %q", len(lines), out)
	}
	checkTableRow(t, lines[0], []string{"NAME", "STATUS", "MAP", "VERSION", "ASA_PORT", "INTERNET_REACHABLE", "BACKUP_SCHEDULED", "LAST_BACKUP"})
	checkTableRow(t, lines[1], []string{"a", "running", "TheIsland_WP", "-", "27015", "-", "no", "-"})
	checkTableRow(t, lines[2], []string{"b", "stopped", "ScorchedEarth", "-", "27016", "stopped", "no", "-"})
}

func TestRunListInstancesUsesRunningSnapshotWhenAvailable(t *testing.T) {
	out := captureStdout(t, func() {
		withPatchedAppDeps(t, func(d *core.AppDeps) {
			d.ComposeService = &arktest.MockCompose{
				LoadProjectFn: func(_ context.Context, _ string) (*ark.ComposeProject, error) {
					return fakeProject(types.Services{
						"a": {
							Name:        "a",
							Labels:      types.Labels{ark.PortASALabel: "27015"},
							Ports:       testPorts("27015:27015/udp"),
							Environment: testEnv("MAP_NAME=TheIsland_WP"),
						},
						"b": {
							Name:        "b",
							Labels:      types.Labels{ark.PortASALabel: "27016"},
							Ports:       testPorts("27016:27016/udp"),
							Environment: testEnv("MAP_NAME=ScorchedEarth"),
						},
					}), nil
				},
				RunningServicesFn: func(_ context.Context, _ string) (map[string]struct{}, error) {
					t.Fatal("RunningServicesFn should not be called when running snapshot is available")
					return nil, nil
				},
			}
			d.RunningServiceSnapshot = func(_ string) (map[string]struct{}, error) {
				return map[string]struct{}{"b": {}}, nil
			}
			d.Runtime.RunCommand = func(_ []string, _ ark.CommandOptions) (ark.ExecResult, error) {
				return ark.ExecResult{ExitCode: 1}, nil
			}
			d.HostDeps.FetchText = func(_ string) (string, error) {
				return "", errors.New("mock")
			}
		}, func() {
			if err := show.RunListInstances(core.CurrentAppDeps(), nil); err != nil {
				t.Fatalf("runListInstances: %v", err)
			}
		})
	})
	for _, line := range strings.Split(out, "\n") {
		fields := strings.Fields(line)
		if len(fields) > 0 && fields[0] == "a" {
			checkTableRow(t, line, []string{"a", "stopped", "TheIsland_WP", "-", "27015", "stopped", "no", "-"})
		}
		if len(fields) > 0 && fields[0] == "b" {
			checkTableRow(t, line, []string{"b", "running", "ScorchedEarth", "-", "27016", "-", "no", "-"})
		}
	}
}

func TestRunShowInstanceOutputsDetails(t *testing.T) {
	backupRoot := filepath.Join(t.TempDir(), "backups")
	setEnv(t, "ARK_BACKUP_DIR", backupRoot)
	instanceBackupDir := filepath.Join(backupRoot, "asa1")
	if err := os.MkdirAll(instanceBackupDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	backupFile := filepath.Join(instanceBackupDir, "asa1-backup-20240201-120000.tar.gz")
	if err := os.WriteFile(backupFile, []byte("backup"), 0o644); err != nil {
		t.Fatalf("write backup: %v", err)
	}

	out := captureStdout(t, func() {
		withPatchedAppDeps(t, func(d *core.AppDeps) {
			d.ComposeService = &arktest.MockCompose{
				LoadProjectFn: func(_ context.Context, _ string) (*ark.ComposeProject, error) {
					return fakeProject(types.Services{
						"asa1": {
							Name:        "asa1",
							Labels:      types.Labels{ark.PortASALabel: "27015"},
							Image:       "ark-server:latest",
							Ports:       testPorts("27015:27015/udp", "27020:27020/tcp"),
							Environment: testEnv("MAP_NAME=TheIsland_WP", "SESSION_NAME=MySession"),
						},
					}), nil
				},
				RunningServicesFn: func(_ context.Context, _ string) (map[string]struct{}, error) {
					return map[string]struct{}{}, nil
				},
			}
			d.Runtime.RunCommand = func(_ []string, _ ark.CommandOptions) (ark.ExecResult, error) {
				return ark.ExecResult{ExitCode: 1}, nil
			}
			d.HostDeps.FetchText = func(_ string) (string, error) {
				return "", errors.New("mock")
			}
		}, func() {
			if err := show.RunShowInstance(core.CurrentAppDeps(), []string{"--instance", "asa1"}); err != nil {
				t.Fatalf("runShowInstance: %v", err)
			}
		})
	})

	for _, want := range []string{
		"=== INSTANCE ===",
		"=== COMPOSE ===",
		"=== VOLUMES ===",
		"=== PORT STATUS ===",
		"=== SCHEDULES ===",
		"=== BACKUPS ===",
		"=== Game.ini ===",
		"=== GameUserSettings.ini ===",
		"asa1-backup-20240201-120000.tar.gz",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("output missing %q\n%s", want, out)
		}
	}
}
