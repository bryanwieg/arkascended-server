package backup

import (
	"ark-cli/internal/core"

	pkgbackup "pkg/ark/backup"
)

func (e BackupCreateExecution) runTarget(target string) error {
	if target == pkgbackup.ClusterTarget {
		return e.runCluster()
	}
	if err := e.config.EnsureService(target); err != nil {
		return err
	}

	d := core.CurrentAppDeps()
	result, err := d.BackupService.CreateForTarget(core.CurrentAppDeps().Context(), e.request(target), target)
	if err != nil {
		return err
	}
	core.Infoln(result.ArchivePath)
	return nil
}

func (e BackupCreateExecution) runCluster() error {
	cv, err := pkgbackup.ResolveClusterVolume(e.config)
	if err != nil {
		return err
	}
	d := core.CurrentAppDeps()
	result, err := d.BackupService.CreateCluster(d.Context(), e.request(pkgbackup.ClusterTarget), cv)
	if err != nil {
		return err
	}
	core.Infoln(result.ArchivePath)
	return nil
}

func (e BackupCreateExecution) request(target string) pkgbackup.CreateRequest {
	return pkgbackup.CreateRequest{
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
	}
}
