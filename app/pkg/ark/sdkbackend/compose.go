package sdkbackend

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/compose-spec/compose-go/v2/types"
	"github.com/docker/cli/cli"
	"github.com/docker/cli/cli/command"
	cliflags "github.com/docker/cli/cli/flags"
	"github.com/docker/compose/v5/pkg/api"
	"github.com/docker/compose/v5/pkg/compose"
	"github.com/spf13/pflag"

	"pkg/ark"
)

// SDKCompose implements ark.ComposeService using the Docker Compose SDK.
type SDKCompose struct {
	svc       api.Compose
	dockerCli command.Cli
	logger    *slog.Logger

	mu       sync.Mutex
	projects map[string]*projectCache // keyed by absolute compose file path
}

type projectCache struct {
	name string
}

// NewSDKCompose creates a ComposeService backed by the Docker Compose SDK.
// The client auto-discovers the Docker socket from DOCKER_HOST or the default path.
func NewSDKCompose(logger *slog.Logger) (*SDKCompose, error) {
	return newSDKCompose(logger, "")
}

// NewSDKComposeWithSocket creates a ComposeService backed by the Docker Compose SDK,
// connecting to the specified socket path (e.g., "unix:///run/podman/podman.sock").
func NewSDKComposeWithSocket(logger *slog.Logger, socketPath string) (*SDKCompose, error) {
	return newSDKCompose(logger, socketPath)
}

// newSDKCompose is the internal implementation of NewSDKCompose and NewSDKComposeWithSocket.
func newSDKCompose(logger *slog.Logger, socketPath string) (*SDKCompose, error) {
	// If a custom socket path is specified, set DOCKER_HOST so the CLI uses it.
	if socketPath != "" {
		if err := os.Setenv("DOCKER_HOST", socketPath); err != nil {
			return nil, fmt.Errorf("failed to set DOCKER_HOST: %w", err)
		}
	}
	dockerCli, err := command.NewDockerCli()
	if err != nil {
		return nil, fmt.Errorf("docker CLI init: %w", err)
	}
	// Register the client flags so their defaults pick up DOCKER_TLS_VERIFY and
	// DOCKER_CERT_PATH, as the docker CLI does; a bare NewClientOptions() talks
	// plain HTTP to TLS daemons.
	clientOpts := cliflags.NewClientOptions()
	flags := pflag.NewFlagSet("ark", pflag.ContinueOnError)
	clientOpts.InstallFlags(flags)
	clientOpts.SetDefaultOptions(flags)
	if err := dockerCli.Initialize(clientOpts); err != nil {
		return nil, fmt.Errorf("docker CLI initialize: %w", err)
	}
	svc, err := compose.NewComposeService(dockerCli,
		compose.WithOutputStream(io.Discard),
		compose.WithErrorStream(os.Stderr),
	)
	if err != nil {
		return nil, fmt.Errorf("compose service: %w", err)
	}
	return &SDKCompose{
		svc:       svc,
		dockerCli: dockerCli,
		logger:    logger,
		projects:  make(map[string]*projectCache),
	}, nil
}

// loadProject loads the compose project, caching the project name.
func (c *SDKCompose) loadProject(ctx context.Context, composeFile string) (*api.ProjectLoadOptions, string, error) {
	opts := api.ProjectLoadOptions{ConfigPaths: []string{composeFile}}

	c.mu.Lock()
	cached, ok := c.projects[composeFile]
	c.mu.Unlock()
	if ok {
		return &opts, cached.name, nil
	}

	project, err := c.svc.LoadProject(ctx, opts)
	if err != nil {
		return nil, "", fmt.Errorf("load project %q: %w", composeFile, err)
	}

	c.mu.Lock()
	c.projects[composeFile] = &projectCache{name: project.Name}
	c.mu.Unlock()

	return &opts, project.Name, nil
}

// --- Project loading ---

func (c *SDKCompose) LoadProject(ctx context.Context, composeFile string) (*ark.ComposeProject, error) {
	c.logger.Debug("LoadProject", "file", composeFile)
	project, err := c.svc.LoadProject(ctx, api.ProjectLoadOptions{ConfigPaths: []string{composeFile}})
	if err != nil {
		return nil, fmt.Errorf("load project %q: %w", composeFile, err)
	}

	c.mu.Lock()
	c.projects[composeFile] = &projectCache{name: project.Name}
	c.mu.Unlock()

	return &ark.ComposeProject{Project: project}, nil
}

