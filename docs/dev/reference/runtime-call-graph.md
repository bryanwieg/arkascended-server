# Runtime Call Graph

Call order for container entry points and CLI commands, down to the domain
services (`app/pkg/ark/*`) and container scripts (`container/scripts/`).

## Startup + Shutdown Flow

`container-entrypoint.sh` → `container-entrypoint.py` → `ark-server-start.py`
(all in `container/scripts/runtime/`).

```mermaid
flowchart TD
    A[ENTRYPOINT container-entrypoint.sh] --> C{Running as root?}
    C -->|"yes"| D[exec setpriv as ark-user: python3.14 container-entrypoint.py]
    C -->|"no"| E[exec python3.14 container-entrypoint.py]
    D --> F[main]
    E --> F

    F --> G{Command override args?}
    G -->|"yes"| H[os.execvp the command; sets ARK_MAINTENANCE_MODE=restore if it runs ark-restore.py]
    G -->|"no"| I[run_default_startup]

    I --> J[check_memory_headroom: warn only]
    J --> J2[check_host_kernel_params: warn only]
    J2 --> K{check_volume_mounts}
    K -->|"fail"| X[exit 1]
    K -->|"ok"| M[install_default_settings: copy baked default if GameUserSettings.ini missing]
    M --> N[apply_rcon_settings from env]
    M -->|"fail"| X
    N -->|"fail"| X
    N --> R[os.execv ark-server-start.py]

    R --> R1[rotate_runtime_logs, clear stale stop status file]
    R1 --> S[start server_command via Proton, new session]
    S --> T[tail -F ShooterGame.log once it exists]
    T --> U{SIGTERM or SIGINT?}
    U -->|"no"| V[server exits]
    U -->|"yes"| W[run_graceful_stop_script: ark-server-stop.py]
    W --> X2{script ok and server exits in time?}
    X2 -->|"yes"| V
    X2 -->|"no"| Z[killpg SIGTERM, then SIGKILL]
    Z --> V
    V --> AA[report_oom_kill, print crash log, parse stop status file, choose exit code]
```

## Default GameUserSettings.ini (image build)

`container/scripts/installer/generate-default-ini.py`, run in the `ini_defaults`
stage of `container/dockerfile`.

```mermaid
flowchart TD
    A[stage ini_defaults: generate-default-ini.py] --> B[start ARK via server_command]
    B --> C{GameUserSettings.ini appears within 300 s?}
    C -->|"no"| X[exit 1: build fails]
    C -->|"yes"| D[SIGTERM the process group]
    D --> E{complete file: has ServerSettings, size settled, within 120 s?}
    E -->|"no"| X
    E -->|"yes"| F[SIGKILL leftovers]
    F --> G[strip SessionSettings SessionName]
    G --> H[write /home/ark-user/config-defaults/GameUserSettings.ini]
    H --> I[final stage copies only that file]
```

## Backup + Restore Invocation Paths

Instance targets. Host side: `app/pkg/ark/backup/create.go`, `restore.go`.
Container side: `ark-backup.py` (`ark_backup/`), `ark-restore.py`.

```mermaid
flowchart TD
    A[ark-cli create backup] --> A1[BackupService.CreateForTarget: lock backups/instance]
    A1 --> A2{instance running?}
    A2 -->|"yes"| B[ComposeService.Exec → python3.14 ark-backup.py --output maintenance/...]
    A2 -->|"no"| B2[ComposeService.RunOneOffDetached: same script in a helper container, then touch .ready]
    B --> C[archive_args: validate extension tar/tar.gz/tar.zst]
    B2 --> C
    C --> D[pre_backup_save: RCON saveworld + wait for save marker, skipped if server not running]
    D --> D1[write metadata, tar Saved excluding Logs and clusters]
    D1 --> D2[validate_archive_contents: SQLite check of SavedArks .ark files]
    D2 -->|"fail, retries left"| D
    D2 -->|"ok"| D3[host: copy archive out, ApplyRetention]

    E[ark-cli restore backup] --> E1[BackupService.Restore: refuse if instance running]
    E1 --> F[ComposeService.RunOneOff as uid 3000 → python3.14 ark-restore.py --archive ... --pre-restore ...]
    F --> F1[require ARK_MAINTENANCE_MODE=restore and no server process]
    F1 --> H[list entries, reject unsafe paths]
    H --> I[validate_archive_contents]
    I --> I1[swap_blocker check; remove stale Saved/.restore-staging]
    I1 --> J[pre-restore tar.gz of Saved, excluding clusters]
    J --> K[extract archive into Saved/.restore-staging, excluding clusters]
    K --> M{tar succeeded?}
    M -->|"no"| N[remove staging; existing saves unchanged]
    M -->|"yes"| L[swap_in: old contents to .restore-previous, staged ones into Saved, delete previous]
    L --> L1[host: check pre-restore archive exists; --auto-start → ComposeService.Up]
```

