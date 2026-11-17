package sdkbackend

import (
	"archive/tar"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/containerd/errdefs"
	goarchive "github.com/moby/go-archive"
	"github.com/moby/moby/api/types/build"
	dockerclient "github.com/moby/moby/client"
	"github.com/moby/moby/client/pkg/jsonmessage"
	"github.com/moby/patternmatcher/ignorefile"

	"pkg/ark"
)

// SDKEngine implements ark.ContainerEngine using the Docker Engine SDK.
type SDKEngine struct {
	cli    *dockerclient.Client
	logger *slog.Logger
}

// NewSDKEngine creates a ContainerEngine backed by the Docker Engine SDK.
// The client auto-discovers the Docker socket from DOCKER_HOST or the default path.
func NewSDKEngine(logger *slog.Logger) (*SDKEngine, error) {
	return newSDKEngine(logger, "")
}

// NewSDKEngineWithSocket creates a ContainerEngine backed by the Docker Engine SDK,
// connecting to the specified socket path (e.g., "unix:///run/podman/podman.sock").
func NewSDKEngineWithSocket(logger *slog.Logger, socketPath string) (*SDKEngine, error) {
	return newSDKEngine(logger, socketPath)
}

// newSDKEngine is the internal implementation of NewSDKEngine and NewSDKEngineWithSocket.
func newSDKEngine(logger *slog.Logger, socketPath string) (*SDKEngine, error) {
	// Build options list.
	var opts []dockerclient.Opt
	opts = append(opts, dockerclient.FromEnv)
	if socketPath != "" {
		opts = append(opts, dockerclient.WithHost(socketPath))
	}
	cli, err := dockerclient.New(opts...)
	if err != nil {
		return nil, fmt.Errorf("docker SDK client: %w", err)
	}
	return &SDKEngine{cli: cli, logger: logger}, nil
}

// ValidateDaemonVersion checks the daemon meets minimum version requirements.
// Docker >= 28, Podman >= 5. Uses the SDK's ServerVersion API.
// The runtime parameter indicates which container runtime is in use.
func (e *SDKEngine) ValidateDaemonVersion(ctx context.Context, runtime string) error {
	serverVersion, err := e.cli.ServerVersion(ctx, dockerclient.ServerVersionOptions{})
	if err != nil {
		return fmt.Errorf("failed to query daemon version: %w", err)
	}
	e.logger.Debug("daemon version", "version", serverVersion.Version, "api", serverVersion.APIVersion)

	return ValidateDaemonVersionString(serverVersion.Version, serverVersion.APIVersion, runtime)
}

// ValidateDaemonVersionString validates a version string against minimum
// requirements for the given container runtime. Exported for testing.
// Returns nil for unparseable (non-numeric) versions.
func ValidateDaemonVersionString(version, apiVersion, runtime string) error {
	majorStr := strings.SplitN(version, ".", 2)[0]
	major, err := strconv.Atoi(majorStr)
	if err != nil {
		return nil
	}

	switch runtime {
	case "podman":
		if major < 5 {
			return fmt.Errorf("Error: podman 5.0+ required, found version %s (api %s)", version, apiVersion)
		}
	default:
		if major < 28 {
			return fmt.Errorf("Error: docker 28.0+ required, found version %s (api %s)", version, apiVersion)
		}
	}
	return nil
}

// Close releases the underlying Docker SDK client resources.
func (e *SDKEngine) Close() error {
	return e.cli.Close()
}

func (e *SDKEngine) InspectState(ctx context.Context, container string) (ark.ContainerState, error) {
	e.logger.Debug("InspectState", "container", container)
	result, err := e.cli.ContainerInspect(ctx, container, dockerclient.ContainerInspectOptions{})
	if err != nil {
		if errdefs.IsNotFound(err) {
			return ark.ContainerState{}, fmt.Errorf("container %q: %w", container, ark.ErrNotFound)
		}
		return ark.ContainerState{}, err
	}
	state := result.Container.State
	if state == nil {
		return ark.ContainerState{}, nil
	}
	return ark.ContainerState{Running: state.Running, ExitCode: state.ExitCode}, nil
}

func (e *SDKEngine) RemoveContainer(ctx context.Context, container string) error {
	e.logger.Debug("RemoveContainer", "container", container)
	_, err := e.cli.ContainerRemove(ctx, container, dockerclient.ContainerRemoveOptions{Force: true})
	if err != nil {
		if errdefs.IsNotFound(err) {
			return fmt.Errorf("container %q: %w", container, ark.ErrNotFound)
		}
		return err
	}
	return nil
}

func (e *SDKEngine) ContainerLogs(ctx context.Context, container string, opts ark.LogOptions) (io.ReadCloser, error) {
	e.logger.Debug("ContainerLogs", "container", container, "tail", opts.Tail, "follow", opts.Follow)
	sdkOpts := dockerclient.ContainerLogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Follow:     opts.Follow,
		Timestamps: opts.Timestamps,
	}
	if opts.Tail > 0 {
		sdkOpts.Tail = fmt.Sprintf("%d", opts.Tail)
	}
	if opts.Since != nil {
		sdkOpts.Since = opts.Since.UTC().Format("2006-01-02T15:04:05Z")
	}
	return e.cli.ContainerLogs(ctx, container, sdkOpts)
}

