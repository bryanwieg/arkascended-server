package core

import (
	"encoding/json"
	"fmt"

	"pkg/ark/health"
)

// PrintJSONOutput marshals v as indented JSON and prints it to stdout.
func PrintJSONOutput(v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Errorf("Error: failed to marshal JSON: %w", err)
	}
	fmt.Println(string(b))
	return nil
}

type InstanceSummaryJSON struct {
	Name              string `json:"name"`
	Status            string `json:"status"`
	Map               string `json:"map,omitempty"`
	Version           string `json:"version,omitempty"`
	Port              int    `json:"asa_port,omitempty"`
	InternetReachable string `json:"internet_reachable,omitempty"`
	BackupScheduled   bool   `json:"backup_scheduled"`
	LastBackup        string `json:"last_backup,omitempty"`
}

type BackupEntryJSON struct {
	Name      string `json:"name"`
	SizeBytes int64  `json:"size_bytes"`
	Date      string `json:"date"`
}

type BackupEntryWithInstanceJSON struct {
	Instance  string `json:"instance"`
	Name      string `json:"name"`
	SizeBytes int64  `json:"size_bytes"`
	Date      string `json:"date"`
}

type ScheduleEntryJSON struct {
	Name     string `json:"name"`
	Enabled  bool   `json:"enabled"`
	Type     string `json:"type"`
	Instance string `json:"instance"`
	Cron     string `json:"cron"`
}

type VolumeInspectJSON struct {
	Volume          string `json:"volume"`
	DockerVolume    string `json:"docker_volume"`
	ContainerTarget string `json:"container_target"`
	HostPath        string `json:"host_path"`
}

type PortStatusJSON struct {
	Port      int    `json:"port"`
	Localhost string `json:"localhost"`
	Internet  string `json:"internet"`
}

type InstanceInspectJSON struct {
	Name                string              `json:"name"`
	Status              string              `json:"status"`
	Image               string              `json:"image,omitempty"`
	Version             string              `json:"version,omitempty"`
	Map                 string              `json:"map,omitempty"`
	Session             string              `json:"session,omitempty"`
	ASAPort             int                 `json:"asa_port,omitempty"`
	ComposeFile         string              `json:"compose_file,omitempty"`
	Volumes             []VolumeInspectJSON `json:"volumes"`
	PortStatus          PortStatusJSON      `json:"port_status"`
	Schedules           []ScheduleEntryJSON `json:"schedules"`
	BackupDir           string              `json:"backup_dir,omitempty"`
	Backups             []BackupEntryJSON   `json:"backups"`
	GameIni             string              `json:"game_ini,omitempty"`
	GameUserSettingsIni string              `json:"game_user_settings_ini,omitempty"`
}

type BackupDetailJSON struct {
	Instance        string `json:"instance"`
	Name            string `json:"name"`
	Format          string `json:"format"`
	Path            string `json:"path"`
	SizeBytes       int64  `json:"size_bytes"`
	ModifiedUTC     string `json:"modified_utc"`
	TimestampHint   string `json:"timestamp_hint,omitempty"`
	ServerVersion   string `json:"server_version,omitempty"`
	BuildID         string `json:"build_id,omitempty"`
	InstanceName    string `json:"instance_name,omitempty"`
	MapName         string `json:"map_name,omitempty"`
	BackupTimestamp string `json:"backup_timestamp,omitempty"`
}

type ScheduleDetailJSON struct {
	Name           string `json:"name"`
	Enabled        bool   `json:"enabled"`
	Type           string `json:"type"`
	Instance       string `json:"instance"`
	Cron           string `json:"cron"`
	Command        string `json:"command,omitempty"`
	Format         string `json:"format,omitempty"`
	RetainCount    int    `json:"retain_count,omitempty"`
	RetainDays     int    `json:"retain_days,omitempty"`
	RetainSizeGB   int    `json:"retain_size_gb,omitempty"`
	ComposeFile    string `json:"compose_file,omitempty"`
	LastStatus     string `json:"last_status,omitempty"`
	LastExitCode   int    `json:"last_exit_code,omitempty"`
	LastStartedAt  string `json:"last_started_at,omitempty"`
	LastFinishedAt string `json:"last_finished_at,omitempty"`
	LastError      string `json:"last_error,omitempty"`
}

// PlayerJSON is one connected player from RCON ListPlayers.
type PlayerJSON struct {
	Name string `json:"name"`
	ID   string `json:"id"`
}

// InstancePlayersJSON is `list players --json` output for one instance.
type InstancePlayersJSON struct {
	Instance string       `json:"instance"`
	Players  []PlayerJSON `json:"players"`
	Error    string       `json:"error,omitempty"`
}

// IniKeyLineJSON is one line of an INI key in `get ini --json`.
type IniKeyLineJSON struct {
	Line   int    `json:"line"`
	Value  string `json:"value"`
	Origin string `json:"origin"`
}

// IniKeyJSON is `get ini --json` output (ark-ini.py get's reply): every line for the key.
type IniKeyJSON struct {
	File    string           `json:"file"`
	Section string           `json:"section"`
	Key     string           `json:"key"`
	Lines   []IniKeyLineJSON `json:"lines"`
	History string           `json:"history"`
}

// IniRevisionJSON is one revision of an instance's INI history (ark-ini.py history's revision).
type IniRevisionJSON struct {
	Revision string   `json:"revision"`
	Date     string   `json:"date"`
	By       string   `json:"by"`
	How      string   `json:"how"`
	Author   string   `json:"author"`
	Files    []string `json:"files"`
	Message  string   `json:"message"`
	Patch    string   `json:"patch,omitempty"`
}

// IniHistoryJSON is `list ini-history --json` output (ark-ini.py history's reply), newest first.
type IniHistoryJSON struct {
	Revisions []IniRevisionJSON `json:"revisions"`
	More      bool              `json:"more"`
}

// SchedulerHealthJSON is `check scheduler --json` output: the monitor's report as written to health.json.
type SchedulerHealthJSON = health.Report