### Cluster backup + restore (`--instance cluster`)

The shared cluster volume is found by its mount at `Saved/clusters`
(`pkgbackup.ResolveClusterVolume`). Both paths run in a removed one-off container of
the first instance that mounts it, as uid 3000, under the `cluster` backup lock.

```mermaid
flowchart TD
    A[ark-cli create backup --instance cluster] --> B[ResolveClusterVolume]
    B --> C[BackupService.CreateCluster: lock backups/cluster]
    C --> D[ComposeService.RunOneOff → python3.14 ark-cluster-backup.py --output maintenance/output/...]
    D --> E{tar exit}
    E -->|"1: files changed, under 3 attempts"| D2[retry after 2s]
    D2 --> E
    E -->|"0"| F[tar -tf the .partial archive, rename into place]
    E -->|"other / 3rd change"| X[fail, no archive left]
    F --> G[ApplyRetention on backups/cluster]

    R[ark-cli restore backup --instance cluster] --> S[ResolveClusterVolume]
    S --> V[BackupService.RestoreCluster]
    V --> T{any instance using the cluster running?}
    T -->|"yes"| U[refuse, naming them]
    T -->|"no"| V2[lock, no wait]
    V2 --> W[ComposeService.RunOneOff → python3.14 ark-restore.py --archive ... --pre-restore ... --cluster]
    W --> Y[unsafe-path check, no SQLite check, pre-restore tar.gz of clusters/]
    Y --> Z[extract into clusters/.restore-staging, swap_in with nothing kept]
```

## Create Instance Call Graph

Text-level YAML edit (`app/ark-cli/internal/create/create.go`); no container
engine calls. Writes the file directly, not through `SaveProject`.

```mermaid
flowchart TD
    A[ark-cli create instance] --> A1[Read existing compose file, reject if instance exists]
    A1 --> B[Parse flags + resolve inputs]
    B --> C[Assign ports: FindUsedPorts + PickNextPort]
    C --> D[RenderTemplate: substitute instance/map/password into YAML]
    D --> E{Compose file exists?}
    E -->|"no"| F[Write rendered text as new file]
    E -->|"yes"| G[EnsureServiceAbsent]
    G --> H[ExtractServiceBlock from rendered YAML]
    H --> I[InsertService into existing services section]
    I --> J[EnsureVolumeEntry for saved-config volume]
    J --> K[EnsureVolumeEntry for cluster volume]
    K --> F2[Write merged compose file to disk]
```

## Update Instance Call Graph

CLI: `app/ark-cli/internal/update/`. Domain: `app/pkg/ark/update/` (`local.go`,
`build.go`, `rollout.go`). `--restart` and `--rollout` are aliases.

