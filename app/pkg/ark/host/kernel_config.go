package host

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const (
	// SysctlPersistPath is the persistent sysctl.d config file written by fix.
	SysctlPersistPath = "/etc/sysctl.d/99-ark-server.conf"
	// SysctlConfPath is the main sysctl.conf that may contain conflicting values.
	SysctlConfPath = "/etc/sysctl.conf"
	// BackupSysctlConfPrefix is the key prefix used in backup files for sysctl.conf entries.
	BackupSysctlConfPrefix = "sysctl.conf."
	// NofileLimitsPersistPath is the persistent limits.d config file for the NOFILE fix.
	NofileLimitsPersistPath = "/etc/security/limits.d/99-ark-server.conf"
	// NofileSystemdPersistPath is the persistent systemd override for the NOFILE fix.
	NofileSystemdPersistPath = "/etc/systemd/system.conf.d/99-ark-server.conf"
	// MinNofileHardLimit is the minimum required NOFILE hard limit for Proton/Wine.
	MinNofileHardLimit = uint64(1_048_576)
	// BackupNofileKey is the key used in backup files for the nofile hard limit.
	BackupNofileKey = "rlimit.nofile.hard"
	// NofileSettingName is the display name for the nofile setting in status tables.
	NofileSettingName = "nofile hard limit"
)

// SysctlConfConflict describes a managed param found in /etc/sysctl.conf with a
// lower-than-required value.
type SysctlConfConflict struct {
	Name    string
	OldVal  int
	NewVal  int
	LineIdx int
}

// ParseSysctlConfLine accepts both "key=value" and "key value" sysctl.conf syntaxes.
func ParseSysctlConfLine(line string) (string, int, bool) {
	line, _, _ = strings.Cut(line, "#")
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return "", 0, false
	}
	if name, valStr, ok := strings.Cut(trimmed, "="); ok {
		val, err := strconv.Atoi(strings.TrimSpace(valStr))
		return strings.TrimSpace(name), val, err == nil
	}
	fields := strings.Fields(trimmed)
	if len(fields) < 2 {
		return "", 0, false
	}
	val, err := strconv.Atoi(fields[1])
	return fields[0], val, err == nil
}

// ManagedSysctlConflicts finds managed sysctl.conf entries below our required minimums.
func ManagedSysctlConflicts(lines []string) []SysctlConfConflict {
	managed := make(map[string]int, len(HostKernelParams))
	for _, p := range HostKernelParams {
		managed[p.Name] = p.Minimum
	}
	var conflicts []SysctlConfConflict
	for i, line := range lines {
		name, oldVal, ok := ParseSysctlConfLine(line)
		if !ok {
			continue
		}
		if minimum, found := managed[name]; found && oldVal < minimum {
			conflicts = append(conflicts, SysctlConfConflict{Name: name, OldVal: oldVal, NewVal: minimum, LineIdx: i})
		}
	}
	return conflicts
}

// RestoreSysctlConfLines applies backed-up values to matching sysctl.conf lines.
func RestoreSysctlConfLines(lines []string, backup map[string]string) bool {
	changed := false
	for key, val := range backup {
		name, ok := strings.CutPrefix(key, BackupSysctlConfPrefix)
		if !ok {
			continue
		}
		oldVal, err := strconv.Atoi(val)
		if err != nil {
			continue
		}
		for i, line := range lines {
			lineName, _, ok := ParseSysctlConfLine(line)
			if ok && lineName == name {
				lines[i] = fmt.Sprintf("%s=%d", name, oldVal)
				changed = true
				break
			}
		}
	}
	return changed
}

// ScanSysctlConf reads /etc/sysctl.conf and returns all lines plus any of our
// managed params whose values are below the required minimum.
func ScanSysctlConf() (lines []string, conflicts []SysctlConfConflict) {
	data, err := os.ReadFile(SysctlConfPath)
	if err != nil {
		return nil, nil
	}
	lines = strings.Split(string(data), "\n")
	return lines, ManagedSysctlConflicts(lines)
}

