package host

// Tests: host kernel config commands: check status reporting (OK/FAIL/error), fix/restore root requirements

import (
	"ark-cli/internal/core"
	"errors"
	"os"
	"strings"
	"testing"

	host "pkg/ark/host"
)

// TestCheckHostKernelConfigAllOK verifies all "OK" rows when every param meets its minimum.
func TestCheckHostKernelConfigAllOK(t *testing.T) {
	out := captureStdout(t, func() {
		withPatchedAppDeps(t, func(d *core.AppDeps) {
			d.HostDeps.ReadKernelParam = func(_ string) (int, error) { return 16_777_216, nil }
			d.HostDeps.GetNofileHardLimit = func() (uint64, error) { return host.MinNofileHardLimit, nil }
		}, func() {
			if err := RunCheckHostKernelConfig(core.CurrentAppDeps(), []string{}); err != nil {
				t.Fatalf("runCheckHostKernelConfig: %v", err)
			}
		})
	})
	if strings.Contains(out, "FAIL") || strings.Contains(out, "error") {
		t.Fatalf("unexpected failure in output:\n%s", out)
	}
	if !strings.Contains(out, "OK") {
		t.Fatalf("expected OK rows in output:\n%s", out)
	}
}

// TestCheckHostKernelConfigParamBelowMinimum verifies "FAIL" when a sysctl param is below minimum.
func TestCheckHostKernelConfigParamBelowMinimum(t *testing.T) {
	out := captureStdout(t, func() {
		withPatchedAppDeps(t, func(d *core.AppDeps) {
			d.HostDeps.ReadKernelParam = func(path string) (int, error) {
				if strings.Contains(path, "max_map_count") {
					return 65530, nil // below 16_777_216 minimum
				}
				return 16_777_216, nil
			}
			d.HostDeps.GetNofileHardLimit = func() (uint64, error) { return host.MinNofileHardLimit, nil }
		}, func() {
			if err := RunCheckHostKernelConfig(core.CurrentAppDeps(), []string{}); err != nil {
				t.Fatalf("runCheckHostKernelConfig: %v", err)
			}
		})
	})
	if !strings.Contains(out, "FAIL") {
		t.Fatalf("expected FAIL row, got:\n%s", out)
	}
	if !strings.Contains(out, "vm.max_map_count") {
		t.Fatalf("expected vm.max_map_count in output:\n%s", out)
	}
}

// TestCheckHostKernelConfigReadError verifies "error" status when a param path is unreadable.
func TestCheckHostKernelConfigReadError(t *testing.T) {
	out := captureStdout(t, func() {
		withPatchedAppDeps(t, func(d *core.AppDeps) {
			d.HostDeps.ReadKernelParam = func(_ string) (int, error) {
				return 0, errors.New("permission denied")
			}
			d.HostDeps.GetNofileHardLimit = func() (uint64, error) { return host.MinNofileHardLimit, nil }
		}, func() {
			if err := RunCheckHostKernelConfig(core.CurrentAppDeps(), []string{}); err != nil {
				t.Fatalf("runCheckHostKernelConfig: %v", err)
			}
		})
	})
	if !strings.Contains(out, "error") {
		t.Fatalf("expected error status row, got:\n%s", out)
	}
}

// TestCheckHostKernelConfigNofileBelowMinimum verifies "FAIL" when nofile hard limit is too low.
func TestCheckHostKernelConfigNofileBelowMinimum(t *testing.T) {
	out := captureStdout(t, func() {
		withPatchedAppDeps(t, func(d *core.AppDeps) {
			d.HostDeps.ReadKernelParam = func(_ string) (int, error) { return 16_777_216, nil }
			d.HostDeps.GetNofileHardLimit = func() (uint64, error) { return 65535, nil } // below 1_048_576
		}, func() {
			if err := RunCheckHostKernelConfig(core.CurrentAppDeps(), []string{}); err != nil {
				t.Fatalf("runCheckHostKernelConfig: %v", err)
			}
		})
	})
	if !strings.Contains(out, "FAIL") {
		t.Fatalf("expected FAIL row for nofile limit, got:\n%s", out)
	}
	if !strings.Contains(out, host.NofileSettingName) {
		t.Fatalf("expected nofile setting name in output:\n%s", out)
	}
}

// TestFixHostKernelConfigRequiresRoot verifies a helpful error with sudo hint when not root.
func TestFixHostKernelConfigRequiresRoot(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("running as root; non-root path not exercised")
	}
	err := RunFixHostKernelConfig(core.CurrentAppDeps(), []string{})
	if err == nil {
		t.Fatal("expected root-required error")
	}
	if !strings.Contains(err.Error(), "root privileges are required") {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(err.Error(), "sudo ark-cli fix host-kernel-config") {
		t.Fatalf("expected sudo hint in error: %v", err)
	}
}

// TestRestoreHostKernelConfigRequiresRoot verifies a helpful error with sudo hint when not root.
func TestRestoreHostKernelConfigRequiresRoot(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("running as root; non-root path not exercised")
	}
	err := RunRestoreHostKernelConfig(core.CurrentAppDeps(), []string{})
	if err == nil {
		t.Fatal("expected root-required error")
	}
	if !strings.Contains(err.Error(), "root privileges are required") {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(err.Error(), "sudo ark-cli restore host-kernel-config") {
		t.Fatalf("expected sudo hint in error: %v", err)
	}
}