```mermaid
flowchart TD
    A[ark-cli update instance] --> B[LoadProject]
    B --> C[Resolve target services]
    C --> D{Image repo is ark-server?}
    D -->|"no"| W[Row: warn, only locally built ark-server images are supported]
    D -->|"yes"| F[CheckLocalUpdate]
    F --> F1[InspectImageVersionInfo: build ID + recipe labels]
    F1 --> F2[FetchSteamLatestBuildID]
    F2 --> G{build ID or recipe differs?}
    G -->|"no"| H[Row: skip]
    G -->|"yes, not --check"| K[BuildLocalImage once for all pending; failure recorded as build_failed]
    K --> P0[Print plan table]
    H --> P0
    W --> P0
    P0 --> M{--check / --rollout?}
    M -->|"--check"| Q0[Done, no changes]
    M -->|"neither"| Q[Print: Built ... Switch with --rollout; RemoveOldImages]
    M -->|"--rollout"| R0[update.Rollout per instance, sorted]

    R0 --> N[Stop: graceful, saves world]
    N --> N1[Backup: CreateForTarget, name pre-update]
    N1 --> N2[setPrevious: record rollback image in state]
    N1 -->|"backup or state failed"| RB0[StartAndWait old image; record rolled_back]
    N2 --> N3[SetImage: LoadProject, MutateService, SaveProject to new tag]
    N3 --> O[StartAndWait: start, wait for ready; start removes unused images]
    O -->|"ready"| S[record succeeded]
    O -->|"failed"| R[SetImage old tag, Stop, Restore backup, StartAndWait]
    R --> R1[record rolled_back or rollback_failed]
    RB0 --> R2
    R1 --> R2[CLI: alertFailure webhook; stop, skip remaining instances, exit 1]
```

## Reconfigure Instance Call Graph

```mermaid
flowchart TD
    A[ark-cli reconfigure instance] --> B[Validate inputs]
    B --> C[ServiceRunningStatus]
    C --> D{running and no --force?}
    D -->|"yes"| X[refuse]
    D -->|"no"| F[LoadProject]
    F --> G[guardReconfigureMapName: reject MAP_NAME change]
    G --> H[Env updates via SetEnv in MutateService]
    H --> I[Port updates: checkPortConflict, then MutateService]
    I --> K{changed?}
    K -->|"yes"| K1[SaveProject]
    K -->|"no"| K2[Print no-changes message]
    K1 --> L{--restart?}
    K2 --> L
    L -->|"yes, was running"| M[DoRestartStopInstance, DoRestartStartInstance with ready wait]
    L -->|"yes, was stopped"| M2[Print: not started]
    L -->|"no, changed"| P[Print restart-required message]
```

## Health Check / Wait Call Graph

`app/ark-cli/internal/health/health.go` → `app/pkg/ark/health/checker.go`.

```mermaid
flowchart TD
    A[ark-cli check/wait instance] --> B[Resolve compose file + service config]
    B --> C{check or wait?}
    C -->|"check"| D[RuntimeHelper.BuildInstanceSummary: status]
    D --> E{running?}
    E -->|"yes"| F[IsReady: LogsContainMarker with ReadyMarker]
    F --> G[healthy / starting]
    E -->|"no"| H[stopped / unhealthy]
    G --> G1[printLastUpdate from update state]
    H --> G1
    C -->|"wait"| I{--for}
    I -->|"started / ready, advertising"| J[waitForLogMarker: poll until timeout]
    J --> L[IsRunning: ComposeService.RunningServices]
    L --> M{running?}
    M -->|"yes"| N[LogsContainMarker: last 300 log lines]
    N -->|"found"| P[success]
    N -->|"not found"| J
    M -->|"no"| R[CheckNotExited: ExitedServices]
    R -->|"exited"| T[error: premature exit]
    R -->|"not exited"| J
    I -->|"stopped"| WS[WaitForStopped: poll IsRunning until false]
```

## Host Kernel Config Call Graph

`app/ark-cli/internal/host/host.go` → `app/pkg/ark/host/kernel_config.go`. Host
only; no container calls.

