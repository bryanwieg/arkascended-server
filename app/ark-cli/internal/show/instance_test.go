package show

// Tests: show backup, show compose, show instance, show volume-paths, list instances commands

import (
	"ark-cli/internal/core"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"pkg/ark"
	"pkg/ark/arktest"

	"github.com/compose-spec/compose-go/v2/types"
)

func TestRunShowBackupOutputsMetadata(t *testing.T) {
	backupRoot := filepath.Join(t.TempDir(), "backups")
	setEnv(t, "ARK_BACKUP_DIR", backupRoot)
	instanceDir := filepath.Join(backupRoot, "asa1")
	if err := os.MkdirAll(instanceDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	archive := filepath.Join(instanceDir, "asa1-backup-20240201-120000.tar.gz")
	if err := os.WriteFile(archive, []byte("backup-data"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	now := time.Unix(1707000000, 0)
	if err := os.Chtimes(archive, now, now); err != nil {
		t.Fatalf("chtimes: %v", err)
	}

	out := captureStdout(t, func() {
		if err := RunShowBackup(core.CurrentAppDeps(), []string{"--instance", "asa1", "--backup", "latest"}); err != nil {
			t.Fatalf("runShowBackup: %v", err)
		}
	})
	for _, want := range []string{
		"instance: asa1",
		"name: asa1-backup-20240201-120000.tar.gz",
		"format: tar.gz",
		"path: " + archive,
		"size_bytes: 11",
		"timestamp_hint: 20240201-120000",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in output:\n%s", want, out)
		}
	}
}

func TestRunShowComposeOutputsSummary(t *testing.T) {
	composePath := filepath.Join(t.TempDir(), "compose.yaml")
	if err := os.WriteFile(composePath, []byte("services:\n  a: {}\n  b: {}\n"), 0o644); err != nil {
		t.Fatalf("write compose: %v", err)
	}
	out := captureStdout(t, func() {
		withPatchedAppDeps(t, func(d *core.AppDeps) {
			d.ComposeService = &arktest.MockCompose{
				LoadProjectFn: func(_ context.Context, _ string) (*ark.ComposeProject, error) {
					return fakeProjectWithVolumes(
						types.Services{
							"b": {Name: "b", Image: "img-b"},
							"a": {Name: "a", Image: "img-a"},
						},
						types.Volumes{
							"va": types.VolumeConfig{},
						},
					), nil
				},
			}
		}, func() {
			if err := RunShowCompose(core.CurrentAppDeps(), []string{"--compose-file", composePath}); err != nil {
				t.Fatalf("runShowCompose: %v", err)
			}
		})
	})
	for _, want := range []string{
		"compose_file: " + composePath,
		"services: 2",
		"volumes: 1",
		"service_names: a,b",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in output:\n%s", want, out)
		}
	}
}

func TestRunShowVolumePathsOutputsResolvedMountpoints(t *testing.T) {
	composePath := filepath.Join(t.TempDir(), "compose.yaml")
	if err := os.WriteFile(composePath, []byte("services:\n  asa1: {}\n"), 0o644); err != nil {
		t.Fatalf("write compose: %v", err)
	}
	out := captureStdout(t, func() {
		withPatchedAppDeps(t, func(d *core.AppDeps) {
			d.ComposeService = &arktest.MockCompose{
				LoadProjectFn: func(_ context.Context, _ string) (*ark.ComposeProject, error) {
					return fakeProjectWithVolumes(
						types.Services{
							"asa1": {
								Name: "asa1",
								Volumes: []types.ServiceVolumeConfig{
									{Source: "asa1-saved", Target: "/home/ark-user/ark-server/ShooterGame/Saved", Type: "volume"},
									{Source: "ark-cluster-1", Target: "/home/ark-user/ark-server/ShooterGame/Saved/clusters", Type: "volume"},
								},
							},
						},
						types.Volumes{
							"asa1-saved":    types.VolumeConfig{},
							"ark-cluster-1": types.VolumeConfig{},
						},
					), nil
				},
			}
			d.Engine = &arktest.MockEngine{
				VolumeMountpointFn: func(_ context.Context, volume string) (string, bool) {
					switch volume {
					case "myproj_asa1-saved":
						return "/var/lib/docker/volumes/myproj_asa1-saved/_data", true
					case "myproj_ark-cluster-1":
						return "/var/lib/docker/volumes/myproj_ark-cluster-1/_data", true
					default:
						return "", false
					}
				},
			}
			setEnv(t, "COMPOSE_PROJECT_NAME", "myproj")
		}, func() {
			if err := RunShowVolumePaths(core.CurrentAppDeps(), []string{"--compose-file", composePath, "--instance", "asa1"}); err != nil {
				t.Fatalf("runShowVolumePaths: %v", err)
			}
		})
	})
	// Header block
	for _, want := range []string{
		"instance:",
		"asa1",
		"compose_file:",
		composePath,
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in output:\n%s", want, out)
		}
	}
	// Table rows — check each volume's values appear together on a line
	for _, line := range strings.Split(out, "\n") {
		fields := strings.Fields(line)
		if len(fields) > 0 && fields[0] == "asa1-saved" {
			checkTableRow(t, line, []string{
				"asa1-saved",
				"myproj_asa1-saved",
				"/home/ark-user/ark-server/ShooterGame/Saved",
				"/var/lib/docker/volumes/myproj_asa1-saved/_data",
			})
		}
		if len(fields) > 0 && fields[0] == "ark-cluster-1" {
			checkTableRow(t, line, []string{
				"ark-cluster-1",
				"myproj_ark-cluster-1",
				"/home/ark-user/ark-server/ShooterGame/Saved/clusters",
				"/var/lib/docker/volumes/myproj_ark-cluster-1/_data",
			})
		}
	}
	if !strings.Contains(out, "Notice: Directly altering files in a container volume from outside the container can result in corrupted file permissions.") {
		t.Fatalf("missing notice in output:\n%s", out)
	}
}

