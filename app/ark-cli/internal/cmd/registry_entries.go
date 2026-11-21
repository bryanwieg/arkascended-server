package cmd

import (
	"ark-cli/internal/backup"
	"ark-cli/internal/compose"
	"ark-cli/internal/core"
	"ark-cli/internal/create"
	"ark-cli/internal/health"
	"ark-cli/internal/host"
	"ark-cli/internal/ini"
	"ark-cli/internal/listenport"
	"ark-cli/internal/reachability"
	"ark-cli/internal/reconfigure"
	"ark-cli/internal/remove"
	"ark-cli/internal/schedule"
	"ark-cli/internal/scheduler"
	"ark-cli/internal/show"
	"ark-cli/internal/update"
)

type CommandRegistryEntry struct {
	Verb         string
	Object       string
	Route        string
	Handler      func(*core.AppDeps, []string) error
	TopLevelHelp []string
}

type CommandRouteParseSpec struct {
	MissingErr string
	WrongFmt   string
}

var CommandRegistry = []CommandRegistryEntry{
	{Verb: "create", Object: "instance", Route: "create", Handler: create.RunCreate, TopLevelHelp: []string{"create instance --name <name>               Create or extend a docker-compose.yaml with an ARK server"}},
	{Verb: "create", Object: "backup", Route: "backup", Handler: backup.RunBackup, TopLevelHelp: []string{"create backup --instance <inst|cluster|all>  Create backup archives"}},
	{Verb: "create", Object: "schedule", Route: "create_schedule", Handler: schedule.RunCreateSchedule, TopLevelHelp: []string{"create schedule backup --instance <inst> --name <name>  Create a managed backup schedule"}},
	{Verb: "edit", Object: "ini", Route: "edit_ini", Handler: ini.RunEditIni, TopLevelHelp: []string{"edit ini --instance <inst> --file <ini>     Edit Game.ini or GameUserSettings.ini in the container"}},
	{Verb: "get", Object: "ini", Route: "get_ini", Handler: ini.RunGetIni, TopLevelHelp: []string{"get ini --instance <inst> --file <ini> --section <s> --key <k>  Print one INI key's value"}},
	{Verb: "set", Object: "ini", Route: "set_ini", Handler: ini.RunSetIni, TopLevelHelp: []string{"set ini --instance <inst> --file <ini> --section <s> --key <k> --value <v>  Set one INI key without an editor"}},
	{Verb: "show", Object: "ini", Route: "show_ini", Handler: ini.RunShowIni, TopLevelHelp: []string{"show ini --instance <inst> --file <ini>     Print Game.ini or GameUserSettings.ini to the terminal"}},
	{Verb: "reconfigure", Object: "instance", Route: "reconfigure", Handler: reconfigure.RunReconfigure, TopLevelHelp: []string{"reconfigure instance --instance <inst>      Update selected instance compose settings"}},
	{Verb: "update", Object: "instance", Route: "update", Handler: update.RunUpdate, TopLevelHelp: []string{"update instance [--instance <inst|all>]     Check/apply ARK image updates (local or remote)"}},
	{Verb: "update", Object: "schedule", Route: "update_schedule", Handler: schedule.RunUpdateSchedule, TopLevelHelp: []string{"update schedule --name <name>               Update a managed schedule"}},
	{Verb: "enable", Object: "schedule", Route: "enable_schedule", Handler: schedule.RunEnableSchedule, TopLevelHelp: []string{"enable schedule --name <name>               Enable a managed schedule"}},
	{Verb: "disable", Object: "schedule", Route: "disable_schedule", Handler: schedule.RunDisableSchedule, TopLevelHelp: []string{"disable schedule --name <name>              Disable a managed schedule"}},
	{Verb: "enable", Object: "remote-rcon", Route: "enable_remote_rcon", Handler: reconfigure.RunEnableRemoteRCON, TopLevelHelp: []string{"enable remote-rcon --instance <inst>        Publish RCON on all host interfaces"}},
	{Verb: "disable", Object: "remote-rcon", Route: "disable_remote_rcon", Handler: reconfigure.RunDisableRemoteRCON, TopLevelHelp: []string{"disable remote-rcon --instance <inst>       Publish RCON on 127.0.0.1 only (default)"}},
	{Verb: "apply", Object: "schedules", Route: "apply_schedules", Handler: schedule.RunApplySchedules, TopLevelHelp: []string{"apply schedules                             Install the generated managed crontab"}},
	{Verb: "start", Object: "instance", Route: "start", Handler: compose.RunStart, TopLevelHelp: []string{"start instance --instance <inst|all>        Start compose service(s)"}},
	{Verb: "stop", Object: "instance", Route: "stop", Handler: compose.RunStop, TopLevelHelp: []string{"stop instance --instance <inst|all>         Graceful stop via ark-server-stop.py"}},
	{Verb: "restart", Object: "instance", Route: "restart", Handler: compose.RunRestart, TopLevelHelp: []string{"restart instance --instance <inst|all>      Restart instance(s) (stop then start)"}},
	{Verb: "save", Object: "instance", Route: "save", Handler: compose.RunSave, TopLevelHelp: []string{"save instance --instance <inst|all>         Trigger RCON saveworld"}},
	{Verb: "logs", Object: "instance", Route: "logs", Handler: compose.RunLogs, TopLevelHelp: []string{"logs instance --instance <inst>             Show container logs for an instance"}},
	{Verb: "wait", Object: "instance", Route: "wait", Handler: health.RunWait, TopLevelHelp: []string{"wait instance --instance <inst> --for <state>  Wait for started, advertising (ready) or stopped"}},
	{Verb: "validate", Object: "backup", Route: "validate_backup", Handler: backup.RunVerifyBackup, TopLevelHelp: []string{"validate backup --instance <inst> --backup <backup>  Validate SQLite integrity inside a backup archive"}},
	{Verb: "prune", Object: "backups", Route: "prune_backups", Handler: backup.RunPruneBackups, TopLevelHelp: []string{"prune backups --instance <inst|cluster|all>  Apply retention policies without creating backups"}},
	{Verb: "restore", Object: "backup", Route: "restore", Handler: backup.RunRestore, TopLevelHelp: []string{
		"restore backup --instance <inst> --archive <archive>  Restore to an existing instance from a backup archive",
		"restore backup --new-instance <inst> --archive <archive>  Create a new instance and restore from a backup archive",
		"restore backup --instance cluster --archive <archive>  Restore the shared cluster data from a backup archive",
	}},
	{Verb: "show", Object: "instance", Route: "show_instance", Handler: show.RunShowInstance, TopLevelHelp: []string{"show instance --instance <inst>             Show instance status and config summary"}},
	{Verb: "show", Object: "backup", Route: "show_backup", Handler: show.RunShowBackup, TopLevelHelp: []string{"show backup --instance <inst> --backup <backup>  Show backup archive metadata"}},
	{Verb: "show", Object: "compose", Route: "show_compose", Handler: show.RunShowCompose, TopLevelHelp: []string{"show compose                                Show compose configuration summary"}},
	{Verb: "show", Object: "volume-paths", Route: "show_volume_paths", Handler: show.RunShowVolumePaths, TopLevelHelp: []string{"show volume-paths --instance <inst>         Show volume mount paths for an instance"}},
	{Verb: "show", Object: "vol-paths", Route: "show_vol_paths_compat", Handler: show.RunShowVolumePaths, TopLevelHelp: nil},
	{Verb: "show", Object: "schedule", Route: "show_schedule", Handler: schedule.RunShowSchedule, TopLevelHelp: []string{"show schedule --name <name>                 Show managed schedule details"}},
	{Verb: "run", Object: "schedule", Route: "run_schedule", Handler: schedule.RunRunSchedule, TopLevelHelp: []string{"run schedule --name <name>                  Run a managed schedule immediately"}},
	{Verb: "run", Object: "scheduler", Route: "run_scheduler", Handler: scheduler.RunRunScheduler, TopLevelHelp: []string{"run scheduler                               Run schedules and health checks (scheduler service)"}},
	{Verb: "check", Object: "host-kernel-config", Route: "check_host_kernel_config", Handler: host.RunCheckHostKernelConfig, TopLevelHelp: []string{"check host-kernel-config                    Check host kernel settings required for Proton/Wine"}},
	{Verb: "fix", Object: "host-kernel-config", Route: "fix_host_kernel_config", Handler: host.RunFixHostKernelConfig, TopLevelHelp: []string{"fix host-kernel-config                      Apply and persist required host kernel settings (requires root)"}},
	{Verb: "restore", Object: "host-kernel-config", Route: "restore_host_kernel_config", Handler: host.RunRestoreHostKernelConfig, TopLevelHelp: []string{"restore host-kernel-config                  Restore host kernel settings from pre-fix backup (requires root)"}},
	{Verb: "check", Object: "reachability", Route: "check_reachability", Handler: reachability.RunCheckReachability, TopLevelHelp: []string{
		"check reachability --port <num>             Check port reachability (use --protocol tcp|udp|both)",
		"check reachability --instance <inst>        Check instance port reachability (reads ports from compose)",
	}},
	{Verb: "listen", Object: "port", Route: "listen_port", Handler: listenport.RunListenPort, TopLevelHelp: []string{
		"listen port --port <num>                    Open listeners on a port and wait (no probe; for firewall testing)",
	}},
	{Verb: "check", Object: "instance", Route: "check_instance", Handler: health.RunCheckInstance, TopLevelHelp: []string{"check instance --instance <inst>            Check instance health (running + ready)"}},
	{Verb: "check", Object: "schedule", Route: "check_schedule", Handler: schedule.RunCheckSchedule, TopLevelHelp: []string{"check schedule --name <name>                Check schedule running status and last result"}},
	{Verb: "list", Object: "instances", Route: "list_instances", Handler: show.RunListInstances, TopLevelHelp: []string{"list instances                              List instances in the compose project"}},
	{Verb: "list", Object: "backups", Route: "list_backups", Handler: show.RunListBackups, TopLevelHelp: []string{"list backups --instance <inst|cluster|all>   List backups"}},
	{Verb: "list", Object: "players", Route: "list_players", Handler: compose.RunListPlayers, TopLevelHelp: []string{"list players --instance <inst|all>          List players connected to running instances"}},
	{Verb: "list", Object: "schedules", Route: "list_schedules", Handler: schedule.RunListSchedules, TopLevelHelp: []string{"list schedules                              List configured schedules"}},
	{Verb: "teardown", Object: "instances", Route: "teardown_instances", Handler: remove.RunTeardown, TopLevelHelp: []string{"teardown instances                          Tear down the full compose project (compose down)"}},
	{Verb: "remove", Object: "instance", Route: "remove", Handler: remove.RunRemove, TopLevelHelp: []string{"remove instance --instance <inst>           Remove one instance from compose and delete its saved volume"}},
	{Verb: "remove", Object: "schedule", Route: "remove_schedule", Handler: schedule.RunRemoveSchedule, TopLevelHelp: []string{"remove schedule --name <name>               Remove a managed schedule"}},
	{Verb: "validate", Object: "compose", Route: "validate_compose", Handler: health.RunValidateCompose, TopLevelHelp: []string{"validate compose                            Validate compose configuration"}},
	{Verb: "validate", Object: "schedule", Route: "validate_schedule", Handler: schedule.RunValidateSchedule, TopLevelHelp: []string{"validate schedule [--name <name>]           Validate one or all schedules"}},
}
