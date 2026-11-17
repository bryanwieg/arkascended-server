//go:build integration

// Package conformance provides test suites that verify ark.ContainerEngine and
// ark.ComposeService implementations behave identically. Both the shell and SDK
// backends run these same tests against a real Docker daemon.
package conformance

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/moby/moby/api/types/container"
	dockerclient "github.com/moby/moby/client"

	"pkg/ark"
)

// TestContainerEngine runs the ContainerEngine conformance suite against the
// provided engine implementation. Call this from each backend's own test file.
func TestContainerEngine(t *testing.T, engine ark.ContainerEngine) {
	RequireDocker(t)
	ctx := context.Background()

	t.Run("ImageExists/present", func(t *testing.T) {
		// alpine:latest should be available if Docker is running with any images.
		// Pull it first to guarantee.
		pullAlpine(t)
		exists, err := engine.ImageExists(ctx, "alpine:latest")
		if err != nil {
			t.Fatalf("ImageExists: %v", err)
		}
		if !exists {
			t.Fatal("expected alpine:latest to exist")
		}
	})

	t.Run("PullImage/pulls", func(t *testing.T) {
		if err := engine.PullImage(ctx, "alpine:latest"); err != nil {
			t.Fatalf("PullImage: %v", err)
		}
		exists, err := engine.ImageExists(ctx, "alpine:latest")
		if err != nil || !exists {
			t.Fatalf("expected alpine:latest after the pull, exists=%v err=%v", exists, err)
		}
	})

	t.Run("PullImage/missing_returns_error", func(t *testing.T) {
		if err := engine.PullImage(ctx, "ghcr.io/bryanwieg/ark-conformance-does-not-exist:0"); err == nil {
			t.Fatal("expected an error pulling a missing image")
		}
	})

	t.Run("ImageExists/absent_returns_false", func(t *testing.T) {
		exists, err := engine.ImageExists(ctx, "ark-conformance-nonexistent:never")
		if err != nil {
			t.Fatalf("ImageExists: %v", err)
		}
		if exists {
			t.Fatal("expected nonexistent image to not exist")
		}
	})

	t.Run("ImageLabels/present", func(t *testing.T) {
		pullAlpine(t)
		labels, err := engine.ImageLabels(ctx, "alpine:latest")
		if err != nil {
			t.Fatalf("ImageLabels: %v", err)
		}
		// alpine:latest may or may not have labels, but should not error.
		_ = labels
	})

	t.Run("ImageLabels/absent_returns_ErrNotFound", func(t *testing.T) {
		_, err := engine.ImageLabels(ctx, "ark-conformance-nonexistent:never")
		if !errors.Is(err, ark.ErrNotFound) {
			t.Fatalf("expected ErrNotFound, got: %v", err)
		}
	})

	t.Run("ReadFileFromImage", func(t *testing.T) {
		pullAlpine(t)
		content, err := engine.ReadFileFromImage(ctx, "alpine:latest", "/etc/alpine-release")
		if err != nil {
			t.Fatalf("ReadFileFromImage: %v", err)
		}
		if content == "" {
			t.Fatal("expected non-empty content")
		}
	})

	t.Run("ReadFileFromImage/missing_file_returns_error", func(t *testing.T) {
		pullAlpine(t)
		_, err := engine.ReadFileFromImage(ctx, "alpine:latest", "/nonexistent/path/file.txt")
		if err == nil {
			t.Fatal("expected error for missing file")
		}
	})

	t.Run("RemoveContainer/not_found_returns_ErrNotFound", func(t *testing.T) {
		err := engine.RemoveContainer(ctx, "ark-conformance-nonexistent-container")
		if !errors.Is(err, ark.ErrNotFound) {
			t.Fatalf("expected ErrNotFound, got: %v", err)
		}
	})

	t.Run("InspectState/not_found_returns_ErrNotFound", func(t *testing.T) {
		_, err := engine.InspectState(ctx, "ark-conformance-nonexistent-container")
		if !errors.Is(err, ark.ErrNotFound) {
			t.Fatalf("expected ErrNotFound, got: %v", err)
		}
	})

	t.Run("ContainerLogs/basic", func(t *testing.T) {
		// Create and start a container that produces output.
		pullAlpine(t)
		id := createAndStartContainer(t, "echo hello-conformance")
		CleanupContainer(t, engine, id)

		// Wait briefly for container to finish.
		waitForExit(t, id)

		reader, err := engine.ContainerLogs(ctx, id, ark.LogOptions{Tail: 10})
		if err != nil {
			t.Fatalf("ContainerLogs: %v", err)
		}
		defer reader.Close()
		buf := make([]byte, 4096)
		n, _ := reader.Read(buf)
		output := string(buf[:n])
		if !strings.Contains(output, "hello-conformance") {
			t.Fatalf("expected logs to contain 'hello-conformance', got: %q", output)
		}
	})

	t.Run("ImageTags_and_RemoveImage", func(t *testing.T) {
		pullAlpine(t)
		cli := sharedClient(t)
		repo := "ark-conformance-images"
		// A built image whose only tags are this repo's, so removing them can delete it.
		dir := t.TempDir()
		writeFiles(t, dir, map[string]string{"Dockerfile": "FROM alpine:latest\nLABEL ark-conformance=images\n"})
		if err := engine.BuildImage(ctx, ark.BuildRequest{Dockerfile: filepath.Join(dir, "Dockerfile"), Tag: repo + ":inuse", Context: dir}); err != nil {
			t.Fatalf("BuildImage: %v", err)
		}
		if _, err := cli.ImageTag(ctx, dockerclient.ImageTagOptions{Source: repo + ":inuse", Target: repo + ":a"}); err != nil {
			t.Fatalf("tag: %v", err)
		}
		for _, tag := range []string{repo + ":a", repo + ":inuse"} {
			t.Cleanup(func() {
				_, _ = cli.ImageRemove(context.Background(), tag, dockerclient.ImageRemoveOptions{Force: true})
			})
		}
		tags, err := engine.ImageTags(ctx, repo)
		if err != nil {
			t.Fatalf("ImageTags: %v", err)
		}
		if len(tags) != 2 || tags[repo+":a"] == "" || tags[repo+":a"] != tags[repo+":inuse"] {
			t.Fatalf("expected both tags on one image ID, got %v", tags)
		}

		if err := engine.RemoveImage(ctx, repo+":a"); err != nil {
			t.Fatalf("RemoveImage: %v", err)
		}
		if err := engine.RemoveImage(ctx, repo+":a"); !errors.Is(err, ark.ErrNotFound) {
			t.Fatalf("expected ErrNotFound for a removed tag, got: %v", err)
		}

		// A created (never started) container still pins its image: its last tag can't be removed.
		created, err := cli.ContainerCreate(ctx, dockerclient.ContainerCreateOptions{
			Config: &container.Config{Image: repo + ":inuse", Cmd: []string{"true"}},
		})
		if err != nil {
			t.Fatalf("create container: %v", err)
		}
		t.Cleanup(func() {
			_, _ = cli.ContainerRemove(context.Background(), created.ID, dockerclient.ContainerRemoveOptions{Force: true})
		})
		if err := engine.RemoveImage(ctx, repo+":inuse"); !errors.Is(err, ark.ErrConflict) {
			t.Fatalf("expected ErrConflict for an image a container uses, got: %v", err)
		}
	})

	t.Run("PruneDangling", func(t *testing.T) {
		if err := engine.PruneDangling(ctx); err != nil {
			t.Fatalf("PruneDangling: %v", err)
		}
	})

	t.Run("RemoveVolume/not_found_returns_ErrNotFound", func(t *testing.T) {
		err := engine.RemoveVolume(ctx, "ark-conformance-nonexistent-volume")
		if !errors.Is(err, ark.ErrNotFound) {
			t.Fatalf("expected ErrNotFound, got: %v", err)
		}
	})

	t.Run("BuildImage/buildkit_features_and_dockerignore", func(t *testing.T) {
		pullAlpine(t)
		dir := t.TempDir()
		writeFiles(t, dir, map[string]string{
			// COPY --chmod and RUN --mount need BuildKit; the image's Dockerfile uses both.
			"Dockerfile":    "FROM alpine:latest\nCOPY --chmod=0755 . /ctx/\nRUN --mount=type=cache,target=/cache touch /cache/x\n",
			".dockerignore": "ignored.txt\n",
			"kept.txt":      "kept",
			"ignored.txt":   "ignored",
		})
		tag := "ark-conformance-build:test"
		if err := engine.BuildImage(ctx, ark.BuildRequest{Dockerfile: filepath.Join(dir, "Dockerfile"), Tag: tag, Context: dir}); err != nil {
			t.Fatalf("BuildImage: %v", err)
		}
		cli := sharedClient(t)
		t.Cleanup(func() {
			_, _ = cli.ImageRemove(context.Background(), tag, dockerclient.ImageRemoveOptions{Force: true})
		})
		if _, err := engine.ReadFileFromImage(ctx, tag, "/ctx/kept.txt"); err != nil {
			t.Fatalf("kept.txt missing from image: %v", err)
		}
		if _, err := engine.ReadFileFromImage(ctx, tag, "/ctx/ignored.txt"); err == nil {
			t.Fatal("ignored.txt is in the image; .dockerignore was not applied")
		}
	})

	t.Run("BuildImage/failing_build_returns_error", func(t *testing.T) {
		dir := t.TempDir()
		writeFiles(t, dir, map[string]string{"Dockerfile": "FROM alpine:latest\nRUN exit 3\n"})
		err := engine.BuildImage(ctx, ark.BuildRequest{Dockerfile: filepath.Join(dir, "Dockerfile"), Tag: "ark-conformance-build:fail", Context: dir})
		if err == nil {
			t.Fatal("expected an error from a failing build")
		}
	})

	t.Run("VolumeMountpoint/absent_returns_false", func(t *testing.T) {
		_, ok := engine.VolumeMountpoint(ctx, "ark-conformance-nonexistent-volume")
		if ok {
			t.Fatal("expected ok=false for nonexistent volume")
		}
	})
}

func writeFiles(t *testing.T, dir string, files map[string]string) {
	t.Helper()
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}
