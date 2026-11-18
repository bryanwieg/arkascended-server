package backup

import (
	"ark-cli/internal/compose"
	"ark-cli/internal/core"

	pkgbackup "pkg/ark/backup"
)

// runRestoreCluster restores the shared cluster volume from <backupRoot>/cluster/<backupName>.
func runRestoreCluster(backupName, composeFileFlag string) error {
	backupRoot, archive, err := resolveBackupArchive(pkgbackup.ClusterTarget, backupName, composeFileFlag)
	if err != nil {
		return err
	}
	d := core.CurrentAppDeps()
	composeFile, config, err := compose.ResolveRequiredComposeConfig(d, composeFileFlag)
	if err != nil {
		return err
	}
	cv, err := pkgbackup.ResolveClusterVolume(config)
	if err != nil {
		return err
	}
	result, err := d.BackupService.RestoreCluster(d.Context(), pkgbackup.RestoreRequest{
		ComposeFile: composeFile,
		ArchivePath: archive,
		BackupRoot:  backupRoot,
	}, cv)
	if err != nil {
		return err
	}
	core.Infof("Restored cluster data (%s). Pre-restore backup: %s\n", cv.Source, result.PreRestorePath)
	return nil
}
