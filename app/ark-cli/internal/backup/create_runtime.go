package backup

import (
	"ark-cli/internal/core"

	pkgbackup "pkg/ark/backup"
)

func (e BackupCreateExecution) runTarget(target string) error {
	if err := e.config.EnsureService(target); err != nil {
		return err
	}

	d := core.CurrentAppDeps()
	result, err := d.BackupService.CreateForTarget(core.CurrentAppDeps().Context(), pkgbackup.CreateRequest{
		ComposeFile:       e.composeFile,
		Service:           target,
		BackupRoot:        e.backupRoot,
		DestOverride:      e.destOverride,
		NameOverride:      e.nameOverride,
		Format:            e.format,
		Timestamp:         e.timestamp,
		PID:               e.pid,
		LockTimeout:       e.lockTimeout,
		BackupWaitTimeout: e.backupWaitTimeout,
		Retention:         e.retentionPolicy,
	}, target)
	if err != nil {
		return err
	}
	core.Infoln(result.ArchivePath)
	return nil
}
