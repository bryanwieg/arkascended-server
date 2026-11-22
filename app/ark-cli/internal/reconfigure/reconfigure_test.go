package reconfigure

// Tests: reconfigure command: request building, setting-flag validation, environment value mutation

import (
	"ark-cli/internal/core"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ark-cli/internal/testutil"

	"pkg/ark"
	"pkg/ark/arktest"
	"pkg/ark/composeutil"
	"pkg/ark/runtime"
)

func TestBuildReconfigureCommandRequestRequiresSettingFlag(t *testing.T) {
	_, err := buildReconfigureCommandRequest(
		"asa1",
		"",
		false,
		false,
		"",
		"",
		"",
		"",
		"",
		"",
		"",
		"",
		"",
	)
	if err == nil {
		t.Fatal("expected missing settings error")
	}
}

func TestBuildReconfigureCommandRequestRejectsAllTarget(t *testing.T) {
	_, err := buildReconfigureCommandRequest(
		"all",
		"",
		false,
		false,
		"America/New_York",
		"",
		"",
		"",
		"",
		"",
		"",
		"",
		"",
	)
	if err == nil {
		t.Fatal("expected all target rejection")
	}
}

func TestBuildReconfigureCommandRequestParsesPorts(t *testing.T) {
	req, err := buildReconfigureCommandRequest(
		"asa1",
		"",
		false,
		false,
		"",
		"",
		"",
		"",
		"",
		"",
		"",
		"27015",
		"27020",
	)
	if err != nil {
		t.Fatalf("buildReconfigureCommandRequest: %v", err)
	}
	if !req.asaPortSet || req.asaPort != 27015 {
		t.Fatalf("expected asa port 27015, got set=%t value=%d", req.asaPortSet, req.asaPort)
	}
	if !req.rconPortSet || req.rconPort != 27020 {
		t.Fatalf("expected rcon port 27020, got set=%t value=%d", req.rconPortSet, req.rconPort)
	}
}

func TestBuildReconfigureCommandRequestValidatesPasswords(t *testing.T) {
	_, err := buildReconfigureCommandRequest(
		"asa1",
		"",
		false,
		false,
		"",
		"",
		"",
		"bad-pass",
		"",
		"",
		"",
		"",
		"",
	)
	if err == nil {
		t.Fatal("expected password validation error")
	}
}

func TestBuildReconfigureCommandRequestRejectsMalformedMods(t *testing.T) {
	for _, mods := range []string{"940975;1386057", "abc", "940975,"} {
		_, err := buildReconfigureCommandRequest("asa1", "", false, false, "", "", "", "", "", mods, "", "", "")
		if err == nil || !strings.Contains(err.Error(), "--mods") {
			t.Fatalf("mods %q: expected --mods error, got %v", mods, err)
		}
	}
}

func TestBuildReconfigureCommandRequestRejectsMalformedClusterID(t *testing.T) {
	_, err := buildReconfigureCommandRequest("asa1", "", false, false, "", "", "", "", "", "", "my cluster", "", "")
	if err == nil || !strings.Contains(err.Error(), "--cluster-id") {
		t.Fatalf("expected --cluster-id error, got %v", err)
	}
}

