# Runtime Call Graph

This document maps the execution flow of CLI commands and container runtime scripts.
Some flows start from a CLI handler (e.g., `create instance`); others start from
container entry points (e.g., startup). Each shows the call chain through domain
services to backend interfaces or container-side scripts.

## Startup + Shutdown Flow

```mermaid
flowchart TD
    A[container/dockerfile ENTRYPOINT] --> B[container-entrypoint.sh]
    B --> C{Running as root?}
    C -- yes --> D[exec setpriv ark-user python3 container-entrypoint.py]
    C -- no --> E[exec python3 container-entrypoint.py]
    D --> F[container-entrypoint.py main]
    E --> F

    F --> G{Command override args present?}
    G -- yes --> H[os.execvp provided command]
    G -- no --> I[run_default_startup]

    I --> J[check_memory_headroom]
    J --> J2[check_host_kernel_params]
    J2 --> K[check_volume_mounts]
    K --> L{GameUserSettings.ini exists?}
    L -- no --> M[install_default_settings: copy the baked default]
    L -- yes --> N[apply_rcon_settings from env]
    M --> N
    N --> R[exec ark-server-start.py]

    R --> S[start server via Proton]
    S --> T[tail ShooterGame.log]
    T --> U{SIGTERM or SIGINT?}
    U -- no --> V[wait for server exit]
    U -- yes --> W[run ark-server-stop.py]
    W --> X{graceful stop succeeded?}
    X -- yes --> Y[wait for process exit]
    X -- no --> Z[force kill process group]
    Y --> V
    Z --> V
    V --> AA[parse status + logs, choose exit code]
```

## Default GameUserSettings.ini (image build)

```mermaid
flowchart TD
    A[dockerfile stage ini_defaults: generate-default-ini.py] --> B[start ARK via server_command]
    B --> C{GameUserSettings.ini appears within 300 s?}
    C -- no --> X[exit 1: build fails]
    C -- yes --> D[SIGTERM the process group]
    D --> E{complete file: has ServerSettings, size settled, within 120 s?}
    E -- no --> X
    E -- yes --> F[SIGKILL leftovers]
    F --> G[strip SessionSettings SessionName]
    G --> H[write /home/ark-user/config-defaults/GameUserSettings.ini]
    H --> I[final stage copies only that file]
```

## Backup + Restore Invocation Paths

```mermaid
flowchart TD
    A[ark-cli create backup] --> B[ComposeService.Exec → python3 ark-backup.py --output ...]
    B --> C[validate extension tar/tar.gz/tar.zst]
    C --> D[tar archive ShooterGame/Saved]

    E[ark-cli restore backup] --> F[ComposeService.Run --rm --no-deps ... python3 ark-restore.py --archive ... --pre-restore ...]
    F --> G[list archive entries]
    G --> H[reject unsafe paths or link targets]
    H --> I[validate_archive_contents: SQLite integrity check on .ark files]
    I --> J[create pre-restore backup tar.gz]
    J --> K[extract archive into Saved/.restore-staging]
    K --> M{tar succeeded?}
    M -- no --> N[remove staging; existing saves unchanged]
    M -- yes --> L[swap_in: move old contents to .restore-previous, staged ones into Saved, delete previous]
```

## Create Instance Call Graph

`ark-cli create instance` generates a compose service definition from a template and
merges it into an existing `docker-compose.yaml`. It operates entirely on YAML text —
no Docker commands are issued.

```mermaid
flowchart TD
    A[ark-cli create instance] --> B[Parse flags + resolve inputs]
    B --> C[Assign ports: FindUsedPorts + PickNextPort]
    C --> D[RenderTemplate: substitute instance/map/password into YAML]
    D --> E{Compose file exists?}
    E -- no --> F[Write rendered text as new file]
    E -- yes --> G[EnsureServiceAbsent: reject if instance already in file]
    G --> H[ExtractServiceBlock from rendered YAML]
    H --> I[InsertService into existing services section]
    I --> J[EnsureVolumeEntry for saved-config volume]
    J --> K[EnsureVolumeEntry for cluster volume]
    K --> F2[Write merged compose file to disk]
```

