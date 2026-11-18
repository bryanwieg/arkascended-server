package backup

import (
	"ark-cli/internal/compose"
	"ark-cli/internal/core"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"pkg/ark"
	pkgbackup "pkg/ark/backup"
)

func RunVerifyBackup(d *core.AppDeps, args []string) error {
	fs := core.NewCommandFlagSet(
		"validate backup",
		[]string{"validate backup --instance <instance> --backup <backup|latest> [options]"},
		"Validate SQLite integrity of databases inside a backup archive.",
	)
	instanceFlag := fs.String("instance", "", "Target instance name.")
	backupFlag := fs.String("backup", "", "Backup name or 'latest'.")
	if err := core.ParseNoPositionalArgs(fs, args); err != nil {
		return err
	}
	if *instanceFlag == "" || *backupFlag == "" {
		return errors.New(ErrInstanceAndBackupNameRequired)
	}
	instance := *instanceFlag
	backupName := *backupFlag

	backupRoot := core.BackupRootDir()
	backupsDir := filepath.Join(backupRoot, instance)
	archivePath, err := pkgbackup.ResolveArchive(backupsDir, backupName)
	if err != nil {
		return err
	}

	tempDir, err := ark.TempDir("ark-backup-verify-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tempDir)

	if err := d.BackupService.ExtractArchive(d.Context(), archivePath, tempDir); err != nil {
		return err
	}
	sqliteFiles, err := pkgbackup.FindSQLiteFiles(tempDir)
	if err != nil {
		return err
	}
	if len(sqliteFiles) == 0 {
		return errors.New("Error: backup verification found no .sqlite files in the archive.")
	}

	failures := 0
	for _, path := range sqliteFiles {
		rel, _ := filepath.Rel(tempDir, path)
		if err := d.BackupService.CheckSQLiteIntegrity(d.Context(), path); err != nil {
			core.Warnf("VERIFY FAIL\t%s\t%s\n", rel, err)
			failures++
			continue
		}
		core.Infof("VERIFY OK\t%s\n", rel)
	}
	if failures > 0 {
		return fmt.Errorf("Error: backup verification failed for %d SQLite file(s)", failures)
	}
	core.Infof("Verification passed: %s (%d SQLite file(s))\n", filepath.Base(archivePath), len(sqliteFiles))
	return nil
}

func RunPruneBackups(d *core.AppDeps, args []string) error {
	fs := core.NewCommandFlagSet(
		"prune backups",
		[]string{"prune backups --instance <instance|cluster|all> [options]"},
		"Apply backup retention policies without creating a new backup.",
	)
	instanceFlag := fs.String("instance", "", "Target instance name, 'cluster' or 'all'.")
	retainCount := fs.Int("retain-count", 20, "Max backups to keep per instance (0 disables count-based pruning).")
	retainDays := fs.Int("retain-days", 7, "Max age in days for backups (0 disables age-based pruning).")
	retainSizeGB := fs.Int("retain-size-gb", 5, "Max total backup size per instance in GiB (0 disables size-based pruning).")
	composeFileFlag := fs.String("compose-file", "", "Path to docker-compose.yaml (required for all).")
	dryRun := fs.Bool("dry-run", false, "Print what would be pruned without deleting anything.")
	if err := core.ParseNoPositionalArgs(fs, args); err != nil {
		return err
	}
	if *instanceFlag == "" {
		return errors.New("Error: --instance is required for prune backups.")
	}
	instance := *instanceFlag
	if err := validateNonNegativeIntFlag("retain-count", *retainCount); err != nil {
		return err
	}
	if err := validateNonNegativeIntFlag("retain-days", *retainDays); err != nil {
		return err
	}
	if err := validateNonNegativeIntFlag("retain-size-gb", *retainSizeGB); err != nil {
		return err
	}

	policy := pkgbackup.NewRetentionPolicy(*retainCount, *retainDays, *retainSizeGB)
	backupRoot := core.BackupRootDirForComposeFile(compose.ResolveComposeFile(*composeFileFlag))

	if *dryRun {
		if instance != "all" {
			destDir := filepath.Join(backupRoot, instance)
			return dryRunRetention(destDir, instance, policy)
		}
		_, config, err := compose.ResolveRequiredComposeConfig(d, *composeFileFlag)
		if err != nil {
			return err
		}
		services := config.ServiceNames()
		if len(services) == 0 {
			return errors.New("Error: compose config has no services to prune.")
		}
		for _, service := range append(services, pkgbackup.ClusterTarget) {
			destDir := filepath.Join(backupRoot, service)
			if _, err := os.Stat(destDir); err != nil && service == pkgbackup.ClusterTarget {
				continue // no cluster backups yet
			}
			if err := dryRunRetention(destDir, service, policy); err != nil {
				return err
			}
		}
		return nil
	}

	if instance != "all" {
		return pruneBackupsForService(d, backupRoot, instance, policy)
	}

	_, config, err := compose.ResolveRequiredComposeConfig(d, *composeFileFlag)
	if err != nil {
		return err
	}
	services := config.ServiceNames()
	if len(services) == 0 {
		return errors.New("Error: compose config has no services to prune.")
	}
	for _, service := range append(services, pkgbackup.ClusterTarget) {
		if err := pruneBackupsForService(d, backupRoot, service, policy); err != nil {
			return err
		}
	}
	return nil
}

func pruneBackupsForService(d *core.AppDeps, backupRoot, service string, policy pkgbackup.RetentionPolicy) error {
	destDir, err := pkgbackup.DirectoryForInstance(backupRoot, service)
	if err != nil {
		core.Infof("Nothing to prune for %s\n", service)
		return nil
	}
	archives, listErr := pkgbackup.ListArchives(destDir)
	if listErr != nil || len(archives) == 0 {
		core.Infof("Nothing to prune for %s\n", service)
		return nil
	}
	// Same lock as create/restore so prune never deletes an archive mid-write or mid-restore.
	if err := pkgbackup.WithInstanceLock(destDir, service, 0, func() error {
		return d.BackupService.ApplyRetention(destDir, service, policy)
	}); err != nil {
		return err
	}
	core.Infof("Pruned backups for %s\n", service)
	return nil
}

func dryRunRetention(destDir, instance string, policy pkgbackup.RetentionPolicy) error {
	deps := core.CurrentAppDeps()
	results, err := pkgbackup.DryRunRetention(destDir, instance, policy, deps.DiskUsageForPath, deps.Logger)
	if err != nil {
		return err
	}
	for _, r := range results {
		if r.Remove {
			core.Infof("would remove: %s (%s)\n", r.Path, r.Reason)
		} else {
			core.Infof("would keep:   %s\n", r.Path)
		}
	}
	return nil
}
