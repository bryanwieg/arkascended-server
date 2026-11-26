package update

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	arkcontainer "ark-container"
	"pkg/ark"
	"pkg/ark/arktest"
)

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func testService(engine ark.ContainerEngine) *Service {
	s := New(engine, &arktest.MockCompose{}, nil, discardLogger())
	return s
}

// --- Metadata tests ---

func TestInspectImageVersionInfo(t *testing.T) {
	engine := &arktest.MockEngine{
		ImageLabelsFn: func(_ context.Context, _ string) (map[string]string, error) {
			return map[string]string{
				LabelARKVersion: "81.2",
				LabelARKBuildID: "16491427",
			}, nil
		},
	}
	s := testService(engine)
	info, err := s.InspectImageVersionInfo(context.Background(), "ark-server:latest")
	if err != nil {
		t.Fatal(err)
	}
	if info.Version != "81.2" || info.BuildID != "16491427" {
		t.Fatalf("got version=%q buildID=%q", info.Version, info.BuildID)
	}
}

func TestInspectImageVersionInfoNilEngine(t *testing.T) {
	s := testService(nil)
	_, err := s.InspectImageVersionInfo(context.Background(), "ark-server:latest")
	if err == nil {
		t.Fatal("expected error for nil engine")
	}
}

func TestExtractBuildIDFromDescription(t *testing.T) {
	got := ExtractBuildIDFromDescription("ARK Server Build ID: 16491427")
	if got != "16491427" {
		t.Fatalf("got %q", got)
	}
	if ExtractBuildIDFromDescription("") != "" {
		t.Fatal("expected empty")
	}
}

// --- Local check tests ---

func TestCheckLocalUpdateDetectsNewBuildID(t *testing.T) {
	engine := &arktest.MockEngine{
		ImageLabelsFn: func(_ context.Context, _ string) (map[string]string, error) {
			return map[string]string{LabelARKVersion: "81.1", LabelARKBuildID: "100"}, nil
		},
	}
	s := testService(engine)
	s.FetchText = func(_ string) (string, error) {
		return `{"data":{"2430930":{"depots":{"branches":{"public":{"buildid":"200"}}}}}}`, nil
	}
	result, err := s.CheckLocalUpdate(context.Background(), "ark-server:latest")
	if err != nil {
		t.Fatal(err)
	}
	if !result.UpdateAvailable {
		t.Fatal("expected update available")
	}
}

func TestCheckLocalUpdateDetectsRecipeChange(t *testing.T) {
	for _, tc := range []struct {
		name   string
		recipe string
		want   bool
	}{
		{name: "same recipe", recipe: arkcontainer.RecipeID(), want: false},
		{name: "other recipe", recipe: "000000000000", want: true},
		{name: "no recipe label", recipe: "", want: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := testService(&arktest.MockEngine{
				ImageLabelsFn: func(_ context.Context, _ string) (map[string]string, error) {
					return map[string]string{LabelARKBuildID: "200", LabelARKRecipe: tc.recipe}, nil
				},
			})
			s.FetchText = func(string) (string, error) { return steamBuild200, nil }
			result, err := s.CheckLocalUpdate(context.Background(), "ark-server:200")
			if err != nil {
				t.Fatal(err)
			}
			if result.UpdateAvailable != tc.want {
				t.Fatalf("UpdateAvailable = %v, want %v (%s)", result.UpdateAvailable, tc.want, result.Message)
			}
		})
	}
}

func TestCheckLocalUpdateMissingBuildID(t *testing.T) {
	engine := &arktest.MockEngine{
		ImageLabelsFn: func(_ context.Context, _ string) (map[string]string, error) {
			return map[string]string{LabelARKVersion: "81.1"}, nil
		},
	}
	s := testService(engine)
	result, _ := s.CheckLocalUpdate(context.Background(), "ark-server:latest")
	if !result.CheckFailed || !strings.Contains(result.Message, "build id label missing") {
		t.Fatalf("expected check failed, got: %+v", result)
	}
}

// --- Steam API tests ---

func TestParseSteamLatestBuildID(t *testing.T) {
	body := `{"data":{"2430930":{"depots":{"branches":{"public":{"buildid":"16491427"}}}}}}`
	got, err := ParseSteamLatestBuildID(body)
	if err != nil || got != "16491427" {
		t.Fatalf("got %q, err=%v", got, err)
	}
}

func TestParseSteamLatestBuildIDErrors(t *testing.T) {
	for _, body := range []string{
		`{"data":{"999999":{}}}`,
		`{"data":{"2430930":{"depots":{"branches":{"beta":{}}}}}}`,
		`{not json`,
	} {
		if _, err := ParseSteamLatestBuildID(body); err == nil {
			t.Errorf("expected error for %q", body)
		}
	}
}

