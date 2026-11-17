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

	"pkg/ark/arktest"
	"pkg/ark/runtime"
	update "pkg/ark/update"
)

// runLocalUpdate runs `update instance` for one instance on ark-server:100 while
// Steam reports build 200, whose image ark-server:200 already exists (no build).
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
				return map[string]string{"io.ark.server.build_id": strings.TrimPrefix(image, "ark-server:")}, nil
			},
		}
		d.RestartStopInstance = func(req runtime.StopCommandRequest) error {
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
			if startFails && image == "ark-server:200" {
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

func TestRunUpdateLocalRolloutSwitchesImage(t *testing.T) {
	restored := fakeBackups(t)
	compose, calls, attempt, err := runLocalUpdate(t, []string{"--rollout"}, false)
	if err != nil {
		t.Fatal(err)
	}
	if attempt == nil || attempt.Result != update.ResultSucceeded {
		t.Fatalf("recorded %+v, want succeeded", attempt)
	}
	if !strings.Contains(compose, "image: ark-server:200") {
		t.Fatalf("compose not switched:\n%s", compose)
	}
	if strings.Join(calls, ",") != "stop,start ark-server:200" || len(*restored) != 0 {
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
	if strings.Join(calls, ",") != "stop,start ark-server:200,stop,start ark-server:100" {
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
	if len(bodies) != 1 || !strings.Contains(bodies[0], "ARK update of asa1 rolled_back: ark-server:100 -> ark-server:200") {
		t.Fatalf("webhook bodies %q, want one rolled_back alert", bodies)
	}
}
