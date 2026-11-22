package health

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"ark-cli/internal/core"
	"ark-cli/internal/testutil"

	"pkg/ark/arktest"
	pkghealth "pkg/ark/health"
	"pkg/ark/update"
)

func writeComposeFile(t *testing.T, dir string, content string) string {
	t.Helper()
	p := filepath.Join(dir, "docker-compose.yaml")
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatalf("write compose file: %v", err)
	}
	return p
}

func withComposeEnv(t *testing.T, composeFile string) {
	t.Helper()
	testutil.SetEnv(t, "COMPOSE_FILE", composeFile)
}

func TestRunCheckInstanceStopped(t *testing.T) {
	dir := t.TempDir()
	composeFile := writeComposeFile(t, dir, "services:\n  asa1:\n    labels: {ark.port.asa: '27015'}\n    image: ark-server:latest\n")
	withComposeEnv(t, composeFile)

	out := captureStdout(t, func() {
		withPatchedAppDeps(t, func(d *core.AppDeps) {
			d.ComposeService.(*arktest.MockCompose).RunningServicesFn = func(_ context.Context, _ string) (map[string]struct{}, error) {
				return map[string]struct{}{}, nil
			}
		}, func() {
			if err := RunCheckInstance(core.CurrentAppDeps(), []string{"--instance", "asa1"}); err != nil {
				t.Fatalf("RunCheckInstance: %v", err)
			}
		})
	})
	if !strings.Contains(out, "result: stopped") {
		t.Fatalf("expected 'result: stopped' in output:\n%s", out)
	}
}

func TestRunCheckInstanceHealthy(t *testing.T) {
	dir := t.TempDir()
	composeFile := writeComposeFile(t, dir, "services:\n  asa1:\n    labels: {ark.port.asa: '27015'}\n    image: ark-server:latest\n")
	withComposeEnv(t, composeFile)

	out := captureStdout(t, func() {
		withPatchedAppDeps(t, func(d *core.AppDeps) {
			d.ComposeService.(*arktest.MockCompose).RunningServicesFn = func(_ context.Context, _ string) (map[string]struct{}, error) {
				return map[string]struct{}{"asa1": {}}, nil
			}
			d.HealthChecker = &testutil.MockHealthChecker{
				IsReadyFn: func(_ context.Context, _, _ string, _ *time.Time) (bool, error) { return true, nil },
			}
		}, func() {
			if err := RunCheckInstance(core.CurrentAppDeps(), []string{"--instance", "asa1"}); err != nil {
				t.Fatalf("RunCheckInstance: %v", err)
			}
		})
	})
	if !strings.Contains(out, "result: healthy") {
		t.Fatalf("expected 'result: healthy' in output:\n%s", out)
	}
}

func TestRunCheckInstanceStarting(t *testing.T) {
	dir := t.TempDir()
	composeFile := writeComposeFile(t, dir, "services:\n  asa1:\n    labels: {ark.port.asa: '27015'}\n    image: ark-server:latest\n")
	withComposeEnv(t, composeFile)

	withPatchedAppDeps(t, func(d *core.AppDeps) {
		d.ComposeService.(*arktest.MockCompose).RunningServicesFn = func(_ context.Context, _ string) (map[string]struct{}, error) {
			return map[string]struct{}{"asa1": {}}, nil
		}
		d.HealthChecker = &testutil.MockHealthChecker{
			IsReadyFn: func(_ context.Context, _, _ string, _ *time.Time) (bool, error) { return false, nil },
		}
	}, func() {
		err := RunCheckInstance(core.CurrentAppDeps(), []string{"--instance", "asa1"})
		if err == nil {
			t.Fatal("expected error for starting instance")
		}
		if !strings.Contains(err.Error(), "starting") {
			t.Fatalf("expected 'starting' in error, got: %v", err)
		}
	})
}

func TestRunCheckInstanceMissingInstance(t *testing.T) {
	dir := t.TempDir()
	composeFile := writeComposeFile(t, dir, "services:\n  other:\n    labels: {ark.port.asa: '27015'}\n    image: ark-server:latest\n")
	withComposeEnv(t, composeFile)

	withPatchedAppDeps(t, nil, func() {
		err := RunCheckInstance(core.CurrentAppDeps(), []string{"--instance", "asa1"})
		if err == nil {
			t.Fatal("expected error for missing instance")
		}
	})
}

