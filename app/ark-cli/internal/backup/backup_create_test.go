package backup

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ark-cli/internal/core"
	"ark-cli/internal/testutil"

	"pkg/ark"
	"pkg/ark/arktest"
	pkgbackup "pkg/ark/backup"
	"pkg/ark/cmdrunner"

	"github.com/compose-spec/compose-go/v2/types"
)

func TestRunBackupParsesFlagsAndCallsCreateForTarget(t *testing.T) {
	tmp := t.TempDir()
	setEnv(t, "ARK_BACKUP_DIR", tmp)
	setEnv(t, "COMPOSE_FILE", "docker-compose.yaml")

	instance := "asa1"
	var execCalled bool
	composeMock := &arktest.MockCompose{
		LoadProjectFn: func(_ context.Context, _ string) (*ark.ComposeProject, error) {
			return fakeProject(types.Services{instance: {Name: instance, Labels: types.Labels{ark.PortASALabel: "27015"}}}), nil
		},
		RunningServicesFn: func(_ context.Context, _ string) (map[string]struct{}, error) {
			return map[string]struct{}{instance: {}}, nil
		},
		ExecFn: func(_ context.Context, _, _ string, _ []string, _ ark.ExecOptions) (ark.ExecResult, error) {
			execCalled = true
			return ark.ExecResult{ExitCode: 0}, nil
		},
		CopyFromServiceFn: func(_ context.Context, _, _, _, dstPath string) error {
			return nil
		},
	}
	runner := &arktest.MockCommandRunner{
		RunFn: func(_ context.Context, _ []string, _ cmdrunner.RunOptions) (cmdrunner.RunResult, error) {
			return cmdrunner.RunResult{ExitCode: 0, Stdout: "ok"}, nil
		},
	}
	bs := pkgbackup.New(nil, composeMock, nil, runner, slog.Default())
	bs.DiskUsage = func(_ string) (int64, int64, error) {
		return pkgbackup.MinFreeBackupBytes + 1024, 0, nil
	}

	withPatchedAppDeps(t, func(d *core.AppDeps) {
		d.ComposeService = composeMock
		d.BackupService = bs
	}, func() {
		err := RunBackup(core.CurrentAppDeps(), []string{"--instance", instance})
		if err != nil {
			t.Fatalf("RunBackup: %v", err)
		}
		if !execCalled {
			t.Fatal("expected Exec to be called for running service backup")
		}
	})
}

