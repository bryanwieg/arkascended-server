package scheduler

import (
	"os"
	"path/filepath"
	"strings"

	"ark-cli/internal/core"

	"pkg/ark"
)

// Header lines of the crontab ark-cli used to generate and install whole.
const (
	managedCrontabHeader     = "# ark-cli managed schedules"
	managedCrontabHeaderNote = "# Do not edit manually; regenerate via ark-cli schedule commands."
)

// StripManagedCrontab returns text without the two header lines and every "# schedule <name>:" line plus
// the line after it; changed is false when the header is absent.
func StripManagedCrontab(text string) (out string, changed bool) {
	lines := strings.Split(text, "\n")
	found := false
	for _, line := range lines {
		if line == managedCrontabHeader {
			found = true
			break
		}
	}
	if !found {
		return text, false
	}
	kept := make([]string, 0, len(lines))
	for i := 0; i < len(lines); i++ {
		line := lines[i]
		switch {
		case line == managedCrontabHeader, line == managedCrontabHeaderNote:
		case strings.HasPrefix(line, "# schedule ") && strings.Contains(line, ":"):
			i++ // the cron line generated for that schedule
		default:
			kept = append(kept, line)
		}
	}
	return strings.Join(kept, "\n"), true
}

// RemoveManagedCrontab: no crontab binary, or `crontab -l` exits non-zero (no crontab) → nothing.
// Header found → install the rest with `crontab <tmpfile>`, or `crontab -r` when only blank lines remain,
// then core.Infof("Removed ark-cli schedules from your crontab\n").
func RemoveManagedCrontab(d *core.AppDeps) error {
	if _, err := d.Runtime.LookPath("crontab"); err != nil {
		return nil //nolint:nilerr // no crontab binary: ark-cli never installed one here
	}
	current, err := d.Runtime.RunCommand([]string{"crontab", "-l"}, ark.CommandOptions{Capture: true})
	if err != nil {
		return err
	}
	if current.ExitCode != 0 {
		return nil
	}
	rest, changed := StripManagedCrontab(current.Stdout)
	if !changed {
		return nil
	}
	if err := installCrontab(d, rest); err != nil {
		return err
	}
	core.Infof("Removed ark-cli schedules from your crontab\n")
	return nil
}

func installCrontab(d *core.AppDeps, text string) error {
	if strings.TrimSpace(text) == "" {
		return core.RunCommandExpectZero(d, []string{"crontab", "-r"}, ark.CommandOptions{Capture: true}, nil)
	}
	dir, err := ark.TempDir("crontab-")
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(dir) }()
	path := filepath.Join(dir, "crontab")
	if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
		return err
	}
	return core.RunCommandExpectZero(d, []string{"crontab", path}, ark.CommandOptions{Capture: true}, nil)
}