func TestRunReconfigureUpdatesTargetEnvironmentValues(t *testing.T) {
	composePath := filepath.Join(t.TempDir(), "docker-compose.yaml")
	seed := "" +
		"name: test\n" +
		"services:\n" +
		"  asa1:\n" +
		"    labels: {ark.port.asa: '27015'}\n    image: ark-server:latest\n" +
		"    environment:\n" +
		"      - TZ=America/New_York\n" +
		"      - MAP_NAME=TheIsland_WP\n" +
		"      - SESSION_NAME=asa1\n" +
		"      - SERVER_ADMIN_PASSWORD=AdminPass123\n" +
		"      - SERVER_PASSWORD=ServerPass123\n" +
		"  asa2:\n" +
		"    labels: {ark.port.asa: '27015'}\n    image: ark-server:latest\n" +
		"    environment:\n" +
		"      TZ: Europe/Berlin\n" +
		"      MAP_NAME: Aberration\n"
	if err := os.WriteFile(composePath, []byte(seed), 0o644); err != nil {
		t.Fatalf("write compose: %v", err)
	}

	withPatchedAppDeps(t, nil, func() {
		if err := RunReconfigure(core.CurrentAppDeps(), []string{
			"--compose-file", composePath,
			"--instance", "asa1",
			"--tz", "UTC",
			"--session-name", "new-session",
			"--mods", "940975, 1386057",
			"--cluster-id", "cluster",
		}); err != nil {
			t.Fatalf("runReconfigure: %v", err)
		}
	})

	project, err := testutil.LoadComposeFromPathForTest(composePath)
	if err != nil {
		t.Fatalf("LoadComposeFromPathForTest: %v", err)
	}
	asa1 := project.Services["asa1"]
	asa1Env := composeutil.EnvAsMap(asa1.Environment)
	if asa1Env["TZ"] != "UTC" {
		t.Fatalf("expected TZ update, got %q", asa1Env["TZ"])
	}
	if asa1Env["MAP_NAME"] != "TheIsland_WP" {
		t.Fatalf("expected MAP_NAME unchanged, got %q", asa1Env["MAP_NAME"])
	}
	if asa1Env["SESSION_NAME"] != "new-session" {
		t.Fatalf("expected SESSION_NAME update, got %q", asa1Env["SESSION_NAME"])
	}
	if asa1Env["MOD_IDS"] != "940975,1386057" {
		t.Fatalf("expected MOD_IDS update, got %q", asa1Env["MOD_IDS"])
	}
	if asa1Env["CLUSTER_ID"] != "cluster" {
		t.Fatalf("expected CLUSTER_ID update, got %q", asa1Env["CLUSTER_ID"])
	}
	if asa1Env["SERVER_ADMIN_PASSWORD"] != "AdminPass123" {
		t.Fatalf("expected unchanged admin password, got %q", asa1Env["SERVER_ADMIN_PASSWORD"])
	}

	asa2 := project.Services["asa2"]
	asa2Env := composeutil.EnvAsMap(asa2.Environment)
	if asa2Env["TZ"] != "Europe/Berlin" {
		t.Fatalf("expected asa2 TZ unchanged, got %q", asa2Env["TZ"])
	}
	if asa2Env["MAP_NAME"] != "Aberration" {
		t.Fatalf("expected asa2 MAP_NAME unchanged, got %q", asa2Env["MAP_NAME"])
	}
}

func TestRunReconfigureExplicitEmptyClearsValues(t *testing.T) {
	composePath := filepath.Join(t.TempDir(), "docker-compose.yaml")
	seed := "name: test\nservices:\n  asa1:\n    labels: {ark.port.asa: '27015'}\n    image: ark-server:latest\n    environment:\n" +
		"      - SERVER_PASSWORD=ServerPass123\n      - MOD_IDS=940975\n      - CLUSTER_ID=c1\n      - TZ=UTC\n"
	if err := os.WriteFile(composePath, []byte(seed), 0o644); err != nil {
		t.Fatalf("write compose: %v", err)
	}
	withPatchedAppDeps(t, nil, func() {
		if err := RunReconfigure(core.CurrentAppDeps(), []string{
			"--compose-file", composePath, "--instance", "asa1",
			"--server-password", "", "--mods", "", "--cluster-id", "", "--tz", "",
		}); err != nil {
			t.Fatalf("runReconfigure: %v", err)
		}
	})
	project, err := testutil.LoadComposeFromPathForTest(composePath)
	if err != nil {
		t.Fatal(err)
	}
	env := composeutil.EnvAsMap(project.Services["asa1"].Environment)
	for _, k := range []string{"SERVER_PASSWORD", "MOD_IDS", "CLUSTER_ID"} {
		if env[k] != "" {
			t.Fatalf("%s = %q, want cleared", k, env[k])
		}
	}
	if env["TZ"] != "UTC" {
		t.Fatalf("TZ = %q; an empty non-clearable flag must stay a no-op", env["TZ"])
	}
}

func TestRunReconfigureRejectsWhenNoSettingFlagsProvided(t *testing.T) {
	composePath := filepath.Join(t.TempDir(), "docker-compose.yaml")
	seed := "name: test\nservices:\n  asa1:\n    labels: {ark.port.asa: '27015'}\n    image: ark-server:latest\n"
	if err := os.WriteFile(composePath, []byte(seed), 0o644); err != nil {
		t.Fatalf("write compose: %v", err)
	}

	withPatchedAppDeps(t, nil, func() {
		if err := RunReconfigure(core.CurrentAppDeps(), []string{"--compose-file", composePath, "--instance", "asa1"}); err == nil {
			t.Fatal("expected error for missing setting flags")
		}
	})
}

