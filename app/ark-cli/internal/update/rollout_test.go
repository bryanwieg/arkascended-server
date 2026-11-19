package update

// Tests: local update rollout: build only without --rollout, switch on success,
// rollback (compose back, backup restored, non-zero exit) when the new image fails.

import (
	"ark-cli/internal/core"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"pkg/ark"
	"pkg/ark/arktest"
	"pkg/ark/runtime"
	update "pkg/ark/update"

	arkcontainer "ark-container"
)

// newImage is the image `update instance` builds for Steam build 200.
var newImage = update.LocalImageRef("200")

// runLocalUpdate runs `update instance` for one instance on ark-server:100 while
// Steam reports build 200, whose image newImage already exists (no build).
// startFails makes starting the new image fail. Returns the compose file afterwards,
// the stop/start calls, the recorded last attempt and the command error.
func runLocalUpdate(t *testing.T, args []string, startFails bool) (string, []string, *update.UpdateAttempt, error) {
	t.Helper()
	composePath := filepath.Join(t.TempDir(), "docker-compose.yaml")
	writeUpdateComposeFixture(t, composePath, "name: test\nservices:\n  asa1:\n    image: ark-server:100\n")
	var calls []string
	var runErr error
	withPatchedAppDeps(t, func(d *core.AppDeps) {
		d.HostDeps.FetchText = func(string) (string, error) { return fakeSteamAPIResponse("200"), nil }
		d.Engine = &arktest.MockEngine{
			ImageExistsFn: func(context.Context, string) (bool, error) { return true, nil },
			ImageLabelsFn: func(_ context.Context, image string) (map[string]string, error) {
				buildID, _, _ := strings.Cut(strings.TrimPrefix(image, "ark-server:"), "-")
				versions := map[string]string{"100": "98.6", "200": "98.9"}
				return map[string]string{
					"io.ark.server.build_id": buildID, "io.ark.server.version": versions[buildID], "io.ark.recipe": arkcontainer.RecipeID(),
				}, nil
			},
		}
		d.RestartStopInstance = func(req runtime.StopCommandRequest) error {
			if req.Timeout >= 0 || req.Kill {
				t.Fatalf("rollout stop must use the default grace (Timeout<0), got %v kill=%v", req.Timeout, req.Kill)
			}
			calls = append(calls, "stop")
			return nil
		}
		d.RestartStartInstance = func(req runtime.StartCommandRequest) error {
			project, err := d.ComposeService.LoadProject(context.Background(), composePath)
			if err != nil {
				return err
			}
			image := project.Project.Services["asa1"].Image
			calls = append(calls, "start "+image)
			if startFails && image == newImage {
				return errors.New("not ready")
			}
			return nil
		}
	}, func() {
		runErr = RunUpdate(core.CurrentAppDeps(), append([]string{"--compose-file", composePath}, args...))
	})
	after, err := os.ReadFile(composePath)
	if err != nil {
		t.Fatal(err)
	}
	st, err := update.LoadState(update.StateDir(composePath), "asa1")
	if err != nil {
		t.Fatal(err)
	}
	return string(after), calls, st.LastAttempt, runErr
}

func TestRunUpdateLocalWithoutRolloutOnlyBuilds(t *testing.T) {
	compose, calls, _, err := runLocalUpdate(t, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(compose, "image: ark-server:100") || len(calls) != 0 {
		t.Fatalf("compose must stay on ark-server:100 with no restarts, got calls %v:\n%s", calls, compose)
	}
}

func TestRunUpdateReportsVersionsAfterBuild(t *testing.T) {
	out := captureStdout(t, func() {
		if _, _, _, err := runLocalUpdate(t, nil, false); err != nil {
			t.Fatal(err)
		}
	})
	for _, want := range []string{"asa1 | 98.6 | 98.9 | update |", "asa1: 98.6 -> 98.9 (build 200)"} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in output:\n%s", want, out)
		}
	}
}

func TestRunUpdateLocalRolloutSwitchesImage(t *testing.T) {
	restored := fakeBackups(t)
	compose, calls, attempt, err := runLocalUpdate(t, []string{"--rollout"}, false)
	if err != nil {
		t.Fatal(err)
	}
	if attempt == nil || attempt.Result != update.ResultSucceeded {
		t.Fatalf("recorded %+v, want succeeded", attempt)
	}
	if !strings.Contains(compose, "image: "+newImage) {
		t.Fatalf("compose not switched:\n%s", compose)
	}
	if strings.Join(calls, ",") != "stop,start "+newImage || len(*restored) != 0 {
		t.Fatalf("calls %v, restored %v", calls, *restored)
	}
}