func TestRunBackupMissingInstance(t *testing.T) {
	tmp := t.TempDir()
	setEnv(t, "ARK_BACKUP_DIR", tmp)
	setEnv(t, "COMPOSE_FILE", "docker-compose.yaml")

	withPatchedAppDeps(t, func(d *core.AppDeps) {
		d.ComposeService = &arktest.MockCompose{
			LoadProjectFn: func(_ context.Context, _ string) (*ark.ComposeProject, error) {
				return fakeProject(types.Services{"other": {Name: "other", Labels: types.Labels{ark.PortASALabel: "27015"}}}), nil
			},
		}
	}, func() {
		err := RunBackup(core.CurrentAppDeps(), []string{"--instance", "asa1"})
		if err == nil {
			t.Fatal("expected error for missing instance")
		}
		if !strings.Contains(err.Error(), "compose service not found") {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}

func TestRunBackupCustomName(t *testing.T) {
	req, err := ParseBackupCreateCommandRequest([]string{"--instance", "asa1", "--name", "manual-name"})
	if err != nil {
		t.Fatalf("ParseBackupCreateCommandRequest: %v", err)
	}
	if req.Name != "manual-name" {
		t.Fatalf("expected name 'manual-name', got %q", req.Name)
	}
}

func TestRunBackupInvalidFormat(t *testing.T) {
	err := RunBackup(nil, []string{"--instance", "asa1", "--format", "invalid"})
	if err == nil {
		t.Fatal("expected error for invalid format")
	}
	if !strings.Contains(err.Error(), pkgbackup.FormatValidationError) {
		t.Fatalf("unexpected error: %v", err)
	}
}

// clusterServices returns services that all mount ark-cluster-1 at the cluster path (none when withCluster is false).
func clusterServices(withCluster bool, names ...string) types.Services {
	services := types.Services{}
	for _, name := range names {
		svc := types.ServiceConfig{Name: name, Labels: types.Labels{ark.PortASALabel: "27015"}}
		if withCluster {
			svc.Volumes = []types.ServiceVolumeConfig{{Type: "volume", Source: "ark-cluster-1", Target: pkgbackup.ClusterMountPath}}
		}
		services[name] = svc
	}
	return services
}

// clusterBackupDeps wires a compose mock whose one-off containers write the requested archive and record their command.
func clusterBackupDeps(t *testing.T, services types.Services, commands *[][]string) func(*core.AppDeps) {
	t.Helper()
	composeMock := &arktest.MockCompose{
		LoadProjectFn: func(_ context.Context, _ string) (*ark.ComposeProject, error) {
			return fakeProject(services), nil
		},
		RunningServicesFn: func(_ context.Context, _ string) (map[string]struct{}, error) {
			return map[string]struct{}{}, nil
		},
		RunOneOffDetachedFn: func(context.Context, string, string, string, []string, ark.RunOptions) error {
			return nil
		},
		RunOneOffFn: func(_ context.Context, _, _ string, command []string, opts ark.RunOptions) (ark.ExecResult, error) {
			*commands = append(*commands, command)
			hostDir := strings.TrimSuffix(opts.Volumes[0], ":/home/ark-user/maintenance/output")
			if err := os.WriteFile(filepath.Join(hostDir, filepath.Base(command[len(command)-1])), []byte("x"), 0o644); err != nil {
				t.Fatal(err)
			}
			return ark.ExecResult{}, nil
		},
	}
	bs := pkgbackup.New(&arktest.MockEngine{}, composeMock, nil, &arktest.MockCommandRunner{}, slog.Default())
	return func(d *core.AppDeps) {
		d.ComposeService = composeMock
		d.BackupService = bs
	}
}

func TestRunBackupCluster(t *testing.T) {
	tmp := t.TempDir()
	setEnv(t, "ARK_BACKUP_DIR", tmp)
	setEnv(t, "COMPOSE_FILE", "docker-compose.yaml")
	var commands [][]string
	withPatchedAppDeps(t, clusterBackupDeps(t, clusterServices(true, "asa1", "asa2"), &commands), func() {
		if err := RunBackup(core.CurrentAppDeps(), []string{"--instance", "cluster"}); err != nil {
			t.Fatalf("RunBackup: %v", err)
		}
	})
	if len(commands) != 1 || !strings.HasSuffix(commands[0][1], "ark-cluster-backup.py") {
		t.Fatalf("commands = %v", commands)
	}
	if archives, _ := pkgbackup.ListArchives(filepath.Join(tmp, "cluster")); len(archives) != 1 {
		t.Fatalf("archives = %v", archives)
	}
}

func TestRunBackupAllIncludesCluster(t *testing.T) {
	setEnv(t, "ARK_BACKUP_DIR", t.TempDir())
	setEnv(t, "COMPOSE_FILE", "docker-compose.yaml")
	var commands [][]string
	withPatchedAppDeps(t, clusterBackupDeps(t, clusterServices(true, "asa1"), &commands), func() {
		out := testutil.CaptureStdout(t, func() {
			// The stopped instance's helper backup is mocked out, so its row fails; only the cluster row matters here.
			_ = RunBackup(core.CurrentAppDeps(), []string{"--instance", "all", "--backup-wait-timeout", "1ms"})
		})
		if !strings.Contains(out, "Backup result cluster: OK") {
			t.Fatalf("output = %q", out)
		}
	})
}

func TestRunBackupAllWithoutClusterVolume(t *testing.T) {
	setEnv(t, "ARK_BACKUP_DIR", t.TempDir())
	setEnv(t, "COMPOSE_FILE", "docker-compose.yaml")
	var commands [][]string
	withPatchedAppDeps(t, clusterBackupDeps(t, clusterServices(false, "asa1"), &commands), func() {
		out := testutil.CaptureStdout(t, func() {
			_ = RunBackup(core.CurrentAppDeps(), []string{"--instance", "all", "--backup-wait-timeout", "1ms"})
		})
		if strings.Contains(out, "cluster") {
			t.Fatalf("unexpected cluster row: %q", out)
		}
	})
	if len(commands) != 0 {
		t.Fatalf("cluster backup ran: %v", commands)
	}
}
