package update

// Tests: update command: target/channel request building, remote service image mutation,
// local steam-based update detection, local rebuild deduplication, missing service handling

import (
	"ark-cli/internal/core"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"pkg/ark"
	"pkg/ark/arktest"
	"pkg/ark/runtime"

	arkcontainer "ark-container"
)

func writeUpdateComposeFixture(t *testing.T, composePath, seed string) {
	t.Helper()
	if err := os.WriteFile(composePath, []byte(seed), 0o644); err != nil {
		t.Fatalf("write compose: %v", err)
	}
}

// fakeSteamAPIResponse returns a JSON string mimicking the steamcmd.net API
// with the given public branch build ID.
func fakeSteamAPIResponse(buildID string) string {
	resp := map[string]interface{}{
		"data": map[string]interface{}{
			"2430930": map[string]interface{}{
				"depots": map[string]interface{}{
					"branches": map[string]interface{}{
						"public": map[string]interface{}{
							"buildid": buildID,
						},
					},
				},
			},
		},
	}
	b, _ := json.Marshal(resp)
	return string(b)
}

// fakeImageLabelsJSON returns a JSON string mimicking docker image inspect labels output.
func fakeImageLabelsJSON(version, buildID string) string {
	labels := map[string]string{
		"io.ark.server.version":  version,
		"io.ark.server.build_id": buildID,
	}
	b, _ := json.Marshal(labels)
	return string(b)
}

// withLocalSteamCheck sets up mocked deps for local image update checks.
// It mocks fetchText for Steam API and engine.ImageLabels for label inspection.
func withLocalSteamCheck(
	t *testing.T,
	currentVersion, currentBuildID, latestBuildID string,
	mutate func(*core.AppDeps),
	run func(),
) {
	t.Helper()
	withPatchedAppDeps(t, func(d *core.AppDeps) {
		d.HostDeps.FetchText = func(url string) (string, error) {
			if strings.Contains(url, "steamcmd.net") {
				return fakeSteamAPIResponse(latestBuildID), nil
			}
			return "", nil
		}
		d.Engine = &arktest.MockEngine{
			ImageLabelsFn: func(_ context.Context, image string) (map[string]string, error) {
				return map[string]string{
					"io.ark.server.version":  currentVersion,
					"io.ark.server.build_id": currentBuildID,
					"io.ark.recipe":          arkcontainer.RecipeID(),
				}, nil
			},
		}
		if mutate != nil {
			mutate(d)
		}
	}, run)
}

func TestBuildUpdateCommandRequestDefaultsTargetAll(t *testing.T) {
	req, err := buildUpdateCommandRequest("all", "", true, false, false)
	if err != nil {
		t.Fatalf("buildUpdateCommandRequest: %v", err)
	}
	if req.Target != "all" {
		t.Fatalf("expected default target all, got %q", req.Target)
	}
}

func TestRunUpdateCheckDefaultsToAllTarget(t *testing.T) {
	composePath := filepath.Join(t.TempDir(), "docker-compose.yaml")
	seed := strings.Join([]string{
		"name: test",
		"services:",
		"  asa1:",
		"    labels: {ark.port.asa: '27015'}",
		"    image: ark-server:100",
		"  asa2:",
		"    labels: {ark.port.asa: '27015'}",
		"    image: ark-server:100",
		"",
	}, "\n")
	writeUpdateComposeFixture(t, composePath, seed)

	withLocalSteamCheck(t, "81.1", "100", "200", nil, func() {
		out := captureStdout(t, func() {
			if err := RunUpdate(core.CurrentAppDeps(), []string{"--check", "--compose-file", composePath}); err != nil {
				t.Fatalf("runUpdate: %v", err)
			}
		})
		for _, instance := range []string{"asa1", "asa2"} {
			if !strings.Contains(out, instance+" | 81.1 |") {
				t.Fatalf("expected %s row in output:\n%s", instance, out)
			}
		}
	})

	after, err := os.ReadFile(composePath)
	if err != nil {
		t.Fatalf("read compose: %v", err)
	}
	if string(after) != seed {
		t.Fatalf("expected --check to avoid compose mutations\nwant:\n%s\n\ngot:\n%s", seed, string(after))
	}
}

func TestRunUpdateWarnsOnNonLocalImage(t *testing.T) {
	composePath := filepath.Join(t.TempDir(), "docker-compose.yaml")
	writeUpdateComposeFixture(t, composePath, "name: test\nservices:\n  asa1:\n    labels: {ark.port.asa: '27015'}\n    image: example/ark-server:81.1\n")
	withLocalSteamCheck(t, "81.1", "100", "200", nil, func() {
		out := captureStdout(t, func() {
			if err := RunUpdate(core.CurrentAppDeps(), []string{"--compose-file", composePath, "--rollout"}); err != nil {
				t.Fatalf("runUpdate: %v", err)
			}
		})
		if !strings.Contains(out, "asa1 | example/ark-server:81.1 | - | warn | only locally built ark-server images are supported") {
			t.Fatalf("expected a warn row:\n%s", out)
		}
	})
}