// --- Build tests ---

const steamBuild200 = `{"data":{"2430930":{"depots":{"branches":{"public":{"buildid":"200"}}}}}}`

type buildRig struct {
	builds  []ark.BuildRequest
	runs    []ark.RunContainerRequest
	copies  []string
	copyErr error
	removed []string
	exists  map[string]bool
	pulls   []string
	pullErr error
	baseID  string
	free    int64
	steam   error
}

func (r *buildRig) service() *Service {
	s := testService(&arktest.MockEngine{
		ImageExistsFn: func(_ context.Context, image string) (bool, error) { return r.exists[image], nil },
		ImageLabelsFn: func(_ context.Context, _ string) (map[string]string, error) {
			return map[string]string{LabelARKVersion: "98.6", LabelARKBuildID: "200", LabelARKRecipe: arkcontainer.RecipeID()}, nil
		},
		PullImageFn: func(_ context.Context, ref string) error {
			r.pulls = append(r.pulls, ref)
			return r.pullErr
		},
		BuildImageFn: func(_ context.Context, req ark.BuildRequest) error {
			r.builds = append(r.builds, req)
			return nil
		},
		ReadFileFromImageFn: func(_ context.Context, _, path string) (string, error) {
			if strings.Contains(path, "VERSION") {
				return "98.6", nil
			}
			return r.baseID, nil
		},
		RemoveImageFn: func(_ context.Context, ref string) error {
			r.removed = append(r.removed, ref)
			return nil
		},
		RunContainerFn: func(_ context.Context, req ark.RunContainerRequest) (int, error) {
			r.runs = append(r.runs, req)
			return 0, nil
		},
		CopyFromImageFn: func(_ context.Context, image, src, dst string) error {
			r.copies = append(r.copies, image+":"+src+" -> "+dst)
			return r.copyErr
		},
	})
	s.FetchText = func(string) (string, error) { return steamBuild200, r.steam }
	s.DiskUsage = func(string) (int64, int64, error) { return r.free, r.free, nil }
	return s
}

func newBuildRig() *buildRig {
	return &buildRig{exists: map[string]bool{arkcontainer.BaseImageRef(): true}, baseID: "200", free: 100 << 30}
}

func TestBuildLocalImageBuildsFromEmbeddedRecipe(t *testing.T) {
	r := newBuildRig()
	stateDir := t.TempDir()
	result, err := r.service().BuildLocalImage(context.Background(), stateDir)
	if err != nil {
		t.Fatal(err)
	}
	if result != (BuildResult{Image: LocalImageRef("200"), BuildID: "200", Version: "98.6", Built: true}) {
		t.Fatalf("result %+v", result)
	}
	contextDir := filepath.Join(stateDir, "build", "context")
	if _, err := os.Stat(filepath.Join(contextDir, "dockerfile")); err != nil {
		t.Fatalf("recipe not extracted: %v", err)
	}
	if len(r.builds) != 4 {
		t.Fatalf("expected 4 builds, got %d", len(r.builds))
	}
	if r.builds[2].Tag != "ark-server:base" || r.builds[2].BuildArgs["ARK_FILES_IMAGE"] != "ark-server-files:200" ||
		r.builds[2].BuildArgs["BASE_IMAGE"] != arkcontainer.BaseImageRef() {
		t.Fatalf("base build %+v", r.builds[2])
	}
	labeled := r.builds[3]
	if labeled.Tag != LocalImageRef("200") || strings.Join(labeled.ExtraTags, ",") != "ark-server:98.6,ark-server:latest" {
		t.Fatalf("labeled build tags %q %v", labeled.Tag, labeled.ExtraTags)
	}
	if labeled.BuildArgs["BASE_IMAGE"] != "ark-server:base" || labeled.BuildArgs["ARK_BUILD_ID"] != "200" || labeled.BuildArgs["ARK_SERVER_VERSION"] != "98.6" ||
		labeled.BuildArgs["ARK_RECIPE_ID"] != arkcontainer.RecipeID() {
		t.Fatalf("labeled build args %v", labeled.BuildArgs)
	}
	if strings.Join(r.removed, ",") != "ark-server-files:200" {
		t.Fatalf("removed %v, want the files image", r.removed)
	}
}

