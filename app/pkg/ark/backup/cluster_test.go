package backup

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/compose-spec/compose-go/v2/types"

	"pkg/ark"
	"pkg/ark/arktest"
)

func clusterProject(mounts map[string]string) *ark.ComposeProject {
	services := types.Services{}
	for name, source := range mounts {
		volumes := []types.ServiceVolumeConfig{{Type: "volume", Source: name + "-saved", Target: "/home/ark-user/ark-server/ShooterGame/Saved"}}
		if source != "" {
			volumes = append(volumes, types.ServiceVolumeConfig{Type: "volume", Source: source, Target: ClusterMountPath})
		}
		services[name] = types.ServiceConfig{Name: name, Volumes: volumes}
	}
	return &ark.ComposeProject{Project: &types.Project{Services: services}}
}

func TestResolveClusterVolume(t *testing.T) {
	cv, err := ResolveClusterVolume(clusterProject(map[string]string{"b": "ark-cluster-1", "a": "ark-cluster-1", "c": ""}))
	if err != nil || cv.Source != "ark-cluster-1" || !slices.Equal(cv.Services, []string{"a", "b"}) {
		t.Fatalf("got %+v, %v", cv, err)
	}

	if _, err := ResolveClusterVolume(clusterProject(map[string]string{"a": ""})); !errors.Is(err, ErrNoClusterVolume) {
		t.Fatalf("no mount: got %v", err)
	}

	_, err = ResolveClusterVolume(clusterProject(map[string]string{"a": "c2", "b": "c1"}))
	if err == nil || !strings.Contains(err.Error(), "(c1, c2)") {
		t.Fatalf("two sources: got %v", err)
	}

	_, err = ResolveClusterVolume(clusterProject(map[string]string{"cluster": "ark-cluster-1"}))
	if err == nil || !strings.Contains(err.Error(), "reserved") {
		t.Fatalf("reserved name: got %v", err)
	}

	bind := &ark.ComposeProject{Project: &types.Project{Services: types.Services{
		"a": {Name: "a", Volumes: []types.ServiceVolumeConfig{{Type: "bind", Source: "/srv/cluster", Target: ClusterMountPath}}},
	}}}
	if cv, err := ResolveClusterVolume(bind); err != nil || cv.Source != "/srv/cluster" {
		t.Fatalf("bind mount: got %+v, %v", cv, err)
	}
}

// hostOutputDir returns the host side of the bind mount at the container output directory.
func hostOutputDir(t *testing.T, opts ark.RunOptions) string {
	t.Helper()
	for _, v := range opts.Volumes {
		if dir, ok := strings.CutSuffix(v, ":"+containerOutputDir); ok {
			return dir
		}
	}
	t.Fatalf("no output mount in %v", opts.Volumes)
	return ""
}

