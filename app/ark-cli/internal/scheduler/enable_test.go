package scheduler

// Tests: enable scheduler inserts/refreshes one service block and ups it, keeps the health port, refuses a
// dynamic binary or a bad port before any change, works from another working directory, warns about a
// leftover schedules.yaml; disable removes the block with the stop_grace_period timeout and is a no-op when
// absent.

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"ark-cli/internal/core"
	"ark-cli/internal/testutil"

	"pkg/ark"
	"pkg/ark/arktest"
)

const instanceCompose = `# my ark servers
services:
  asa1:
    labels: {ark.port.asa: '27015'}
    image: ark-server:abc123 # pinned
    environment:
      - TZ=Europe/Berlin
volumes:
  asa1-saved:
`

// enableFixture is a compose dir (the working directory) and the engine/compose calls the commands made.
type enableFixture struct {
	dir, file string
	images    map[string]bool // built images
	builds    int
	ups       [][]string
	stops     []*time.Duration
	removes   [][]string
}

func newEnableFixture(t *testing.T, binary string) *enableFixture {
	t.Helper()
	clearSchedulerEnv(t)
	testutil.SetEnv(t, "COMPOSE_FILE", "")
	testutil.SetEnv(t, "ARK_SCHEDULER_SOCKET", "/var/run/docker.sock")
	dir := filepath.Join(t.TempDir(), "ark") // the test loader names the project after the dir
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	f := &enableFixture{dir: dir, file: filepath.Join(dir, "docker-compose.yaml"), images: map[string]bool{}}
	if err := os.WriteFile(f.file, []byte(instanceCompose), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)
	orig := executablePath
	executablePath = func() (string, error) { return binary, nil }
	t.Cleanup(func() { executablePath = orig })
	return f
}

// run runs a command handler with mocked engine and compose (the compose file is loaded for real).
func (f *enableFixture) run(t *testing.T, handler func(*core.AppDeps, []string) error, args ...string) error {
	t.Helper()
	var err error
	testutil.WithPatchedAppDeps(t, testutil.TestDepsConfig{}, func(d *core.AppDeps) {
		d.Runtime.LookPath = func(string) (string, error) { return "", errors.New("not found") } // never the real crontab
		d.Engine = &arktest.MockEngine{
			RootlessFn:    rootlessIs(true),
			ImageExistsFn: func(_ context.Context, ref string) (bool, error) { return f.images[ref], nil },
			BuildImageFn: func(_ context.Context, req ark.BuildRequest) error {
				f.builds++
				f.images[req.Tag] = true
				return nil
			},
		}
		d.ComposeService = &arktest.MockCompose{
			LoadProjectFn: func(_ context.Context, file string) (*ark.ComposeProject, error) {
				abs, err := filepath.Abs(file)
				if err != nil {
					return nil, err
				}
				project, err := testutil.LoadComposeFromPathForTest(abs)
				if err != nil {
					return nil, err
				}
				return &ark.ComposeProject{Project: project}, nil
			},
			UpFn: func(_ context.Context, _ string, services []string, _ ark.UpOptions) error {
				f.ups = append(f.ups, services)
				return nil
			},
			RunningServicesFn: func(context.Context, string) (map[string]struct{}, error) {
				return map[string]struct{}{ark.SchedulerService: {}}, nil
			},
			AllServicesFn: func(context.Context, string) (map[string]struct{}, error) {
				return map[string]struct{}{ark.SchedulerService: {}}, nil
			},
			StopFn: func(_ context.Context, _ string, _ []string, timeout *time.Duration) error {
				f.stops = append(f.stops, timeout)
				return nil
			},
			RemoveFn: func(_ context.Context, _ string, services []string) error {
				f.removes = append(f.removes, services)
				return nil
			},
		}
	}, func() {
		err = handler(core.CurrentAppDeps(), args)
	})
	return err
}