// SaveProject writes the image, environment, label and port changes in project back to
// composeFile as line edits, keeping everything else in the file byte for byte (see compose_save.go).
// The result is loaded and compared with project before the file is replaced.
func (c *SDKCompose) SaveProject(ctx context.Context, composeFile string, project *ark.ComposeProject) error {
	text, err := os.ReadFile(composeFile)
	if err != nil {
		return fmt.Errorf("failed to read compose file %s: %w", composeFile, err)
	}
	before, err := c.LoadProject(ctx, composeFile)
	if err != nil {
		return err
	}
	patched, err := patchComposeText(string(text), before.Project, project.Project)
	if err != nil {
		return fmt.Errorf("cannot update %s without rewriting it: %w", composeFile, err)
	}
	if patched == string(text) {
		return nil
	}

	// The check copy sits next to the original so .env, env_file and the project name resolve the same.
	check := filepath.Join(filepath.Dir(composeFile), "."+filepath.Base(composeFile)+".ark-save.tmp")
	if err := os.WriteFile(check, []byte(patched), 0o600); err != nil { //nolint:gosec // G703: the path is the user's own --compose-file
		return fmt.Errorf("failed to write %s: %w", check, err)
	}
	defer os.Remove(check)
	saved, err := c.svc.LoadProject(ctx, api.ProjectLoadOptions{ConfigPaths: []string{check}})
	if err != nil {
		return fmt.Errorf("edited compose file does not load, %s left unchanged: %w", composeFile, err)
	}
	if err := verifySaved(project.Project, saved); err != nil {
		return fmt.Errorf("cannot update %s without rewriting it: %w; edit it by hand", composeFile, err)
	}
	if err := os.WriteFile(composeFile, []byte(patched), 0o644); err != nil { //nolint:gosec // G703: the path is the user's own --compose-file
		return fmt.Errorf("failed to write compose file %s: %w", composeFile, err)
	}
	return nil
}

// --- Lifecycle ---

func (c *SDKCompose) Up(ctx context.Context, composeFile string, services []string, opts ark.UpOptions) error {
	c.logger.Debug("Up", "file", composeFile, "services", services)
	project, err := c.svc.LoadProject(ctx, api.ProjectLoadOptions{ConfigPaths: []string{composeFile}})
	if err != nil {
		return fmt.Errorf("load project for up: %w", err)
	}
	// compose's start step ignores StartOptions.Services and starts every
	// service in StartOptions.Project (or every created container when that is
	// nil), so narrow the project first, as `docker compose up <svc>` does.
	if len(services) > 0 {
		if project, err = project.WithSelectedServices(services); err != nil {
			return fmt.Errorf("select services for up: %w", err)
		}
	}
	return c.svc.Up(ctx, project, api.UpOptions{
		Create: api.CreateOptions{Services: services},
		Start:  api.StartOptions{Project: project, Services: services},
	})
}

func (c *SDKCompose) Stop(ctx context.Context, composeFile string, services []string, timeout *time.Duration) error {
	c.logger.Debug("Stop", "file", composeFile, "services", services)
	_, name, err := c.loadProject(ctx, composeFile)
	if err != nil {
		return err
	}
	return c.svc.Stop(ctx, name, api.StopOptions{
		Timeout:  timeout,
		Services: services,
	})
}

func (c *SDKCompose) Kill(ctx context.Context, composeFile string, services []string, signal string) error {
	c.logger.Debug("Kill", "file", composeFile, "services", services, "signal", signal)
	_, name, err := c.loadProject(ctx, composeFile)
	if err != nil {
		return err
	}
	return c.svc.Kill(ctx, name, api.KillOptions{
		Services: services,
		Signal:   signal,
	})
}

func (c *SDKCompose) Down(ctx context.Context, composeFile string) error {
	c.logger.Debug("Down", "file", composeFile)
	_, name, err := c.loadProject(ctx, composeFile)
	if err != nil {
		return err
	}
	return c.svc.Down(ctx, name, api.DownOptions{})
}

func (c *SDKCompose) Remove(ctx context.Context, composeFile string, services []string) error {
	c.logger.Debug("Remove", "file", composeFile, "services", services)
	_, name, err := c.loadProject(ctx, composeFile)
	if err != nil {
		return err
	}
	return c.svc.Remove(ctx, name, api.RemoveOptions{
		Force:    true,
		Services: services,
	})
}