func TestRunShowInstanceOutputsDetails(t *testing.T) {
	backupRoot := filepath.Join(t.TempDir(), "backups")
	setEnv(t, "ARK_BACKUP_DIR", backupRoot)
	composePath := filepath.Join(t.TempDir(), "compose.yaml")
	if err := os.WriteFile(composePath, []byte("services:\n  asa1: {}\n"), 0o644); err != nil {
		t.Fatalf("write compose: %v", err)
	}

	out := captureStdout(t, func() {
		withPatchedAppDeps(t, func(d *core.AppDeps) {
			d.ComposeService = &arktest.MockCompose{
				LoadProjectFn: func(_ context.Context, _ string) (*ark.ComposeProject, error) {
					return fakeProject(types.Services{
						"asa1": {
							Name:        "asa1",
							Image:       "ark-server:latest",
							Ports:       testPorts("27015:27015/udp"),
							Environment: testEnv("MAP_NAME=TheIsland_WP", "SESSION_NAME=MySession"),
						},
					}), nil
				},
				RunningServicesFn: func(_ context.Context, _ string) (map[string]struct{}, error) {
					return map[string]struct{}{"asa1": {}}, nil
				},
			}
			d.Engine = &arktest.MockEngine{
				ImageLabelsFn: func(_ context.Context, image string) (map[string]string, error) {
					if image == "ark-server:latest" {
						return map[string]string{
							"io.ark.server.version": "1.2.3",
						}, nil
					}
					return map[string]string{}, nil
				},
			}
			d.HostDeps.FetchText = func(_ string) (string, error) {
				return "", errors.New("mock")
			}
		}, func() {
			if err := RunShowInstance(core.CurrentAppDeps(), []string{"--instance", "asa1", "--compose-file", composePath}); err != nil {
				t.Fatalf("RunShowInstance: %v", err)
			}
		})
	})

	for _, want := range []string{
		"name:",
		"status:",
		"image:",
		"version:",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in output:\n%s", want, out)
		}
	}
}