## Update Instance Call Graph

`ark-cli update instance` compares each instance's image with Steam's current public
build, builds the new image if needed, and with `--rollout` switches to it safely.

```mermaid
flowchart TD
    A[ark-cli update instance] --> B[Load compose project]
    B --> C[Resolve target services]
    C --> D{Image repo is ark-server?}
    D -- no --> W[Warn: only locally built ark-server images are supported]
    D -- yes --> F[CheckLocalUpdate]
    F --> F1[InspectImageVersionInfo: read build ID label]
    F1 --> F2[FetchSteamLatestBuildID via HTTP]
    F2 --> G{Action?}
    G -- skip/warn or --check --> H[Print plan table, no changes]
    G -- update --> K[Build image: files image, base, labeled]
    K --> M{--rollout or --restart?}
    M -- no --> Q[Print: Built ark-server:id. Switch with --rollout]
    M -- yes --> N[Stop instance: saves world]
    N --> N1[Pre-update backup]
    N1 --> N2[SaveProject: compose to new tag]
    N2 --> O[Start new container]
    O --> P[WaitForReady: poll for advertising marker]
    P -- failed --> R[Compose back to old tag, restore backup, start old image]
    R --> R1[Record attempt, webhook, exit 1]
    P -- ready --> S[Record succeeded, cleanup old images]
```

## Reconfigure Instance Call Graph

`ark-cli reconfigure instance` modifies environment variables and port bindings in the
compose file, then optionally restarts the instance.

```mermaid
flowchart TD
    A[ark-cli reconfigure instance] --> B[Validate inputs]
    B --> C{--restart flag?}
    C -- yes --> D[Check service is running]
    C -- no --> E[Continue without status check]
    D --> E
    E --> F[Load compose project]
    F --> G[Guard: reject MAP_NAME change on existing instance]
    G --> H[Apply env var updates via SetEnv]
    H --> I[Apply port updates via MutateService]
    I --> J[Check for port conflicts]
    J --> K[SaveProject: write compose file]
    K --> L{--restart and was running?}
    L -- yes --> M[Stop instance via Lifecycle]
    M --> N[Start instance via Lifecycle]
    N --> O[WaitForReady]
    L -- no, changed --> P[Print restart-required message]
    L -- no, unchanged --> Q[Print no-changes message]
```

## Health Check / Wait Call Graph

`ark-cli check instance` takes a snapshot; `ark-cli wait instance --for <state>` polls
until a state is reached.

```mermaid
flowchart TD
    A[ark-cli check/wait instance] --> B[Resolve compose file + service config]
    B --> C{check or wait?}
    C -- check --> D[IsRunning: ComposeService.RunningServices]
    D --> E{Running?}
    E -- yes --> F[IsReady: LogsContainMarker with ReadyMarker]
    F --> G[Classify: healthy/starting/unhealthy]
    E -- no --> H[Result: stopped]
    C -- wait --> I[WaitForStarted/Ready/Stopped]
    I --> J[Poll loop: check at interval until timeout]
    J --> K{Each iteration}
    K --> L[IsRunning: ComposeService.RunningServices]
    L --> M{Running?}
    M -- yes --> N[LogsContainMarker: scan last 300 log lines]
    N --> O{Marker found?}
    O -- yes --> P[Return success]
    O -- no --> Q[Continue polling]
    M -- no --> R[CheckNotExited: ExitedServices]
    R --> S{Exited?}
    S -- yes --> T[Return error: premature exit]
    S -- no --> Q
```

## Host Kernel Config Call Graph

The `check/fix/restore host-kernel-config` commands inspect and patch host sysctl
parameters and NOFILE limits. They require root (fix/restore) and operate entirely on
the host — no Docker calls.