func (f *enableFixture) text(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile(f.file)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func (f *enableFixture) loaded(t *testing.T) *ark.ComposeProject {
	t.Helper()
	project, err := testutil.LoadComposeFromPathForTest(f.file)
	if err != nil {
		t.Fatalf("compose file does not load: %v\n%s", err, f.text(t))
	}
	return &ark.ComposeProject{Project: project}
}

// staticBinary builds a tiny static ELF to stand in for ark-cli.
func staticBinary(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for name, content := range map[string]string{"go.mod": "module tiny\n\ngo 1.21\n", "main.go": "package main\n\nfunc main() {}\n"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	bin := filepath.Join(dir, "ark-cli")
	cmd := exec.Command("go", "build", "-o", bin, ".")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0", "GOWORK=off", "GOFLAGS=")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}
	return bin
}

func TestEnableSchedulerInsertsServiceAndIsIdempotent(t *testing.T) {
	f := newEnableFixture(t, staticBinary(t))
	if err := f.run(t, RunEnableScheduler); err != nil {
		t.Fatalf("first enable: %v", err)
	}
	first := f.text(t)
	if err := f.run(t, RunEnableScheduler); err != nil {
		t.Fatalf("second enable: %v", err)
	}
	if f.builds != 1 {
		t.Fatalf("builds = %d, want 1 (second enable reuses the image)", f.builds)
	}
	if len(f.ups) != 2 || f.ups[0][0] != ark.SchedulerService || len(f.ups[0]) != 1 {
		t.Fatalf("Up calls = %v, want ark-scheduler twice", f.ups)
	}
	got := f.text(t)
	if got != first {
		t.Fatalf("second enable changed the file:\n%s\n---\n%s", first, got)
	}
	if n := strings.Count(got, "  "+ark.SchedulerService+":"); n != 1 {
		t.Fatalf("%d scheduler blocks:\n%s", n, got)
	}
	if !strings.HasPrefix(got, "# my ark servers\nservices:\n  asa1:\n    labels: {ark.port.asa: '27015'}\n    image: ark-server:abc123 # pinned\n    environment:\n      - TZ=Europe/Berlin\n") {
		t.Fatalf("instance text changed:\n%s", got)
	}
	project := f.loaded(t)
	svc := project.Project.Services[ark.SchedulerService]
	if !project.HasScheduler() || !strings.HasPrefix(svc.Image, "ark-scheduler:") || publishedHealthPort(project) != HealthContainerPort {
		t.Fatalf("scheduler service = %+v", svc)
	}
	if tz := svc.Environment["TZ"]; tz == nil || *tz != "Europe/Berlin" {
		t.Fatalf("scheduler TZ = %v", tz)
	}
	for _, dir := range []string{".ark-manage", ".ark-tmp"} {
		if _, err := os.Stat(filepath.Join(f.dir, dir)); err != nil {
			t.Fatalf("%s not created: %v", dir, err)
		}
	}
	if _, err := os.Stat(filepath.Join(f.dir, ".docker-compose.yaml.ark-save.tmp")); !os.IsNotExist(err) {
		t.Fatalf("temp file left behind: %v", err)
	}
}

func TestEnableSchedulerKeepsPublishedHealthPort(t *testing.T) {
	f := newEnableFixture(t, staticBinary(t))
	if err := f.run(t, RunEnableScheduler, "--health-port", "9000"); err != nil {
		t.Fatal(err)
	}
	if err := f.run(t, RunEnableScheduler); err != nil {
		t.Fatal(err)
	}
	if got := publishedHealthPort(f.loaded(t)); got != 9000 {
		t.Fatalf("health port = %d, want 9000", got)
	}
	if err := f.run(t, RunEnableScheduler, "--health-port", "0"); err != nil {
		t.Fatal(err)
	}
	if err := f.run(t, RunEnableScheduler); err != nil {
		t.Fatal(err)
	}
	if got := publishedHealthPort(f.loaded(t)); got != 0 {
		t.Fatalf("health port = %d, want 0 (unpublished stays unpublished)", got)
	}
}

func TestEnableSchedulerFailsBeforeAnyChange(t *testing.T) {
	dynamic, err := filepath.EvalSymlinks("/bin/sh")
	if err != nil {
		t.Skip("no /bin/sh")
	}
	cases := []struct {
		name, wantErr string
		binary        func(t *testing.T) string
		chdirAway     bool
		args          []string
	}{
		{"dynamic binary", "is dynamically linked", func(*testing.T) string { return dynamic }, false, nil},
		{"port out of range", "Error: --health-port must be between 0 and 65535", staticBinary, false, []string{"--health-port", "70000"}},
		{"negative port", "Error: --health-port must be between 0 and 65535", staticBinary, false, []string{"--health-port", "-1"}},
		{"other working dir", "", staticBinary, true, nil}, // succeeds: schedules and state follow the compose file
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newEnableFixture(t, tc.binary(t))
			args := tc.args
			if tc.chdirAway {
				t.Chdir(t.TempDir())
				args = []string{"--compose-file", f.file}
			}
			err := f.run(t, RunEnableScheduler, args...)
			if tc.wantErr == "" {
				if err != nil || !f.loaded(t).HasScheduler() || len(f.ups) != 1 {
					t.Fatalf("err = %v, ups = %v, want the scheduler enabled\n%s", err, f.ups, f.text(t))
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("err = %v, want %q", err, tc.wantErr)
			}
			if f.text(t) != instanceCompose || f.builds != 0 || len(f.ups) != 0 {
				t.Fatalf("changed something: builds=%d ups=%v\n%s", f.builds, f.ups, f.text(t))
			}
			if _, err := os.Stat(filepath.Join(f.dir, ".ark-manage")); !os.IsNotExist(err) {
				t.Fatalf(".ark-manage created: %v", err)
			}
		})
	}
}

func TestDisableSchedulerRemovesBlockWithGraceTimeout(t *testing.T) {
	f := newEnableFixture(t, staticBinary(t))
	if err := f.run(t, RunEnableScheduler); err != nil {
		t.Fatal(err)
	}
	if err := f.run(t, RunDisableScheduler); err != nil {
		t.Fatalf("disable: %v", err)
	}
	if len(f.stops) != 1 || f.stops[0] != nil {
		t.Fatalf("stop timeouts = %v, want one nil (stop_grace_period)", f.stops)
	}
	if len(f.removes) != 1 || f.removes[0][0] != ark.SchedulerService {
		t.Fatalf("removes = %v", f.removes)
	}
	if f.loaded(t).HasScheduler() {
		t.Fatalf("scheduler still in compose file:\n%s", f.text(t))
	}
	if got := f.text(t); !strings.HasPrefix(got, "# my ark servers\nservices:\n  asa1:\n    labels: {ark.port.asa: '27015'}\n    image: ark-server:abc123 # pinned\n    environment:\n      - TZ=Europe/Berlin\n") {
		t.Fatalf("instance text changed:\n%s", got)
	}
}

func TestDisableSchedulerNoopWhenAbsent(t *testing.T) {
	f := newEnableFixture(t, "/nonexistent")
	if err := f.run(t, RunDisableScheduler); err != nil {
		t.Fatalf("disable: %v", err)
	}
	if f.text(t) != instanceCompose || len(f.stops) != 0 || len(f.removes) != 0 {
		t.Fatalf("disable changed something: stops=%v removes=%v", f.stops, f.removes)
	}
}

func TestEnableSchedulerRefusesComposeLinkOutsideDir(t *testing.T) {
	f := newEnableFixture(t, staticBinary(t))
	outside := filepath.Join(t.TempDir(), "docker-compose.yaml")
	if err := os.WriteFile(outside, []byte(instanceCompose), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(f.file); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, f.file); err != nil {
		t.Fatal(err)
	}
	err := f.run(t, RunEnableScheduler)
	if err == nil || !strings.Contains(err.Error(), "outside") {
		t.Fatalf("err = %v, want the outside-link refusal", err)
	}
	if got, _ := os.ReadFile(outside); string(got) != instanceCompose || f.builds != 0 || len(f.ups) != 0 {
		t.Fatalf("changed something: builds=%d ups=%v\n%s", f.builds, f.ups, got)
	}
}

func TestEnableSchedulerWarnsAboutOldSchedulesFile(t *testing.T) {
	const warning = "Warning: .ark-manage/schedules.yaml is no longer read; schedules live in docker-compose.yaml under x-ark-schedules. Recreate them with 'ark-cli create schedule' and delete the file."
	binary := staticBinary(t)
	for _, leftover := range []bool{true, false} {
		t.Run(fmt.Sprintf("leftover=%v", leftover), func(t *testing.T) {
			f := newEnableFixture(t, binary)
			old := filepath.Join(f.dir, ".ark-manage", "schedules.yaml")
			if leftover {
				if err := os.MkdirAll(filepath.Dir(old), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(old, []byte("schedules: []\n"), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			var warns strings.Builder
			origWarnf := core.Warnf
			core.Warnf = func(format string, args ...any) { fmt.Fprintf(&warns, format, args...) }
			t.Cleanup(func() { core.Warnf = origWarnf })
			if err := f.run(t, RunEnableScheduler); err != nil {
				t.Fatalf("enable: %v", err)
			}
			if got := strings.Contains(warns.String(), warning); got != leftover {
				t.Fatalf("warned = %v, want %v:\n%s", got, leftover, warns.String())
			}
			if data, err := os.ReadFile(old); leftover && (err != nil || string(data) != "schedules: []\n") {
				t.Fatalf("old file changed: %q, %v", data, err)
			}
		})
	}
}
