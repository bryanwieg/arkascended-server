package scheduler

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"ark-cli/internal/core"

	"pkg/ark"
	"pkg/ark/host"
	"pkg/ark/runtime"
)

// ServiceSettings are the host settings the ark-scheduler compose service is rendered from.
type ServiceSettings struct {
	Image       string // ark-scheduler:<hash>
	ComposeDir  string // absolute
	Runtime     string // "docker" | "podman"
	Socket      string // host socket path (no unix:// prefix)
	User        string // "<uid>:<gid>" on a rootful engine, "" on a rootless one (container root = host user)
	GroupAdd    string // socket gid on a rootful engine, "" on a rootless one
	TZ          string
	BackupDir   string // ARK_BACKUP_DIR when set, else ""
	ComposeFile string // absolute path of the compose file enable scheduler used
	ProjectName string // COMPOSE_PROJECT_NAME when set, else "" (the directory name then matches the host's)
	HealthPort  int    // host port published for the health endpoint (container port 8099); 0 = not published
}

// HealthContainerPort is the scheduler's health endpoint port inside the container.
const HealthContainerPort = 8099

// DetectServiceSettings fills everything but Image; HealthPort starts at HealthContainerPort (enable's default).
func DetectServiceSettings(d *core.AppDeps, composeFile string, project *ark.ComposeProject) (ServiceSettings, error) {
	absFile, err := filepath.Abs(composeFile)
	if err != nil {
		return ServiceSettings{}, err
	}
	composeDir := filepath.Dir(absFile)
	podman := d.Runtime.ContainerRuntime == runtime.RuntimePodman
	socket, err := detectSocket(podman)
	if err != nil {
		return ServiceSettings{}, err
	}
	user, groupAdd, err := userAndGroup(d, podman, socket)
	if err != nil {
		return ServiceSettings{}, err
	}
	return ServiceSettings{
		ComposeDir:  composeDir,
		Runtime:     string(d.Runtime.ContainerRuntime),
		Socket:      socket,
		User:        user,
		GroupAdd:    groupAdd,
		TZ:          detectTZ(project),
		BackupDir:   absBackupDir(composeDir),
		ComposeFile: absFile,
		ProjectName: os.Getenv("COMPOSE_PROJECT_NAME"),
		HealthPort:  HealthContainerPort,
	}, nil
}

// userAndGroup returns the container user and extra group: both "" on a rootless engine, where container
// root is the host user (a host uid there is a sub-UID that cannot write the bind mounts).
func userAndGroup(d *core.AppDeps, podman bool, socket string) (string, string, error) {
	if podman { // ark-cli runs only rootless Podman (app.go), so Podman is not asked
		return "", "", nil
	}
	rootless, err := d.Engine.Rootless(d.Context())
	if err != nil || rootless {
		return "", "", err
	}
	gid, err := socketGID(socket)
	if err != nil {
		return "", "", err
	}
	return fmt.Sprintf("%d:%d", os.Getuid(), os.Getgid()), gid, nil
}

// absBackupDir returns ARK_BACKUP_DIR made absolute against the compose dir, or "" when unset.
func absBackupDir(composeDir string) string {
	dir := os.Getenv("ARK_BACKUP_DIR")
	if dir == "" || filepath.IsAbs(dir) {
		return dir
	}
	return filepath.Join(composeDir, dir)
}

// detectSocket returns the engine's unix socket path on the host.
func detectSocket(podman bool) (string, error) {
	if s := os.Getenv("ARK_SCHEDULER_SOCKET"); s != "" {
		if !filepath.IsAbs(s) {
			return "", fmt.Errorf("Error: ARK_SCHEDULER_SOCKET must be an absolute path, got %q", s)
		}
		return s, nil
	}
	if podman {
		uri, _ := host.HasPodmanService()
		if uri == "" {
			return "", fmt.Errorf("Error: the scheduler needs the rootless Podman socket; none found (start it with 'systemctl --user enable --now podman.socket')")
		}
		return strings.TrimPrefix(uri, "unix://"), nil
	}
	dockerHost := os.Getenv("DOCKER_HOST")
	if dockerHost == "" {
		return "/var/run/docker.sock", nil
	}
	if path, ok := strings.CutPrefix(dockerHost, "unix://"); ok {
		return path, nil
	}
	return "", fmt.Errorf("Error: the scheduler needs the engine's unix socket; DOCKER_HOST is %s; set ARK_SCHEDULER_SOCKET to the socket path on the engine host", dockerHost)
}