func TestRunUpdateMissingService(t *testing.T) {
	composePath := filepath.Join(t.TempDir(), "docker-compose.yaml")
	writeUpdateComposeFixture(t, composePath, "name: test\nservices:\n  asa1:\n    labels: {ark.port.asa: '27015'}\n    image: ark-server:100\n")
	withLocalSteamCheck(t, "81.1", "100", "200", nil, func() {
		if err := RunUpdate(core.CurrentAppDeps(), []string{"--compose-file", composePath, "--instance", "asa2"}); err == nil {
			t.Fatal("expected missing service error")
		}
	})
}

// TestRunUpdateSkipsSchedulerService verifies --instance all never targets the scheduler and
// naming it directly is rejected.
func TestRunUpdateSkipsSchedulerService(t *testing.T) {
	composePath := filepath.Join(t.TempDir(), "docker-compose.yaml")
	writeUpdateComposeFixture(t, composePath, strings.Join([]string{
		"name: test",
		"services:",
		"  asa1:",
		"    labels: {ark.port.asa: '27015'}",
		"    image: ark-server:100",
		"  ark-scheduler:",
		"    image: ark-scheduler:abc",
		"    labels:",
		"      ark.role: scheduler",
		"",
	}, "\n"))
	withLocalSteamCheck(t, "81.1", "100", "200", nil, func() {
		out := captureStdout(t, func() {
			if err := RunUpdate(core.CurrentAppDeps(), []string{"--check", "--compose-file", composePath}); err != nil {
				t.Fatalf("runUpdate: %v", err)
			}
		})
		if !strings.Contains(out, "asa1 | 81.1 |") || strings.Contains(out, "ark-scheduler") {
			t.Fatalf("expected only the asa1 row:\n%s", out)
		}
		err := RunUpdate(core.CurrentAppDeps(), []string{"--check", "--compose-file", composePath, "--instance", "ark-scheduler"})
		if err == nil || !strings.Contains(err.Error(), "is the scheduler service") {
			t.Fatalf("expected scheduler error, got %v", err)
		}
	})
}

// TestRunUpdateCheckLocalImageReportsUpdateViaSteamAPI validates that local image
// update detection queries the Steam API and compares build IDs.
func TestRunUpdateCheckLocalImageReportsUpdateViaSteamAPI(t *testing.T) {
	composePath := filepath.Join(t.TempDir(), "docker-compose.yaml")
	seed := "name: test\nservices:\n  asa1:\n    labels: {ark.port.asa: '27015'}\n    image: ark-server:latest\n"
	writeUpdateComposeFixture(t, composePath, seed)

	withLocalSteamCheck(t, "81.1", "100", "200", nil, func() {
		out := captureStdout(t, func() {
			if err := RunUpdate(core.CurrentAppDeps(), []string{"--check", "--compose-file", composePath}); err != nil {
				t.Fatalf("runUpdate: %v", err)
			}
		})
		// The target is Steam's build: its ARK version is unknown until the image is built.
		if !strings.Contains(out, "asa1 | 81.1 | build 200 | update |") {
			t.Fatalf("expected an asa1 update row targeting build 200:\n%s", out)
		}
	})
}

// TestRunUpdateCheckLocalImageNoUpdateWhenBuildIDMatches validates that matching
// build IDs result in a skip action.
func TestRunUpdateCheckLocalImageNoUpdateWhenBuildIDMatches(t *testing.T) {
	composePath := filepath.Join(t.TempDir(), "docker-compose.yaml")
	seed := "name: test\nservices:\n  asa1:\n    labels: {ark.port.asa: '27015'}\n    image: ark-server:latest\n"
	writeUpdateComposeFixture(t, composePath, seed)

	withLocalSteamCheck(t, "81.1", "100", "100", nil, func() {
		out := captureStdout(t, func() {
			if err := RunUpdate(core.CurrentAppDeps(), []string{"--check", "--compose-file", composePath}); err != nil {
				t.Fatalf("runUpdate: %v", err)
			}
		})
		if !strings.Contains(out, "skip") {
			t.Fatalf("expected skip action when build IDs match:\n%s", out)
		}
	})
}

