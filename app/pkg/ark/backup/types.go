package backup

import "time"

// RetentionPolicy governs backup pruning. Exported version of the CLI's
// backupRetentionPolicy.
type RetentionPolicy struct {
	RetainCount     int
	RetainDays      int
	RetainSizeBytes int64
}

// NewRetentionPolicy creates a RetentionPolicy, converting size from GiB.
func NewRetentionPolicy(retainCount, retainDays, retainSizeGB int) RetentionPolicy {
	return RetentionPolicy{
		RetainCount:     retainCount,
		RetainDays:      retainDays,
		RetainSizeBytes: int64(retainSizeGB) * 1024 * 1024 * 1024,
	}
}

// CreateRequest describes a backup creation operation for a single service.
type CreateRequest struct {
	ComposeFile       string
	Service           string
	BackupRoot        string
	DestOverride      string
	NameOverride      string
	Format            string
	Timestamp         string
	PID               int
	LockTimeout       time.Duration
	BackupWaitTimeout time.Duration
	Retention         RetentionPolicy
	// LockHeld: the caller already holds the instance lock, so CreateForTarget does not take it.
	LockHeld bool
}

// CreateResult reports what happened during backup creation.
type CreateResult struct {
	ArchivePath string
}

// RestoreRequest describes a backup restore operation.
type RestoreRequest struct {
	ComposeFile string
	Service     string
	Archive     string
	BackupRoot  string
	AutoStart   bool
	// ArchivePath bypasses the Archive/BackupRoot lookup when non-empty.
	// When set, Restore uses this path directly as the input archive.
	ArchivePath string
	// LockHeld: the caller already holds the instance lock, so Restore does not take it.
	LockHeld bool
}

// VerifyRequest describes a backup verification operation.
type VerifyRequest struct {
	ArchivePath string
}

// PruneRequest describes a backup retention/prune operation.
type PruneRequest struct {
	BackupDir string
	Instance  string
	Policy    RetentionPolicy
	DryRun    bool
}
