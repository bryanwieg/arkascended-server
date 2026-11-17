//go:build integration

package conformance

import (
	"context"
	"io"
	"testing"
	"time"

	"github.com/moby/moby/api/types/container"
	dockerclient "github.com/moby/moby/client"

	"pkg/ark"
)

// RequireDocker skips the test if no Docker daemon is reachable.
func RequireDocker(t *testing.T) {
	t.Helper()
	cli, err := dockerclient.New(dockerclient.FromEnv)
	if err != nil {
		t.Skipf("Docker client creation failed: %v", err)
	}
	defer cli.Close()
	_, err = cli.Ping(context.Background(), dockerclient.PingOptions{})
	if err != nil {
		t.Skipf("Docker daemon not reachable: %v", err)
	}
}

// CleanupContainer registers a cleanup function that force-removes the named container.
func CleanupContainer(t *testing.T, engine ark.ContainerEngine, id string) {
	t.Helper()
	t.Cleanup(func() {
		_ = engine.RemoveContainer(context.Background(), id)
	})
}

// CleanupVolume registers a cleanup function that removes the named volume.
func CleanupVolume(t *testing.T, engine ark.ContainerEngine, name string) {
	t.Helper()
	t.Cleanup(func() {
		_ = engine.RemoveVolume(context.Background(), name)
	})
}

// sharedClient returns a raw Docker client for helper operations that need
// direct SDK access (pulling images, creating containers for test fixtures).
func sharedClient(t *testing.T) *dockerclient.Client {
	t.Helper()
	cli, err := dockerclient.New(dockerclient.FromEnv)
	if err != nil {
		t.Fatalf("Docker client: %v", err)
	}
	t.Cleanup(func() { cli.Close() })
	return cli
}

// pullAlpine ensures alpine:latest is available locally.
func pullAlpine(t *testing.T) {
	t.Helper()
	cli := sharedClient(t)
	reader, err := cli.ImagePull(context.Background(), "alpine:latest", dockerclient.ImagePullOptions{})
	if err != nil {
		t.Fatalf("pull alpine: %v", err)
	}
	defer reader.Close()
	_, _ = io.Copy(io.Discard, reader)
}

// createAndStartContainer creates and starts a container running the given shell command,
// returning the container ID.
func createAndStartContainer(t *testing.T, shellCmd string) string {
	t.Helper()
	cli := sharedClient(t)
	resp, err := cli.ContainerCreate(context.Background(), dockerclient.ContainerCreateOptions{
		Config: &container.Config{
			Image: "alpine:latest",
			Cmd:   []string{"/bin/sh", "-c", shellCmd},
		},
	})
	if err != nil {
		t.Fatalf("create container: %v", err)
	}
	if _, err := cli.ContainerStart(context.Background(), resp.ID, dockerclient.ContainerStartOptions{}); err != nil {
		t.Fatalf("start container: %v", err)
	}
	return resp.ID
}

// waitForExit waits up to 10 seconds for a container to exit.
func waitForExit(t *testing.T, containerID string) {
	t.Helper()
	cli := sharedClient(t)
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		result, err := cli.ContainerInspect(context.Background(), containerID, dockerclient.ContainerInspectOptions{})
		if err != nil {
			t.Fatalf("inspect during wait: %v", err)
		}
		if result.Container.State != nil && !result.Container.State.Running {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("container %s did not exit within 10s", containerID)
}
