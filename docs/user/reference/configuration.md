# Configuration Variables

Two kinds:

- **Container variables**: set in the instance's `environment:` list in
  `docker-compose.yaml`; take effect on the next container start. Read by the
  scripts inside the image.
- **Host variables**: set in the shell that runs `ark-cli`.

Integer container variables must be whole numbers of at least 1 (0 allowed
where noted). Any other value makes the script exit with a configuration
error.

## Server identity

| Variable | Default | Effect |
|---|---|---|
| `MAP_NAME` | `TheIsland_WP` | Map the server loads. Current ASA maps end in `_WP`; any other value logs a warning (CLI and container), as the server may fail to start or hang. |
| `SESSION_NAME` | instance name (written by `create instance`) | Name shown in the server browser. Empty: ARK's own default. |
| `SERVER_ADMIN_PASSWORD` | none | Admin/RCON password. Needed for saves before stop, `save instance` and backups. Written to `GameUserSettings.ini` on every start; unset logs a warning and skips the RCON settings. |
| `SERVER_PASSWORD` | empty (public) | Join password. Written to `GameUserSettings.ini` on every start (only when `SERVER_ADMIN_PASSWORD` is set). |
| `MOD_IDS` | empty | Comma-separated CurseForge mod IDs (e.g. `940975,1386057`), passed as `-mods=`. The server downloads and updates them on start. Spaces are ignored; any other non-digit is a configuration error. Set with `ark-cli reconfigure instance --mods`. |
| `CLUSTER_ID` | empty | Cluster to join, passed as `-clusterid=`. Instances with the same ID share transfers through the `ark-cluster-1` volume at `Saved/clusters`. Letters, digits, `_`, `-` only. Set with `ark-cli reconfigure instance --cluster-id`. |

## Networking

| Variable | Default | Effect |
|---|---|---|
| `ASA_PORT` | `27015` | UDP game port the server binds (`?Port=`). Change with `ark-cli reconfigure instance --asa-port`, which keeps `ASA_PORT`, the port binding and the `ark.port.asa` label in sync. |
| `RCON_PORT` | `27020` | RCON port inside the container: written to `RCONPort` in `GameUserSettings.ini` on start and used by the stop/save/backup scripts. If you change this, edit the compose binding's container port to match; `reconfigure instance --rcon-port` and `check reachability` then use the new port, and `check reachability` errors if no binding matches. |
| `RCON_HOST` | `localhost` | Host the in-container RCON client connects to. |

The UDP binding must be 1:1 (`<ASA_PORT>:<ASA_PORT>/udp`). ASA registers its
bound port with Steam, so a remap such as `27016:27015/udp` advertises the
wrong port and clients fail to connect.

The RCON host port (`--rcon-port` on `reconfigure instance`) is published on
`127.0.0.1` only; `ark-cli enable remote-rcon` publishes it on all interfaces.

## System

| Variable | Default | Effect |
|---|---|---|
| `TZ` | `America/New_York` (written by `create instance`) | Container timezone; affects log timestamps. `enable scheduler` gives the scheduler the `TZ` of the instance whose name sorts first; schedules run in it. |
| `NoBattlEye` | `false` | `true` (case-insensitive) passes `-NoBattlEye`. |
| `ARK_SKIP_RAM_CHECK` | `0` | `1` silences the startup warning when host `MemAvailable` is below the container's `mem_reservation`. |
| `LOG_ROTATION_KEEP_FILES` | `10` | Rotated `ShooterGame.log.*` and `crashcallstack.txt.*` files kept at each server start. `0` allowed. |

## Proton/Wine

Set in the base image (`container/base.dockerfile`). Overriding them at
runtime is unsupported.

