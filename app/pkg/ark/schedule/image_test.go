package schedule

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"pkg/ark"
	"pkg/ark/arktest"
)

func writeFile(t *testing.T, path, content string) string {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestCheckStatic(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "go.mod"), "module tiny\n\ngo 1.21\n")
	writeFile(t, filepath.Join(dir, "main.go"), "package main\n\nfunc main() {}\n")
	bin := filepath.Join(dir, "tiny")
	cmd := exec.Command("go", "build", "-o", bin, ".")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0", "GOWORK=off", "GOFLAGS=")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}
	if err := CheckStatic(bin); err != nil {
		t.Fatalf("static binary rejected: %v", err)
	}
	if err := CheckStatic(writeFile(t, filepath.Join(dir, "script"), "#!/bin/sh\n")); err == nil {
		t.Fatal("non-ELF file accepted")
	}
}

func TestImageRefStable(t *testing.T) {
	dir := t.TempDir()
	a, errA := ImageRef(writeFile(t, filepath.Join(dir, "a"), "same bytes"))
	b, errB := ImageRef(writeFile(t, filepath.Join(dir, "b"), "same bytes"))
	c, errC := ImageRef(writeFile(t, filepath.Join(dir, "c"), "other bytes"))
	if errA != nil || errB != nil || errC != nil {
		t.Fatal(errA, errB, errC)
	}
	if a != b || a == c || !strings.HasPrefix(a, ImageRepo+":") || len(a) != len(ImageRepo)+1+12 {
		t.Fatalf("refs a=%q b=%q c=%q", a, b, c)
	}
}

func TestBuildImage(t *testing.T) {
	dir := t.TempDir()
	bin := writeFile(t, filepath.Join(dir, "ark-cli-bin"), "binary bytes")
	ref, _ := ImageRef(bin)
	buildDir := filepath.Join(dir, "build")

	t.Run("skips existing image", func(t *testing.T) {
		engine := &arktest.MockEngine{
			ImageExistsFn: func(context.Context, string) (bool, error) { return true, nil },
			BuildImageFn: func(context.Context, ark.BuildRequest) error {
				t.Fatal("BuildImage called for an existing image")
				return nil
			},
		}
		got, built, err := BuildImage(context.Background(), engine, bin, buildDir)
		if err != nil || built || got != ref {
			t.Fatalf("got %q built=%v err=%v", got, built, err)
		}
	})

	t.Run("builds missing image", func(t *testing.T) {
		if err := os.MkdirAll(buildDir, 0o755); err != nil {
			t.Fatal(err)
		}
		stale := writeFile(t, filepath.Join(buildDir, "stale"), "")
		var reqs []ark.BuildRequest
		engine := &arktest.MockEngine{
			BuildImageFn: func(_ context.Context, req ark.BuildRequest) error {
				reqs = append(reqs, req)
				return nil
			},
		}
		got, built, err := BuildImage(context.Background(), engine, bin, buildDir)
		if err != nil || !built || got != ref {
			t.Fatalf("got %q built=%v err=%v", got, built, err)
		}
		want := ark.BuildRequest{Dockerfile: filepath.Join(buildDir, "Dockerfile"), Tag: ref, Context: buildDir}
		if len(reqs) != 1 || reqs[0].Dockerfile != want.Dockerfile || reqs[0].Tag != want.Tag || reqs[0].Context != want.Context {
			t.Fatalf("requests %+v", reqs)
		}
		copied, err := os.ReadFile(filepath.Join(buildDir, "ark-cli"))
		if err != nil || string(copied) != "binary bytes" {
			t.Fatalf("copied binary %q err=%v", copied, err)
		}
		if info, _ := os.Stat(filepath.Join(buildDir, "ark-cli")); info.Mode().Perm() != 0o755 {
			t.Fatalf("binary mode %v", info.Mode().Perm())
		}
		if _, err := os.Stat(stale); !os.IsNotExist(err) {
			t.Fatalf("build dir not recreated: %v", err)
		}
		dockerfile, _ := os.ReadFile(want.Dockerfile)
		if string(dockerfile) != schedulerDockerfile {
			t.Fatalf("Dockerfile %q", dockerfile)
		}
	})
}

func TestRemoveOldImages(t *testing.T) {
	var removedCalls []string
	engine := &arktest.MockEngine{
		ImageTagsFn: func(_ context.Context, repo string) (map[string]string, error) {
			if repo != ImageRepo {
				t.Fatalf("repo %q", repo)
			}
			return map[string]string{"ark-scheduler:keep": "id1", "ark-scheduler:old1": "id2", "ark-scheduler:old2": "id3"}, nil
		},
		RemoveImageFn: func(_ context.Context, ref string) error {
			removedCalls = append(removedCalls, ref)
			return nil
		},
	}
	removed, err := RemoveOldImages(context.Background(), engine, "ark-scheduler:keep")
	want := []string{"ark-scheduler:old1", "ark-scheduler:old2"}
	if err != nil || !slices.Equal(removed, want) || !slices.Equal(removedCalls, want) {
		t.Fatalf("removed %v calls %v err %v", removed, removedCalls, err)
	}
}
