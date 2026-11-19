# CLI Commands (ark-cli)

Reference for every `ark-cli` command and flag. How the CLI and the container
split the work: [How It Works](../explanation/architecture.md).

Syntax: `ark-cli [global options] <verb> <object> [flags]`. Targets are flags
(`--instance`, `--name`, `--backup`, `--port`), never positional arguments.

## Global options

Place these before `<verb> <object>`.

| Option | Description |
|---|---|
| `-h`, `--help` | Top-level usage and command list. `<verb> <object> --help` shows a command's flags. |
| `--version` | Print the CLI version (`dev` for local builds). |
| `-v`, `--verbose` | Print executed commands and captured output. |
| `-v <logfile>`, `-v=<logfile>`, `--verbose=<logfile>` | Verbose, and also write verbose output to `<logfile>`. |
| `--runtime docker\|podman` | Container runtime. Default: `ARK_CONTAINER_RUNTIME`, else auto-detect from `PATH` (prefers Docker). |

Every command checks the runtime first: daemon Docker 28.0+ or Podman 5.0+, and
`<runtime> compose version` must succeed (a Compose provider is required).

Common flags:

- `--compose-file <path>`: compose file to use. Default `./docker-compose.yaml`,
  or `COMPOSE_FILE` if set. Accepted by every command that reads compose,
  except `validate backup` and `show backup` (see below).
- `--instance all`: where accepted, targets every service in the compose file.
- Backup root: `<compose-dir>/backups/`, or `ARK_BACKUP_DIR`. Archives for an
  instance go in `<backup-root>/<instance>/`.

## Instances

### create instance

Add an instance service to the compose file (created if missing). Session name
is set to the instance name. RCON host port is the next free TCP port from
27020, published on `127.0.0.1` only. The new service uses
`image: ark-server:latest` until the first `start instance` builds and pins it.