func TestRunShowInstanceStopped(t *testing.T) {
	backupRoot := filepath.Join(t.TempDir(), "backups")
	setEnv(t, "ARK_BACKUP_DIR", backupRoot)
	composePath := filepath.Join(t.TempDir(), "compose.yaml")
	if err := os.WriteFile(composePath, []byte("services:\n  asa1: {}\n"), 0o644); err != nil {
		t.Fatalf("write compose: %v", err)
	}

	out := captureStdout(t, func() {
		withPatchedAppDeps(t, func(d *core.AppDeps) {
			d.ComposeService = &arktest.MockCompose{
				LoadProjectFn: func(_ context.Context, _ string) (*ark.ComposeProject, error) {
					return fakeProject(types.Services{
						"asa1": {
							Name:  "asa1",
							Image: "ark-server:latest",
							Ports: testPorts("27015:27015/udp"),
						},
					}), nil
				},
				RunningServicesFn: func(_ context.Context, _ string) (map[string]struct{}, error) {
					return map[string]struct{}{}, nil
				},
			}
			d.HostDeps.FetchText = func(_ string) (string, error) {
				return "", errors.New("mock")
			}
		}, func() {
			if err := RunShowInstance(core.CurrentAppDeps(), []string{"--instance", "asa1", "--compose-file", composePath}); err != nil {
				t.Fatalf("RunShowInstance: %v", err)
			}
		})
	})

	if !strings.Contains(out, "stopped") {
		t.Fatalf("missing 'stopped' in output:\n%s", out)
	}
}

func TestRunShowInstanceJSON(t *testing.T) {
	backupRoot := filepath.Join(t.TempDir(), "backups")
	setEnv(t, "ARK_BACKUP_DIR", backupRoot)
	composePath := filepath.Join(t.TempDir(), "compose.yaml")
	if err := os.WriteFile(composePath, []byte("services:\n  asa1: {}\n"), 0o644); err != nil {
		t.Fatalf("write compose: %v", err)
	}

	out := captureStdout(t, func() {
		withPatchedAppDeps(t, func(d *core.AppDeps) {
			d.ComposeService = &arktest.MockCompose{
				LoadProjectFn: func(_ context.Context, _ string) (*ark.ComposeProject, error) {
					return fakeProject(types.Services{
						"asa1": {
							Name:        "asa1",
							Image:       "ark-server:latest",
							Ports:       testPorts("27015:27015/udp"),
							Environment: testEnv("MAP_NAME=TheIsland_WP", "SESSION_NAME=MyServer"),
						},
					}), nil
				},
				RunningServicesFn: func(_ context.Context, _ string) (map[string]struct{}, error) {
					return map[string]struct{}{}, nil
				},
			}
			d.HostDeps.FetchText = func(_ string) (string, error) {
				return "", errors.New("mock")
			}
		}, func() {
			if err := RunShowInstance(core.CurrentAppDeps(), []string{"--instance", "asa1", "--compose-file", composePath, "--json"}); err != nil {
				t.Fatalf("RunShowInstance: %v", err)
			}
		})
	})

	var parsed core.InstanceInspectJSON
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("stdout is not valid JSON: %v\nstdout:\n%s", err, out)
	}
	if parsed.Name != "asa1" {
		t.Errorf("Name = %q; want asa1", parsed.Name)
	}
	if parsed.Status == "" {
		t.Errorf("Status is empty; want non-empty")
	}
}