func (c *SDKCompose) Create(ctx context.Context, composeFile string, services []string) error {
	c.logger.Debug("Create", "file", composeFile, "services", services)
	project, err := c.svc.LoadProject(ctx, api.ProjectLoadOptions{ConfigPaths: []string{composeFile}})
	if err != nil {
		return fmt.Errorf("load project for create: %w", err)
	}
	return c.svc.Create(ctx, project, api.CreateOptions{Services: services})
}

// --- Status ---

func (c *SDKCompose) RunningServices(ctx context.Context, composeFile string) (map[string]struct{}, error) {
	c.logger.Debug("RunningServices", "file", composeFile)
	_, name, err := c.loadProject(ctx, composeFile)
	if err != nil {
		return nil, err
	}
	containers, err := c.svc.Ps(ctx, name, api.PsOptions{All: true})
	if err != nil {
		return nil, err
	}
	return filterByState(containers, "running"), nil
}

func (c *SDKCompose) ExitedServices(ctx context.Context, composeFile string) (map[string]struct{}, error) {
	c.logger.Debug("ExitedServices", "file", composeFile)
	_, name, err := c.loadProject(ctx, composeFile)
	if err != nil {
		return nil, err
	}
	containers, err := c.svc.Ps(ctx, name, api.PsOptions{All: true})
	if err != nil {
		return nil, err
	}
	return filterByState(containers, "exited"), nil
}

func (c *SDKCompose) AllServices(ctx context.Context, composeFile string) (map[string]struct{}, error) {
	c.logger.Debug("AllServices", "file", composeFile)
	_, name, err := c.loadProject(ctx, composeFile)
	if err != nil {
		return nil, err
	}
	containers, err := c.svc.Ps(ctx, name, api.PsOptions{All: true})
	if err != nil {
		return nil, err
	}
	result := make(map[string]struct{}, len(containers))
	for _, ct := range containers {
		if ct.Service != "" {
			result[ct.Service] = struct{}{}
		}
	}
	return result, nil
}

// --- Exec ---

// withStreams returns a compose service whose container output goes to out and
// errOut. The shared c.svc discards stdout, so every call that must return or
// show container output uses one of these.
func (c *SDKCompose) withStreams(out, errOut io.Writer) (api.Compose, error) {
	return compose.NewComposeService(c.dockerCli, compose.WithOutputStream(out), compose.WithErrorStream(errOut))
}

// exitStatusIsResult drops the SDK's message-less error for a non-zero container
// exit: the code is in ExecResult.ExitCode, which callers check.
func exitStatusIsResult(err error) error {
	var status cli.StatusError
	var statusPtr *cli.StatusError
	if errors.As(err, &status) || errors.As(err, &statusPtr) {
		return nil
	}
	return err
}

// interactiveService attaches the CLI's own stdio; a TTY session needs stdin to be a terminal.
// Stdin stays the docker CLI's own stream: a wrapped reader would hide the terminal fd.
func (c *SDKCompose) interactiveService() (api.Compose, error) {
	if !c.dockerCli.In().IsTerminal() {
		return nil, errors.New("Error: this command needs an interactive terminal (stdin is not a terminal).")
	}
	return c.withStreams(os.Stdout, os.Stderr)
}

func (c *SDKCompose) Exec(ctx context.Context, composeFile, service string, command []string, opts ark.ExecOptions) (ark.ExecResult, error) {
	c.logger.Debug("Exec", "file", composeFile, "service", service, "cmd", command)
	_, name, err := c.loadProject(ctx, composeFile)
	if err != nil {
		return ark.ExecResult{}, err
	}

	// With a TTY the container has one output stream, so stderr arrives in Stdout.
	var stdout, stderr bytes.Buffer
	svc, err := c.withStreams(&stdout, &stderr)
	if err != nil {
		return ark.ExecResult{}, err
	}
	exitCode, err := svc.Exec(ctx, name, api.RunOptions{
		Service:     service,
		Command:     command,
		Tty:         !opts.DisableTTY,
		Environment: opts.Env,
		User:        opts.User,
	})
	return ark.ExecResult{ExitCode: exitCode, Stdout: stdout.String(), Stderr: stderr.String()}, exitStatusIsResult(err)
}

