package cmd

import (
	"ark-cli/internal/backup"
	"ark-cli/internal/core"
	"ark-cli/internal/remove"
	"ark-cli/internal/schedule"
)

// WireDefaults sets CLI-orchestration function seams on deps.
// This avoids package main needing to reference individual implementation
// functions, keeping them unexported in the cmd package.
func WireDefaults(deps *core.AppDeps) {
	// Compose wire defaults are now handled by compose.WireDefaults.
	// Schedule wire defaults are now handled by schedule.WireDefaults.
	// This function only wires non-compose, non-schedule seams.
	deps.RunBackupFromSchedule = backup.RunBackupFromScheduleImpl
	deps.InstallManagedSchedulesCrontab = schedule.InstallManagedSchedulesCrontabImpl
	deps.StopService = remove.StopServiceImpl
	deps.RemoveVolume = remove.RemoveVolumeImpl
}