| Variable | Image value | Notes |
|---|---|---|
| `PROTON_VERSION` | `11-12` | Build `ARG` of the base image; selects the GE-Proton release. |
| `PROTON_PATH` | `/home/ark-user/.steam/compatibilitytools.d/GE-Proton<PROTON_VERSION>/` | Directory holding the `proton` executable used to start the server. |
| `PROTON_NO_ESYNC` | `0` | `1` disables esync. |
| `PROTON_NO_FSYNC` | `0` | `1` disables fsync. |
| `WINEDEBUG` | `-all` | Wine debug channels (e.g. `+all` for verbose output). |
| `STEAM_COMPAT_DATA_PATH` | `<PROTON_PATH>files/share/default_pfx` | Proton prefix. |
| `STEAM_COMPAT_CLIENT_INSTALL_PATH` | `/home/ark-user/.local/share/Steam` | SteamCMD client files. |

## Shutdown and save

Stop sequence: optional player warning, `saveworld` and wait for a new
`World Save Complete` log line, `DoExit`, wait for the log to close; then
SIGTERM and SIGKILL to the server process group if it is still running.

| Variable | Default | Effect |
|---|---|---|
| `SHUTDOWN_WARNING_MINUTES` | `5` | Countdown before saving, only while players are online (RCON `ListPlayers`; if RCON can't answer, players are assumed). Warnings go out as `ServerChat` and `Broadcast` at the start and at 10 min, 5 min, 1 min and 30 s left, naming the reason when `ark-cli` gave one (`updating to 98.9`). With nobody online the stop saves at once. `0`: no warning. |
| `SAVE_MAX_RETRIES` | `2` | `saveworld` confirmation attempts. |
| `SAVE_RETRY_DELAY_SECONDS` | `3` | Delay between attempts. |
| `SAVE_CONFIRM_TIMEOUT_SECONDS` | `20` | Per-attempt wait for a `World Save Complete` line written after the request. Earlier markers never count. Raise for large worlds. |
| `STOP_CHECK_TIMEOUT_SECONDS` | `60` | Wait for the server log to close after `DoExit`. |
| `STOP_CHECK_INTERVAL_SECONDS` | `5` | Poll interval for that wait. |
| `GRACEFUL_STOP_SCRIPT_TIMEOUT_SECONDS` | `120` | Limit for the whole stop script. `SHUTDOWN_WARNING_MINUTES` is added on top. |
| `GRACEFUL_STOP_WAIT_SECONDS` | `15` | After a successful stop script, max wait for the process to exit before SIGTERM. |
| `FORCE_TERM_WAIT_SECONDS` | `20` | Wait after SIGTERM before SIGKILL. |
| `FORCE_STOP_POLL_SECONDS` | `1` | Poll interval for the two waits above and the server monitor loop. |

Keep compose `stop_grace_period` (default `8m`) at least
`SHUTDOWN_WARNING_MINUTES*60 + GRACEFUL_STOP_SCRIPT_TIMEOUT_SECONDS +
GRACEFUL_STOP_WAIT_SECONDS + FORCE_TERM_WAIT_SECONDS` plus overhead, or the
runtime kills the container mid-save.

## Backup

Apply to backups run inside the container (`backup instance`, scheduled
backups).

| Variable | Default | Effect |
|---|---|---|
| `BACKUP_RCON_SAVE_MAX_RETRIES` | `3` | Retries for sending `saveworld`. |
| `BACKUP_RCON_SAVE_RETRY_DELAY_SECONDS` | `2` | Delay between send retries. |
| `BACKUP_SAVE_CONFIRM_MAX_RETRIES` | `3` | Confirmation attempts (each sends `saveworld`, polls the log for the save marker, then waits until no save file changes between two polls). |
| `BACKUP_SAVE_CONFIRM_RETRY_DELAY_SECONDS` | `3` | Delay between confirmation attempts. |
| `BACKUP_SAVE_CONFIRM_TIMEOUT_SECONDS` | `60` | Per-attempt limit for the save marker, and again for the save files to stop changing. |
| `BACKUP_SAVE_CONFIRM_POLL_SECONDS` | `2` | Polling interval for the log and the save files; files must be unchanged for one interval. |
| `BACKUP_VERIFY_MAX_RETRIES` | `1` | Archive verification attempts (`1` = verify once). |
| `BACKUP_VERIFY_RETRY_DELAY_SECONDS` | `3` | Delay between verification attempts. |

`ARK_MAINTENANCE_MODE` is internal: the entrypoint sets it to `restore` when
the container runs the restore script, and the restore script refuses to run
without it. See [restore-backup.md](../how-to/restore-backup.md).

## Host variables (ark-cli)

| Variable | Default | Effect |
|---|---|---|
| `COMPOSE_FILE` | `docker-compose.yaml` (CWD) | Compose file path. `--compose-file` takes priority. |
| `COMPOSE_PROJECT_NAME` | parent directory name of the compose file | Compose project name. |
| `ARK_CONTAINER_RUNTIME` | auto (`$PATH` probe, prefers docker) | `docker` or `podman` (rootless, 5.0+). `--runtime` takes priority. With `DOCKER_HOST` also set, ark-cli needs no `docker`/`podman` binary or Compose provider on `PATH` and reaches either runtime through `DOCKER_HOST` (the scheduler container runs this way). |
| `DOCKER_HOST` | `/var/run/docker.sock` | Docker daemon endpoint. A `unix://` path is the socket `enable scheduler` mounts into the scheduler. |
| `ARK_SCHEDULER_SOCKET` | unset | Absolute host path of the engine's unix socket for `enable scheduler`; required when `DOCKER_HOST` is not `unix://` (e.g. `tcp://`). |
| `ARK_SCHEDULER_SOCKET_GID` | group of the socket file | Group `enable scheduler` adds to the scheduler on rootful Docker so it can use the socket. Non-negative integer. |
| `TZ` | `UTC` | Scheduler time zone when the instance whose name sorts first sets no `TZ` (read by `enable scheduler`). |
| `XDG_RUNTIME_DIR` | `/run/user/<uid>` | Locates the rootless Podman socket (`$XDG_RUNTIME_DIR/podman/podman.sock`). |
| `ARK_START_WAIT_TIMEOUT` | `5m` | Default for `start --wait-timeout`; Go duration (e.g. `10m`). Invalid or non-positive values are ignored. |
| `ARK_BACKUP_DIR` | `<compose dir>/backups` | Backup root; archives go in `<root>/<instance>/`. Set it before `enable scheduler`, which passes it to the scheduler and mounts it. |
| `ARK_TMP_DIR` | `./.ark-tmp/` (CWD) | Temporary directory for CLI operations. |
| `RCON_HOST`, `RCON_PORT` | unset | When set, passed into the container for `save instance`, overriding the container values. |
| `ARK_STEAM_API_URL` | `https://api.steamcmd.net/v1/info/2430930` | Steam app-info endpoint for update checks. For testing. |
| `ARK_BUILD_NO_CACHE` | unset | Any non-empty value builds the image without layer cache. |
| `ARK_BUILD_REQUIRED_FREE_GB` | `70` | GiB that must be free in `<compose dir>/.ark-manage` before an image build, minus the size of the SteamCMD cache already in `.ark-manage/build/work/cache`; positive integer. |
| `ARK_UPDATE_WEBHOOK_URL` | unset | `http`/`https` URL. On update result `build_failed`, `rolled_back` or `rollback_failed`, POSTs `{"content": "<host>: ARK update of <instance> <result>: <from> -> <to>. <message>"}` (Discord webhook format). A bad URL or failed post only warns. For scheduled updates, set it before `enable scheduler`, which copies it into the scheduler (stored in plain text in `docker-compose.yaml`); rerunning `enable scheduler` without it removes it. Alerts from scheduled updates name the scheduler container as `<host>`, not the host. |
| `TERM` | | `dumb` disables the progress spinner. |