func (c *SDKCompose) ExecInteractive(ctx context.Context, composeFile, service string, command []string) error {
	c.logger.Debug("ExecInteractive", "file", composeFile, "service", service, "cmd", command)
	_, name, err := c.loadProject(ctx, composeFile)
	if err != nil {
		return err
	}
	svc, err := c.interactiveService()
	if err != nil {
		return err
	}
	_, err = svc.Exec(ctx, name, api.RunOptions{
		Service:     service,
		Command:     command,
		Tty:         true,
		Interactive: true,
	})
	return err
}

func (c *SDKCompose) RunOneOff(ctx context.Context, composeFile, service string, command []string, opts ark.RunOptions) (ark.ExecResult, error) {
	c.logger.Debug("RunOneOff", "file", composeFile, "service", service, "cmd", command)
	project, err := c.svc.LoadProject(ctx, api.ProjectLoadOptions{ConfigPaths: []string{composeFile}})
	if err != nil {
		return ark.ExecResult{}, fmt.Errorf("load project for run: %w", err)
	}
	if project, err = selectRunService(project, service, opts.NoDeps); err != nil {
		return ark.ExecResult{}, err
	}
	c.injectVolumes(project, service, opts.Volumes)

	runOpts := c.buildRunOptions(service, command, opts)
	runOpts.Tty = !opts.DisableTTY

	var stdout, stderr bytes.Buffer
	svc, err := c.withStreams(&stdout, &stderr)
	if err != nil {
		return ark.ExecResult{}, err
	}
	exitCode, err := svc.RunOneOffContainer(ctx, project, runOpts)
	return ark.ExecResult{ExitCode: exitCode, Stdout: stdout.String(), Stderr: stderr.String()}, exitStatusIsResult(err)
}

func (c *SDKCompose) RunOneOffDetached(ctx context.Context, composeFile, service, containerName string, command []string, opts ark.RunOptions) error {
	c.logger.Debug("RunOneOffDetached", "file", composeFile, "service", service, "name", containerName, "cmd", command)
	project, err := c.svc.LoadProject(ctx, api.ProjectLoadOptions{ConfigPaths: []string{composeFile}})
	if err != nil {
		return fmt.Errorf("load project for run detached: %w", err)
	}
	if project, err = selectRunService(project, service, opts.NoDeps); err != nil {
		return err
	}
	c.injectVolumes(project, service, opts.Volumes)

	runOpts := c.buildRunOptions(service, command, opts)
	runOpts.Detach = true
	runOpts.Name = containerName

	_, err = c.svc.RunOneOffContainer(ctx, project, runOpts)
	return err
}

func (c *SDKCompose) RunOneOffInteractive(ctx context.Context, composeFile, service string, command []string, opts ark.RunOptions) error {
	c.logger.Debug("RunOneOffInteractive", "file", composeFile, "service", service, "cmd", command)
	project, err := c.svc.LoadProject(ctx, api.ProjectLoadOptions{ConfigPaths: []string{composeFile}})
	if err != nil {
		return fmt.Errorf("load project for run interactive: %w", err)
	}
	if project, err = selectRunService(project, service, opts.NoDeps); err != nil {
		return err
	}
	c.injectVolumes(project, service, opts.Volumes)

	runOpts := c.buildRunOptions(service, command, opts)
	runOpts.Tty = true
	runOpts.Interactive = true

	svc, err := c.interactiveService()
	if err != nil {
		return err
	}
	_, err = svc.RunOneOffContainer(ctx, project, runOpts)
	return err
}

// selectRunService narrows the project to the run target (plus its dependencies
// unless noDeps), as `docker compose run` does. RunOneOffContainer starts every
// other service in the project it is given as a "dependency".
func selectRunService(project *types.Project, service string, noDeps bool) (*types.Project, error) {
	var opts []types.DependencyOption
	if noDeps {
		opts = append(opts, types.IgnoreDependencies)
	}
	project, err := project.WithSelectedServices([]string{service}, opts...)
	if err != nil {
		return nil, fmt.Errorf("select service %s for run: %w", service, err)
	}
	return project, nil
}

// injectVolumes adds volume mounts to a service in the project.
func (c *SDKCompose) injectVolumes(project *types.Project, service string, volumes []string) {
	if len(volumes) == 0 {
		return
	}
	svc := project.Services[service]
	svc.Volumes = append(svc.Volumes, parseVolumeStrings(volumes)...)
	project.Services[service] = svc
}