// TestRunUpdateRebuildsLocalImageOncePerUniqueImage validates that two services
// sharing the same local image trigger only one Steam check and one rebuild.
func TestRunUpdateRebuildsLocalImageOncePerUniqueImage(t *testing.T) {
	composeTmp := t.TempDir()
	composePath := filepath.Join(composeTmp, "docker-compose.yaml")
	seed := strings.Join([]string{
		"name: test",
		"services:",
		"  asa1:",
		"    labels: {ark.port.asa: '27015'}",
		"    image: ark-server:latest",
		"  asa2:",
		"    labels: {ark.port.asa: '27015'}",
		"    image: ark-server:latest",
		"",
	}, "\n")
	writeUpdateComposeFixture(t, composePath, seed)

	steamCalls := 0
	rebuildCalls := 0
	withPatchedAppDeps(t, func(d *core.AppDeps) {
		d.HostDeps.FetchText = func(url string) (string, error) {
			if strings.Contains(url, "steamcmd.net") {
				steamCalls++
				return fakeSteamAPIResponse("200"), nil
			}
			return "", nil
		}
		d.Runtime.RunCommand = func([]string, ark.CommandOptions) (ark.ExecResult, error) { return ark.ExecResult{}, nil }
		d.DiskUsageForPath = func(string) (int64, int64, error) { return 100 << 30, 100 << 30, nil }
		d.Engine = &arktest.MockEngine{
			ImageLabelsFn: func(_ context.Context, image string) (map[string]string, error) {
				return map[string]string{
					"io.ark.server.version":  "81.1",
					"io.ark.server.build_id": "100",
				}, nil
			},
			PullImageFn: func(context.Context, string) error { return nil }, // the published base
			ReadFileFromImageFn: func(_ context.Context, image, path string) (string, error) {
				if strings.Contains(path, "VERSION") {
					return "82.3", nil
				}
				if strings.Contains(path, "BUILDID") {
					return "200", nil
				}
				return "", nil
			},
			BuildImageFn: func(_ context.Context, req ark.BuildRequest) error {
				rebuildCalls++
				return nil
			},
		}
	}, func() {
		if err := RunUpdate(core.CurrentAppDeps(), []string{"--compose-file", composePath}); err != nil {
			t.Fatalf("runUpdate: %v", err)
		}
	})
	// One update check shared by both instances, plus one lookup for the build's target build ID.
	if steamCalls != 2 {
		t.Fatalf("expected one cached steam check call and one build lookup, got %d", steamCalls)
	}
	if rebuildCalls != 2 {
		t.Fatalf("expected one rebuild (2 buildx stages) for shared image, got %d", rebuildCalls)
	}
}

func TestRunUpdateRestartModes(t *testing.T) {
	tests := []struct {
		name             string
		seed             string
		modeArg          string
		wantRestartOrder []string
		wantUpdatedCount int
	}{
		{
			name: "restart rolls updated instances in order",
			seed: strings.Join([]string{
				"name: test",
				"services:",
				"  asa2:",
				"    labels: {ark.port.asa: '27015'}",
				"    image: ark-server:100",
				"  asa1:",
				"    labels: {ark.port.asa: '27015'}",
				"    image: ark-server:100",
				"",
			}, "\n"),
			modeArg:          "--restart",
			wantRestartOrder: []string{"asa1", "asa2"},
			wantUpdatedCount: 2,
		},
		{
			name:             "rollout alias triggers restart",
			seed:             "name: test\nservices:\n  asa1:\n    labels: {ark.port.asa: '27015'}\n    image: ark-server:100\n",
			modeArg:          "--rollout",
			wantRestartOrder: []string{"asa1"},
			wantUpdatedCount: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			composePath := filepath.Join(t.TempDir(), "docker-compose.yaml")
			writeUpdateComposeFixture(t, composePath, tt.seed)

			stopCalls := []string{}
			startCalls := []string{}
			fakeBackups(t)
			withLocalSteamCheck(t, "81.1", "100", "200", func(d *core.AppDeps) {
				// ark-server:200 is built already, so the update only rolls out.
				d.Engine.(*arktest.MockEngine).ImageExistsFn = func(context.Context, string) (bool, error) { return true, nil }
				d.RestartStopInstance = func(req runtime.StopCommandRequest) error {
					stopCalls = append(stopCalls, req.Instance)
					return nil
				}
				d.RestartStartInstance = func(req runtime.StartCommandRequest) error {
					startCalls = append(startCalls, req.Instance)
					return nil
				}
			}, func() {
				if err := RunUpdate(core.CurrentAppDeps(), []string{"--compose-file", composePath, tt.modeArg}); err != nil {
					t.Fatalf("runUpdate: %v", err)
				}
			})

			if !reflect.DeepEqual(stopCalls, tt.wantRestartOrder) {
				t.Fatalf("unexpected stop order: %#v", stopCalls)
			}
			if !reflect.DeepEqual(startCalls, tt.wantRestartOrder) {
				t.Fatalf("unexpected start order: %#v", startCalls)
			}

			after, err := os.ReadFile(composePath)
			if err != nil {
				t.Fatalf("read compose: %v", err)
			}
			if got := strings.Count(string(after), "image: ark-server:200"); got != tt.wantUpdatedCount {
				t.Fatalf("expected %d updated services, got %d:\n%s", tt.wantUpdatedCount, got, string(after))
			}
		})
	}
}