func TestBuildLocalImageGetsTheBaseImage(t *testing.T) {
	base := arkcontainer.BaseImageRef()
	for _, tc := range []struct {
		name       string
		local      bool
		pullErr    error
		wantPull   bool
		wantBuilds int // base build + the steamcmd, files and two runtime builds
	}{
		{name: "local copy", local: true, wantBuilds: 4},
		{name: "pulled", wantPull: true, wantBuilds: 4},
		{name: "pull fails: built here", pullErr: errors.New("denied"), wantPull: true, wantBuilds: 5},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := newBuildRig()
			r.exists = map[string]bool{base: tc.local}
			r.pullErr = tc.pullErr
			if _, err := r.service().BuildLocalImage(context.Background(), t.TempDir()); err != nil {
				t.Fatal(err)
			}
			if pulled := len(r.pulls) > 0; pulled != tc.wantPull || (pulled && r.pulls[0] != base) {
				t.Fatalf("pulls %v, want pull=%v of %s", r.pulls, tc.wantPull, base)
			}
			if len(r.builds) != tc.wantBuilds {
				t.Fatalf("got %d builds, want %d", len(r.builds), tc.wantBuilds)
			}
			if tc.wantBuilds == 5 && (r.builds[0].Tag != base || !strings.HasSuffix(r.builds[0].Dockerfile, "base.dockerfile")) {
				t.Fatalf("first build %+v, want base.dockerfile tagged %s", r.builds[0], base)
			}
			if got := r.builds[len(r.builds)-2].BuildArgs["BASE_IMAGE"]; got != base {
				t.Fatalf("runtime build BASE_IMAGE %q, want %s", got, base)
			}
		})
	}
}

func TestBuildLocalImageSkipsExistingBuild(t *testing.T) {
	r := newBuildRig()
	r.exists[LocalImageRef("200")] = true
	result, err := r.service().BuildLocalImage(context.Background(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if result.Built || result.Image != LocalImageRef("200") || result.Version != "98.6" || len(r.runs) != 0 || len(r.builds) != 0 {
		t.Fatalf("result %+v, runs %d, builds %d; want the existing image and no work", result, len(r.runs), len(r.builds))
	}
}

func TestBuildLocalImageRefusesWithoutWork(t *testing.T) {
	for _, tc := range []struct {
		name, env, want string
		setup           func(*buildRig)
	}{
		{name: "low disk", want: "needs about 50 GB free", setup: func(r *buildRig) { r.free = 10 << 30 }},
		{name: "bad disk override", env: "lots", want: "ARK_BUILD_REQUIRED_FREE_GB must be a positive whole number"},
		{name: "steam offline", want: "cannot read ARK's current build", setup: func(r *buildRig) { r.steam = errors.New("offline") }},
		{name: "steam moved on", want: "built ARK build 201, but Steam reported 200", setup: func(r *buildRig) { r.baseID = "201" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("ARK_BUILD_REQUIRED_FREE_GB", tc.env)
			r := newBuildRig()
			if tc.setup != nil {
				tc.setup(r)
			}
			_, err := r.service().BuildLocalImage(context.Background(), t.TempDir())
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want %q", err, tc.want)
			}
			if tc.name != "steam moved on" && len(r.builds) != 0 {
				t.Fatalf("no image build expected, got %d", len(r.builds))
			}
		})
	}
}

func TestBuildLocalImageStage1Failure(t *testing.T) {
	r := newBuildRig()
	s := r.service()
	s.engine = &arktest.MockEngine{
		BuildImageFn: func(_ context.Context, _ ark.BuildRequest) error { return errors.New("build failed") },
	}
	_, err := s.BuildLocalImage(context.Background(), t.TempDir())
	if err == nil || !strings.Contains(err.Error(), "base image build failed") {
		t.Fatalf("expected base build error, got: %v", err)
	}
}

// TestBuildLocalImageReportsDownloadFailureReason: a failed download reports download.sh's
// one-line reason, and a reason left by an earlier run is never reported.
func TestBuildLocalImageReportsDownloadFailureReason(t *testing.T) {
	for _, tc := range []struct {
		name, writes, want string
	}{
		{"reason written", "SteamCMD download failed: state is 0x6\n", "server files build failed: SteamCMD download failed: state is 0x6"},
		{"no reason", "", "server files build failed: exit code 8"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stateDir := t.TempDir()
			failure := filepath.Join(stateDir, "build", "work", "failure")
			if err := os.MkdirAll(filepath.Dir(failure), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(failure, []byte("stale reason from an earlier run"), 0o644); err != nil {
				t.Fatal(err)
			}
			s := newBuildRig().service()
			s.engine.(*arktest.MockEngine).RunContainerFn = func(context.Context, ark.RunContainerRequest) (int, error) {
				if tc.writes != "" {
					if err := os.WriteFile(failure, []byte(tc.writes), 0o644); err != nil {
						t.Fatal(err)
					}
				}
				return 8, nil
			}
			_, err := s.BuildLocalImage(context.Background(), stateDir)
			if err == nil || !strings.HasSuffix(err.Error(), tc.want) {
				t.Fatalf("got %v, want ...%q", err, tc.want)
			}
		})
	}
}

// TestBuildLocalImageRunsSteamCMDThroughEngine: the SteamCMD image, its run and the
// files image go through the engine with build.sh's names, binds, env and user.
func TestBuildLocalImageRunsSteamCMDThroughEngine(t *testing.T) {
	for _, tc := range []struct {
		name     string
		rootless bool
		wantUser string
	}{
		{"rootful", false, fmt.Sprintf("%d:%d", os.Getuid(), os.Getgid())},
		{"rootless", true, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := newBuildRig()
			s := r.service()
			s.engine.(*arktest.MockEngine).RootlessFn = func(context.Context) (bool, error) { return tc.rootless, nil }
			var output bytes.Buffer
			s.Output = &output
			stateDir := t.TempDir()
			if _, err := s.BuildLocalImage(context.Background(), stateDir); err != nil {
				t.Fatal(err)
			}
			filesDir := filepath.Join(stateDir, "build", "context", "files")
			workDir := filepath.Join(stateDir, "build", "work")
			if len(r.builds) != 4 || len(r.runs) != 1 {
				t.Fatalf("got %d builds and %d runs, want 4 and 1", len(r.builds), len(r.runs))
			}
			steamcmd := r.builds[0]
			if steamcmd.Tag != "arkascended-steamcmd" || steamcmd.Context != filesDir || steamcmd.Dockerfile != filepath.Join(filesDir, "steamcmd.dockerfile") {
				t.Fatalf("steamcmd build %+v", steamcmd)
			}
			sum := sha256.Sum256([]byte(stateDir))
			run := r.runs[0]
			if run.Name != "ark-steamcmd-build-"+hex.EncodeToString(sum[:])[:12] || run.Image != "arkascended-steamcmd" ||
				strings.Join(run.Cmd, " ") != "bash /src/download.sh /work" ||
				strings.Join(run.Env, " ") != "ARK_STEAM_BUILD_ID=200" ||
				strings.Join(run.Binds, " ") != filesDir+":/src:ro "+workDir+":/work" ||
				run.User != tc.wantUser || run.Output != &output {
				t.Fatalf("steamcmd run %+v", run)
			}
			files := r.builds[1]
			outDir := filepath.Join(workDir, "out")
			if files.Tag != "ark-server-files:200" || files.Context != outDir || files.Dockerfile != filepath.Join(outDir, "dockerfile") ||
				files.BuildArgs["ARK_STEAM_BUILD_ID"] != "200" {
				t.Fatalf("files build %+v", files)
			}
			if _, err := os.Stat(outDir); !os.IsNotExist(err) {
				t.Fatalf("work/out must be removed after the build, stat: %v", err)
			}
		})
	}
}

