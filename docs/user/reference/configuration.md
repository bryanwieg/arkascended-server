# Configuration Variables

## Server identity

- `MAP_NAME` (default: `TheIsland_WP`) — the map the server loads. All current
  ARK: Survival Ascended map names end in `_WP` (e.g., `TheIsland_WP`,
  `Aberration_WP`). If you specify a map name that does not end in `_WP`, the
  CLI and container logs will emit a warning, as the server may fail to start or
  hang indefinitely.
- `SESSION_NAME` — in-game server name shown in the browser; defaults to the
  instance name if unset
- `SERVER_ADMIN_PASSWORD` — password for in-game admin commands (required for
  RCON-driven stop/save/backup). Applied to `GameUserSettings.ini` on every
  container start, so changing it only needs a restart.
- `SERVER_PASSWORD` — password players need to join; leave unset for a public
  server. Applied to `GameUserSettings.ini` on every container start.

- `MOD_IDS` — comma-separated CurseForge mod IDs (e.g. `940975,1386057`),
  passed to the server as `-mods=`. The server downloads and updates them on
  start. Spaces are ignored; any other non-digit makes the container exit
  with a configuration error. Set it with `ark-cli reconfigure instance --mods`.
- `CLUSTER_ID` — cluster this server joins, passed as `-clusterid=`. Instances
  with the same ID share character, item and dino transfers through the
  `ark-cluster-1` volume at `Saved/clusters`. Letters, digits, `_` and `-`
  only. Set it with `ark-cli reconfigure instance --cluster-id`.

## Networking

- `ASA_PORT` (default: `27015`) — game port the server binds on; the compose
  UDP port binding is always 1:1 (`ASA_PORT:ASA_PORT/udp`) so the published
  host port equals the container port
- `RCON_PORT` — RCON port for in-game commands. Defaults to `27020`. Configurable
  via the `RCON_PORT` environment variable. Must match the `RCONPort` setting in
  `GameUserSettings.ini`. New instances publish RCON on `127.0.0.1` only; use
  `ark-cli enable remote-rcon` if you need to reach it from another machine

## System

- `TZ` — container timezone (e.g. `America/New_York`); affects log timestamps
- `NoBattlEye` (default: `false`) — set to `true` to disable BattlEye; useful
  for servers where BattlEye causes issues

## Proton/Wine (build-time)

- `PROTON_VERSION` — GE-Proton version string; a Dockerfile `ARG` that overrides
  the version baked into the image (e.g. `11-12`). This is set at image build
  time, not at container runtime.
- `PROTON_PATH` — full path to the Proton executable; overrides the version-based
  default path under `/home/ark-user/.steam/compatibilitytools.d/`. Also a
  build-time setting, not a runtime env var.

## Log management

- `LOG_ROTATION_KEEP_FILES` (default: `10`) — number of rotated `ShooterGame.log.*`
  and `crashcallstack.txt.*` files to retain on each restart
- `RCON_HOST` (default: `localhost`) — RCON connection host

## Maintenance mode

- `ARK_MAINTENANCE_MODE` — when set to `restore`, the container enters
  maintenance mode for restore operations. The restore script refuses to run
  without this env var set, preventing accidental restore over a live instance.
  See [restore-backup.md](../how-to/restore-backup.md) for details.

## Other runtime variables

- `ARK_TMP_DIR` — override the default temporary directory location.
  Defaults to `./.ark-tmp/` relative to the current working directory.
  Use this when `/tmp` is unavailable or too small.
- `ARK_SKIP_RAM_CHECK` — set to `1` in the instance's compose `environment` to
  silence the startup warning about low host RAM
- `PROTON_NO_ESYNC` — set to `1` to disable esync in Proton
- `PROTON_NO_FSYNC` — set to `1` to disable fsync in Proton
- `WINEDEBUG` — Wine debug channel flags (e.g. `+all` for verbose output)
- `STEAM_COMPAT_DATA_PATH` — Proton compatibility data path
- `STEAM_COMPAT_CLIENT_INSTALL_PATH` — Steam client install path for Proton

## Backup behavior

- `BACKUP_RCON_SAVE_MAX_RETRIES` (default: `3`) — max retries for the RCON
  `saveworld` command during backup
- `BACKUP_RCON_SAVE_RETRY_DELAY_SECONDS` (default: `2`) — delay between RCON
  save command retries
- `BACKUP_SAVE_CONFIRM_MAX_RETRIES` (default: `3`) — max overall confirmation
  attempts (each attempt issues `saveworld` and polls for the log marker)