// PatchSysctlConf updates /etc/sysctl.conf in-place for any managed params below minimum.
// Returns the list of changes made (empty if none needed or file absent).
func PatchSysctlConf() ([]SysctlConfConflict, error) {
	lines, conflicts := ScanSysctlConf()
	if len(conflicts) == 0 {
		return nil, nil
	}
	for _, c := range conflicts {
		lines[c.LineIdx] = fmt.Sprintf("%s=%d", c.Name, c.NewVal)
	}
	return conflicts, os.WriteFile(SysctlConfPath, []byte(strings.Join(lines, "\n")), 0o644)
}

// RevertSysctlConf restores values in /etc/sysctl.conf from the backup map.
func RevertSysctlConf(backup map[string]string) {
	lines, _ := ScanSysctlConf()
	if lines == nil {
		return
	}
	if RestoreSysctlConfLines(lines, backup) {
		_ = os.WriteFile(SysctlConfPath, []byte(strings.Join(lines, "\n")), 0o644)
	}
}

// AppendSysctlConfToBackup appends sysctl.conf pre-patch values to an existing backup file.
func AppendSysctlConfToBackup(path string, conflicts []SysctlConfConflict) error {
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	for _, c := range conflicts {
		fmt.Fprintf(f, "%s%s=%d\n", BackupSysctlConfPrefix, c.Name, c.OldVal)
	}
	return nil
}

// KernelStatusRow is one row in a kernel settings status table.
type KernelStatusRow struct {
	Name   string
	Col1   string
	Col2   string
	Status string
}

// writerOrDefault returns w if non-nil, otherwise os.Stdout.
func writerOrDefault(w io.Writer) io.Writer {
	if w != nil {
		return w
	}
	return os.Stdout
}

// stderrOrDefault returns w if non-nil, otherwise os.Stderr.
func stderrOrDefault(w io.Writer) io.Writer {
	if w != nil {
		return w
	}
	return os.Stderr
}

// PrintKernelStatusTable prints a dynamic-width table of kernel setting rows to w.
func PrintKernelStatusTable(w io.Writer, col1, col2 string, rows []KernelStatusRow) {
	w = writerOrDefault(w)
	tableRows := make([][]string, 0, len(rows)+1)
	tableRows = append(tableRows, []string{"setting", col1, col2, "status"})
	for _, r := range rows {
		tableRows = append(tableRows, []string{r.Name, r.Col1, r.Col2, r.Status})
	}
	formatTable(w, tableRows)
}

// formatTable writes a space-aligned table to w (2-space column padding).
func formatTable(w io.Writer, rows [][]string) {
	if len(rows) == 0 {
		return
	}
	cols := 0
	for _, row := range rows {
		if len(row) > cols {
			cols = len(row)
		}
	}
	widths := make([]int, cols)
	for _, row := range rows {
		for j, cell := range row {
			if w := len(cell); w > widths[j] {
				widths[j] = w
			}
		}
	}
	for _, row := range rows {
		for j, cell := range row {
			fmt.Fprintf(w, "%s", cell)
			if j < len(row)-1 {
				padding := widths[j] - len(cell) + 2
				fmt.Fprintf(w, "%s", strings.Repeat(" ", padding))
			}
		}
		fmt.Fprintln(w)
	}
}

// KernelParamCheck describes a required host sysctl setting.
type KernelParamCheck struct {
	Name    string // sysctl key (e.g. "vm.max_map_count")
	Path    string // /proc path to read current value
	Minimum int    // required minimum value
}

// HostKernelParams lists all sysctl settings checked and fixed by this tool.
var HostKernelParams = []KernelParamCheck{
	// Proton/Wine requires a high mmap limit; default (65530) causes crashes.
	{Name: "vm.max_map_count", Path: "/proc/sys/vm/max_map_count", Minimum: 16_777_216},
	// UDP receive/send buffers; ARK game traffic is UDP and drops packets with small buffers.
	{Name: "net.core.rmem_max", Path: "/proc/sys/net/core/rmem_max", Minimum: 16_777_216},
	{Name: "net.core.wmem_max", Path: "/proc/sys/net/core/wmem_max", Minimum: 16_777_216},
	// Kernel-wide file handle ceiling; must be above per-process rlimits (esync uses many fds).
	{Name: "fs.file-max", Path: "/proc/sys/fs/file-max", Minimum: 1_048_576},
}