// TestBuildLocalImageSeedsDepotCache: the depot cache is seeded from the seed image only
// when there is none, and a failed copy does not fail the build.
func TestBuildLocalImageSeedsDepotCache(t *testing.T) {
	for _, tc := range []struct {
		name               string
		hasCache, hasImage bool
		copyErr            error
		wantCopy           bool
	}{
		{"no cache, seed image", false, true, nil, true},
		{"copy fails", false, true, errors.New("no such path"), true},
		{"cache exists", true, true, nil, false},
		{"no seed image", false, false, nil, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := newBuildRig()
			r.exists["ark-server:latest"] = tc.hasImage
			r.copyErr = tc.copyErr
			stateDir := t.TempDir()
			depotCache := filepath.Join(stateDir, "build", "work", "cache", "depotcache")
			if tc.hasCache {
				if err := os.MkdirAll(depotCache, 0o755); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := r.service().BuildLocalImage(context.Background(), stateDir); err != nil {
				t.Fatal(err)
			}
			want := []string{"ark-server:latest:/home/ark-user/.local/share/Steam/depotcache -> " + depotCache}
			if !tc.wantCopy {
				want = nil
			}
			if strings.Join(r.copies, "\n") != strings.Join(want, "\n") {
				t.Fatalf("copies %v, want %v", r.copies, want)
			}
			if len(r.runs) != 1 {
				t.Fatalf("SteamCMD ran %d times, want 1", len(r.runs))
			}
		})
	}
}

// --- PlanLocalUpdate tests ---

func TestPlanLocalUpdateAvailable(t *testing.T) {
	check := LocalCheckResult{UpdateAvailable: true, Message: "update available"}
	row := PlanLocalUpdate("asa1", "ark-server:latest", check)
	if row.Action != ActionUpdate {
		t.Fatalf("expected update, got %s", row.Action)
	}
}

func TestPlanLocalUpdateCheckFailed(t *testing.T) {
	check := LocalCheckResult{CheckFailed: true, Message: "failed"}
	row := PlanLocalUpdate("asa1", "ark-server:latest", check)
	if row.Action != ActionWarn {
		t.Fatalf("expected warn, got %s", row.Action)
	}
}