func (e *SDKEngine) ImageExists(ctx context.Context, image string) (bool, error) {
	e.logger.Debug("ImageExists", "image", image)
	_, err := e.cli.ImageInspect(ctx, image)
	if err != nil {
		if errdefs.IsNotFound(err) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func (e *SDKEngine) PullImage(ctx context.Context, ref string) error {
	e.logger.Debug("PullImage", "ref", ref)
	resp, err := e.cli.ImagePull(ctx, ref, dockerclient.ImagePullOptions{})
	if err != nil {
		return fmt.Errorf("pull %s: %w", ref, err)
	}
	defer resp.Close()
	if err := resp.Wait(ctx); err != nil {
		return fmt.Errorf("pull %s: %w", ref, err)
	}
	return nil
}

func (e *SDKEngine) ImageLabels(ctx context.Context, image string) (map[string]string, error) {
	e.logger.Debug("ImageLabels", "image", image)
	result, err := e.cli.ImageInspect(ctx, image)
	if err != nil {
		if errdefs.IsNotFound(err) {
			return nil, fmt.Errorf("image %q: %w", image, ark.ErrNotFound)
		}
		return nil, err
	}
	if result.Config == nil {
		return map[string]string{}, nil
	}
	labels := result.Config.Labels
	if labels == nil {
		return map[string]string{}, nil
	}
	return labels, nil
}

func (e *SDKEngine) BuildImage(ctx context.Context, req ark.BuildRequest) error {
	e.logger.Debug("BuildImage", "tag", req.Tag, "dockerfile", req.Dockerfile, "context", req.Context)

	// The context is tarred here, so .dockerignore has to be applied here too.
	var excludes []string
	if f, err := os.Open(filepath.Join(req.Context, ".dockerignore")); err == nil {
		excludes, err = ignorefile.ReadAll(f)
		f.Close()
		if err != nil {
			return fmt.Errorf("read .dockerignore in %s: %w", req.Context, err)
		}
	}
	tarCtx, err := goarchive.TarWithOptions(req.Context, &goarchive.TarOptions{ExcludePatterns: excludes})
	if err != nil {
		return fmt.Errorf("tar build context %s: %w", req.Context, err)
	}
	defer tarCtx.Close()

	buildArgs := make(map[string]*string, len(req.BuildArgs))
	for k, v := range req.BuildArgs {
		buildArgs[k] = &v
	}

	tags := []string{req.Tag}
	tags = append(tags, req.ExtraTags...)

	// The Docker SDK requires Dockerfile to be relative to the build context.
	dockerfile := req.Dockerfile
	if abs, err := filepath.Abs(req.Context); err == nil {
		if rel, err := filepath.Rel(abs, req.Dockerfile); err == nil {
			dockerfile = rel
		}
	}

	result, err := e.cli.ImageBuild(ctx, tarCtx, dockerclient.ImageBuildOptions{
		Tags:       tags,
		Dockerfile: dockerfile,
		BuildArgs:  buildArgs,
		NoCache:    req.NoCache,
		Remove:     true,
		// The API's default is the legacy builder, which rejects COPY --chmod and RUN --mount.
		Version: build.BuilderBuildKit,
	})
	if err != nil {
		return fmt.Errorf("image build for %s: %w", req.Tag, err)
	}
	defer result.Body.Close()

	// Build failures arrive as error messages inside an HTTP 200 stream, so the stream
	// must be decoded, not just drained. BuildKit progress, including each step's output,
	// comes as aux messages; buildLog logs it and keeps the tail for the error.
	out := newBuildLog(e.logger)
	if err := jsonmessage.DisplayJSONMessagesStream(result.Body, io.Discard, 0, false, out.handle); err != nil {
		if tail := out.tailText(); tail != "" {
			return fmt.Errorf("image build for %s: %w\nLast build output:\n%s", req.Tag, err, tail)
		}
		return fmt.Errorf("image build for %s: %w", req.Tag, err)
	}
	out.flush()
	return nil
}

func (e *SDKEngine) ReadFileFromImage(ctx context.Context, image, path string) (string, error) {
	e.logger.Debug("ReadFileFromImage", "image", image, "path", path)

	// Create a throwaway container (not started).
	created, err := e.cli.ContainerCreate(ctx, dockerclient.ContainerCreateOptions{Image: image})
	if err != nil {
		return "", fmt.Errorf("create temp container from %s: %w", image, err)
	}
	defer func() {
		_, _ = e.cli.ContainerRemove(ctx, created.ID, dockerclient.ContainerRemoveOptions{Force: true})
	}()

	// Copy the file out.
	copyResult, err := e.cli.CopyFromContainer(ctx, created.ID, dockerclient.CopyFromContainerOptions{SourcePath: path})
	if err != nil {
		return "", fmt.Errorf("copy %s from %s: %w", path, image, err)
	}
	defer copyResult.Content.Close()

	// The result is a tar stream; extract the single file.
	tr := tar.NewReader(copyResult.Content)
	_, err = tr.Next()
	if err != nil {
		return "", fmt.Errorf("read tar entry for %s from %s: %w", path, image, err)
	}
	data, err := io.ReadAll(tr)
	if err != nil {
		return "", fmt.Errorf("read %s from %s: %w", path, image, err)
	}
	value := strings.TrimSpace(string(data))
	if value == "" {
		return "", fmt.Errorf("empty value reading %s from %s", path, image)
	}
	return value, nil
}

func (e *SDKEngine) VolumeMountpoint(ctx context.Context, volume string) (string, bool) {
	e.logger.Debug("VolumeMountpoint", "volume", volume)
	result, err := e.cli.VolumeInspect(ctx, volume, dockerclient.VolumeInspectOptions{})
	if err != nil {
		return "", false
	}
	mp := result.Volume.Mountpoint
	if mp == "" {
		return "", false
	}
	return mp, true
}

func (e *SDKEngine) ImageTags(ctx context.Context, repo string) (map[string]string, error) {
	e.logger.Debug("ImageTags", "repo", repo)
	result, err := e.cli.ImageList(ctx, dockerclient.ImageListOptions{Filters: dockerclient.Filters{}.Add("reference", repo)})
	if err != nil {
		return nil, err
	}
	tags := map[string]string{}
	for _, img := range result.Items {
		for _, ref := range img.RepoTags {
			// The reference filter also matches other registries' repos with the same name.
			if i := strings.LastIndex(ref, ":"); i > 0 && ref[:i] == repo {
				tags[ref] = img.ID
			}
		}
	}
	return tags, nil
}

func (e *SDKEngine) RemoveImage(ctx context.Context, ref string) error {
	e.logger.Debug("RemoveImage", "ref", ref)
	if _, err := e.cli.ImageRemove(ctx, ref, dockerclient.ImageRemoveOptions{PruneChildren: true}); err != nil {
		if errdefs.IsNotFound(err) {
			return fmt.Errorf("image %q: %w", ref, ark.ErrNotFound)
		}
		if errdefs.IsConflict(err) {
			return fmt.Errorf("image %q: %w", ref, ark.ErrConflict)
		}
		return err
	}
	return nil
}

func (e *SDKEngine) PruneDangling(ctx context.Context) error {
	e.logger.Debug("PruneDangling")
	if _, err := e.cli.ImagePrune(ctx, dockerclient.ImagePruneOptions{Filters: dockerclient.Filters{}.Add("dangling", "true")}); err != nil {
		return err
	}
	_, err := e.cli.BuildCachePrune(ctx, dockerclient.BuildCachePruneOptions{})
	return err
}

func (e *SDKEngine) RemoveVolume(ctx context.Context, volume string) error {
	e.logger.Debug("RemoveVolume", "volume", volume)
	_, err := e.cli.VolumeRemove(ctx, volume, dockerclient.VolumeRemoveOptions{})
	if err != nil {
		if errdefs.IsNotFound(err) {
			return fmt.Errorf("volume %q: %w", volume, ark.ErrNotFound)
		}
		if errdefs.IsConflict(err) {
			return fmt.Errorf("volume %q: %w", volume, ark.ErrConflict)
		}
		return err
	}
	return nil
}

func (e *SDKEngine) CopyFromContainer(ctx context.Context, container, srcPath, dstPath string) error {
	e.logger.Debug("CopyFromContainer", "container", container, "src", srcPath, "dst", dstPath)

	copyResult, err := e.cli.CopyFromContainer(ctx, container, dockerclient.CopyFromContainerOptions{SourcePath: srcPath})
	if err != nil {
		return fmt.Errorf("copy from container %s:%s: %w", container, srcPath, err)
	}
	defer copyResult.Content.Close()

	// Extract the tar manually to avoid any chown attempts by go-archive.
	// go-archive's NoLchown option is unreliable on this kernel/Docker version.
	tr := tar.NewReader(copyResult.Content)
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return fmt.Errorf("read tar from %s:%s: %w", container, srcPath, err)
		}
		// Like `docker cp`: the archive is rooted at base(srcPath), which lands at dstPath.
		rel, ok := strings.CutPrefix(hdr.Name, filepath.Base(srcPath))
		if !ok || (rel != "" && !strings.HasPrefix(rel, "/")) {
			return fmt.Errorf("unexpected entry %q copying %s:%s", hdr.Name, container, srcPath)
		}
		path := dstPath + filepath.FromSlash(rel)
		if hdr.Typeflag == tar.TypeDir {
			if err := os.MkdirAll(path, os.FileMode(hdr.Mode&0o777)); err != nil {
				return err
			}
			continue
		}
		// Create parent directory if needed.
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return err
		}
		f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY, os.FileMode(hdr.Mode&0o777))
		if err != nil {
			return err
		}
		if _, err := io.Copy(f, tr); err != nil {
			f.Close()
			return err
		}
		f.Close()
	}
	return nil
}
