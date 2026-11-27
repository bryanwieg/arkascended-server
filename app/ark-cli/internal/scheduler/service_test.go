package scheduler

// Tests: DetectServiceSettings (rootful/rootless, docker/podman, socket and gid overrides, TZ, backup dir)
// and RenderService golden output (with the healthcheck) that loads with compose-go

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"

	"ark-cli/internal/core"
	"ark-cli/internal/testutil"

	"github.com/compose-spec/compose-go/v2/loader"
	"github.com/compose-spec/compose-go/v2/types"

	"pkg/ark"
	"pkg/ark/arktest"
	"pkg/ark/runtime"
)

// clearSchedulerEnv unsets every variable DetectServiceSettings reads.
func clearSchedulerEnv(t *testing.T) {
	t.Helper()
	for _, k := range []string{"DOCKER_HOST", "TZ", "ARK_BACKUP_DIR", "COMPOSE_PROJECT_NAME", "XDG_RUNTIME_DIR", "ARK_SCHEDULER_SOCKET", "ARK_SCHEDULER_SOCKET_GID", "ARK_UPDATE_WEBHOOK_URL"} {
		testutil.SetEnv(t, k, "")
	}
}

func detectDeps(rt runtime.ContainerRuntime, rootless func(context.Context) (bool, error)) *core.AppDeps {
	return &core.AppDeps{
		Engine:  &arktest.MockEngine{RootlessFn: rootless},
		Runtime: core.RuntimeDeps{ContainerRuntime: rt},
	}
}

func rootlessIs(v bool) func(context.Context) (bool, error) {
	return func(context.Context) (bool, error) { return v, nil }
}

func fakeSocket(t *testing.T, path string) string {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	return strconv.FormatUint(uint64(info.Sys().(*syscall.Stat_t).Gid), 10)
}

func projectWithTZ(tz string) *ark.ComposeProject {
	return &ark.ComposeProject{Project: &types.Project{Services: types.Services{
		"asa2":               {Name: "asa2", Labels: types.Labels{ark.PortASALabel: "27015"}},
		"asa1":               {Name: "asa1", Labels: types.Labels{ark.PortASALabel: "27015"}, Environment: types.NewMappingWithEquals([]string{"TZ=" + tz})},
		ark.SchedulerService: {Name: ark.SchedulerService, Labels: types.Labels{ark.RoleLabel: ark.SchedulerRole}},
	}}}
}

func TestDetectRootfulDockerUsesHostUserAndSocketGroup(t *testing.T) {
	clearSchedulerEnv(t)
	dir := t.TempDir()
	socket := filepath.Join(dir, "docker.sock")
	gid := fakeSocket(t, socket)
	testutil.SetEnv(t, "DOCKER_HOST", "unix://"+socket)
	testutil.SetEnv(t, "TZ", "Asia/Tokyo")
	testutil.SetEnv(t, "ARK_BACKUP_DIR", "backups-elsewhere")
	testutil.SetEnv(t, "COMPOSE_PROJECT_NAME", "myark")

	got, err := DetectServiceSettings(detectDeps(runtime.RuntimeDocker, rootlessIs(false)), filepath.Join(dir, "compose.yml"), projectWithTZ("Europe/Berlin"))
	if err != nil {
		t.Fatalf("DetectServiceSettings: %v", err)
	}
	want := ServiceSettings{
		ComposeDir:  dir,
		Runtime:     "docker",
		Socket:      socket,
		User:        strconv.Itoa(os.Getuid()) + ":" + strconv.Itoa(os.Getgid()),
		GroupAdd:    gid,
		TZ:          "Europe/Berlin",
		BackupDir:   filepath.Join(dir, "backups-elsewhere"),
		ComposeFile: filepath.Join(dir, "compose.yml"),
		ProjectName: "myark",
		HealthPort:  HealthContainerPort,
	}
	if got != want {
		t.Fatalf("settings:\n got %+v\nwant %+v", got, want)
	}
}