// socketGID returns ARK_SCHEDULER_SOCKET_GID when set, else the group owning the socket file.
func socketGID(socket string) (string, error) {
	if v := os.Getenv("ARK_SCHEDULER_SOCKET_GID"); v != "" {
		if gid, err := strconv.Atoi(v); err != nil || gid < 0 {
			return "", fmt.Errorf("Error: ARK_SCHEDULER_SOCKET_GID must be a non-negative integer, got %q", v)
		}
		return v, nil
	}
	info, err := os.Stat(socket)
	if err != nil {
		return "", fmt.Errorf("Error: cannot read the engine socket %s: %w", socket, err)
	}
	return strconv.FormatUint(uint64(info.Sys().(*syscall.Stat_t).Gid), 10), nil
}

// detectTZ returns the TZ of the first instance, else $TZ, else "UTC".
func detectTZ(project *ark.ComposeProject) string {
	if names := project.ServiceNames(); len(names) > 0 {
		if tz := project.Project.Services[names[0]].Environment["TZ"]; tz != nil && *tz != "" {
			return *tz
		}
	}
	if tz := os.Getenv("TZ"); tz != "" {
		return tz
	}
	return "UTC"
}

// RenderService returns a full compose document ("services:\n  ark-scheduler:\n …") for ExtractServiceBlock.
func RenderService(s ServiceSettings) string {
	q := strconv.Quote
	var b strings.Builder
	line := func(format string, args ...any) { fmt.Fprintf(&b, format+"\n", args...) }
	line("services:")
	line("  %s:", ark.SchedulerService)
	line("    # Managed by ark-cli (enable/disable scheduler); edits here are replaced.")
	line("    image: %s", q(s.Image))
	line("    restart: unless-stopped")
	line("    init: true")
	line("    stop_grace_period: 8m # lets a running backup finish; same as the instances")
	if s.User != "" {
		line("    user: %s", q(s.User))
	}
	if s.GroupAdd != "" {
		line("    group_add: [%s]", q(s.GroupAdd))
	}
	line(`    security_opt: ["label=disable"]`)
	line("    working_dir: %s", q(s.ComposeDir))
	line("    environment:")
	env := []string{
		"TZ=" + s.TZ,
		"DOCKER_HOST=unix:///var/run/docker.sock",
		"ARK_CONTAINER_RUNTIME=" + s.Runtime,
		"HOME=" + filepath.Join(s.ComposeDir, ".ark-manage"),
		"TMPDIR=" + filepath.Join(s.ComposeDir, ".ark-tmp"),
		"COMPOSE_FILE=" + s.ComposeFile,
	}
	if s.ProjectName != "" {
		env = append(env, "COMPOSE_PROJECT_NAME="+s.ProjectName)
	}
	if s.BackupDir != "" {
		env = append(env, "ARK_BACKUP_DIR="+s.BackupDir)
	}
	for _, e := range env {
		line("      - %s", q(e))
	}
	line("    volumes:")
	line("      - %s", q(s.Socket+":/var/run/docker.sock"))
	line("      - %s", q(s.ComposeDir+":"+s.ComposeDir))
	if s.BackupDir != "" && !insideDir(s.ComposeDir, s.BackupDir) {
		line("      - %s", q(s.BackupDir+":"+s.BackupDir))
	}
	if s.HealthPort > 0 {
		line("    ports:")
		line("      - %s", q(fmt.Sprintf("%d:%d/tcp", s.HealthPort, HealthContainerPort)))
	}
	line("    labels:")
	line("      %s: %s", ark.RoleLabel, ark.SchedulerRole)
	return b.String()
}

// insideDir reports whether path is dir or below it (the compose dir mount already covers it).
func insideDir(dir, path string) bool {
	rel, err := filepath.Rel(dir, path)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, "../")
}