func TestRunListInstancesOutputsSortedSummary(t *testing.T) {
	composePath := filepath.Join(t.TempDir(), "compose.yaml")
	if err := os.WriteFile(composePath, []byte("services:\n  a: {}\n  b: {}\n"), 0o644); err != nil {
		t.Fatalf("write compose: %v", err)
	}

	out := captureStdout(t, func() {
		withPatchedAppDeps(t, func(d *core.AppDeps) {
			d.ComposeService = &arktest.MockCompose{
				LoadProjectFn: func(_ context.Context, _ string) (*ark.ComposeProject, error) {
					return fakeProject(types.Services{
						"b": {
							Name:        "b",
							Image:       "ark-server:latest",
							Ports:       testPorts("27016:27015/udp"),
							Environment: testEnv("MAP_NAME=ScorchedEarth"),
						},
						"a": {
							Name:        "a",
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
			d.RunningServiceSnapshot = func(_ string) (map[string]struct{}, error) {
				return nil, errors.New("snapshot unavailable")
			}
			d.Runtime.RunCommand = func(_ []string, _ ark.CommandOptions) (ark.ExecResult, error) {
				return ark.ExecResult{ExitCode: 1}, nil
			}
			d.HostDeps.FetchText = func(_ string) (string, error) {
				return "", errors.New("mock")
			}
		}, func() {
			if err := RunListInstances(core.CurrentAppDeps(), []string{"--compose-file", composePath}); err != nil {
				t.Fatalf("RunListInstances: %v", err)
			}
		})
	})

	if !strings.Contains(out, "a") {
		t.Fatalf("missing service 'a' in output:\n%s", out)
	}
	if !strings.Contains(out, "b") {
		t.Fatalf("missing service 'b' in output:\n%s", out)
	}
}

func TestRunListInstancesJSON(t *testing.T) {
	composePath := filepath.Join(t.TempDir(), "compose.yaml")
	if err := os.WriteFile(composePath, []byte("services:\n  a: {}\n  b: {}\n"), 0o644); err != nil {
		t.Fatalf("write compose: %v", err)
	}

	out := captureStdout(t, func() {
		withPatchedAppDeps(t, func(d *core.AppDeps) {
			d.ComposeService = &arktest.MockCompose{
				LoadProjectFn: func(_ context.Context, _ string) (*ark.ComposeProject, error) {
					return fakeProject(types.Services{
						"a": {
							Name:        "a",
							Image:       "ark-server:latest",
							Ports:       testPorts("27015:27015/udp"),
							Environment: testEnv("MAP_NAME=TheIsland_WP"),
						},
						"b": {
							Name:        "b",
							Image:       "ark-server:latest",
							Ports:       testPorts("27016:27015/udp"),
							Environment: testEnv("MAP_NAME=ScorchedEarth"),
						},
					}), nil
				},
				RunningServicesFn: func(_ context.Context, _ string) (map[string]struct{}, error) {
					return map[string]struct{}{"a": {}}, nil
				},
			}
			d.RunningServiceSnapshot = func(_ string) (map[string]struct{}, error) {
				return nil, errors.New("snapshot unavailable")
			}
			d.Runtime.RunCommand = func(_ []string, _ ark.CommandOptions) (ark.ExecResult, error) {
				return ark.ExecResult{ExitCode: 1}, nil
			}
			d.HostDeps.FetchText = func(_ string) (string, error) {
				return "", errors.New("mock")
			}
		}, func() {
			if err := RunListInstances(core.CurrentAppDeps(), []string{"--compose-file", composePath, "--json"}); err != nil {
				t.Fatalf("RunListInstances: %v", err)
			}
		})
	})

	var parsed []core.InstanceSummaryJSON
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("stdout is not valid JSON: %v\nstdout:\n%s", err, out)
	}
	if len(parsed) != 2 {
		t.Fatalf("expected 2 entries; got %d\nstdout:\n%s", len(parsed), out)
	}
}

func TestRunListInstancesEmpty(t *testing.T) {
	composePath := filepath.Join(t.TempDir(), "compose.yaml")
	if err := os.WriteFile(composePath, []byte("services: {}\n"), 0o644); err != nil {
		t.Fatalf("write compose: %v", err)
	}

	var err error
	captureStdout(t, func() {
		withPatchedAppDeps(t, func(d *core.AppDeps) {
			d.ComposeService = &arktest.MockCompose{
				LoadProjectFn: func(_ context.Context, _ string) (*ark.ComposeProject, error) {
					return fakeProject(types.Services{}), nil
				},
			}
		}, func() {
			err = RunListInstances(core.CurrentAppDeps(), []string{"--compose-file", composePath})
		})
	})

	if err == nil {
		t.Fatal("expected error for empty services, got nil")
	}
	if !strings.Contains(err.Error(), "no services") {
		t.Fatalf("expected 'no services' error, got: %v", err)
	}
}

func TestRunShowComposeReportsSummary(t *testing.T) {
	composePath := filepath.Join(t.TempDir(), "compose.yaml")
	if err := os.WriteFile(composePath, []byte("services:\n  svc1: {}\n  svc2: {}\n"), 0o644); err != nil {
		t.Fatalf("write compose: %v", err)
	}
	out := captureStdout(t, func() {
		withPatchedAppDeps(t, func(d *core.AppDeps) {
			d.ComposeService = &arktest.MockCompose{
				LoadProjectFn: func(_ context.Context, _ string) (*ark.ComposeProject, error) {
					return fakeProjectWithVolumes(
						types.Services{
							"svc2": {Name: "svc2", Image: "img-2"},
							"svc1": {Name: "svc1", Image: "img-1"},
						},
						types.Volumes{},
					), nil
				},
			}
		}, func() {
			if err := RunShowCompose(core.CurrentAppDeps(), []string{"--compose-file", composePath}); err != nil {
				t.Fatalf("RunShowCompose: %v", err)
			}
		})
	})
	for _, want := range []string{
		"compose_file:",
		"services: 2",
		"svc1",
		"svc2",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in output:\n%s", want, out)
		}
	}
}

func TestRunShowVolumePathsOutputsMountPaths(t *testing.T) {
	composePath := filepath.Join(t.TempDir(), "compose.yaml")
	if err := os.WriteFile(composePath, []byte("services:\n  asa1: {}\n"), 0o644); err != nil {
		t.Fatalf("write compose: %v", err)
	}
	out := captureStdout(t, func() {
		withPatchedAppDeps(t, func(d *core.AppDeps) {
			d.ComposeService = &arktest.MockCompose{
				LoadProjectFn: func(_ context.Context, _ string) (*ark.ComposeProject, error) {
					return fakeProjectWithVolumes(
						types.Services{
							"asa1": {
								Name: "asa1",
								Volumes: []types.ServiceVolumeConfig{
									{Source: "asa1-saved", Target: "/home/ark-user/ark-server/ShooterGame/Saved", Type: "volume"},
								},
							},
						},
						types.Volumes{
							"asa1-saved": types.VolumeConfig{},
						},
					), nil
				},
			}
			d.Engine = &arktest.MockEngine{
				VolumeMountpointFn: func(_ context.Context, volume string) (string, bool) {
					if volume == "myproj_asa1-saved" {
						return "/var/lib/docker/volumes/myproj_asa1-saved/_data", true
					}
					return "", false
				},
			}
			setEnv(t, "COMPOSE_PROJECT_NAME", "myproj")
		}, func() {
			if err := RunShowVolumePaths(core.CurrentAppDeps(), []string{"--compose-file", composePath, "--instance", "asa1"}); err != nil {
				t.Fatalf("RunShowVolumePaths: %v", err)
			}
		})
	})
	for _, want := range []string{
		"asa1-saved",
		"/home/ark-user/ark-server/ShooterGame/Saved",
		"/var/lib/docker/volumes/myproj_asa1-saved/_data",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in output:\n%s", want, out)
		}
	}
}
