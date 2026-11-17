package host

import (
	"os"
	"strings"
	"testing"
)

func TestManagedSysctlConflictsDetectsEqualsAndWhitespaceSyntax(t *testing.T) {
	lines := []string{
		"# comment",
		"vm.max_map_count = 65530",
		"net.core.rmem_max 1048576 # valid whitespace syntax",
		"net.core.wmem_max=16777216",
		"fs.file-max 1048575",
	}

	conflicts := ManagedSysctlConflicts(lines)
	if len(conflicts) != 3 {
		t.Fatalf("expected 3 conflicts, got %d: %#v", len(conflicts), conflicts)
	}
	if conflicts[0].Name != "vm.max_map_count" {
		t.Fatalf("expected vm.max_map_count conflict, got %#v", conflicts[0])
	}
	if conflicts[1].Name != "net.core.rmem_max" {
		t.Fatalf("expected net.core.rmem_max conflict, got %#v", conflicts[1])
	}
	if conflicts[2].Name != "fs.file-max" {
		t.Fatalf("expected fs.file-max conflict, got %#v", conflicts[2])
	}
}

func TestRestoreSysctlConfLinesRewritesWhitespaceSyntaxEntries(t *testing.T) {
	lines := []string{
		"vm.max_map_count 16777216",
		"net.core.rmem_max=16777216",
	}
	backup := map[string]string{
		BackupSysctlConfPrefix + "vm.max_map_count": "65530",
	}

	if !RestoreSysctlConfLines(lines, backup) {
		t.Fatal("expected RestoreSysctlConfLines to report a change")
	}
	if got, want := lines[0], "vm.max_map_count=65530"; got != want {
		t.Fatalf("expected first line %q, got %q", want, got)
	}
	if got, want := lines[1], "net.core.rmem_max=16777216"; got != want {
		t.Fatalf("expected second line unchanged, got %q", got)
	}
}

func TestReadKernelParamWithMock(t *testing.T) {
	deps := HostDeps{
		ReadKernelParam: func(path string) (int, error) { return 16_777_216, nil },
	}
	val, err := ReadKernelParam(deps, "/proc/sys/vm/max_map_count")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if val != 16_777_216 {
		t.Fatalf("expected 16777216, got %d", val)
	}
}

func TestCheckOneParamOK(t *testing.T) {
	deps := HostDeps{
		ReadKernelParam: func(_ string) (int, error) { return 16_777_216, nil },
	}
	p := KernelParamCheck{Name: "vm.max_map_count", Path: "/proc/sys/vm/max_map_count", Minimum: 16_777_216}
	ok, val, err := CheckOneParam(deps, p)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok {
		t.Fatal("expected ok=true for param at minimum")
	}
	if val != 16_777_216 {
		t.Fatalf("expected 16777216, got %d", val)
	}
}

func TestCheckOneParamBelowMinimum(t *testing.T) {
	deps := HostDeps{
		ReadKernelParam: func(_ string) (int, error) { return 65530, nil },
	}
	p := KernelParamCheck{Name: "vm.max_map_count", Path: "/proc/sys/vm/max_map_count", Minimum: 16_777_216}
	ok, val, err := CheckOneParam(deps, p)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ok {
		t.Fatal("expected ok=false for param below minimum")
	}
	if val != 65530 {
		t.Fatalf("expected 65530, got %d", val)
	}
}

func TestCheckNofileLimitOK(t *testing.T) {
	deps := HostDeps{
		GetNofileHardLimit: func() (uint64, error) { return MinNofileHardLimit, nil },
	}
	ok, val, err := CheckNofileLimit(deps)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok {
		t.Fatal("expected ok=true for nofile at minimum")
	}
	if val != MinNofileHardLimit {
		t.Fatalf("expected %d, got %d", MinNofileHardLimit, val)
	}
}

func TestCheckNofileLimitBelowMinimum(t *testing.T) {
	deps := HostDeps{
		GetNofileHardLimit: func() (uint64, error) { return 65535, nil },
	}
	ok, _, err := CheckNofileLimit(deps)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ok {
		t.Fatal("expected ok=false for nofile below minimum")
	}
}

func TestSnapshotKernelParams(t *testing.T) {
	deps := HostDeps{
		ReadKernelParam: func(path string) (int, error) {
			return 16_777_216, nil
		},
	}
	snap, err := SnapshotKernelParams(deps)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(snap) != len(HostKernelParams) {
		t.Fatalf("expected %d params, got %d", len(HostKernelParams), len(snap))
	}
	for _, p := range HostKernelParams {
		if _, ok := snap[p.Name]; !ok {
			t.Fatalf("missing param %s in snapshot", p.Name)
		}
	}
}

func TestPrintKernelStatusTable(t *testing.T) {
	rows := []KernelStatusRow{
		{Name: "vm.max_map_count", Col1: "16777216", Col2: "16777216", Status: "OK"},
		{Name: "net.core.rmem_max", Col1: "1048576", Col2: "16777216", Status: "FAIL"},
	}
	var buf strings.Builder
	PrintKernelStatusTable(&buf, "current", "minimum", rows)
	output := buf.String()
	if !strings.Contains(output, "vm.max_map_count") {
		t.Fatalf("expected vm.max_map_count in output, got:\n%s", output)
	}
	if !strings.Contains(output, "FAIL") {
		t.Fatalf("expected FAIL in output, got:\n%s", output)
	}
	if !strings.Contains(output, "current") {
		t.Fatalf("expected 'current' header in output, got:\n%s", output)
	}
}

func TestSysctlPersistContent(t *testing.T) {
	content := SysctlPersistContent()
	if !strings.Contains(content, "vm.max_map_count") {
		t.Fatal("expected vm.max_map_count in persist content")
	}
}

func TestNofileLimitsContent(t *testing.T) {
	content := NofileLimitsContent()
	if !strings.Contains(content, "soft nofile") {
		t.Fatal("expected 'soft nofile' in limits content")
	}
	if !strings.Contains(content, "hard nofile") {
		t.Fatal("expected 'hard nofile' in limits content")
	}
}

func TestNofileSystemdContent(t *testing.T) {
	content := NofileSystemdContent()
	if !strings.Contains(content, "[Manager]") {
		t.Fatal("expected [Manager] section in systemd content")
	}
	if !strings.Contains(content, "DefaultLimitNOFILE") {
		t.Fatal("expected DefaultLimitNOFILE in systemd content")
	}
}

func TestParseKernelBackupFile(t *testing.T) {
	data := []byte("# comment\nvm.max_map_count=65530\nrlimit.nofile.hard=65535\n")
	result := ParseKernelBackupFile(data)
	if result["vm.max_map_count"] != "65530" {
		t.Fatalf("expected vm.max_map_count=65530, got %s", result["vm.max_map_count"])
	}
	if result["rlimit.nofile.hard"] != "65535" {
		t.Fatalf("expected rlimit.nofile.hard=65535, got %s", result["rlimit.nofile.hard"])
	}
}

func TestRequireRoot(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("running as root; non-root path not exercised")
	}
	err := RequireRoot("fix host-kernel-config")
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