func TestRunWaitInvalidForValue(t *testing.T) {
	dir := t.TempDir()
	composeFile := writeComposeFile(t, dir, "services:\n  asa1:\n    labels: {ark.port.asa: '27015'}\n    image: ark-server:latest\n")
	withComposeEnv(t, composeFile)

	withPatchedAppDeps(t, nil, func() {
		err := RunWait(core.CurrentAppDeps(), []string{"--instance", "asa1", "--for", "invalid", "--timeout", "1s", "--interval", "10ms"})
		if err == nil {
			t.Fatal("expected error for invalid --for value")
		}
		if !strings.Contains(err.Error(), "must be") {
			t.Fatalf("expected 'must be' in error, got: %v", err)
		}
	})
}

func TestRunWaitNonPositiveTimeout(t *testing.T) {
	err := RunWait(core.CurrentAppDeps(), []string{"--instance", "asa1", "--for", "started", "--timeout", "0"})
	if err == nil {
		t.Fatal("expected error for zero timeout")
	}
	if !strings.Contains(err.Error(), "must be > 0") {
		t.Fatalf("expected 'must be > 0' in error, got: %v", err)
	}
}

func TestRunWaitNonPositiveInterval(t *testing.T) {
	err := RunWait(core.CurrentAppDeps(), []string{"--instance", "asa1", "--for", "started", "--timeout", "1s", "--interval", "-1s"})
	if err == nil {
		t.Fatal("expected error for negative interval")
	}
	if !strings.Contains(err.Error(), "must be > 0") {
		t.Fatalf("expected 'must be > 0' in error, got: %v", err)
	}
}

func TestRunWaitForStarted(t *testing.T) {
	dir := t.TempDir()
	composeFile := writeComposeFile(t, dir, "services:\n  asa1:\n    labels: {ark.port.asa: '27015'}\n    image: ark-server:latest\n")
	withComposeEnv(t, composeFile)

	out := captureStdout(t, func() {
		withPatchedAppDeps(t, func(d *core.AppDeps) {
			d.HealthChecker = &testutil.MockHealthChecker{
				WaitForStartedFn: func(_ context.Context, _, _ string, _ pkghealth.WaitOptions) error { return nil },
			}
		}, func() {
			if err := RunWait(core.CurrentAppDeps(), []string{"--instance", "asa1", "--for", "started", "--timeout", "1s", "--interval", "10ms"}); err != nil {
				t.Fatalf("RunWait: %v", err)
			}
		})
	})
	if !strings.Contains(out, "started") {
		t.Fatalf("expected 'started' in output:\n%s", out)
	}
}

func TestRunWaitForStopped(t *testing.T) {
	dir := t.TempDir()
	composeFile := writeComposeFile(t, dir, "services:\n  asa1:\n    labels: {ark.port.asa: '27015'}\n    image: ark-server:latest\n")
	withComposeEnv(t, composeFile)

	out := captureStdout(t, func() {
		withPatchedAppDeps(t, func(d *core.AppDeps) {
			d.HealthChecker = &testutil.MockHealthChecker{
				WaitForStoppedFn: func(_ context.Context, _, _ string, _ pkghealth.WaitOptions) error { return nil },
			}
		}, func() {
			if err := RunWait(core.CurrentAppDeps(), []string{"--instance", "asa1", "--for", "stopped", "--timeout", "1s", "--interval", "10ms"}); err != nil {
				t.Fatalf("RunWait: %v", err)
			}
		})
	})
	if !strings.Contains(out, "stopped") {
		t.Fatalf("expected 'stopped' in output:\n%s", out)
	}
}