func TestRunUpdateLocalRolloutRollsBack(t *testing.T) {
	restored := fakeBackups(t)
	compose, calls, attempt, err := runLocalUpdate(t, []string{"--rollout"}, true)
	if err == nil || !strings.Contains(err.Error(), "rolled back") {
		t.Fatalf("expected a rolled-back error, got %v", err)
	}
	if !strings.Contains(compose, "image: ark-server:100") {
		t.Fatalf("compose not switched back:\n%s", compose)
	}
	if strings.Join(calls, ",") != "stop,start "+newImage+",stop,start ark-server:100" {
		t.Fatalf("calls %v", calls)
	}
	if strings.Join(*restored, ",") != "/backups/pre-update.tar.gz" {
		t.Fatalf("restored %v, want the pre-update archive", *restored)
	}
	if attempt == nil || attempt.Result != update.ResultRolledBack || attempt.Backup != "/backups/pre-update.tar.gz" {
		t.Fatalf("recorded %+v, want rolled_back with the backup", attempt)
	}
}

func TestRunUpdateLocalRolloutAlertsWebhookOnRollback(t *testing.T) {
	fakeBackups(t)
	var bodies []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]string
		json.NewDecoder(r.Body).Decode(&body)
		bodies = append(bodies, body["content"])
	}))
	defer srv.Close()
	t.Setenv("ARK_UPDATE_WEBHOOK_URL", srv.URL)

	if _, _, _, err := runLocalUpdate(t, []string{"--rollout"}, true); err == nil {
		t.Fatal("expected the rollout to fail")
	}
	if len(bodies) != 1 || !strings.Contains(bodies[0], "ARK update of asa1 rolled_back: ark-server:100 -> "+newImage) {
		t.Fatalf("webhook bodies %q, want one rolled_back alert", bodies)
	}
}

// TestRunUpdateBuildFailureTouchesNoInstance pins the order: the image is built before
// any instance is stopped, so a failed build (SteamCMD 0x6, ...) leaves every instance running.
func TestRunUpdateBuildFailureTouchesNoInstance(t *testing.T) {
	composePath := filepath.Join(t.TempDir(), "docker-compose.yaml")
	seed := "name: test\nservices:\n  asa1:\n    image: ark-server:100\n  asa2:\n    image: ark-server:100\n"
	writeUpdateComposeFixture(t, composePath, seed)
	fakeBackups(t)
	var calls []string
	var runErr error
	withLocalSteamCheck(t, "98.6", "100", "200", func(d *core.AppDeps) {
		d.Engine.(*arktest.MockEngine).PullImageFn = func(context.Context, string) error { return nil }
		d.DiskUsageForPath = func(string) (int64, int64, error) { return 100 << 30, 100 << 30, nil }
		d.Runtime.RunCommand = func([]string, ark.CommandOptions) (ark.ExecResult, error) {
			return ark.ExecResult{ExitCode: 8}, nil // SteamCMD's exit on state 0x6
		}
		d.RestartStopInstance = func(req runtime.StopCommandRequest) error {
			calls = append(calls, "stop "+req.Instance)
			return nil
		}
		d.RestartStartInstance = func(req runtime.StartCommandRequest) error {
			calls = append(calls, "start "+req.Instance)
			return nil
		}
	}, func() {
		runErr = RunUpdate(core.CurrentAppDeps(), []string{"--compose-file", composePath, "--rollout"})
	})
	if runErr == nil || !strings.Contains(runErr.Error(), "server files build failed") {
		t.Fatalf("expected the build error, got %v", runErr)
	}
	if len(calls) != 0 {
		t.Fatalf("a failed build must not stop or start instances, got %v", calls)
	}
	after, err := os.ReadFile(composePath)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != seed {
		t.Fatalf("compose changed after a failed build:\n%s", after)
	}
	for _, instance := range []string{"asa1", "asa2"} {
		st, err := update.LoadState(update.StateDir(composePath), instance)
		if err != nil {
			t.Fatal(err)
		}
		if st.LastAttempt == nil || st.LastAttempt.Result != update.ResultBuildFailed {
			t.Fatalf("%s recorded %+v, want build_failed", instance, st.LastAttempt)
		}
	}
}