// writeArchiveFromCommand simulates the container writing the file named by --output / --pre-restore.
func writeArchiveFromCommand(t *testing.T, flag string, command []string, opts ark.RunOptions) {
	t.Helper()
	i := slices.Index(command, flag)
	if i < 0 {
		t.Fatalf("%s missing from %v", flag, command)
	}
	if err := os.WriteFile(filepath.Join(hostOutputDir(t, opts), filepath.Base(command[i+1])), []byte("archive"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func clusterCreateRequest(root string) CreateRequest {
	return CreateRequest{
		ComposeFile: "compose.yaml",
		BackupRoot:  root,
		Format:      "tar.gz",
		Timestamp:   "20260101-120000",
		PID:         1,
		LockTimeout: time.Second,
	}
}

var testCluster = ClusterVolume{Source: "ark-cluster-1", Services: []string{"a", "b"}}

func TestCreateClusterWritesArchive(t *testing.T) {
	root := t.TempDir()
	var gotService string
	var gotOpts ark.RunOptions
	compose := &arktest.MockCompose{
		RunOneOffFn: func(_ context.Context, _, service string, command []string, opts ark.RunOptions) (ark.ExecResult, error) {
			gotService, gotOpts = service, opts
			if !strings.HasSuffix(command[1], "ark-cluster-backup.py") {
				t.Fatalf("command = %v", command)
			}
			writeArchiveFromCommand(t, "--output", command, opts)
			return ark.ExecResult{}, nil
		},
	}
	result, err := newTestService(compose, &arktest.MockEngine{}).CreateCluster(context.Background(), clusterCreateRequest(root), testCluster)
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(root, "cluster", "cluster-backup-20260101-120000.tar.gz")
	if result.ArchivePath != want {
		t.Fatalf("archive = %s, want %s", result.ArchivePath, want)
	}
	if gotService != "a" || gotOpts.User != "3000:3000" || !gotOpts.Remove {
		t.Fatalf("service %q opts %+v", gotService, gotOpts)
	}
}

func TestCreateClusterHonoursDest(t *testing.T) {
	dest := filepath.Join(t.TempDir(), "elsewhere")
	compose := &arktest.MockCompose{
		RunOneOffFn: func(_ context.Context, _, _ string, command []string, opts ark.RunOptions) (ark.ExecResult, error) {
			writeArchiveFromCommand(t, "--output", command, opts)
			return ark.ExecResult{}, nil
		},
	}
	req := clusterCreateRequest(t.TempDir())
	req.DestOverride = dest
	result, err := newTestService(compose, &arktest.MockEngine{}).CreateCluster(context.Background(), req, testCluster)
	if err != nil || filepath.Dir(result.ArchivePath) != dest {
		t.Fatalf("got %+v, %v", result, err)
	}
}

func TestCreateClusterFailures(t *testing.T) {
	for name, exitCode := range map[string]int{"non-zero exit": 1, "no archive": 0} {
		t.Run(name, func(t *testing.T) {
			compose := &arktest.MockCompose{
				RunOneOffFn: func(context.Context, string, string, []string, ark.RunOptions) (ark.ExecResult, error) {
					return ark.ExecResult{ExitCode: exitCode}, nil
				},
			}
			_, err := newTestService(compose, &arktest.MockEngine{}).CreateCluster(context.Background(), clusterCreateRequest(t.TempDir()), testCluster)
			if err == nil {
				t.Fatal("expected an error")
			}
		})
	}
}

func holdClusterLock(t *testing.T, root string) {
	t.Helper()
	dir := filepath.Join(root, ClusterTarget)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	lock := filepath.Join(dir, ".cluster-backup.lock")
	if err := os.WriteFile(lock, []byte(fmt.Sprintf("pid=%d started=now\n", os.Getpid())), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestCreateClusterRespectsLock(t *testing.T) {
	root := t.TempDir()
	holdClusterLock(t, root)
	compose := &arktest.MockCompose{
		RunOneOffFn: func(context.Context, string, string, []string, ark.RunOptions) (ark.ExecResult, error) {
			t.Fatal("RunOneOff called while the lock was held")
			return ark.ExecResult{}, nil
		},
	}
	req := clusterCreateRequest(root)
	req.LockTimeout = 0
	if _, err := newTestService(compose, &arktest.MockEngine{}).CreateCluster(context.Background(), req, testCluster); err == nil || !strings.Contains(err.Error(), "lock") {
		t.Fatalf("got %v", err)
	}
}

func TestCreateClusterAppliesRetention(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, ClusterTarget)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, ts := range []string{"20250101-000000", "20250102-000000", "20250103-000000"} {
		if err := os.WriteFile(filepath.Join(dir, "cluster-backup-"+ts+".tar.gz"), []byte("old"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	compose := &arktest.MockCompose{
		RunOneOffFn: func(_ context.Context, _, _ string, command []string, opts ark.RunOptions) (ark.ExecResult, error) {
			writeArchiveFromCommand(t, "--output", command, opts)
			return ark.ExecResult{}, nil
		},
	}
	s := newTestService(compose, &arktest.MockEngine{})
	s.DiskUsage = func(string) (int64, int64, error) { return 100e9, 200e9, nil }
	req := clusterCreateRequest(root)
	req.Retention = RetentionPolicy{RetainCount: 2}
	if _, err := s.CreateCluster(context.Background(), req, testCluster); err != nil {
		t.Fatal(err)
	}
	left, _ := ListArchives(dir)
	if len(left) != 2 || filepath.Base(left[0]) != "cluster-backup-20260101-120000.tar.gz" {
		t.Fatalf("left = %v", left)
	}
}

func clusterRestoreRequest(t *testing.T) RestoreRequest {
	t.Helper()
	root := t.TempDir()
	dir := filepath.Join(root, ClusterTarget)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	archive := filepath.Join(dir, "cluster-backup-20260101-120000.tar.gz")
	if err := os.WriteFile(archive, []byte("archive"), 0o644); err != nil {
		t.Fatal(err)
	}
	return RestoreRequest{ComposeFile: "compose.yaml", ArchivePath: archive, BackupRoot: root}
}

func runningCompose(running ...string) *arktest.MockCompose {
	return &arktest.MockCompose{
		RunningServicesFn: func(context.Context, string) (map[string]struct{}, error) {
			out := map[string]struct{}{}
			for _, name := range running {
				out[name] = struct{}{}
			}
			return out, nil
		},
	}
}

func TestRestoreClusterRunsWhenUsersStopped(t *testing.T) {
	req := clusterRestoreRequest(t)
	compose := runningCompose("unrelated")
	var gotService string
	var gotCommand []string
	var gotOpts ark.RunOptions
	compose.RunOneOffFn = func(_ context.Context, _, service string, command []string, opts ark.RunOptions) (ark.ExecResult, error) {
		gotService, gotCommand, gotOpts = service, command, opts
		writeArchiveFromCommand(t, "--pre-restore", command, opts)
		return ark.ExecResult{}, nil
	}
	result, err := newTestService(compose, &arktest.MockEngine{}).RestoreCluster(context.Background(), req, testCluster)
	if err != nil {
		t.Fatal(err)
	}
	if gotService != "a" || gotCommand[len(gotCommand)-1] != "--cluster" {
		t.Fatalf("service %q command %v", gotService, gotCommand)
	}
	if !strings.HasSuffix(gotOpts.Volumes[0], ":ro") {
		t.Fatalf("archive not mounted read-only: %v", gotOpts.Volumes)
	}
	if !strings.HasPrefix(filepath.Base(result.PreRestorePath), "cluster-pre-restore-") || filepath.Dir(result.PreRestorePath) != filepath.Dir(req.ArchivePath) {
		t.Fatalf("pre-restore = %s", result.PreRestorePath)
	}
}

func TestRestoreClusterRefusesWhileAUserRuns(t *testing.T) {
	compose := runningCompose("b")
	compose.RunOneOffFn = func(context.Context, string, string, []string, ark.RunOptions) (ark.ExecResult, error) {
		t.Fatal("RunOneOff called while an instance was running")
		return ark.ExecResult{}, nil
	}
	_, err := newTestService(compose, &arktest.MockEngine{}).RestoreCluster(context.Background(), clusterRestoreRequest(t), testCluster)
	if err == nil || !strings.Contains(err.Error(), "restoring it: b.") {
		t.Fatalf("got %v", err)
	}
}

func TestRestoreClusterRespectsLock(t *testing.T) {
	req := clusterRestoreRequest(t)
	holdClusterLock(t, req.BackupRoot)
	_, err := newTestService(runningCompose(), &arktest.MockEngine{}).RestoreCluster(context.Background(), req, testCluster)
	if err == nil || !strings.Contains(err.Error(), "lock") {
		t.Fatalf("got %v", err)
	}
}

func TestRestoreClusterReportsContainerFailure(t *testing.T) {
	compose := runningCompose()
	compose.RunOneOffFn = func(context.Context, string, string, []string, ark.RunOptions) (ark.ExecResult, error) {
		return ark.ExecResult{ExitCode: 1}, nil
	}
	_, err := newTestService(compose, &arktest.MockEngine{}).RestoreCluster(context.Background(), clusterRestoreRequest(t), testCluster)
	if err == nil || !strings.Contains(err.Error(), "Error: restore failed:") {
		t.Fatalf("got %v", err)
	}
}