```mermaid
flowchart TD
    A{Command?} --> B[check host-kernel-config]
    A --> C[fix host-kernel-config]
    A --> D[restore host-kernel-config]

    B --> B1[Read /proc/sys/vm/max_map_count etc.]
    B1 --> B2[Read NOFILE hard limit via getrlimit]
    B2 --> B3[Scan /etc/sysctl.conf for conflicts]
    B3 --> B4[Print status table]

    C --> C1[RequireRoot]
    C1 --> C2[SnapshotKernelParams: read current values]
    C2 --> C3[WriteKernelParamBackup to ~/.config/ark-cli/]
    C3 --> C4[sysctl -w for each param below minimum]
    C4 --> C5[ApplyNofilePersistFiles: write limits.d + systemd override]
    C5 --> C6[Write /etc/sysctl.d/99-ark-server.conf]
    C6 --> C7[PatchSysctlConf: update /etc/sysctl.conf in-place]
    C7 --> C8[Print status table]

    D --> D1[RequireRoot]
    D1 --> D2[Read backup file from ~/.config/ark-cli/]
    D2 --> D3[sysctl -w to restore each backed-up value]
    D3 --> D4[Remove /etc/sysctl.d/99-ark-server.conf]
    D4 --> D5[Revert /etc/sysctl.conf from backup values]
    D5 --> D6[Remove limits.d + systemd override files]
    D6 --> D7[Print status table]
```

## Schedule Call Graph

Schedule commands manage backup schedules stored in
`.ark-manage/schedules.yaml`. `apply` generates crontab entries from the schedule file.

```mermaid
flowchart TD
    A[ark-cli schedule command] --> B{Verb?}

    B -- create/update --> C[Parse schedule flags + validate]
    C --> D[Load or create ScheduleFile]
    D --> E[Write .ark-manage/schedules.yaml]

    B -- apply --> F[Load ScheduleFile]
    F --> G[Filter enabled schedules]
    G --> H[Generate crontab entries per schedule]
    H --> I[Write .ark-manage/generated/schedules.crontab]
    I --> J[Install crontab via crontab command]

    B -- check --> K[Load ScheduleFile]
    K --> L[For each schedule: check last-run state file]
    L --> M[Print status table]

    B -- run --> N[Load ScheduleFile]
    N --> O[Find schedule by name]
    O --> P[Call backup service: BackupService.CreateForTarget]
    P --> Q[Write last-run state file]

    B -- enable/disable --> R[Load ScheduleFile]
    R --> S[Toggle enabled flag]
    S --> T[Write .ark-manage/schedules.yaml]
```

## INI Edit/Show Call Graph

`edit ini` and `show ini` use container exec or ephemeral containers to interact with
game config files inside the instance.

```mermaid
flowchart TD
    A[ark-cli edit/show ini] --> B[Resolve INI file spec]
    B --> C[Load compose project]
    C --> D{Instance running?}

    D -- yes, edit --> E[ExecInteractive: open editor in running container]
    D -- yes, show --> F[ExecInteractive: cat INI file in running container]
    D -- no, edit --> G[RunOneOffInteractive: start temp container with editor]
    D -- no, show --> H[RunOneOffInteractive: start temp container, cat INI file]

    E --> I{--restart flag?}
    I -- yes --> J[Stop + Start + WaitForReady]
    I -- no --> K[Done]
```

## Prune Backups Call Graph

`prune backups` operates entirely on the host filesystem — no Docker commands. It
applies retention policies to backup archives.

```mermaid
flowchart TD
    A[ark-cli prune backups] --> B[Parse retention flags: --retain-count/days/size-gb]
    B --> C{--dry-run?}

    C -- yes --> D[List archives in backup directory]
    D --> E[Compute retention: sort by newest, apply count/age/size rules]
    E --> F[Print what would be removed, no deletions]

    C -- no --> G[ApplyRetention]
    G --> H[ListRetentionCandidates: os.ReadDir]
    H --> I[SortByNewest: sort by timestamp]
    I --> J[DiskUsage: syscall.Statfs for free space check]
    J --> K[pruneByAge: mark backups older than retain-days]
    K --> L[pruneByCount: mark excess beyond retain-count with 2-backup floor]
    L --> M[pruneBySize: mark if total exceeds retain-size-gb]
    M --> N[pruneForFreeSpace: mark if disk usage too high]
    N --> O[removeRetainedBackups: os.Remove for each marked file]
```