// ReadKernelParam reads an integer value from a /proc/sys path.
// If deps.ReadKernelParam is set, it is used (for testing); otherwise os.ReadFile is used.
func ReadKernelParam(deps HostDeps, path string) (int, error) {
	if deps.ReadKernelParam != nil {
		return deps.ReadKernelParam(path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	return strconv.Atoi(strings.TrimSpace(string(data)))
}

// GetNofileHardLimit reads the current process NOFILE hard limit via syscall.
func GetNofileHardLimit() (uint64, error) {
	var rlimit syscall.Rlimit
	if err := syscall.Getrlimit(syscall.RLIMIT_NOFILE, &rlimit); err != nil {
		return 0, err
	}
	return rlimit.Max, nil
}

// RequireRoot returns an error with a helpful re-run message if not running as root.
func RequireRoot(commandName string) error {
	if os.Geteuid() == 0 {
		return nil
	}
	return fmt.Errorf("Error: root privileges are required. Re-run with:\n  sudo ark-cli %s", commandName)
}

// CheckOneParam reads and evaluates a single sysctl kernel parameter.
func CheckOneParam(deps HostDeps, p KernelParamCheck) (ok bool, val int, err error) {
	val, err = ReadKernelParam(deps, p.Path)
	if err != nil {
		return false, 0, err
	}
	return val >= p.Minimum, val, nil
}

// CheckNofileLimit reads and evaluates the current NOFILE hard rlimit.
func CheckNofileLimit(deps HostDeps) (ok bool, val uint64, err error) {
	val, err = deps.GetNofileHardLimit()
	if err != nil {
		return false, 0, err
	}
	return val >= MinNofileHardLimit, val, nil
}

// SnapshotKernelParams reads the current value of every tracked sysctl param.
func SnapshotKernelParams(deps HostDeps) (map[string]int, error) {
	snap := make(map[string]int, len(HostKernelParams))
	for _, p := range HostKernelParams {
		val, err := ReadKernelParam(deps, p.Path)
		if err != nil {
			return nil, fmt.Errorf("could not read %s: %w", p.Name, err)
		}
		snap[p.Name] = val
	}
	return snap, nil
}

// WarnBelowMinimum emits a warning block for any failing params.
// Non-blocking; used as a preflight check before starting an instance.
func WarnBelowMinimum(deps HostDeps) {
	var failed []string
	for _, p := range HostKernelParams {
		ok, val, err := CheckOneParam(deps, p)
		if err != nil {
			failed = append(failed, fmt.Sprintf("  %s (unreadable: %v)", p.Name, err))
		} else if !ok {
			failed = append(failed, fmt.Sprintf("  %s=%d (minimum: %d)", p.Name, val, p.Minimum))
		}
	}
	if ok, val, err := CheckNofileLimit(deps); err != nil {
		failed = append(failed, fmt.Sprintf("  nofile hard limit (unreadable: %v)", err))
	} else if !ok {
		failed = append(failed, fmt.Sprintf("  nofile hard limit=%d (minimum: %d)", val, MinNofileHardLimit))
	}
	if len(failed) == 0 {
		return
	}
	stderr := stderrOrDefault(deps.Stderr)
	fmt.Fprintf(stderr, "Warning: the following host settings are below the required minimum for Proton/Wine:\n")
	for _, msg := range failed {
		fmt.Fprintf(stderr, "%s\n", msg)
	}
	fmt.Fprintf(stderr, "Run 'sudo ark-cli fix host-kernel-config' to apply required settings.\n")
}

// KernelParamBackupPath returns the path for the pre-change backup file.
func KernelParamBackupPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "ark-cli", "kernel-params-backup.conf"), nil
}

// WriteKernelParamBackup saves a sysctl-loadable backup of the pre-change values.
func WriteKernelParamBackup(snapshot map[string]int, nofileHardLimit uint64, backupPath string) error {
	if err := os.MkdirAll(filepath.Dir(backupPath), 0o755); err != nil {
		return err
	}
	var sb strings.Builder
	sb.WriteString("# ARK server kernel parameter backup (pre-fix values)\n")
	fmt.Fprintf(&sb, "# Created: %s\n", time.Now().UTC().Format(time.RFC3339))
	fmt.Fprintf(&sb, "# Restore sysctl settings with: sudo sysctl -p %s\n", backupPath)
	for _, p := range HostKernelParams {
		if val, ok := snapshot[p.Name]; ok {
			fmt.Fprintf(&sb, "%s=%d\n", p.Name, val)
		}
	}
	fmt.Fprintf(&sb, "%s=%d\n", BackupNofileKey, nofileHardLimit)
	return os.WriteFile(backupPath, []byte(sb.String()), 0o644)
}

