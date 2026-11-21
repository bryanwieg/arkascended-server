package ark

import (
	"context"
	"io"
)

// ContainerEngine abstracts container runtime operations (Docker Engine,
// Podman, or test mocks). All methods take context.Context as the first
// argument. Backends map native errors to sentinel types (ErrNotFound, etc.)
// so callers can branch with errors.Is.
type ContainerEngine interface {
	// Container operations

	// InspectState returns the runtime state of the named container.
	// Returns ErrNotFound if the container does not exist.
	InspectState(ctx context.Context, container string) (ContainerState, error)

	// RemoveContainer force-removes the named container.
	// Returns ErrNotFound if the container does not exist.
	RemoveContainer(ctx context.Context, container string) error

	// ContainerLogs returns a reader for the container's combined stdout/stderr.
	ContainerLogs(ctx context.Context, container string, opts LogOptions) (io.ReadCloser, error)

	// Image operations

	// ImageExists reports whether the named image exists locally.
	ImageExists(ctx context.Context, image string) (bool, error)

	// ImageLabels returns the OCI labels for the named image.
	// Returns ErrNotFound if the image does not exist.
	ImageLabels(ctx context.Context, image string) (map[string]string, error)

	// PullImage pulls ref (registry/repo:tag) anonymously and returns once the
	// pull has finished; an error reported during the pull is returned.
	PullImage(ctx context.Context, ref string) error

	// BuildImage builds an image from the given request.
	BuildImage(ctx context.Context, req BuildRequest) error

	// ReadFileFromImage runs a throwaway container from the image and returns
	// the contents of the given path. The container is removed after reading.
	ReadFileFromImage(ctx context.Context, image, path string) (string, error)

	// ImageTags returns the local tags of a repository (e.g. "ark-server"),
	// each as a full reference ("ark-server:latest") mapped to its image ID.
	ImageTags(ctx context.Context, repo string) (map[string]string, error)

	// RemoveImage removes one image reference without forcing; the image is
	// deleted once its last tag is gone. An image a container (running or
	// stopped) uses is never deleted: removing its last tag returns
	// ErrConflict (other tags are simply removed). Returns ErrNotFound if the
	// reference does not exist.
	RemoveImage(ctx context.Context, ref string) error

	// ImagesInUse returns the IDs (as ImageTags reports them) of the images that
	// any container on the host uses, running or stopped, in any compose project.
	ImagesInUse(ctx context.Context) (map[string]bool, error)

	// PruneDangling removes untagged images and build cache that nothing uses.
	PruneDangling(ctx context.Context) error

	// Engine operations

	// Rootless reports whether the engine runs without root on the host (Docker rootless mode or rootless
	// Podman): its /info SecurityOptions contain "name=rootless".
	Rootless(ctx context.Context) (bool, error)

	// Volume operations

	// VolumeMountpoint returns the host path where the named volume is
	// mounted. Returns ("", false) if the volume does not exist or has no
	// mountpoint.
	VolumeMountpoint(ctx context.Context, volume string) (string, bool)

	// RemoveVolume removes the named volume. Returns ErrNotFound if it does
	// not exist; returns ErrConflict if it is in use.
	RemoveVolume(ctx context.Context, volume string) error

	// Copy operations

	// CopyFromContainer copies a file or directory from a container to a
	// local host path.
	CopyFromContainer(ctx context.Context, container, srcPath, dstPath string) error
}
