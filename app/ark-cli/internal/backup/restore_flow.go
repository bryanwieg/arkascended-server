package backup

import (
	"cmp"
	"errors"
	"fmt"
	"os"

	"ark-cli/internal/compose"
	"ark-cli/internal/core"

	"pkg/ark"
	pkgbackup "pkg/ark/backup"
	"pkg/ark/runtime"
)

func runRestoreWithInstance(instance, srcInstance, backupName, composeFileFlag string, autoStart bool) error {
	backupRoot, archive, err := resolveBackupArchive(srcInstance, backupName, composeFileFlag)
	if err != nil {
		return err
	}

	composeFile, composeProject, err := compose.ResolveRequiredServiceConfig(core.CurrentAppDeps(), composeFileFlag, instance, compose.ResolveRequiredComposeConfig)
	if err != nil {
		return err
	}

	if err := checkRestoreMapMatch(composeProject, instance, archive); err != nil {
		return err
	}

	return executeRestore(instance, backupName, backupRoot, archive, composeFile, autoStart)
}

// newInstancePasswords holds the password flags for a restore into a new instance.
type newInstancePasswords struct {
	admin, server string
	noServer      bool
}

func runRestoreNewInstance(newInstance, srcInstance, backupName, composeFileFlag string, autoStart bool, flags newInstancePasswords) error {
	backupRoot, archive, err := resolveBackupArchive(srcInstance, backupName, composeFileFlag)
	if err != nil {
		return err
	}

	composeFile := compose.ResolveComposeFile(composeFileFlag)
	if err := compose.RequireComposeFile(composeFile); err != nil {
		return err
	}

	d := core.CurrentAppDeps()
	archiveMapName, err := d.BackupService.ReadArchiveMapName(d.Context(), archive)
	if err != nil {
		return fmt.Errorf("Error: could not read backup archive map name: %v", err)
	}

	adminPassword, serverPassword, err := resolveNewInstancePasswords(d, archive, flags)
	if err != nil {
		return err
	}

	if err := scaffoldComposeService(composeFile, newInstance, archiveMapName, adminPassword, serverPassword); err != nil {
		return err
	}

	return executeRestore(newInstance, backupName, backupRoot, archive, composeFile, autoStart)
}

func resolveBackupArchive(instance, backupName, composeFileFlag string) (string, string, error) {
	backupRoot := core.BackupRootDirForComposeFile(compose.ResolveComposeFile(composeFileFlag))
	backupsDir, err := pkgbackup.DirectoryForInstance(backupRoot, instance)
	if err != nil {
		return "", "", err
	}
	archive, err := pkgbackup.ResolveArchive(backupsDir, backupName)
	if err != nil {
		return "", "", err
	}
	return backupRoot, archive, nil
}

// resolveNewInstancePasswords takes the passwords from the flags, else from the backup's
// GameUserSettings.ini. The restored INI keeps the source's passwords,
// so the compose env must match them or every RCON save and stop fails.
func resolveNewInstancePasswords(d *core.AppDeps, archive string, flags newInstancePasswords) (string, string, error) {
	archiveAdmin, archiveServer, err := d.BackupService.ReadArchivePasswords(d.Context(), archive)
	if err != nil {
		return "", "", fmt.Errorf("Error: %w", err)
	}
	admin := cmp.Or(flags.admin, archiveAdmin)
	server := cmp.Or(flags.server, archiveServer)
	if flags.noServer {
		server = ""
	}
	if admin == "" {
		return "", "", errors.New("Error: the backup has no admin password; pass --admin-password.")
	}
	if err := runtime.ValidatePassword(admin, "admin password"); err != nil {
		return "", "", fmt.Errorf("%w Pass --admin-password to set a different one.", err)
	}
	if server != "" {
		if err := runtime.ValidatePassword(server, "server password"); err != nil {
			return "", "", fmt.Errorf("%w Pass --server-password or --no-server-password.", err)
		}
	}
	return admin, server, nil
}

