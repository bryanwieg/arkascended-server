package backup

import (
	"errors"
	"fmt"
	"strings"

	"ark-cli/internal/core"
)

// ErrInstanceAndBackupNameRequired is the error message for missing instance and backup name arguments.
// Used by sibling commands (validate backup, prune backups) that still use positional args.
const ErrInstanceAndBackupNameRequired = "Error: instance and backup name are required."

func RunRestore(d *core.AppDeps, args []string) error {
	fs := core.NewCommandFlagSet(
		"restore",
		[]string{
			"restore backup --instance <name> --archive [<src-instance>:]<backup>",
			"restore backup --new-instance <name> --archive <src-instance>:<backup> [--admin-password <pw>] [--server-password <pw> | --no-server-password]",
		},
		"Restore an instance from a backup archive. Use --instance for an existing instance or --new-instance to create one.",
	)

	instanceFlag := fs.String("instance", "", "Target existing instance name.")
	newInstanceFlag := fs.String("new-instance", "", "Create and restore to a new instance name.")
	archiveFlag := fs.String("archive", "", "Backup archive in the form [<src-instance>:]<backup-name>.")
	composeFileFlag := fs.String("compose-file", "", "Path to docker-compose.yaml (default: ./docker-compose.yaml or COMPOSE_FILE).")
	autoStart := fs.Bool("auto-start", false, "Start the service after restore completes.")
	adminPassword := fs.String("admin-password", "", "With --new-instance: admin password (default: the one in the backup).")
	serverPassword := fs.String("server-password", "", "With --new-instance: server password (default: the one in the backup).")
	noServerPassword := fs.Bool("no-server-password", false, "With --new-instance: no server password.")

	if err := core.ParseNoPositionalArgs(fs, args); err != nil {
		return err
	}

	if *instanceFlag == "" && *newInstanceFlag == "" {
		return errors.New("Error: must specify --instance or --new-instance.")
	}
	if *instanceFlag != "" && *newInstanceFlag != "" {
		return errors.New("Error: --instance and --new-instance are mutually exclusive.")
	}
	if *instanceFlag != "" && (*adminPassword != "" || *serverPassword != "" || *noServerPassword) {
		return errors.New("Error: password flags apply only with --new-instance. Use 'reconfigure instance' to change an existing instance's passwords.")
	}
	if *serverPassword != "" && *noServerPassword {
		return errors.New("Error: --server-password and --no-server-password are mutually exclusive.")
	}
	if *archiveFlag == "" {
		return errors.New("Error: --archive is required.")
	}

	archive := *archiveFlag
	srcInstance, backupName := parseArchiveFlag(archive, instanceFlag, newInstanceFlag)

	if srcInstance == "" {
		return fmt.Errorf("Error: invalid --archive format %q. Expected [<instance>:]<backup-name>.", archive)
	}
	if backupName == "" {
		return fmt.Errorf("Error: missing backup name in --archive %q.", archive)
	}

	if *newInstanceFlag != "" {
		passwords := newInstancePasswords{admin: *adminPassword, server: *serverPassword, noServer: *noServerPassword}
		return runRestoreNewInstance(*newInstanceFlag, srcInstance, backupName, *composeFileFlag, *autoStart, passwords)
	}
	return runRestoreWithInstance(*instanceFlag, srcInstance, backupName, *composeFileFlag, *autoStart)
}

// parseArchiveFlag splits --archive value into (srcInstance, backupName).
// Format: [<src-instance>:]<backup-name>
func parseArchiveFlag(archive string, instanceFlag, newInstanceFlag *string) (string, string) {
	colonIdx := strings.Index(archive, ":")
	if colonIdx == -1 {
		if *newInstanceFlag != "" {
			return "", ""
		}
		return *instanceFlag, archive
	}
	return archive[:colonIdx], archive[colonIdx+1:]
}