```mermaid
flowchart TD
    A{Command?} --> B[check host-kernel-config]
    A --> C[fix host-kernel-config]
    A --> D[restore host-kernel-config]

    B --> B1[CheckOneParam: read /proc/sys values]
    B1 --> B2[CheckNofileLimit: getrlimit NOFILE hard]
    B2 --> B4[Print status table]
    B4 --> B3[ScanSysctlConf: note /etc/sysctl.conf overrides]

    C --> C1[RequireRoot]
    C1 --> C2[SnapshotKernelParams]
    C2 --> C3[WriteKernelParamBackup to ~/.config/ark-cli/kernel-params-backup.conf]
    C3 --> C4[sysctl -w for each param below minimum]
    C4 --> C5[ApplyNofilePersistFiles: limits.d + systemd override, if not set]
    C5 --> C6[Write /etc/sysctl.d/99-ark-server.conf]
    C6 --> C7[PatchSysctlConf, AppendSysctlConfToBackup]
    C7 --> C8[Print status table]

    D --> D1[RequireRoot]
    D1 --> D2[Read backup file]
    D2 --> D3[sysctl -w each backed-up value]
    D3 --> D4[Remove /etc/sysctl.d/99-ark-server.conf]
    D4 --> D5[RevertSysctlConf]
    D5 --> D6[RemoveNofilePersistFiles]
    D6 --> D7[Print status table]
```

## Schedule Call Graph

`app/ark-cli/internal/schedule/` → `app/pkg/ark/schedule/`. Files under
`<compose-dir>/.ark-manage/`.

```mermaid
flowchart TD
    A[ark-cli schedule command] --> B{Verb?}

    B -->|"create/update/enable/disable"| C[Parse + validate]
    C --> E[SaveScheduleAndCrontab: write schedules.yaml + generated/schedules.crontab]

    B -->|"apply"| F[LoadScheduleFile]
    F --> G[validateScheduleFile unless --skip-validate]
    G --> H[WriteManagedScheduleCrontab: enabled backup schedules only]
    H --> J[crontab generated/schedules.crontab]

    B -->|"check"| K[LoadScheduleByName]
    K --> L[Lock file held? + LoadRunState]
    L --> M[Print status]

    B -->|"run"| N[LoadScheduleByName]
    N --> O[AcquireLock, WriteRunState running]
    O --> P[RunBackupFromSchedule → create backup command path]
    P --> Q[WriteRunState success/failed]
```

## INI Edit/Set/Show Call Graph

`app/ark-cli/internal/ini/`.

```mermaid
flowchart TD
    A[ark-cli edit/set/show ini] --> B[Resolve INI file spec + compose service]
    B --> C[ServiceRunningStatus]
    C --> D{running?}

    D -->|"yes, show"| F[Exec: ark-ini.py get]
    D -->|"no, show"| H[RunOneOff: ark-ini.py get in removed container]
    D -->|"yes, edit/set, no --restart"| X[refuse]
    D -->|"yes, edit/set, --restart"| J[DoRestartStopInstance with default grace]
    J --> G
    D -->|"no, edit/set"| G{command}
    G -->|"edit"| E[RunOneOffInteractive: editor, default nano]
    G -->|"set"| S[RunOneOff: ark-ini.py set, JSON reply]
    E --> K{was running?}
    S --> K
    K -->|"yes, even if the change failed"| L[DoRestartStartInstance with ready wait]
```

## Prune Backups Call Graph

Host filesystem only (`app/pkg/ark/backup/policy.go`). `--instance all` covers every
service plus `cluster`.

```mermaid
flowchart TD
    A[ark-cli prune backups] --> B[Parse --retain-count/days/size-gb]
    B --> C{--dry-run?}
    C -->|"yes"| D[DryRunRetention]
    C -->|"no"| G[ApplyRetention]
    D --> H
    G --> H[ListRetentionCandidates: os.ReadDir]
    H --> I[SortByNewest]
    I --> J[DiskUsage: syscall.Statfs; floor 2 kept, 1 if under 5 GiB free]
    J --> K[pruneByAge]
    K --> L[pruneByCount]
    L --> M[pruneBySize]
    M --> N[pruneForFreeSpace]
    N --> O{dry run?}
    O -->|"yes"| P[Print remove/keep per file]
    O -->|"no"| Q[removeRetainedBackups: os.Remove]
```
