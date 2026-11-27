package schedule

// ScheduleFile is the schedule list of one compose file (its x-ark-schedules block).
type ScheduleFile struct {
	Schedules []BackupSchedule
}

// BackupSchedule is a single schedule entry of any type: backup, rcon, save,
// restart or update. It keeps its historical name. store.go maps it to and from the compose file.
type BackupSchedule struct {
	Name         string
	Type         string
	Instance     string
	Enabled      bool
	Cron         string
	Command      string // rcon only
	Format       string
	RetainCount  int
	RetainDays   int
	RetainSizeGB int
}

// Schedule types.
const (
	TypeBackup  = "backup"
	TypeRCON    = "rcon"
	TypeSave    = "save"
	TypeRestart = "restart"
	TypeUpdate  = "update"
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