func TestDetectRootlessDockerHasNoUser(t *testing.T) {
	clearSchedulerEnv(t)
	testutil.SetEnv(t, "TZ", "Asia/Tokyo")
	got, err := DetectServiceSettings(detectDeps(runtime.RuntimeDocker, rootlessIs(true)), "/srv/ark/docker-compose.yaml", nil)
	if err != nil {
		t.Fatalf("DetectServiceSettings: %v", err)
	}
	if got.Socket != "/var/run/docker.sock" || got.User != "" || got.GroupAdd != "" || got.TZ != "Asia/Tokyo" || got.BackupDir != "" {
		t.Fatalf("settings = %+v", got)
	}
}

func TestDetectPodmanIsRootlessWithoutAskingEngine(t *testing.T) {
	clearSchedulerEnv(t)
	xdg := t.TempDir()
	fakeSocket(t, filepath.Join(xdg, "podman", "podman.sock"))
	testutil.SetEnv(t, "XDG_RUNTIME_DIR", xdg)
	asked := func(context.Context) (bool, error) {
		t.Fatal("Rootless asked for podman")
		return false, nil
	}
	got, err := DetectServiceSettings(detectDeps(runtime.RuntimePodman, asked), "/srv/ark/docker-compose.yaml", nil)
	if err != nil {
		t.Fatalf("DetectServiceSettings: %v", err)
	}
	if got.Runtime != "podman" || got.Socket != filepath.Join(xdg, "podman", "podman.sock") || got.User != "" || got.GroupAdd != "" || got.TZ != "UTC" {
		t.Fatalf("settings = %+v", got)
	}
}

func TestDetectPodmanWithoutSocketFails(t *testing.T) {
	clearSchedulerEnv(t)
	testutil.SetEnv(t, "XDG_RUNTIME_DIR", t.TempDir())
	if _, err := DetectServiceSettings(detectDeps(runtime.RuntimePodman, nil), "/srv/ark/docker-compose.yaml", nil); err == nil {
		t.Fatal("want error without a podman socket")
	}
}

func TestDetectOverridesWithTCPDockerHost(t *testing.T) {
	clearSchedulerEnv(t)
	testutil.SetEnv(t, "DOCKER_HOST", "tcp://10.0.0.1:2375")
	testutil.SetEnv(t, "ARK_SCHEDULER_SOCKET", "/run/engine/docker.sock") // does not exist here: no stat
	testutil.SetEnv(t, "ARK_SCHEDULER_SOCKET_GID", "988")
	got, err := DetectServiceSettings(detectDeps(runtime.RuntimeDocker, rootlessIs(false)), "/srv/ark/docker-compose.yaml", nil)
	if err != nil {
		t.Fatalf("DetectServiceSettings: %v", err)
	}
	if got.Socket != "/run/engine/docker.sock" || got.GroupAdd != "988" || got.User == "" {
		t.Fatalf("settings = %+v", got)
	}
}