func TestRunWaitForReadyAlias(t *testing.T) {
	dir := t.TempDir()
	composeFile := writeComposeFile(t, dir, "services:\n  asa1:\n    labels: {ark.port.asa: '27015'}\n    image: ark-server:latest\n")
	withComposeEnv(t, composeFile)

	out := captureStdout(t, func() {
		withPatchedAppDeps(t, func(d *core.AppDeps) {
			d.HealthChecker = &testutil.MockHealthChecker{
				WaitForReadyFn: func(_ context.Context, _, _ string, _ pkghealth.WaitOptions) error { return nil },
			}
		}, func() {
			if err := RunWait(core.CurrentAppDeps(), []string{"--instance", "asa1", "--for", "ready", "--timeout", "1s", "--interval", "10ms"}); err != nil {
				t.Fatalf("RunWait: %v", err)
			}
		})
	})
	if !strings.Contains(out, "advertising") {
		t.Fatalf("expected 'advertising' in output (ready alias):\n%s", out)
	}
}

func TestRunValidateComposeReportsServiceCount(t *testing.T) {
	dir := t.TempDir()
	composeFile := writeComposeFile(t, dir, "services:\n  svc1:\n    labels: {ark.port.asa: '27015'}\n    image: ark-server:latest\n  svc2:\n    labels: {ark.port.asa: '27015'}\n    image: ark-server:latest\n")
	withComposeEnv(t, composeFile)

	out := captureStdout(t, func() {
		withPatchedAppDeps(t, nil, func() {
			if err := RunValidateCompose(core.CurrentAppDeps(), []string{}); err != nil {
				t.Fatalf("RunValidateCompose: %v", err)
			}
		})
	})
	if !strings.Contains(out, "2 service(s)") {
		t.Fatalf("expected '2 service(s)' in output:\n%s", out)
	}
}

func TestRunValidateComposeMissingFile(t *testing.T) {
	withComposeEnv(t, "/nonexistent/docker-compose.yaml")
	withPatchedAppDeps(t, nil, func() {
		err := RunValidateCompose(core.CurrentAppDeps(), []string{})
		if err == nil {
			t.Fatal("expected error for missing compose file")
		}
	})
}

func TestRunCheckInstanceShowsLastUpdate(t *testing.T) {
	for _, tc := range []struct {
		name    string
		state   string // raw state file; "" = none
		want    string
		wantErr bool
	}{
		{name: "no state", want: ""},
		{name: "rolled back", state: `{"last_attempt":{"time":"2026-11-16T12:00:00Z","from":"ark-server:100","to":"ark-server:200","result":"rolled_back","message":"not ready"}}`,
			want: "last_update: rolled_back at 2026-11-16T12:00:00Z (ark-server:100 -> ark-server:200)\nlast_update_message: not ready\n"},
		{name: "build failed", state: `{"last_attempt":{"time":"2026-11-16T12:00:00Z","from":"ark-server:100","result":"build_failed"}}`,
			want: "last_update: build_failed at 2026-11-16T12:00:00Z (ark-server:100 -> -)\n"},
		{name: "corrupt", state: `{nope`, want: "Warning: could not read update state"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			composeFile := writeComposeFile(t, dir, "services:\n  asa1:\n    labels: {ark.port.asa: '27015'}\n    image: ark-server:latest\n")
			withComposeEnv(t, composeFile)
			if tc.state != "" {
				path := update.StatePath(update.StateDir(composeFile), "asa1")
				os.MkdirAll(filepath.Dir(path), 0o755)
				os.WriteFile(path, []byte(tc.state), 0o644)
			}
			var warned strings.Builder
			origWarn := core.Warnf
			core.Warnf = func(f string, a ...any) { fmt.Fprintf(&warned, f, a...) }
			t.Cleanup(func() { core.Warnf = origWarn })

			out := captureStdout(t, func() {
				withPatchedAppDeps(t, func(d *core.AppDeps) {
					d.ComposeService.(*arktest.MockCompose).RunningServicesFn = func(_ context.Context, _ string) (map[string]struct{}, error) {
						return map[string]struct{}{}, nil
					}
				}, func() {
					if err := RunCheckInstance(core.CurrentAppDeps(), []string{"--instance", "asa1"}); err != nil {
						t.Fatalf("a stopped instance must still pass: %v", err)
					}
				})
			})
			got := out + warned.String()
			if tc.want == "" {
				if strings.Contains(got, "last_update") || warned.Len() != 0 {
					t.Fatalf("unexpected output:\n%s", got)
				}
				return
			}
			if !strings.Contains(got, tc.want) {
				t.Fatalf("want %q in:\n%s", tc.want, got)
			}
		})
	}
}
