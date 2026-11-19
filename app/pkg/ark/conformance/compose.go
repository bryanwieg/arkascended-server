//go:build integration

package conformance

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/compose-spec/compose-go/v2/types"
	"pkg/ark"
	"pkg/ark/composeutil"
)

const testComposeContent = `services:
  test-svc:
    image: alpine:latest
    command: ["sleep", "infinity"]
`

// writeTestComposeFile creates a minimal compose file in a temp directory.
func writeTestComposeFile(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "docker-compose.yaml")
	if err := os.WriteFile(path, []byte(testComposeContent), 0o644); err != nil {
		t.Fatalf("write compose file: %v", err)
	}
	return path
}

// TestComposeService runs the ComposeService conformance suite against the
// provided implementation. Call this from each backend's own test file.
func TestComposeService(t *testing.T, svc ark.ComposeService) {
	RequireDocker(t)
	pullAlpine(t)
	ctx := context.Background()

	t.Run("LoadProject", func(t *testing.T) {
		composeFile := writeTestComposeFile(t)
		project, err := svc.LoadProject(ctx, composeFile)
		if err != nil {
			t.Fatalf("LoadProject: %v", err)
		}
		if project == nil {
			t.Fatal("expected non-nil project")
		}
		if project.Project == nil {
			t.Fatal("expected non-nil Project")
		}
		if _, ok := project.Project.Services["test-svc"]; !ok {
			t.Fatal("expected test-svc in services")
		}
	})

	t.Run("SaveProject", func(t *testing.T) {
		dir := t.TempDir()
		composeFile := filepath.Join(dir, "docker-compose.yaml")
		original := "services:\n  test-svc:\n    image: alpine:1\n"
		if err := os.WriteFile(composeFile, []byte(original), 0o644); err != nil {
			t.Fatalf("write: %v", err)
		}

		project, err := svc.LoadProject(ctx, composeFile)
		if err != nil {
			t.Fatalf("LoadProject: %v", err)
		}
		composeutil.MutateService(project.Project, "test-svc", func(s *types.ServiceConfig) {
			s.Image = "alpine:2"
		})
		if err := svc.SaveProject(ctx, composeFile, project); err != nil {
			t.Fatalf("SaveProject: %v", err)
		}

		reloaded, err := svc.LoadProject(ctx, composeFile)
		if err != nil {
			t.Fatalf("LoadProject after save: %v", err)
		}
		if reloaded.Project.Services["test-svc"].Image != "alpine:2" {
			t.Fatalf("expected image alpine:2 after save+reload, got %q", reloaded.Project.Services["test-svc"].Image)
		}
	})

	t.Run("UpAndDown", func(t *testing.T) {
		composeFile := writeTestComposeFile(t)
		// Clean up: down after test.
		t.Cleanup(func() {
			_ = svc.Down(context.Background(), composeFile)
		})

		if err := svc.Up(ctx, composeFile, []string{"test-svc"}, ark.UpOptions{}); err != nil {
			t.Fatalf("Up: %v", err)
		}

		// Verify running.
		running, err := svc.RunningServices(ctx, composeFile)
		if err != nil {
			t.Fatalf("RunningServices: %v", err)
		}
		if _, ok := running["test-svc"]; !ok {
			t.Fatalf("expected test-svc running, got: %v", running)
		}

		// Down.
		if err := svc.Down(ctx, composeFile); err != nil {
			t.Fatalf("Down: %v", err)
		}

		// Verify no longer running.
		running, err = svc.RunningServices(ctx, composeFile)
		if err != nil {
			t.Fatalf("RunningServices after down: %v", err)
		}
		if _, ok := running["test-svc"]; ok {
			t.Fatal("expected test-svc not running after down")
		}
	})

	t.Run("ServiceContainers", func(t *testing.T) {
		composeFile := writeTestComposeFile(t)
		t.Cleanup(func() {
			_ = svc.Down(context.Background(), composeFile)
		})
		if err := svc.Up(ctx, composeFile, []string{"test-svc"}, ark.UpOptions{}); err != nil {
			t.Fatalf("Up: %v", err)
		}
		containers, err := svc.ServiceContainers(ctx, composeFile)
		if err != nil {
			t.Fatalf("ServiceContainers: %v", err)
		}
		c, ok := containers["test-svc"]
		if !ok || c.Name == "" || c.State != "running" {
			t.Fatalf("expected running test-svc with a name, got %+v", containers)
		}
	})

	t.Run("Exec", func(t *testing.T) {
		composeFile := writeTestComposeFile(t)
		t.Cleanup(func() {
			_ = svc.Down(context.Background(), composeFile)
		})

		if err := svc.Up(ctx, composeFile, []string{"test-svc"}, ark.UpOptions{}); err != nil {
			t.Fatalf("Up: %v", err)
		}

		result, err := svc.Exec(ctx, composeFile, "test-svc", []string{"/bin/sh", "-c", "echo hello-exec; echo exec-err >&2; exit 3"}, ark.ExecOptions{DisableTTY: true})
		if err != nil {
			t.Fatalf("Exec: %v", err)
		}
		if result.ExitCode != 3 {
			t.Fatalf("expected exit code 3, got %d", result.ExitCode)
		}
		if result.Stdout != "hello-exec\n" || result.Stderr != "exec-err\n" {
			t.Fatalf("output not captured: stdout=%q stderr=%q", result.Stdout, result.Stderr)
		}
	})

	t.Run("Logs", func(t *testing.T) {
		composeFile := writeTestComposeFile(t)
		t.Cleanup(func() {
			_ = svc.Down(context.Background(), composeFile)
		})

		if err := svc.Up(ctx, composeFile, []string{"test-svc"}, ark.UpOptions{}); err != nil {
			t.Fatalf("Up: %v", err)
		}

		// Exec a command that produces output, then check logs.
		_, _ = svc.Exec(ctx, composeFile, "test-svc", []string{"/bin/sh", "-c", "echo conformance-log-marker"}, ark.ExecOptions{DisableTTY: true})

		// Small delay for log propagation.
		time.Sleep(500 * time.Millisecond)

		reader, err := svc.Logs(ctx, composeFile, "test-svc", ark.LogOptions{Tail: 50})
		if err != nil {
			t.Fatalf("Logs: %v", err)
		}
		defer reader.Close()
		data, err := io.ReadAll(reader)
		if err != nil {
			t.Fatalf("read logs: %v", err)
		}
		// Note: exec'd output may or may not appear in service logs depending
		// on the backend. Just verify Logs returns without error.
		_ = data
	})

	t.Run("StopAndKill", func(t *testing.T) {
		composeFile := writeTestComposeFile(t)
		t.Cleanup(func() {
			_ = svc.Down(context.Background(), composeFile)
		})

		if err := svc.Up(ctx, composeFile, []string{"test-svc"}, ark.UpOptions{}); err != nil {
			t.Fatalf("Up: %v", err)
		}

		timeout := 5 * time.Second
		if err := svc.Stop(ctx, composeFile, []string{"test-svc"}, &timeout); err != nil {
			// Stop may timeout on sleep infinity; that's expected. Kill as fallback.
			if err := svc.Kill(ctx, composeFile, []string{"test-svc"}, "SIGKILL"); err != nil {
				t.Fatalf("Kill after stop timeout: %v", err)
			}
		}

		// Verify not running.
		running, err := svc.RunningServices(ctx, composeFile)
		if err != nil {
			t.Fatalf("RunningServices: %v", err)
		}
		if _, ok := running["test-svc"]; ok {
			t.Fatal("expected test-svc not running after stop/kill")
		}
	})

	t.Run("StopNilTimeoutUsesStopGracePeriod", func(t *testing.T) {
		// sleep as PID 1 ignores SIGTERM, so stop waits the full grace before SIGKILL.
		composeFile := filepath.Join(t.TempDir(), "docker-compose.yaml")
		content := testComposeContent + "    stop_grace_period: 2s\n"
		if err := os.WriteFile(composeFile, []byte(content), 0o644); err != nil {
			t.Fatalf("write compose file: %v", err)
		}
		t.Cleanup(func() {
			_ = svc.Down(context.Background(), composeFile)
		})

		if err := svc.Up(ctx, composeFile, []string{"test-svc"}, ark.UpOptions{}); err != nil {
			t.Fatalf("Up: %v", err)
		}
		start := time.Now()
		if err := svc.Stop(ctx, composeFile, []string{"test-svc"}, nil); err != nil {
			t.Fatalf("Stop: %v", err)
		}
		// The engine default is 10s; well under that means stop_grace_period applied.
		if elapsed := time.Since(start); elapsed < 1500*time.Millisecond || elapsed > 7*time.Second {
			t.Fatalf("stop took %v, want about 2s (stop_grace_period)", elapsed)
		}
	})

	t.Run("RunOneOff", func(t *testing.T) {
		composeFile := writeTestComposeFile(t)
		t.Cleanup(func() {
			_ = svc.Down(context.Background(), composeFile)
		})

		result, err := svc.RunOneOff(ctx, composeFile, "test-svc",
			[]string{"-c", "echo oneoff-test; echo oneoff-err >&2"},
			ark.RunOptions{Remove: true, NoDeps: true, DisableTTY: true, Entrypoint: "/bin/sh"})
		if err != nil {
			t.Fatalf("RunOneOff: %v", err)
		}
		if result.ExitCode != 0 {
			t.Fatalf("expected exit code 0, got %d", result.ExitCode)
		}
		if result.Stdout != "oneoff-test\n" || result.Stderr != "oneoff-err\n" {
			t.Fatalf("output not captured: stdout=%q stderr=%q", result.Stdout, result.Stderr)
		}
	})
}