// ParseKernelBackupFile parses sysctl key=value lines, skipping comments and blanks.
func ParseKernelBackupFile(data []byte) map[string]string {
	result := make(map[string]string)
	for line := range strings.SplitSeq(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if k, v, ok := strings.Cut(line, "="); ok {
			result[strings.TrimSpace(k)] = strings.TrimSpace(v)
		}
	}
	return result
}

// SysctlPersistContent generates the permanent sysctl.d file content.
func SysctlPersistContent() string {
	var sb strings.Builder
	sb.WriteString("# ARK server required kernel settings\n")
	sb.WriteString("# Managed by: ark-cli fix host-kernel-config\n")
	for _, p := range HostKernelParams {
		fmt.Fprintf(&sb, "%s=%d\n", p.Name, p.Minimum)
	}
	return sb.String()
}

// NofileLimitsContent generates the limits.d file content for the NOFILE fix.
func NofileLimitsContent() string {
	var sb strings.Builder
	sb.WriteString("# ARK server open file descriptor limit\n")
	sb.WriteString("# Managed by: ark-cli fix host-kernel-config\n")
	fmt.Fprintf(&sb, "* soft nofile %d\n", MinNofileHardLimit)
	fmt.Fprintf(&sb, "* hard nofile %d\n", MinNofileHardLimit)
	return sb.String()
}

// NofileSystemdContent generates the systemd Manager override for NOFILE.
func NofileSystemdContent() string {
	var sb strings.Builder
	sb.WriteString("# ARK server open file descriptor limit\n")
	sb.WriteString("# Managed by: ark-cli fix host-kernel-config\n")
	sb.WriteString("[Manager]\n")
	fmt.Fprintf(&sb, "DefaultLimitNOFILE=1024:%d\n", MinNofileHardLimit)
	return sb.String()
}

// NofileConfiguredValue reads the nofile hard limit from the limits.d persist file.
// Returns (value, true) if the file exists and its value meets the minimum.
func NofileConfiguredValue() (uint64, bool) {
	data, err := os.ReadFile(NofileLimitsPersistPath)
	if err != nil {
		return 0, false
	}
	for line := range strings.SplitSeq(string(data), "\n") {
		if valStr, ok := strings.CutPrefix(strings.TrimSpace(line), "* hard nofile "); ok {
			if val, parseErr := strconv.ParseUint(strings.TrimSpace(valStr), 10, 64); parseErr == nil {
				return val, val >= MinNofileHardLimit
			}
		}
	}
	return 0, false
}

// ApplyNofilePersistFiles writes limits.d and, if systemd is present, the systemd override.
func ApplyNofilePersistFiles(deps HostDeps) error {
	if err := os.MkdirAll(filepath.Dir(NofileLimitsPersistPath), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(NofileLimitsPersistPath, []byte(NofileLimitsContent()), 0o644); err != nil {
		return err
	}
	// Systemd override is non-fatal if the directory or write fails.
	if _, err := os.Stat("/etc/systemd"); err == nil {
		if mkErr := os.MkdirAll(filepath.Dir(NofileSystemdPersistPath), 0o755); mkErr == nil {
			if wErr := os.WriteFile(NofileSystemdPersistPath, []byte(NofileSystemdContent()), 0o644); wErr != nil {
				deps.Logger.Warn("could not write systemd override", "path", NofileSystemdPersistPath, "error", wErr)
			}
		}
	}
	return nil
}

// RemoveNofilePersistFiles removes the limits.d and systemd override files.
func RemoveNofilePersistFiles(deps HostDeps) {
	for _, path := range []string{NofileLimitsPersistPath, NofileSystemdPersistPath} {
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			deps.Logger.Warn("could not remove persist file", "path", path, "error", err)
		} else if err == nil {
			deps.Logger.Info("removed persist file", "path", path)
		}
	}
}
