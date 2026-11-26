//go:build integration

// Package conformance provides test suites that verify ark.ContainerEngine and
// ark.ComposeService implementations behave identically. Both the shell and SDK
// backends run these same tests against a real Docker daemon.
package conformance

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

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

	t.Run("RunContainer", func(t *testing.T) {
		pullAlpine(t)
		cli := sharedClient(t)
		gone := func(t *testing.T, name string) {
			t.Helper()
			if _, err := engine.InspectState(ctx, name); !errors.Is(err, ark.ErrNotFound) {
				t.Fatalf("container %s left behind (InspectState err: %v)", name, err)
			}
		}

		t.Run("exit_code_output_and_leftover", func(t *testing.T) {
			name := "ark-conformance-run"
			// A container of the same name left by a killed earlier run.
			if _, err := cli.ContainerCreate(ctx, dockerclient.ContainerCreateOptions{
				Name: name, Config: &container.Config{Image: "alpine:latest", Cmd: []string{"true"}},
			}); err != nil {
				t.Fatalf("create leftover: %v", err)
			}
			CleanupContainer(t, engine, name)
			var out bytes.Buffer
			code, err := engine.RunContainer(ctx, ark.RunContainerRequest{
				Name: name, Image: "alpine:latest", Cmd: []string{"sh", "-c", "echo hi; exit 3"}, Output: &out,
			})
			if err != nil || code != 3 || !strings.Contains(out.String(), "hi") {
				t.Fatalf("RunContainer = %d, %v, output %q; want 3, nil, hi", code, err, out.String())
			}
			gone(t, name)
		})

		t.Run("bind_and_user", func(t *testing.T) {
			rootless, err := engine.Rootless(ctx)
			if err != nil {
				t.Fatalf("Rootless: %v", err)
			}
			// On a rootless engine container root maps to the caller.
			user := ""
			if !rootless {
				user = fmt.Sprintf("%d:%d", os.Getuid(), os.Getgid())
			}
			dir := t.TempDir()
			code, err := engine.RunContainer(ctx, ark.RunContainerRequest{
				Image: "alpine:latest", Cmd: []string{"sh", "-c", "echo x > /work/f"}, Binds: []string{dir + ":/work"}, User: user,
			})
			if err != nil || code != 0 {
				t.Fatalf("RunContainer = %d, %v", code, err)
			}
			info, err := os.Stat(filepath.Join(dir, "f"))
			if err != nil {
				t.Fatalf("file not written through the bind: %v", err)
			}
			if st := info.Sys().(*syscall.Stat_t); int(st.Uid) != os.Getuid() {
				t.Fatalf("file owned by uid %d, want %d", st.Uid, os.Getuid())
			}
		})

		t.Run("missing_image_returns_ErrNotFound", func(t *testing.T) {
			name := "ark-conformance-run-missing"
			_, err := engine.RunContainer(ctx, ark.RunContainerRequest{Name: name, Image: "ark-conformance-nonexistent:never", Cmd: []string{"true"}})
			if !errors.Is(err, ark.ErrNotFound) {
				t.Fatalf("expected ErrNotFound, got: %v", err)
			}
			gone(t, name)
		})

		t.Run("cancel_stops_and_removes", func(t *testing.T) {
			name := "ark-conformance-run-cancel"
			CleanupContainer(t, engine, name)
			cctx, cancel := context.WithCancel(ctx)
			defer cancel()
			time.AfterFunc(2*time.Second, cancel)
			_, err := engine.RunContainer(cctx, ark.RunContainerRequest{Name: name, Image: "alpine:latest", Cmd: []string{"sleep", "60"}})
			if !errors.Is(err, context.Canceled) {
				t.Fatalf("expected context.Canceled, got: %v", err)
			}
			gone(t, name)
		})
	})

	t.Run("CopyFromImage", func(t *testing.T) {
		pullAlpine(t)
		cli := sharedClient(t)
		alpineContainers := func(t *testing.T) int {
			t.Helper()
			list, err := cli.ContainerList(ctx, dockerclient.ContainerListOptions{All: true, Filters: dockerclient.Filters{}.Add("ancestor", "alpine:latest")})
			if err != nil {
				t.Fatalf("ContainerList: %v", err)
			}
			return len(list.Items)
		}

		t.Run("copies_directory", func(t *testing.T) {
			before := alpineContainers(t)
			dst := filepath.Join(t.TempDir(), "apk")
			if err := engine.CopyFromImage(ctx, "alpine:latest", "/etc/apk", dst); err != nil {
				t.Fatalf("CopyFromImage: %v", err)
			}
			// Like `docker cp` to a new path: dst holds the directory's contents.
			if _, err := os.Stat(filepath.Join(dst, "world")); err != nil {
				t.Fatalf("expected %s/world: %v", dst, err)
			}
			if info, err := os.Stat(filepath.Join(dst, "keys")); err != nil || !info.IsDir() {
				t.Fatalf("expected directory %s/keys: %v", dst, err)
			}
			if after := alpineContainers(t); after != before {
				t.Fatalf("%d alpine containers before, %d after: one was left", before, after)
			}
		})

		t.Run("missing_returns_ErrNotFound", func(t *testing.T) {
			dir := t.TempDir()
			if err := engine.CopyFromImage(ctx, "ark-conformance-nonexistent:never", "/etc", filepath.Join(dir, "a")); !errors.Is(err, ark.ErrNotFound) {
				t.Fatalf("missing image: expected ErrNotFound, got: %v", err)
			}
			if err := engine.CopyFromImage(ctx, "alpine:latest", "/nonexistent", filepath.Join(dir, "b")); !errors.Is(err, ark.ErrNotFound) {
				t.Fatalf("missing path: expected ErrNotFound, got: %v", err)
			}
		})
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

	t.Run("InspectState/running_has_StartedAt", func(t *testing.T) {
		pullAlpine(t)
		id := createAndStartContainer(t, "sleep 30")
		CleanupContainer(t, engine, id)
		state, err := engine.InspectState(ctx, id)
		if err != nil {
			t.Fatalf("InspectState: %v", err)
		}
		if !state.Running || state.StartedAt.IsZero() || time.Since(state.StartedAt) > time.Minute {
			t.Fatalf("expected running with recent StartedAt, got %+v", state)
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
		inUse, err := engine.ImagesInUse(ctx)
		if err != nil {
			t.Fatalf("ImagesInUse: %v", err)
		}
		if !inUse[tags[repo+":inuse"]] {
			t.Fatalf("ImagesInUse %v misses %s, used by a created container", inUse, tags[repo+":inuse"])
		}
	})

	t.Run("PruneDangling", func(t *testing.T) {
		if err := engine.PruneDangling(ctx); err != nil {
			t.Fatalf("PruneDangling: %v", err)
		}
	})

	t.Run("Rootless", func(t *testing.T) {
		rootless, err := engine.Rootless(ctx)
		if err != nil {
			t.Fatalf("Rootless: %v", err)
		}
		// ARK_TEST_EXPECT_ROOTLESS (1/0) pins the expected answer for the engine under test.
		if want, ok := os.LookupEnv("ARK_TEST_EXPECT_ROOTLESS"); ok && rootless != (want == "1") {
			t.Fatalf("Rootless = %v, ARK_TEST_EXPECT_ROOTLESS=%s", rootless, want)
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
