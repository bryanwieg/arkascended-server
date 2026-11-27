package schedule

import (
	"errors"
	"fmt"
	"strings"

	"github.com/robfig/cron/v3"
)

// ParseCron parses a standard 5-field cron expression (descriptors such as
// @daily allowed). Times are interpreted in time.Local, so the per-schedule
// TZ=/CRON_TZ= prefix and @every, which cron.ParseStandard also accepts, are refused.
func ParseCron(expr string) (cron.Schedule, error) {
	trimmed := strings.TrimSpace(expr)
	if strings.HasPrefix(trimmed, "TZ=") || strings.HasPrefix(trimmed, "CRON_TZ=") {
		return nil, errors.New("time zone prefixes are not supported; schedules use the TZ environment variable")
	}
	if strings.HasPrefix(trimmed, "@every") {
		return nil, errors.New("@every is not supported; use a 5-field expression")
	}
	return cron.ParseStandard(expr)
}

// KnownType reports whether t is a schedule type the runner executes.
func KnownType(t string) bool {
	return t == TypeBackup || t == TypeRCON || t == TypeSave || t == TypeRestart || t == TypeUpdate
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
	return args
}

// BackupCommandPreview returns the ark-cli command a backup schedule entry runs.
func BackupCommandPreview(s BackupSchedule) []string {
	return append([]string{"ark-cli", "create", "backup", "--instance", s.Instance}, BackupFlagArgs(s)...)
}