func scaffoldComposeService(composeFile, instance, mapName, adminPassword, serverPassword string) error {
	composeText, err := os.ReadFile(composeFile)
	if err != nil {
		return err
	}
	composeStr := string(composeText)

	if err := runtime.ValidateComposeName(instance, "instance name"); err != nil {
		return err
	}
	if composeStr != "" && serviceExistsInComposeText(composeStr, instance) {
		return fmt.Errorf("Error: instance '%s' already exists. Use 'restore backup --instance %s --archive ...' instead.", instance, instance)
	}
	if err := runtime.ValidateYAMLScalar(mapName, "map name"); err != nil {
		return err
	}

	usedUDP := runtime.FindUsedPorts(composeStr, "udp")
	usedTCP := runtime.FindUsedPorts(composeStr, "tcp")
	asaPort := runtime.PickNextPort(usedUDP, 27015)
	rconPort := runtime.PickNextPort(usedTCP, 27020)

	settings := compose.CreateSettings{
		Instance:       instance,
		MapName:        mapName,
		SessionName:    instance,
		AdminPassword:  adminPassword,
		ServerPassword: serverPassword,
		ASAPort:        asaPort,
		RCONHostPort:   rconPort,
		TZ:             "America/New_York",
		Image:          "ark-server:latest",
	}
	rendered := compose.RenderTemplate(settings)
	updated, err := mergeCreateServiceIntoCompose(composeStr, rendered, instance)
	if err != nil {
		return err
	}
	if err := os.WriteFile(composeFile, []byte(updated), 0o644); err != nil {
		return err
	}
	core.Infof("Created new instance %s (map: %s) in %s\n", instance, mapName, composeFile)
	return nil
}

func serviceExistsInComposeText(composeText, name string) bool {
	return runtime.EnsureServiceAbsent(composeText, name) != nil
}

func mergeCreateServiceIntoCompose(composeText, rendered string, instance string) (string, error) {
	if composeText == "" {
		return rendered, nil
	}
	if err := runtime.EnsureServiceAbsent(composeText, instance); err != nil {
		return "", err
	}
	serviceBlock, err := runtime.ExtractServiceBlock(rendered)
	if err != nil {
		return "", err
	}
	updated := runtime.InsertService(composeText, serviceBlock)
	updated = runtime.EnsureVolumeEntry(updated, fmt.Sprintf("%s-saved", instance))
	updated = runtime.EnsureVolumeEntry(updated, "ark-cluster-1")
	return updated, nil
}

func executeRestore(instance, backupName, backupRoot, archive, composeFile string, autoStart bool) error {
	d := core.CurrentAppDeps()
	result, err := d.BackupService.Restore(d.Context(), pkgbackup.RestoreRequest{
		ComposeFile: composeFile,
		Service:     instance,
		Archive:     backupName,
		ArchivePath: archive,
		BackupRoot:  backupRoot,
		AutoStart:   autoStart,
	})
	if err != nil {
		return err
	}
	if autoStart {
		core.Infof("Restored to %s. Pre-restore backup: %s. Service restarted.\n", instance, result.PreRestorePath)
	} else {
		core.Infof("Restored to %s. Pre-restore backup: %s\n", instance, result.PreRestorePath)
	}
	return nil
}

func checkRestoreMapMatch(project *ark.ComposeProject, instance, archive string) error {
	service, ok := project.Project.Services[instance]
	if !ok {
		return nil
	}
	composeMapName := runtime.ServiceEnvValue(service, "MAP_NAME")
	if composeMapName == "" {
		return nil
	}
	d := core.CurrentAppDeps()
	archiveMapName, err := d.BackupService.ReadArchiveMapName(d.Context(), archive)
	if err != nil {
		return fmt.Errorf("Error: could not read backup archive map name: %v", err)
	}
	if archiveMapName != "" && archiveMapName != composeMapName {
		return fmt.Errorf(
			"Error: backup map %q does not match instance map %q. Restore aborted to prevent data corruption. Use a backup from the same map.",
			archiveMapName, composeMapName,
		)
	}
	return nil
}
