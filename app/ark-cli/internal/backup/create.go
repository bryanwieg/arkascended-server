package backup

import (
	"errors"
	"os"
	"time"

	"ark-cli/internal/compose"
	"ark-cli/internal/core"

	"pkg/ark"
	pkgbackup "pkg/ark/backup"
)

type BackupCreateExecution struct {
	composeFile       string
	config            *ark.ComposeProject
	backupRoot        string
	destOverride      string
	nameOverride      string
	format            string
	timestamp         string
	pid               int
	lockTimeout       time.Duration
	backupWaitTimeout time.Duration
	retentionPolicy   pkgbackup.RetentionPolicy
}

type BackupCreateCommandRequest struct {
	Target            string
	Dest              string
	Name              string
	Format            string
	RetainCount       int
	RetainDays        int
	RetainSizeGB      int
	LockTimeout       time.Duration
	BackupWaitTimeout time.Duration
	ComposeFileFlag   string
}

func validateNonNegativeIntFlag(flagName string, value int) error {
	if value >= 0 {
		return nil
	}
	return errors.New("Error: " + flagName + " must be >= 0.")
}

func RunBackup(d *core.AppDeps, args []string) error {
	req, err := ParseBackupCreateCommandRequest(args)
	if err != nil {
		return err
	}
	return RunBackupCreateCommand(req)
}

// RunBackupFromScheduleImpl is the default implementation for the RunBackupFromSchedule seam.
func RunBackupFromScheduleImpl(name string, args []string) error {
	req, err := ParseBackupCreateCommandRequest(args)
	if err != nil {
		return err
	}
	return RunBackupCreateCommand(req)
}

func ParseBackupCreateCommandRequest(args []string) (BackupCreateCommandRequest, error) {
	fs := core.NewCommandFlagSet(
		"backup",
		[]string{"create backup --instance <instance|cluster|all> [options]"},
		"Create backup archives for one instance, the shared cluster data, or all of them.",
	)
	instanceFlag := fs.String("instance", "", "Target instance name, 'cluster' (shared cluster data) or 'all'.")
	dest := fs.String("dest", "", "Destination directory for backup archives (default: <compose-dir>/backups/<instance>, or $ARK_BACKUP_DIR/<instance>).")
	name := fs.String("name", "", "Backup name prefix.")
	format := fs.String("format", "tar.gz", "Archive format.")
	retainCount := fs.Int("retain-count", 20, "Max backups to keep per instance (0 disables count-based pruning).")
	retainDays := fs.Int("retain-days", 7, "Max age in days for backups (0 disables age-based pruning).")
	retainSizeGB := fs.Int("retain-size-gb", 5, "Max total backup size per instance in GiB (0 disables size-based pruning).")
	lockTimeout := fs.Duration("lock-timeout", 2*time.Minute, "How long to wait for per-instance backup lock.")
	backupWaitTimeout := fs.Duration("backup-wait-timeout", 10*time.Minute, "How long to wait for backup archive completion in container/helper mode.")
	composeFileFlag := fs.String("compose-file", "", "Path to docker-compose.yaml (default: ./docker-compose.yaml or COMPOSE_FILE).")
	if err := core.ParseNoPositionalArgs(fs, args); err != nil {
		return BackupCreateCommandRequest{}, err
	}
	if *instanceFlag == "" {
		return BackupCreateCommandRequest{}, errors.New("Error: --instance is required for backups.")
	}
	if err := pkgbackup.ValidateFormat(*format); err != nil {
		return BackupCreateCommandRequest{}, err
	}
	if err := validateNonNegativeIntFlag("retain-count", *retainCount); err != nil {
		return BackupCreateCommandRequest{}, err
	}
	if err := validateNonNegativeIntFlag("retain-days", *retainDays); err != nil {
		return BackupCreateCommandRequest{}, err
	}
	if err := validateNonNegativeIntFlag("retain-size-gb", *retainSizeGB); err != nil {
		return BackupCreateCommandRequest{}, err
	}
	if *lockTimeout < 0 {
		return BackupCreateCommandRequest{}, errors.New("Error: lock-timeout must be >= 0.")
	}
	if *backupWaitTimeout <= 0 {
		return BackupCreateCommandRequest{}, errors.New("Error: backup-wait-timeout must be > 0.")
	}
	return BackupCreateCommandRequest{
		Target:            *instanceFlag,
		Dest:              *dest,
		Name:              *name,
		Format:            *format,
		RetainCount:       *retainCount,
		RetainDays:        *retainDays,
		RetainSizeGB:      *retainSizeGB,
		LockTimeout:       *lockTimeout,
		BackupWaitTimeout: *backupWaitTimeout,
		ComposeFileFlag:   *composeFileFlag,
	}, nil
}

func RunBackupCreateCommand(req BackupCreateCommandRequest) error {
	composeFile, config, err := compose.ResolveRequiredComposeConfig(core.CurrentAppDeps(), req.ComposeFileFlag)
	if err != nil {
		return err
	}

	backupRoot := core.BackupRootDirForComposeFile(composeFile)
	execCtx := BackupCreateExecution{
		composeFile:       composeFile,
		config:            config,
		backupRoot:        backupRoot,
		destOverride:      req.Dest,
		nameOverride:      req.Name,
		format:            req.Format,
		timestamp:         time.Now().UTC().Format("20060102-150405"),
		pid:               os.Getpid(),
		lockTimeout:       req.LockTimeout,
		backupWaitTimeout: req.BackupWaitTimeout,
		retentionPolicy:   pkgbackup.NewRetentionPolicy(req.RetainCount, req.RetainDays, req.RetainSizeGB),
	}

	if req.Target == "all" {
		services := config.ServiceNames()
		if len(services) == 0 {
			return errors.New("Error: compose config has no services to back up.")
		}
		// The cluster joins 'all' whenever the compose file has one; other cluster errors show as its failed row.
		if _, err := pkgbackup.ResolveClusterVolume(config); !errors.Is(err, pkgbackup.ErrNoClusterVolume) {
			services = append(services, pkgbackup.ClusterTarget)
		}
		return runBackupsForAllServices(services, execCtx.runTarget)
	}

	return execCtx.runTarget(req.Target)
}

func runBackupsForAllServices(services []string, createBackup func(string) error) error {
	results := make([]struct{ service, detail string }, 0, len(services))
	failures := 0
	for _, service := range services {
		if err := createBackup(service); err != nil {
			message := err.Error()
			if message == "" {
				message = "failed"
			}
			results = append(results, struct{ service, detail string }{service: service, detail: message})
			failures++
			continue
		}
		results = append(results, struct{ service, detail string }{service: service})
	}
	for _, result := range results {
		if result.detail == "" {
			core.Infof("Backup result %s: OK\n", result.service)
		} else {
			core.Warnf("Backup result %s: FAILED (%s)\n", result.service, result.detail)
		}
	}
	if failures > 0 {
		return errors.New("Error: one or more backups failed.")
	}
	return nil
}