| Flag | Default | Description |
|---|---|---|
| `--name` | required | Instance name. `cluster` is reserved. Fails if the service already exists. |
| `--admin-password` | required | `SERVER_ADMIN_PASSWORD`. |
| `--server-password` | | `SERVER_PASSWORD`. One of this or `--no-server-password` is required. |
| `--no-server-password` | `false` | Empty `SERVER_PASSWORD`. Mutually exclusive with `--server-password`. |
| `--map-name` | `TheIsland_WP` | `MAP_NAME`. Warns if the name does not end in `_WP`. |
| `--asa-port` | next free UDP from 27015 | Game port; host and container port kept equal. |
| `--tz` | `America/New_York` | `TZ`. |
| `--interactive` | `false` | Prompt for values not given as flags. |
| `--compose-file` | | See [common flags](#global-options). |

```sh
ark-cli create instance --name asa-server-1 --server-password ChangeMe --admin-password MyAdminPass
```

For rootful installs, don't run the server as UID 1000 if that is your login
user; use a dedicated `ark` user in the `docker` group.

### reconfigure instance

Change compose settings of one existing instance. At least one setting flag is
required. Refuses a running instance unless `--force`.

| Flag | Description |
|---|---|
| `--instance` | Required. One instance (`all` is rejected). |
| `--tz` | `TZ`. |
| `--map-name` | `MAP_NAME`. Only allowed when it equals the current value (or none is set); changing the map of an existing instance is an error. |
| `--session-name` | `SESSION_NAME`. |
| `--admin-password` | `SERVER_ADMIN_PASSWORD`. |
| `--server-password` | `SERVER_PASSWORD`. `--server-password ""` clears it (no join password). |
| `--mods` | `MOD_IDS`: comma-separated CurseForge mod IDs. `""` clears it. |
| `--cluster-id` | `CLUSTER_ID` (letters, digits, `_`, `-`). Instances with the same ID share transfers. `""` clears it. |
| `--asa-port` | Published game UDP port (host and container port kept equal). |
| `--rcon-port` | Published RCON TCP host port (maps to the container's `RCON_PORT`, default 27020/tcp). |
| `--force` | Allow changes while running; a restart is still needed to apply them. |
| `--restart` | After saving, restart the instance if it was running. A stopped instance is not started. |
| `--compose-file` | |

```sh
ark-cli reconfigure instance --instance asa-server-1 --mods 940975,1386057 --cluster-id my-cluster
ark-cli reconfigure instance --instance asa-server-1 --rcon-port 28020 --force --restart
ark-cli reconfigure instance --instance asa-server-1 --server-password ""   # make the server public
```

### enable / disable remote-rcon

`enable remote-rcon` publishes RCON on all interfaces (IPv4 and IPv6);
`disable remote-rcon` publishes it on `127.0.0.1` only (the default for new
instances; instances created before that default keep an all-interfaces binding
until you disable it). Only the RCON port binding changes. The CLI's own RCON
use runs inside the container and works either way.

Flags: `--instance` (required), `--force`, `--restart`, `--compose-file`, with
the same meaning as in [reconfigure instance](#reconfigure-instance).

RCON sends the admin password in plain text, and Docker-published ports bypass
`ufw`/`firewalld`. Restrict remote RCON in the `DOCKER-USER` chain, e.g. allow
only `203.0.113.7` to host port `28020`:

```sh
sudo iptables -I DOCKER-USER -p tcp -m conntrack --ctorigdstport 28020 ! -s 203.0.113.7 -j DROP
```

### start instance

Start service(s) and wait until ready (log line `Server has completed startup
and is now advertising for join.`). Never pulls: builds the `ark-server` image
locally if missing and pins compose to `ark-server:<build-id>-<recipe-id>`
(build takes ~15–25 min and needs the free space set by
`ARK_BUILD_REQUIRED_FREE_GB`). Any non-`ark-server` image is an error. Refuses
to start when backup disk space is critically low; see
[Disk-space check at start](../explanation/backups.md#disk-space-check-at-start)
and [How updates work](../explanation/updates.md).

| Flag | Default | Description |
|---|---|---|
| `--instance` | required | Instance or `all`. |
| `--wait-timeout` | `5m` or `ARK_START_WAIT_TIMEOUT` | Max wait for ready. |
| `--no-wait` | `false` | Return right after starting. |
| `--compose-file` | | |

### stop instance

Graceful stop: the container saves the world over RCON, then exits.

| Flag | Default | Description |
|---|---|---|
| `--instance` | required | Instance or `all`. |
| `--timeout` | `20m` | Grace period before SIGKILL (Go duration, `>= 0`). |
| `--kill` | `false` | SIGKILL immediately, no save. Mutually exclusive with `--timeout`. |
| `--compose-file` | | |

### restart instance

Stop, then start and wait for ready (fixed 5m wait; `ARK_START_WAIT_TIMEOUT`
does not apply).

| Flag | Default | Description |
|---|---|---|
| `--instance` | required | Instance or `all`. |
| `--timeout` | `20m` | Stop grace period, as in `stop instance`. |
| `--compose-file` | | |

### save instance

Run RCON `saveworld` in each running target.

Flags: `--instance` (required; instance or `all`), `--compose-file`.

### list players

List players connected to each target via RCON `ListPlayers` (name and EOS
ID). With `--instance all`, stopped instances are listed as `not running`; a
named instance that is stopped, or an RCON failure, exits non-zero.

Flags: `--instance` (required; instance or `all`), `--json`, `--compose-file`.

```sh
ark-cli list players --instance all
```

### update instance

Compare each instance's image label `io.ark.server.build_id` with Steam's
current public build and build a new image when it differs. `--check` shows
Steam's build in the `target` column (`build <id>`); after a build, the table
shows the new ARK version and each instance gets a line
`<instance>: <current version> -> <new version> (build <id>)`. Without
`--rollout`, compose is left unchanged and the CLI prints
`Built ark-server:<id>. Switch with: ark-cli update instance --instance <name> --rollout`.

| Flag | Default | Description |
|---|---|---|
| `--instance` | `all` | Instance or `all`. |
| `--check` | `false` | Print the update table only; build and change nothing. |
| `--rollout` | `false` | Switch instances to the new image one at a time, rolling back on failure. |
| `--restart` | `false` | Alias for `--rollout`. |
| `--compose-file` | | |

Rollout, per instance in name order: stop (save), `pre-update` backup, switch
compose tag, start, wait for ready; each step prints a line. On failure: switch
compose back, restore the pre-update backup, start the old image, print
`Update of <name> to <new> failed; rolled back to <old>: <reason>`, skip the
remaining instances, exit 1. A failed build changes nothing: its error names the
cause (e.g. `SteamCMD download failed: state is 0x6`) and ends with
`no instances were changed`.

### remove instance

Remove one service from compose and delete its `<instance>-saved` volume. A
regular backup is taken first unless `--skip-backup`. A running instance is
refused unless `--kill`. `--instance all` is rejected; use
`teardown instances`.

| Flag | Description |
|---|---|
| `--instance` | Required. |
| `--kill` | SIGKILL a running instance (no save) before removal. |
| `--skip-backup` | Skip the pre-remove backup. |
| `--dry-run` | Print what would happen. |
| `--compose-file` | |

### teardown instances

Stop all services and run compose down for the project: containers and
networks are removed, named volumes (saves, cluster data) are kept. Prompts for
confirmation on a TTY; without a TTY, `--skip-confirm` is required.

Flags: `--skip-confirm`, `--compose-file`.

### list instances

List instances with status and health indicators. Flags: `--json`,
`--compose-file`.

### show instance

Status and config summary. Flags: `--instance` (required), `--json`,
`--compose-file`.

### show volume-paths

Volume mount paths for an instance. Alias: `show vol-paths`. Flags:
`--instance` (required), `--compose-file`.

### logs instance

| Flag | Default | Description |
|---|---|---|
| `--instance` | required | One instance. |
| `--tail` | `200` | Lines from the end (`>= 0`). |
| `--follow` | `false` | Follow output. |
| `--timestamps` | `false` | Prefix timestamps. |
| `--compose-file` | | |

### check instance

Health check: container running and startup-ready marker seen. Exits non-zero
when not healthy. After `result:`, prints
`last_update: <result> at <time> (<from> -> <to>)` (and
`last_update_message:` when present) for the latest update attempt; this does
not affect the exit code.

Flags: `--instance` (required), `--compose-file`.

### wait instance

| Flag | Default | Description |
|---|---|---|
| `--instance` | required | One instance. |
| `--for` | `ready` | `started` (container running), `advertising` / `ready` (advertising for join), `stopped`. |
| `--timeout` | `10m` | Max wait (`> 0`). |
| `--interval` | `2s` | Poll interval (`> 0`). |
| `--compose-file` | | |

### validate compose / show compose

`validate compose` parses the compose file and prints the service count.
`show compose` prints a summary. Flag: `--compose-file`.

## INI files

### edit ini

Open an INI file in an editor inside the container (running container, or a
one-off container), so volume ownership stays correct. Refuses a running
instance unless `--restart`, which stops it (saving the world), opens the editor
and starts it again; ARK rewrites its INI files when it stops, so editing
while it runs could be undone. `--file gameusersettings` refuses until the
file exists (created on the instance's first start); `Game.ini` is created by
editing it.

| Flag | Default | Description |
|---|---|---|
| `--instance` | required | |
| `--file` | required | `game` (Game.ini), `gameusersettings` or `gus` (GameUserSettings.ini). |
| `--editor` | `nano` | Editor command in the container. The image ships only `nano`. |
| `--restart` | `false` | On a running instance: stop, edit, start. |
| `--compose-file` | | |

### get ini

Print one key as a table: one row per line for the key (more than one when it
repeats), with who wrote each line and when, from the
[INI history](#ini-history). Read-only, so it works whether the instance is
running or not. An unset key exits non-zero.

```sh
ark-cli get ini --instance asa1 --file gus --section ServerSettings --key MaxPlayers
# LINE  KEY         VALUE  ORIGIN
# 6     MaxPlayers  20     written by ARK 2026-11-19 00:01:26 UTC
```

| Flag | Default | Description |
|---|---|---|
| `--instance` | required | |
| `--file` | required | `game`, `gameusersettings` or `gus`. |
| `--section` | required | Section name, with or without brackets. Matched case-insensitively. |
| `--key` | required | Key name. Matched case-insensitively. |
| `--json` | `false` | `{file, section, key, lines: [{line, value, origin}], history}`; exits 0 even when `lines` is empty. |
| `--compose-file` | | |

### set ini

Set one key without an editor, adding the key (and its section) if missing.
Comments, order and line endings are kept, and the file is replaced in one
step. The checks run inside the container (`ark-ini.py`), so they apply however
the command is called:

- A running instance is refused unless `--restart`: ARK rewrites its INI files
  when it stops, so a change made while it runs can be lost. `--restart` stops
  the instance (saving the world), applies the change and starts it again; it
  starts it again even if the change was refused.
- `RCONEnabled`, `RCONPort`, `ServerAdminPassword` and `ServerPassword` in
  `[ServerSettings]` are refused: they are set from the instance's environment
  on every start. Use `reconfigure instance` instead.
- A key that appears more than once in the section is refused, with each line
  listed with its origin from the [INI history](#ini-history): some ARK keys
  (engram and item overrides in `Game.ini`, for example) are lists with one line
  per entry. Use `edit ini` to change one line, or `--replace-all` to replace
  them all with one; the previous file is kept as `<file>.bak`.
- `GameUserSettings.ini` must exist (it is created on the first start);
  `Game.ini` is created if missing.

```sh
ark-cli set ini --instance asa1 --file gus --section ServerSettings --key MaxPlayers --value 20
# GameUserSettings.ini [ServerSettings] MaxPlayers: "70" -> "20"
```

| Flag | Default | Description |
|---|---|---|
| `--instance` | required | |
| `--file` | required | `game`, `gameusersettings` or `gus`. |
| `--section` | required | Section name, with or without brackets, e.g. `ServerSettings` or `/Script/ShooterGame.ShooterGameMode`. Matched case-insensitively. |
| `--key` | required | Key name. Matched case-insensitively. |
| `--value` | required | New value; `--value ""` sets it empty. |
| `--replace-all` | `false` | Replace every line for a repeated key with one line. |
| `--restart` | `false` | On a running instance: stop, apply, start. |
| `--compose-file` | | |

### INI history

Each instance keeps a git history of `Game.ini` and `GameUserSettings.ini`, in
`ShooterGame/Saved/Config/WindowsServer/.git` on its save volume, so backups
carry it and a restore brings back the matching history. Every writer commits
under its own author, which `get ini` and `set ini` show per line (`git blame`):

| Origin | Author | When it is committed |
|---|---|---|
| `written by ARK <when>` | `ARK server` | the image default (ARK generated it at image build), installed on the first start; the server's own writes, after it exits |
| `set from instance env <when>` | `instance env` | the RCON and password settings, applied on every start |
| `set with ark-cli <when>` | `ark-cli set ini` | `set ini` |
| `edited with ark-cli <when>` | `ark-cli edit ini` | `edit ini`, when the editor exits |
| `changed outside ark-cli <when>` | `outside ark-cli` | changes nobody recorded (a hand edit on the volume, a restored file), found at the next start or before `set`/`edit` |
| `changed outside ark-cli (not yet recorded)` | | such a change, before it is found |
| `origin unknown (present <when>)` | `unknown origin` | the files as first seen, on a volume from before the history existed |

`<when>` is the commit time in UTC. To read the full history on the host, run
git against that directory in the volume, e.g.
`git -C <saved volume path>/Config/WindowsServer log -p` (`show volume-paths`
prints the path).

The history lives in the image's scripts, so an existing instance starts it on
its first start with an image built by an ark-cli that has it. After upgrading
ark-cli, rebuild with `ark-cli update instance --instance <name>` (it rebuilds
when the image recipe changed) and restart; the lines already in the files then
show `origin unknown (present <first start>)`. Until the rebuild, `get ini`,
`set ini`, `show ini` and `show instance` say the image needs rebuilding.

### show ini

Print an INI file. Same `--file` values. Reports a missing file.

Flags: `--instance` (required), `--file` (required), `--compose-file`.

## Backups and restore

`cluster` is a reserved target for the shared cluster volume. It works with
`create backup`, `restore backup`, `list backups`, `prune backups` and backup
schedules; archives go to `<backup-root>/cluster/`. `validate backup` and
`show backup` do not apply to it. See
[Cluster backups](../explanation/backups.md#cluster-backups). Retention, safety
floor, locks, metadata and naming:
[Understanding Backups](../explanation/backups.md).

### create backup

Runs inside the instance's container, or a temporary one if stopped. With
`--instance all`, backs up every instance, then the cluster (if the compose file
has one).

| Flag | Default | Description |
|---|---|---|
| `--instance` | required | Instance, `cluster` or `all`. |
| `--dest` | `<backup-root>/<instance>` | Output directory. |
| `--name` | | Archive name becomes `<instance>-<name>-<YYYYmmdd-HHMMSS>.<format>` instead of `<instance>-backup-<YYYYmmdd-HHMMSS>.<format>`. |
| `--format` | `tar.gz` | `tar`, `tar.gz` or `tar.zst`. |
| `--retain-count` | `20` | Max archives kept (`0` disables). |
| `--retain-days` | `7` | Max age in days (`0` disables). |
| `--retain-size-gb` | `5` | Max total GiB (`0` disables). |
| `--lock-timeout` | `2m` | Wait for the per-instance backup lock. |
| `--backup-wait-timeout` | `10m` | Wait for the archive to complete (`> 0`). |
| `--compose-file` | | |

Retention keeps at least two archives (one when free space is below 5 GiB).

### restore backup

Three forms:

```sh
ark-cli restore backup --instance <name> --archive [<src-instance>:]<backup|latest>
ark-cli restore backup --new-instance <name> --archive <src-instance>:<backup|latest>
ark-cli restore backup --instance cluster --archive <backup|latest>
```

| Flag | Description |
|---|---|
| `--instance` | Existing instance, or `cluster`. Mutually exclusive with `--new-instance`. |
| `--new-instance` | Add a new instance to compose (next free ports) and restore into it. |
| `--archive` | Required. Backup name or `latest`; prefix `<src-instance>:` to take it from another instance (required with `--new-instance`, rejected for `cluster`). |
| `--auto-start` | Start the service after restore. Rejected for `cluster`. |
| `--admin-password` | `--new-instance` only. Default: the one in the backup's GameUserSettings.ini; if the backup has none, restore stops before changing anything. |
| `--server-password` | `--new-instance` only. Default: the one in the backup. |
| `--no-server-password` | `--new-instance` only. Mutually exclusive with `--server-password`. |
| `--compose-file` | |

Instance restore: the instance must be stopped. Runs in a one-shot maintenance
container, writes `<instance>-pre-restore-<timestamp>.tar.gz` to the backup
directory, clears the save directory, extracts the archive, and logs the
archive's metadata if present. `Saved/clusters/` is left untouched (and skipped
if an older archive contains it).

Cluster restore: every instance using the cluster must be stopped (the error
names running ones). Writes `cluster-pre-restore-<timestamp>.tar.gz` first.

### list backups

Flags: `--instance` (required; instance, `cluster` or `all`), `--json`,
`--compose-file` (used to find the backup root and, for `all`, the instances).

### show backup

Archive metadata (server version, build ID, instance, map, timestamp from
`backup-metadata.json`; `-` for archives without it).

Flags: `--instance` (required), `--backup` (required; name or `latest`),
`--json`. Backup root comes from `ARK_BACKUP_DIR` or the directory of
`COMPOSE_FILE` / `./docker-compose.yaml`; there is no `--compose-file`.

### validate backup

Extract the archive to a temporary directory and run SQLite integrity checks on
every `.sqlite` file. Reports per file; exits non-zero on any failure or if no
`.sqlite` file is found.

Flags: `--instance` (required), `--backup` (required; name or `latest`).
Backup root as for `show backup`.

### prune backups

Apply retention without creating a backup.

| Flag | Default | Description |
|---|---|---|
| `--instance` | required | Instance, `cluster` or `all`. |
| `--retain-count` | `20` | As in `create backup`. |
| `--retain-days` | `7` | |
| `--retain-size-gb` | `5` | |
| `--dry-run` | `false` | Print what would be removed. |
| `--compose-file` | | Needed to resolve `all`. |

## Schedules

Managed backup schedules. Files, relative to the current directory:

| Path | Content |
|---|---|
| `.ark-manage/schedules.yaml` | Schedule config |
| `.ark-manage/generated/schedules.crontab` | Generated crontab (regenerated by every change) |
| `.ark-manage/logs/schedules/<name>.log` | Run log |
| `.ark-manage/state/schedules/<name>.last.json` | Last run result |
| `.ark-manage/locks/schedule-<name>.lock` | Run lock |

Changes only regenerate the crontab file; run `apply schedules` to install it.

### create schedule backup

| Flag | Default | Description |
|---|---|---|
| `--name` | required | Unique schedule name. |
| `--instance` | required | Instance, `cluster` or `all` (`all` includes the cluster). |
| `--cron` | required | 5-field cron expression. |
| `--format` | `tar.gz` | `tar`, `tar.gz` or `tar.zst`. |
| `--retain-count` | `20` | |
| `--retain-days` | `7` | |
| `--retain-size-gb` | `5` | |
| `--disabled` | `false` | Create disabled. |
| `--compose-file` | | Compose file the scheduled run uses. |

```sh
ark-cli create schedule backup --instance asa-server-1 --name nightly-asa1 --cron "0 3 * * *"
```

### update schedule

`--name` (required) plus at least one of: `--cron`, `--instance`,
`--compose-file`, `--format`, `--retain-count`, `--retain-days`,
`--retain-size-gb`, `--enabled`, `--disabled` (the last two are mutually
exclusive). Unset flags keep their current value.

### Other schedule commands

| Command | Flags | Description |
|---|---|---|
| `list schedules` | `--json` | Schedules with last status, exit code and finish time. |
| `show schedule` | `--name` (required), `--json` | Schedule details. |
| `enable schedule` / `disable schedule` | `--name` (required) | Toggle without removing. |
| `check schedule` | `--name` (required) | Running status and last result; non-zero exit if the last run failed. |
| `run schedule` | `--name` (required) | Run now. |
| `validate schedule` | `--name` (optional; all if omitted) | Report config problems. |
| `remove schedule` | `--name` (required) | Delete the schedule. |
| `apply schedules` | `--skip-validate` | Validate all, regenerate, and install with `crontab <file>`. This replaces the user's entire crontab. |

## Port reachability

### check reachability

Two modes; `--port` and `--instance` are mutually exclusive.

- Instance mode (`--instance`): checks the game UDP port (from the
  `ark.port.asa` label or port binding) and the host port published for the
  RCON container port (`RCON_PORT`, default 27020/tcp). The instance must be running unless `--listen`. If
  RCON is published on `127.0.0.1` only, a note says CLOSED from remote is
  expected.
- Port mode (`--port`): checks one port without reading compose.

External probes use `starrupture-utilities.com`, with `portchecker.io` as a
TCP-only fallback.

| Flag | Default | Description |
|---|---|---|
| `--instance` | | Instance mode. |
| `--port` | | Port mode. |
| `--ports` | `all` | Instance mode only: `asa`, `rcon`, `all`. |
| `--protocol` | `both` | Port mode only: `tcp`, `udp`, `both`. |
| `--from` | `both` | Probe from `local`, `remote`, `both`, or a host address. |
| `--listen` | `false` | Open temporary listeners during the probe (tests forwarding without a running server; UDP listeners reply so external checkers see the port open). Not allowed with `--from remote`. In instance mode, fails if the running server already holds the ports. |
| `--compose-file` | | |

```sh
ark-cli check reachability --instance asa-server-1 --ports asa
ark-cli check reachability --port 27015 --protocol udp --listen
```

### listen port

Open listeners and block until Ctrl+C; no probe.

| Flag | Default | Description |
|---|---|---|
| `--port` | required | |
| `--protocol` | `both` | `tcp`, `udp`, `both`. |

## Host kernel configuration

No flags. `fix` and `restore` require root.

| Command | Description |
|---|---|
| `check host-kernel-config` | Report `vm.max_map_count`, `net.core.rmem_max`, `net.core.wmem_max`, `fs.file-max` and the nofile hard limit against required minimums. |
| `fix host-kernel-config` | Save current values to a backup, apply missing settings, persist them (`/etc/sysctl.d/99-ark-server.conf`; nofile via `/etc/security/limits.d/99-ark-server.conf` and `/etc/systemd/system.conf.d/99-ark-server.conf`). |
| `restore host-kernel-config` | Revert to the values saved by `fix`. |
