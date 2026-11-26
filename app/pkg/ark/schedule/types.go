package schedule

// ScheduleFile represents the on-disk YAML schedule configuration.
type ScheduleFile struct {
	Version   int              `yaml:"version"`
	Schedules []BackupSchedule `yaml:"schedules"`
}

// BackupSchedule is a single backup schedule entry (v1 format).
type BackupSchedule struct {
	Name         string `yaml:"name"`
	Type         string `yaml:"type"`
	Instance     string `yaml:"instance"`
	Enabled      bool   `yaml:"enabled"`
	Cron         string `yaml:"cron"`
	Command      string `yaml:"command,omitempty"` // rcon only
	Format       string `yaml:"format,omitempty"`
	RetainCount  int    `yaml:"retain_count,omitempty"`
	RetainDays   int    `yaml:"retain_days,omitempty"`
	RetainSizeGB int    `yaml:"retain_size_gb,omitempty"`
	ComposeFile  string `yaml:"compose_file,omitempty"`
}

// Schedule types.
const (
	TypeBackup  = "backup"
	TypeRCON    = "rcon"
	TypeSave    = "save"
	TypeRestart = "restart"
)

// RunState tracks the last execution status of a schedule.
type RunState struct {
	Name           string `json:"name"`
	LastStartedAt  string `json:"last_started_at,omitempty"`
	LastFinishedAt string `json:"last_finished_at,omitempty"`
	LastStatus     string `json:"last_status,omitempty"`
	LastExitCode   int    `json:"last_exit_code,omitempty"`
	LastError      string `json:"last_error,omitempty"`
}