func TestRunReconfigureWithoutRestartPrintsRestartRequired(t *testing.T) {
	composePath := filepath.Join(t.TempDir(), "docker-compose.yaml")
	seed := "name: test\nservices:\n  asa1:\n    labels: {ark.port.asa: '27015'}\n    image: ark-server:latest\n"
	if err := os.WriteFile(composePath, []byte(seed), 0o644); err != nil {
		t.Fatalf("write compose: %v", err)
	}

	var err error
	out := captureStdout(t, func() {
		withPatchedAppDeps(t, nil, func() {
			err = RunReconfigure(core.CurrentAppDeps(), []string{
				"--compose-file", composePath,
				"--instance", "asa1",
				"--map-name", "TheCenter",
			})
		})
	})
	if err != nil {
		t.Fatalf("runReconfigure: %v", err)
	}
	if !strings.Contains(out, "Restart required for settings to take effect: asa1") {
		t.Fatalf("expected restart guidance, got:\n%s", out)
	}
}

func TestRunReconfigureRejectsWhenServiceRunningWithoutForce(t *testing.T) {
	composePath := filepath.Join(t.TempDir(), "docker-compose.yaml")
	seed := "name: test\nservices:\n  asa1:\n    labels: {ark.port.asa: '27015'}\n    image: ark-server:latest\n"
	if err := os.WriteFile(composePath, []byte(seed), 0o644); err != nil {
		t.Fatalf("write compose: %v", err)
	}

	withPatchedAppDeps(t, func(d *core.AppDeps) {
		d.ComposeService = &arktest.MockCompose{
			RunningServicesFn: func(_ context.Context, _ string) (map[string]struct{}, error) {
				return map[string]struct{}{"asa1": {}}, nil
			},
		}
	}, func() {
		err := RunReconfigure(core.CurrentAppDeps(), []string{
			"--compose-file", composePath,
			"--instance", "asa1",
			"--map-name", "TheCenter",
		})
		if err == nil {
			t.Fatal("expected error when service is running without --force")
		}
		if !strings.Contains(err.Error(), "cannot reconfigure while service is running") {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}

func TestRunReconfigureAllowsWhenServiceRunningWithForce(t *testing.T) {
	composePath := filepath.Join(t.TempDir(), "docker-compose.yaml")
	seed := "name: test\nservices:\n  asa1:\n    labels: {ark.port.asa: '27015'}\n    image: ark-server:latest\n"
	if err := os.WriteFile(composePath, []byte(seed), 0o644); err != nil {
		t.Fatalf("write compose: %v", err)
	}

	var err error
	out := captureStdout(t, func() {
		withPatchedAppDeps(t, func(d *core.AppDeps) {
			d.ComposeService = &arktest.MockCompose{
				RunningServicesFn: func(_ context.Context, _ string) (map[string]struct{}, error) {
					return map[string]struct{}{"asa1": {}}, nil
				},
				LoadProjectFn: func(ctx context.Context, composeFile string) (*ark.ComposeProject, error) {
					project, err := testutil.LoadComposeFromPathForTest(composeFile)
					if err != nil {
						return nil, err
					}
					return &ark.ComposeProject{Project: project}, nil
				},
				SaveProjectFn: func(_ context.Context, composeFile string, project *ark.ComposeProject) error {
					return testutil.SaveComposeForTest(composeFile, project.Project)
				},
			}
		}, func() {
			err = RunReconfigure(core.CurrentAppDeps(), []string{
				"--compose-file", composePath,
				"--instance", "asa1",
				"--map-name", "TheCenter",
				"--force",
			})
		})
	})
	if err != nil {
		t.Fatalf("runReconfigure: %v", err)
	}
	if !strings.Contains(out, "Restart required for settings to take effect: asa1") {
		t.Fatalf("expected restart guidance, got:\n%s", out)
	}
}

func TestRunReconfigureRestartWhenServiceWasRunning(t *testing.T) {
	composePath := filepath.Join(t.TempDir(), "docker-compose.yaml")
	seed := "name: test\nservices:\n  asa1:\n    labels: {ark.port.asa: '27015'}\n    image: ark-server:latest\n"
	if err := os.WriteFile(composePath, []byte(seed), 0o644); err != nil {
		t.Fatalf("write compose: %v", err)
	}

	stopCalls := 0
	startCalls := 0
	withPatchedAppDeps(t, func(d *core.AppDeps) {
		d.ComposeService = &arktest.MockCompose{
			LoadProjectFn: func(ctx context.Context, composeFile string) (*ark.ComposeProject, error) {
				project, err := testutil.LoadComposeFromPathForTest(composeFile)
				if err != nil {
					return nil, err
				}
				return &ark.ComposeProject{Project: project}, nil
			},
			SaveProjectFn: func(_ context.Context, composeFile string, project *ark.ComposeProject) error {
				return testutil.SaveComposeForTest(composeFile, project.Project)
			},
			RunningServicesFn: func(_ context.Context, _ string) (map[string]struct{}, error) {
				return map[string]struct{}{"asa1": {}}, nil
			},
		}
		d.RestartStopInstance = func(req runtime.StopCommandRequest) error {
			stopCalls++
			if req.Instance != "asa1" {
				t.Fatalf("unexpected stop instance: %s", req.Instance)
			}
			if req.Timeout >= 0 || req.Kill {
				t.Fatalf("restart stop must use the default grace (Timeout<0), got %v kill=%v", req.Timeout, req.Kill)
			}
			return nil
		}
		d.RestartStartInstance = func(req runtime.StartCommandRequest) error {
			startCalls++
			if req.Instance != "asa1" {
				t.Fatalf("unexpected start instance: %s", req.Instance)
			}
			return nil
		}
	}, func() {
		if err := RunReconfigure(core.CurrentAppDeps(), []string{
			"--compose-file", composePath,
			"--instance", "asa1",
			"--map-name", "TheCenter",
			"--force",
			"--restart",
		}); err != nil {
			t.Fatalf("runReconfigure: %v", err)
		}
	})

	if stopCalls != 1 {
		t.Fatalf("expected one stop call, got %d", stopCalls)
	}
	if startCalls != 1 {
		t.Fatalf("expected one start call, got %d", startCalls)
	}
}

func TestRunReconfigureRestartSkipsWhenServiceWasStopped(t *testing.T) {
	composePath := filepath.Join(t.TempDir(), "docker-compose.yaml")
	seed := "name: test\nservices:\n  asa1:\n    labels: {ark.port.asa: '27015'}\n    image: ark-server:latest\n"
	if err := os.WriteFile(composePath, []byte(seed), 0o644); err != nil {
		t.Fatalf("write compose: %v", err)
	}

	withPatchedAppDeps(t, func(d *core.AppDeps) {
		d.ComposeService = &arktest.MockCompose{
			LoadProjectFn: func(ctx context.Context, composeFile string) (*ark.ComposeProject, error) {
				project, err := testutil.LoadComposeFromPathForTest(composeFile)
				if err != nil {
					return nil, err
				}
				return &ark.ComposeProject{Project: project}, nil
			},
			SaveProjectFn: func(_ context.Context, composeFile string, project *ark.ComposeProject) error {
				return testutil.SaveComposeForTest(composeFile, project.Project)
			},
			RunningServicesFn: func(_ context.Context, _ string) (map[string]struct{}, error) {
				return map[string]struct{}{}, nil
			},
		}
		d.RestartStopInstance = func(req runtime.StopCommandRequest) error {
			t.Fatalf("unexpected stop call: %#v", req)
			return nil
		}
		d.RestartStartInstance = func(req runtime.StartCommandRequest) error {
			t.Fatalf("unexpected start call: %#v", req)
			return nil
		}
	}, func() {
		var err error
		out := captureStdout(t, func() {
			err = RunReconfigure(core.CurrentAppDeps(), []string{
				"--compose-file", composePath,
				"--instance", "asa1",
				"--map-name", "TheCenter",
				"--restart",
			})
		})
		if err != nil {
			t.Fatalf("runReconfigure: %v", err)
		}
		if !strings.Contains(out, "was not previously running") {
			t.Fatalf("expected stopped-state message, got:\n%s", out)
		}
	})
}

func TestRunReconfigureRejectsMapNameChange(t *testing.T) {
	composePath := filepath.Join(t.TempDir(), "docker-compose.yaml")
	seed := "" +
		"name: test\n" +
		"services:\n" +
		"  asa1:\n" +
		"    labels: {ark.port.asa: '27015'}\n    image: ark-server:latest\n" +
		"    environment:\n" +
		"      - MAP_NAME=TheIsland_WP\n"
	if err := os.WriteFile(composePath, []byte(seed), 0o644); err != nil {
		t.Fatalf("write compose: %v", err)
	}

	var err error
	withPatchedAppDeps(t, nil, func() {
		err = RunReconfigure(core.CurrentAppDeps(), []string{
			"--compose-file", composePath,
			"--instance", "asa1",
			"--map-name", "Aberration_P",
		})
	})
	if err == nil {
		t.Fatal("expected error when changing existing MAP_NAME")
	}
	if !strings.Contains(err.Error(), "cannot change MAP_NAME") {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(err.Error(), "TheIsland_WP") {
		t.Fatalf("expected current map name in error, got: %v", err)
	}
}

func TestRunReconfigureAllowsMapNameWhenNotSet(t *testing.T) {
	composePath := filepath.Join(t.TempDir(), "docker-compose.yaml")
	seed := "name: test\nservices:\n  asa1:\n    labels: {ark.port.asa: '27015'}\n    image: ark-server:latest\n"
	if err := os.WriteFile(composePath, []byte(seed), 0o644); err != nil {
		t.Fatalf("write compose: %v", err)
	}

	withPatchedAppDeps(t, nil, func() {
		if err := RunReconfigure(core.CurrentAppDeps(), []string{
			"--compose-file", composePath,
			"--instance", "asa1",
			"--map-name", "TheIsland_WP",
		}); err != nil {
			t.Fatalf("expected no error when MAP_NAME not previously set: %v", err)
		}
	})
}

func TestRunReconfigureAllowsMapNameSameValue(t *testing.T) {
	composePath := filepath.Join(t.TempDir(), "docker-compose.yaml")
	seed := "" +
		"name: test\n" +
		"services:\n" +
		"  asa1:\n" +
		"    labels: {ark.port.asa: '27015'}\n    image: ark-server:latest\n" +
		"    environment:\n" +
		"      - MAP_NAME=TheIsland_WP\n"
	if err := os.WriteFile(composePath, []byte(seed), 0o644); err != nil {
		t.Fatalf("write compose: %v", err)
	}

	withPatchedAppDeps(t, nil, func() {
		if err := RunReconfigure(core.CurrentAppDeps(), []string{
			"--compose-file", composePath,
			"--instance", "asa1",
			"--map-name", "TheIsland_WP",
		}); err != nil {
			t.Fatalf("expected no error when MAP_NAME matches existing value: %v", err)
		}
	})
}

func TestBuildReconfigureRequestWarnsForMapNameMissingWP(t *testing.T) {
	var warnBuf strings.Builder
	origWarnf := core.Warnf
	core.Warnf = func(format string, args ...any) { warnBuf.WriteString(fmt.Sprintf(format, args...)) }
	t.Cleanup(func() { core.Warnf = origWarnf })

	req, err := buildReconfigureCommandRequest(
		"asa1",
		"",
		false,
		false,
		"",
		"TheIsland",
		"",
		"",
		"",
		"",
		"",
		"",
		"",
	)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if req.mapName != "TheIsland" {
		t.Fatalf("expected mapName TheIsland, got %q", req.mapName)
	}
	if !strings.Contains(warnBuf.String(), "_WP") {
		t.Fatal("expected _WP warning for map name without suffix")
	}
}

func TestBuildReconfigureRequestNoWarningForMapNameWithWP(t *testing.T) {
	var warnBuf strings.Builder
	origWarnf := core.Warnf
	core.Warnf = func(format string, args ...any) { warnBuf.WriteString(fmt.Sprintf(format, args...)) }
	t.Cleanup(func() { core.Warnf = origWarnf })

	req, err := buildReconfigureCommandRequest(
		"asa1",
		"",
		false,
		false,
		"",
		"TheIsland_WP",
		"",
		"",
		"",
		"",
		"",
		"",
		"",
	)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if req.mapName != "TheIsland_WP" {
		t.Fatalf("expected mapName TheIsland_WP, got %q", req.mapName)
	}
	if strings.Contains(warnBuf.String(), "WARNING") {
		t.Fatal("expected no warning for map name with _WP suffix")
	}
}
