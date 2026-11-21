package schedule

import (
	"errors"
	"fmt"
	"strings"

	"github.com/robfig/cron/v3"
)

// ParseCron parses a standard 5-field cron expression (descriptors such as
// @daily allowed). Times are interpreted in time.Local.
func ParseCron(expr string) (cron.Schedule, error) {
	return cron.ParseStandard(expr)
}

// ValidateCronExpr reports whether expr is a valid standard cron expression.
func ValidateCronExpr(expr string) error {
	if _, err := ParseCron(expr); err != nil {
		return fmt.Errorf("Error: invalid cron expression %q: %w", expr, err)
	}
	return nil
}

// ValidateRetention checks that all retention values are non-negative.
func ValidateRetention(s BackupSchedule) error {
	if s.RetainCount < 0 || s.RetainDays < 0 || s.RetainSizeGB < 0 {
		return errors.New(ErrRetentionValuesNonNegative)
	}
	return nil
}

// BackupFlagArgs returns the CLI flag arguments for a backup schedule entry.
func BackupFlagArgs(s BackupSchedule) []string {
	args := []string{}
	if s.Format != "" {
		args = append(args, "--format", s.Format)
	}
	if s.RetainCount != 0 {
		args = append(args, "--retain-count", fmt.Sprintf("%d", s.RetainCount))
	}
	if s.RetainDays != 0 {
		args = append(args, "--retain-days", fmt.Sprintf("%d", s.RetainDays))
	}
	if s.RetainSizeGB != 0 {
		args = append(args, "--retain-size-gb", fmt.Sprintf("%d", s.RetainSizeGB))
	}
	if s.ComposeFile != "" {
		args = append(args, "--compose-file", s.ComposeFile)
	}
	return args
}

// BackupCommandParts returns the human-readable preview and the full CLI
// command for executing a backup schedule entry.
func (s *Service) BackupCommandParts(sched BackupSchedule) (preview []string, cmd []string) {
	preview = []string{"ark-cli", "create", "backup", "--instance", sched.Instance}
	cmd = append(s.CLICommand, "create", "backup", "--instance", sched.Instance)
	flagArgs := BackupFlagArgs(sched)
	preview = append(preview, flagArgs...)
	cmd = append(cmd, flagArgs...)
	return preview, cmd
}

// ShellQuote returns a single-quoted shell-safe string.
func ShellQuote(value string) string {
	if value == "" {
		return "''"
	}
	return "'" + strings.ReplaceAll(value, "'", `'"'"'`) + "'"
}

// ShellJoin joins arguments with shell quoting.
func ShellJoin(args []string) string {
	quoted := make([]string, 0, len(args))
	for _, a := range args {
		quoted = append(quoted, ShellQuote(a))
	}
	return strings.Join(quoted, " ")
}
