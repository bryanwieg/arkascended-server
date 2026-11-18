package testutil

import (
	"ark-cli/internal/core"

	"pkg/ark/host"
)

// MergeNonZeroAppDepFields copies non-zero fields from src into dst.
// Mirror of core.MergeNonZeroAppDepFields — duplicated here because Go's internal
// package rules prevent testutil from importing core's merge symbols directly.
func MergeNonZeroAppDepFields(dst, src *core.AppDeps) {
	if dst == nil || src == nil {
		return
	}
	if src.Ctx != nil {
		dst.Ctx = src.Ctx
	}
	if src.Engine != nil {
		dst.Engine = src.Engine
	}
	if src.ComposeService != nil {
		dst.ComposeService = src.ComposeService
	}
	if src.HealthChecker != nil {
		dst.HealthChecker = src.HealthChecker
	}
	if src.Lifecycle != nil {
		dst.Lifecycle = src.Lifecycle
	}
	if src.BackupService != nil {
		dst.BackupService = src.BackupService
	}
	if src.UpdateService != nil {
		dst.UpdateService = src.UpdateService
	}
	if src.ScheduleService != nil {
		dst.ScheduleService = src.ScheduleService
	}
	MergeHostDeps(&dst.HostDeps, src.HostDeps)
	MergeRuntimeDeps(&dst.Runtime, src.Runtime)
	if src.RestartStartInstance != nil {
		dst.RestartStartInstance = src.RestartStartInstance
	}
	if src.RestartStopInstance != nil {
		dst.RestartStopInstance = src.RestartStopInstance
	}
	if src.RunBackupFromSchedule != nil {
		dst.RunBackupFromSchedule = src.RunBackupFromSchedule
	}
	if src.InstallManagedSchedulesCrontab != nil {
		dst.InstallManagedSchedulesCrontab = src.InstallManagedSchedulesCrontab
	}
	if src.RunningServiceSnapshot != nil {
		dst.RunningServiceSnapshot = src.RunningServiceSnapshot
	}
	if src.DiskUsageForPath != nil {
		dst.DiskUsageForPath = src.DiskUsageForPath
	}
	if src.StopService != nil {
		dst.StopService = src.StopService
	}
	if src.RemoveVolume != nil {
		dst.RemoveVolume = src.RemoveVolume
	}
}

// MergeRuntimeDeps copies non-zero fields from src into dst.
func MergeRuntimeDeps(dst *core.RuntimeDeps, src core.RuntimeDeps) {
	if src.RunCommand != nil {
		dst.RunCommand = src.RunCommand
	}
	if src.LookPath != nil {
		dst.LookPath = src.LookPath
	}
	if src.ContainerRuntime != "" {
		dst.ContainerRuntime = src.ContainerRuntime
	}
}

// MergeHostDeps copies non-zero fields from src into dst.
func MergeHostDeps(dst *host.HostDeps, src host.HostDeps) {
	if src.RunCommand != nil {
		dst.RunCommand = src.RunCommand
	}
	if src.LookPath != nil {
		dst.LookPath = src.LookPath
	}
	if src.ReadKernelParam != nil {
		dst.ReadKernelParam = src.ReadKernelParam
	}
	if src.GetNofileHardLimit != nil {
		dst.GetNofileHardLimit = src.GetNofileHardLimit
	}
	if src.OpenUDPProbeListener != nil {
		dst.OpenUDPProbeListener = src.OpenUDPProbeListener
	}
	if src.FetchText != nil {
		dst.FetchText = src.FetchText
	}
	if src.Logger != nil {
		dst.Logger = src.Logger
	}
	if src.Stdout != nil {
		dst.Stdout = src.Stdout
	}
	if src.Stderr != nil {
		dst.Stderr = src.Stderr
	}
}
