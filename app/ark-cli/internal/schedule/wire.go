package schedule

import (
	"ark-cli/internal/backup"
	"ark-cli/internal/core"
)

// WireDefaults wires schedule-specific function seams on deps.
func WireDefaults(deps *core.AppDeps) {
	deps.RunBackupFromSchedule = backup.RunBackupFromScheduleImpl
}