- `BACKUP_SAVE_CONFIRM_RETRY_DELAY_SECONDS` (default: `3`) — delay between
  overall confirmation attempts
- `BACKUP_SAVE_CONFIRM_TIMEOUT_SECONDS` (default: `60`) — max seconds to poll
  the log for the save marker within a single confirmation attempt
- `BACKUP_SAVE_CONFIRM_POLL_SECONDS` (default: `2`) — polling interval while
  waiting for the save marker within a single confirmation attempt
- `BACKUP_VERIFY_MAX_RETRIES` (default: `1`) — max retries for backup archive
  verification
- `BACKUP_VERIFY_RETRY_DELAY_SECONDS` (default: `3`) — delay between
  verification retries

If you change the server’s internal RCON port, set `RCON_PORT` to match so the
stop/backup scripts can connect.

To change the server’s game port, use `ark-cli reconfigure instance --instance <name>
--asa-port <port>`. This keeps the compose binding, `ASA_PORT` env var, and
`ark.port.asa` label in sync automatically.

**Important:** The UDP port binding is always 1:1 — the published host port
must equal `ASA_PORT`. ARK: Survival Ascended registers its bound port with
Steam during startup, so a remapped port (e.g. `27016:27015/udp`) would
cause clients to see the wrong port and fail to connect.

Example (port 27017):

```yaml
services:
  asa-server-1:
    environment:
      - ASA_PORT=27017
    ports:
      - "27017:27017/udp"   # published == container port (1:1 required)
    labels:
      ark.port.asa: "27017"
```

## Container runtime

- `ARK_CONTAINER_RUNTIME` — force a container runtime (`docker` or `podman`).
  When unset, the CLI auto-detects from `$PATH` (prefers Docker when both are
  present). The `--runtime` CLI flag takes priority over this variable.
  Podman 5.0+ is required; Docker works with any recent version.

## Image build and update

- `ARK_STEAM_API_URL` — override the Steam API endpoint used by
  `update instance` to check for newer builds. Defaults to
  `https://api.steamcmd.net/v1/info/2430930`. Primarily useful for testing.
- `ARK_BUILD_NO_CACHE` — when set to any non-empty value, passes `--no-cache`
  to the image build command. Forces a full rebuild without layer cache. Used
  by integration tests.
- `ARK_BUILD_REQUIRED_FREE_GB` (default: `50`) — free space (GiB) required in
  `<compose dir>/.ark-manage` before an image build starts. The build stops
  with an error when there is less.
- `ARK_UPDATE_WEBHOOK_URL` (optional, `http`/`https`) — when an update ends in
  `build_failed`, `rolled_back` or `rollback_failed`, `ark-cli` POSTs
  `{"content": "<host>: ARK update of <instance> <result>: <from> -> <to>. <message>"}`
  to this URL (Discord webhook format; other chat webhooks accepting that
  shape work too). A failed post only prints a warning.

## Shutdown and save behavior

- `SHUTDOWN_WARNING_MINUTES` (default: `0`) — minutes of in-game player
  warning broadcast before shutdown begins. When set to `0` (the default),
  no warning is broadcast. To set a warning, pass a positive integer.
- `GRACEFUL_STOP_SCRIPT_TIMEOUT_SECONDS` (default: `600`) — max time (seconds)
  for the graceful stop script to complete running its internal steps. The
  `SHUTDOWN_WARNING_MINUTES` delay is added on top of this, so the warning
  does not use up the time needed to save.
- `GRACEFUL_STOP_WAIT_SECONDS` (default: `180`) — seconds to wait between
  checking if the server process has exited.
- `FORCE_TERM_WAIT_SECONDS` (default: `20`) — seconds to wait before
  force-terminating after graceful stop fails.
- `SAVE_MAX_RETRIES` (default: `3`) — how many times the stop script
  retries the RCON saveworld confirmation before giving up.
- `SAVE_RETRY_DELAY_SECONDS` (default: `3`) — delay (seconds) between save
  confirmation retries.
- `SAVE_CONFIRM_TIMEOUT_SECONDS` (default: `60`) — how long the stop script
  waits, per attempt, for a `World Save Complete` log line written after its
  `saveworld` request. Earlier save markers in the log never count. Raise this
  for very large worlds whose saves take longer than a minute.
- `STOP_CHECK_TIMEOUT_SECONDS` (default: `120`) — how long the stop script
  waits for the server process to exit after issuing `do exit`.
- `STOP_CHECK_INTERVAL_SECONDS` (default: `5`) — polling interval while
  waiting for the server process to exit