func (c *SDKCompose) buildRunOptions(service string, command []string, opts ark.RunOptions) api.RunOptions {
	entrypoint := []string{opts.Entrypoint}
	if opts.Entrypoint == "" {
		entrypoint = nil
	}
	return api.RunOptions{
		Service:    service,
		Command:    command,
		AutoRemove: opts.Remove,
		NoDeps:     opts.NoDeps,
		User:       opts.User,
		Entrypoint: entrypoint,
	}
}

// parseVolumeStrings converts Docker volume strings (e.g., "hostPath:containerPath:ro")
// into compose-go ServiceVolumeConfig for the SDK.
func parseVolumeStrings(volumes []string) []types.ServiceVolumeConfig {
	mounts := make([]types.ServiceVolumeConfig, 0, len(volumes))
	for _, v := range volumes {
		parts := strings.Split(v, ":")
		if len(parts) < 2 {
			continue
		}
		mount := types.ServiceVolumeConfig{Type: "bind", Source: parts[0], Target: parts[1]}
		if len(parts) == 3 && parts[2] == "ro" {
			mount.ReadOnly = true
		}
		mounts = append(mounts, mount)
	}
	return mounts
}

// --- Logs ---

func (c *SDKCompose) Logs(ctx context.Context, composeFile, service string, opts ark.LogOptions) (io.ReadCloser, error) {
	c.logger.Debug("Logs", "file", composeFile, "service", service, "tail", opts.Tail, "follow", opts.Follow)
	_, name, err := c.loadProject(ctx, composeFile)
	if err != nil {
		return nil, err
	}

	pr, pw := io.Pipe()
	consumer := &pipeLogConsumer{pw: pw}

	logOpts := api.LogOptions{
		Services:   []string{service},
		Follow:     opts.Follow,
		Timestamps: opts.Timestamps,
	}
	if opts.Tail > 0 {
		logOpts.Tail = strconv.Itoa(opts.Tail)
	}
	if opts.Since != nil {
		logOpts.Since = opts.Since.UTC().Format(time.RFC3339)
	}

	go func() {
		err := c.svc.Logs(ctx, name, consumer, logOpts)
		pw.CloseWithError(err)
	}()

	return pr, nil
}

type pipeLogConsumer struct {
	pw *io.PipeWriter
}

func (c *pipeLogConsumer) Log(containerName, message string) {
	fmt.Fprintln(c.pw, message)
}

func (c *pipeLogConsumer) Err(containerName, message string) {
	fmt.Fprintln(c.pw, message)
}

func (c *pipeLogConsumer) Status(container, msg string) {}

// --- Copy ---

func (c *SDKCompose) CopyFromService(ctx context.Context, composeFile, service, srcPath, dstPath string) error {
	c.logger.Debug("CopyFromService", "file", composeFile, "service", service, "src", srcPath, "dst", dstPath)
	_, name, err := c.loadProject(ctx, composeFile)
	if err != nil {
		return err
	}
	return c.svc.Copy(ctx, name, api.CopyOptions{
		Source:      fmt.Sprintf("%s:%s", service, srcPath),
		Destination: dstPath,
	})
}

// Ensure SDKCompose satisfies the interface at compile time.
var _ ark.ComposeService = (*SDKCompose)(nil)

// filterByState filters ContainerSummary by container state for RunningServices/ExitedServices.
func (c *SDKCompose) ServiceContainers(ctx context.Context, composeFile string) (map[string]ark.ServiceContainer, error) {
	c.logger.Debug("ServiceContainers", "file", composeFile)
	_, name, err := c.loadProject(ctx, composeFile)
	if err != nil {
		return nil, err
	}
	containers, err := c.svc.Ps(ctx, name, api.PsOptions{All: true})
	if err != nil {
		return nil, err
	}
	result := make(map[string]ark.ServiceContainer, len(containers))
	for _, ct := range containers {
		if ct.Service == "" {
			continue
		}
		result[ct.Service] = ark.ServiceContainer{Name: ct.Name, State: strings.ToLower(string(ct.State))}
	}
	return result, nil
}

func filterByState(containers []api.ContainerSummary, state string) map[string]struct{} {
	result := make(map[string]struct{})
	for _, c := range containers {
		if strings.EqualFold(string(c.State), state) && c.Service != "" {
			result[c.Service] = struct{}{}
		}
	}
	return result
}
