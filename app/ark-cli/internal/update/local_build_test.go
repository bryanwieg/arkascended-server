package update

// Tests: local image builds: build-when-missing trigger, error surfacing,
// docker command sequence, version extraction, stage-2 tagging

import (
	"ark-cli/internal/core"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"pkg/ark"
	"pkg/ark/arktest"

	"github.com/compose-spec/compose-go/v2/types"
)

// TestEnsureLocalImagesBuildsWhenMissing validates local start builds when images are absent.
// - Expect: runLocalBuild is invoked when dockerImageExists returns false.
func TestEnsureLocalImagesBuildsWhenMissing(t *testing.T) {
	tmp := t.TempDir()
	composeFile := filepath.Join(tmp, "docker-compose.yaml")
	project := fakeProject(types.Services{"asa1": {Name: "asa1", Image: "ark-server:latest"}})
	services := []string{"asa1"}
	built := false
	withPatchedAppDeps(t, func(d *core.AppDeps) {
		d.HostDeps.FetchText = func(string) (string, error) {
			return `{"data":{"2430930":{"depots":{"branches":{"public":{"buildid":"16491427"}}}}}}`, nil
		}
		d.DiskUsageForPath = func(string) (int64, int64, error) { return 100 << 30, 100 << 30, nil }
		d.Engine = &arktest.MockEngine{
			// download.sh's output, so the build can move work/out into the context.
			RunContainerFn: func(_ context.Context, req ark.RunContainerRequest) (int, error) {
				work := strings.TrimSuffix(req.Binds[len(req.Binds)-1], ":/work")
				return 0, os.MkdirAll(filepath.Join(work, "out", "steam-client"), 0o755)
			},
			ImageExistsFn: func(_ context.Context, image string) (bool, error) {
				return false, nil
			},
			ReadFileFromImageFn: func(_ context.Context, image, path string) (string, error) {
				if strings.Contains(path, "VERSION") {
					return "82.3", nil
				}
				if strings.Contains(path, "BUILDID") {
					return "16491427", nil
				}
				return "", nil
			},
			BuildImageFn: func(_ context.Context, req ark.BuildRequest) error {
				built = true
				return nil
			},
		}
	}, func() {
		if err := core.CurrentAppDeps().RuntimeHelper.EnsureLocalImages(composeFile, project, services); err != nil {
			t.Fatalf("ensureLocalImages: %v", err)
		}
	})
	if !built {
		t.Fatal("expected local build to run")
	}
}

// TestEnsureLocalImagesErrorsWithoutImage checks behavior when services lack an image field.
// - Expect: ensureLocalImages returns an error about missing images.
func TestEnsureLocalImagesErrorsWithoutImage(t *testing.T) {
	tmpDir := t.TempDir()
	project := fakeProject(emptyServices("asa1"))
	if err := core.CurrentAppDeps().RuntimeHelper.EnsureLocalImages(tmpDir+"/docker-compose.yaml", project, []string{"asa1"}); err == nil {
		t.Fatal("expected error")
	}
}

// TestEnsureLocalImagesSurfaceImageInspectError verifies image inspect errors are surfaced.
// - Expect: ensureLocalImages returns the underlying inspect error.
func TestEnsureLocalImagesSurfaceImageInspectError(t *testing.T) {
	tmpDir := t.TempDir()
	project := fakeProject(types.Services{"asa1": {Name: "asa1", Image: "ark-server:latest"}})
	withPatchedAppDeps(t, func(d *core.AppDeps) {
		d.Engine = &arktest.MockEngine{
			ImageExistsFn: func(_ context.Context, image string) (bool, error) {
				return false, errors.New("boom")
			},
		}
	}, func() {
		if err := core.CurrentAppDeps().RuntimeHelper.EnsureLocalImages(tmpDir+"/docker-compose.yaml", project, []string{"asa1"}); err == nil {
			t.Fatal("expected error")
		}
	})
}

// TestUpdateRefusesBelowBuildDiskDefault validates update instance checks free disk before building.
// - Expect: with too little free disk and no SteamCMD cache, the build refuses with the default.
func TestUpdateRefusesBelowBuildDiskDefault(t *testing.T) {
	t.Setenv("ARK_BUILD_REQUIRED_FREE_GB", "")
	composePath := filepath.Join(t.TempDir(), "docker-compose.yaml")
	writeUpdateComposeFixture(t, composePath, "name: test\nservices:\n  asa1:\n    labels: {ark.port.asa: '27015'}\n    image: ark-server:100\n")
	withLocalSteamCheck(t, "81.1", "100", "200", func(d *core.AppDeps) {
		d.DiskUsageForPath = func(string) (int64, int64, error) { return 89 << 30, 100 << 30, nil }
	}, func() {
		err := RunUpdate(core.CurrentAppDeps(), []string{"--compose-file", composePath, "--instance", "asa1"})
		if err == nil || !strings.Contains(err.Error(), "needs about 90 GB free") {
			t.Fatalf("want the free-disk refusal, got %v", err)
		}
	})
}