func TestDetectRejectsBadInputs(t *testing.T) {
	cases := []struct {
		name, want string
		env        map[string]string
	}{
		{"tcp DOCKER_HOST", "Error: the scheduler needs the engine's unix socket; DOCKER_HOST is tcp://10.0.0.1:2375; set ARK_SCHEDULER_SOCKET to the socket path on the engine host",
			map[string]string{"DOCKER_HOST": "tcp://10.0.0.1:2375"}},
		{"relative socket", "ARK_SCHEDULER_SOCKET must be an absolute path",
			map[string]string{"ARK_SCHEDULER_SOCKET": "run/docker.sock"}},
		{"non-numeric gid", "ARK_SCHEDULER_SOCKET_GID must be a non-negative integer",
			map[string]string{"ARK_SCHEDULER_SOCKET": "/run/docker.sock", "ARK_SCHEDULER_SOCKET_GID": "docker"}},
		{"negative gid", "ARK_SCHEDULER_SOCKET_GID must be a non-negative integer",
			map[string]string{"ARK_SCHEDULER_SOCKET": "/run/docker.sock", "ARK_SCHEDULER_SOCKET_GID": "-1"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			clearSchedulerEnv(t)
			for k, v := range tc.env {
				testutil.SetEnv(t, k, v)
			}
			_, err := DetectServiceSettings(detectDeps(runtime.RuntimeDocker, rootlessIs(false)), "/srv/ark/docker-compose.yaml", nil)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestDetectReturnsRootlessError(t *testing.T) {
	clearSchedulerEnv(t)
	boom := errors.New("engine down")
	_, err := DetectServiceSettings(detectDeps(runtime.RuntimeDocker, func(context.Context) (bool, error) { return false, boom }), "/srv/ark/docker-compose.yaml", nil)
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v, want %v", err, boom)
	}
}

const rootfulGolden = `services:
  ark-scheduler:
    # Managed by ark-cli (enable/disable scheduler); edits here are replaced.
    image: "ark-scheduler:0123456789ab"
    restart: unless-stopped
    init: true
    stop_grace_period: 8m # lets a running backup finish; same as the instances
    user: "1000:1000"
    group_add: ["988"]
    security_opt: ["label=disable"]
    working_dir: "/srv/ark"
    environment:
      - "TZ=Europe/Berlin"
      - "DOCKER_HOST=unix:///var/run/docker.sock"
      - "ARK_CONTAINER_RUNTIME=docker"
      - "HOME=/srv/ark/.ark-manage"
      - "TMPDIR=/srv/ark/.ark-tmp"
      - "COMPOSE_FILE=/srv/ark/compose.yml"
      - "COMPOSE_PROJECT_NAME=myark"
      - "ARK_BACKUP_DIR=/mnt/backups"
    volumes:
      - "/var/run/docker.sock:/var/run/docker.sock"
      - "/srv/ark:/srv/ark"
      - "/mnt/backups:/mnt/backups"
    ports:
      - "9000:8099/tcp"
    healthcheck:
      test: ["CMD", "/ark-cli", "check", "scheduler"]
      interval: 1m
      timeout: 30s
      retries: 1
      start_period: 3m
    labels:
      ark.role: scheduler
`

const rootlessGolden = `services:
  ark-scheduler:
    # Managed by ark-cli (enable/disable scheduler); edits here are replaced.
    image: "ark-scheduler:0123456789ab"
    restart: unless-stopped
    init: true
    stop_grace_period: 8m # lets a running backup finish; same as the instances
    security_opt: ["label=disable"]
    working_dir: "/srv/ark"
    environment:
      - "TZ=UTC"
      - "DOCKER_HOST=unix:///var/run/docker.sock"
      - "ARK_CONTAINER_RUNTIME=podman"
      - "HOME=/srv/ark/.ark-manage"
      - "TMPDIR=/srv/ark/.ark-tmp"
      - "COMPOSE_FILE=/srv/ark/docker-compose.yaml"
      - "ARK_BACKUP_DIR=/srv/ark/backups"
    volumes:
      - "/run/user/1000/podman/podman.sock:/var/run/docker.sock"
      - "/srv/ark:/srv/ark"
    healthcheck:
      test: ["CMD", "/ark-cli", "check", "scheduler"]
      interval: 1m
      timeout: 30s
      retries: 1
      start_period: 3m
    labels:
      ark.role: scheduler
`

func TestRenderServiceGolden(t *testing.T) {
	rootful := ServiceSettings{
		Image: "ark-scheduler:0123456789ab", ComposeDir: "/srv/ark", Runtime: "docker", Socket: "/var/run/docker.sock",
		User: "1000:1000", GroupAdd: "988", TZ: "Europe/Berlin", BackupDir: "/mnt/backups",
		ComposeFile: "/srv/ark/compose.yml", ProjectName: "myark", HealthPort: 9000,
	}
	rootless := ServiceSettings{
		Image: "ark-scheduler:0123456789ab", ComposeDir: "/srv/ark", Runtime: "podman", Socket: "/run/user/1000/podman/podman.sock",
		TZ: "UTC", BackupDir: "/srv/ark/backups", ComposeFile: "/srv/ark/docker-compose.yaml",
	}
	for name, tc := range map[string]struct {
		s    ServiceSettings
		want string
	}{"rootful": {rootful, rootfulGolden}, "rootless": {rootless, rootlessGolden}} {
		t.Run(name, func(t *testing.T) {
			got := RenderService(tc.s)
			if got != tc.want {
				t.Fatalf("RenderService:\n%s\nwant:\n%s", got, tc.want)
			}
			if block, err := runtime.ExtractServiceBlock(got); err != nil || !strings.HasPrefix(block, "  "+ark.SchedulerService+":\n") {
				t.Fatalf("ExtractServiceBlock = %q, %v", block, err)
			}
			project := loadRendered(t, got)
			if !project.HasScheduler() || len(project.ServiceNames()) != 0 {
				t.Fatalf("loaded services = %v, HasScheduler = %v", project.Project.ServiceNames(), project.HasScheduler())
			}
			if hc := project.Project.Services[ark.SchedulerService].HealthCheck; hc == nil || strings.Join(hc.Test, " ") != "CMD /ark-cli check scheduler" {
				t.Fatalf("healthcheck = %+v", hc)
			}
		})
	}
}

func TestRenderServiceOmitsBackupDirWhenUnset(t *testing.T) {
	got := RenderService(ServiceSettings{Image: "i", ComposeDir: "/srv/ark", Runtime: "docker", Socket: "/s", TZ: "UTC", ComposeFile: "/srv/ark/c.yml"})
	if strings.Contains(got, "ARK_BACKUP_DIR") || strings.Count(got, `:/srv/ark"`) != 1 || strings.Contains(got, "ports:") {
		t.Fatalf("RenderService:\n%s", got)
	}
}

func TestRenderServicePassesWebhookURL(t *testing.T) {
	base := ServiceSettings{Image: "i", ComposeDir: "/srv/ark", Runtime: "docker", Socket: "/s", TZ: "UTC", ComposeFile: "/srv/ark/c.yml"}
	if got := RenderService(base); strings.Contains(got, "ARK_UPDATE_WEBHOOK_URL") {
		t.Fatalf("unset webhook rendered:\n%s", got)
	}
	const url = "https://hooks.example/x?sig=a$b"
	withURL := base
	withURL.WebhookURL = url
	got := RenderService(withURL)
	if !strings.Contains(got, `- "ARK_UPDATE_WEBHOOK_URL=https://hooks.example/x?sig=a$$b"`) {
		t.Fatalf("webhook not escaped:\n%s", got)
	}
	env := loadRendered(t, got).Project.Services[ark.SchedulerService].Environment["ARK_UPDATE_WEBHOOK_URL"]
	if env == nil || *env != url {
		t.Fatalf("loaded ARK_UPDATE_WEBHOOK_URL = %v, want %q", env, url)
	}
}

// loadRendered loads the rendered document with compose-go, as the engine's compose loader would.
func loadRendered(t *testing.T, doc string) *ark.ComposeProject {
	t.Helper()
	details := types.ConfigDetails{
		WorkingDir:  "/srv/ark",
		ConfigFiles: []types.ConfigFile{{Filename: "/srv/ark/docker-compose.yaml", Content: []byte(doc)}},
	}
	p, err := loader.LoadWithContext(context.Background(), details, func(o *loader.Options) {
		o.SetProjectName("ark", true)
		o.SkipResolveEnvironment = true
	})
	if err != nil {
		t.Fatalf("compose-go load: %v\n%s", err, doc)
	}
	return &ark.ComposeProject{Project: p}
}
