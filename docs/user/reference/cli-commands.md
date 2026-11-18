# CLI Commands (ark-cli)

Complete reference for `ark-cli` commands. For how the CLI and the container
divide the work, see [How It Works](../explanation/architecture.md).

Command groups:

- Instance lifecycle and inspection (`create/reconfigure/update/show/list/start/stop/restart/save/logs/check/wait/remove instance`, `teardown instances`)
- Container-side INI editing (`edit/show ini`)
- Backups/restore/maintenance (`create/list/show/validate/restore backup`, `prune backups`)
- Schedule management (`create/list/show/update/enable/disable/check/run/validate/apply/remove schedule`)
- Host and network checks (`check/fix/restore host-kernel-config`, `check reachability`, `listen port`, `enable/disable remote-rcon`)

Command style is `<verb> <object>` (for example `create instance`, `create
backup`, `check reachability`).

Global options:

- `-h`, `--help`: show top-level usage and command list.
- `--version`: print the CLI version (`dev` for local builds).
- `-v`, `--verbose`: print executed commands and captured command output.
- `-v <logfile>` or `--verbose=<logfile>`: also write verbose output to a file.
- `--runtime docker|podman`: force a specific container runtime. Auto-detected
  from `$PATH` if omitted (prefers Docker when both are present). Can also be
  set via the `ARK_CONTAINER_RUNTIME` environment variable (flag takes
  priority). Podman 5.0+ is required; Docker works with any recent version.

Subcommand arguments:

- Subcommand targets are passed as flags (`--instance`, `--name`, `--backup`,
  `--port`) rather than positional arguments, making commands self-documenting.
- Global flags (`-v`, `--verbose`) are still parsed before the command route
  (`<verb> <object>`), so place them before `start`, `restore`, etc.

## Create a compose file

Create (or extend) a `docker-compose.yaml` with a new service:

```sh
ark-cli create instance --name asa-server-1 \
  --server-password "ChangeMe" \
  --admin-password "MyAdminPass"
```

The session name is set to the instance name to keep service names, backups, and
in-game session identifiers aligned.

Optional flags include `--map-name`, `--asa-port` (UDP port; host and container port are kept in sync), `--tz`,
and `--compose-file`. Use `--interactive` to prompt for values instead of passing flags.
Use `--no-server-password` to create an instance with no server password (sets `SERVER_PASSWORD` to empty).

Note: For rootful installs, avoid running the server as UID 1000 if it is your
default login user. Consider creating a dedicated `ark` user (in the docker
group) to own the server and manage the repo.

## Instance lifecycle

Start all instances in the compose project:

```sh
ark-cli start instance --instance all
```

Start always makes sure the `ark-server` image exists locally and never
pulls. A new instance's compose file says `image: ark-server:latest` (not built
or pinned yet); the first start builds the image if needed and pins compose to
`ark-server:<build-id>`. The build runs on the host (about 15–25 minutes) and
needs the free disk space set by `ARK_BUILD_REQUIRED_FREE_GB`. Only
`ark-server` images are supported; any other image in compose is an error.
See [How updates work](../explanation/updates.md).

Stop an instance gracefully with configurable timeout (default 15m):

```sh
ark-cli stop instance --instance asa-server-1                    # 15-minute default
ark-cli stop instance --instance asa-server-1 --timeout 0s       # no delay (SIGTERM immediately)
ark-cli stop instance --instance asa-server-1 --timeout 2m       # 2-minute grace period
ark-cli stop instance --instance asa-server-1 --kill             # bypass RCON, kill immediately
```

Stop all instances:

```sh
ark-cli stop instance --instance all
```

Restart an instance (stop then start):

```sh
ark-cli restart instance --instance asa-server-1
```

Trigger an in-game saveworld via RCON:

```sh
ark-cli save instance --instance asa-server-1
```

Remove an instance (backs up Saved first):

```sh
ark-cli remove instance --instance asa-server-1
ark-cli remove instance --instance asa-server-1 --dry-run   # preview without changes
```

Kill a running instance before removal:

```sh
ark-cli remove instance --instance asa-server-1 --kill
```

Skip the pre-remove backup:

```sh
ark-cli remove instance --instance asa-server-1 --skip-backup
```

Tear down the full compose project (stop all services + `compose down`):

```sh
ark-cli teardown instances             # prompts for confirmation in interactive shells
ark-cli teardown instances --skip-confirm   # bypass prompt (for scripts/CI)
```

## Backups and restore

Create a backup archive for a specific instance:

```sh
ark-cli create backup --instance asa-server-1
```

The archive lands in `<compose-dir>/backups/<instance>/` by default.
You can override the backup root for backup/restore/list with `ARK_BACKUP_DIR`.
Backups run inside the container; if the instance is stopped, a temporary
container is started to produce the archive.
Each backup archive includes embedded metadata (`backup-metadata.json`)
containing the server version, build ID, instance name, map name, and backup
timestamp.

Optional flags: use `--format` to choose archive format (default `tar.gz`),
`--dest` to specify an output directory, `--name` to name the archive,
and `--compose-file` to target a non-default compose file.

Retention defaults are enabled: `--retain-count 20`, `--retain-days 7`,
and `--retain-size-gb 5` (set any of these to `0` to disable that rule).
Use `--lock-timeout` to control how long backup waits for a per-instance
backup lock before failing (default `2m`), and `--backup-wait-timeout`
to control helper/archive completion wait time (default `10m`).
Retention size is treated as a hard target, but the CLI keeps at least
two backups by default (drops to one under severe disk pressure).
When free space falls below ~5 GiB, retention prunes aggressively and logs
warnings; startup may be blocked if there is not enough space to keep even
one backup for a service.

Back up all instances, then the shared cluster data:

```sh
ark-cli create backup --instance all
```

`cluster` is a reserved target for the shared cluster volume (uploaded
characters, items and dinos). It works with `create backup`, `restore backup`,
`list backups`, `prune backups` and backup schedules; archives go to
`<compose-dir>/backups/cluster/`. No instance can be named `cluster`.
See [Cluster backups](../explanation/backups.md#cluster-backups).

```sh
ark-cli create backup --instance cluster
ark-cli list backups --instance cluster
ark-cli prune backups --instance cluster
```

Restore a named backup for an instance:

```sh
ark-cli restore backup --instance asa-server-1 --archive asa-server-1-backup-YYYYmmdd-HHMMSS
```

Restore the most recent backup for an instance:

```sh
ark-cli restore backup --instance asa-server-1 --archive latest
```

Restore and automatically start the service after restore:

```sh
ark-cli restore backup --instance asa-server-1 --archive latest --auto-start
```

Restore a backup into a new instance (added to the compose file with the next free ports):

```sh
ark-cli restore backup --new-instance asa-server-2 --archive asa-server-1:latest
```

The new instance takes its admin and server passwords from the backup's
`GameUserSettings.ini`, the same ones the restored server uses. Override them with
`--admin-password`, `--server-password` or `--no-server-password`. If the backup
has no admin password, the restore stops before changing anything and asks for
`--admin-password`.

List backups for an instance:

```sh
ark-cli list backups --instance asa-server-1
```

Verify a backup archive before restore by opening the archive and running
SQLite integrity checks on the `.sqlite` files inside it:

```sh
ark-cli validate backup --instance asa-server-1 --backup latest
```

- Resolve the backup archive (`latest` or named archive)
- Extract to a temporary verification directory
- Find `.sqlite` files inside the backup payload
- Run SQLite integrity checks on each (for example `PRAGMA integrity_check`)
- Report pass/fail per file and return non-zero on integrity errors

Show metadata for a backup archive:

```sh
ark-cli show backup --instance asa-server-1 --backup latest
```

Backup archives include embedded metadata (`backup-metadata.json`) containing
the server version, build ID, instance name, map name, and backup timestamp.
Use `show backup` to view this metadata. Older archives without metadata will
display `-` for metadata fields.

Prune backups without creating a new backup (applies retention rules only):

```sh
ark-cli prune backups --instance asa-server-1
ark-cli prune backups --instance all
ark-cli prune backups --instance asa-server-1 --dry-run   # preview what would be removed
```

Add `--json` to `list backups` and `show backup` for machine-readable output.

Restores run in a one-shot maintenance container path (without starting the
game server), take a pre-restore backup into the backup directory, clear the
save directory, and then extract the selected archive. The shared cluster
directory (`Saved/clusters/`) is left untouched, and is skipped if an older
archive contains it.
If the archive contains embedded metadata, it is logged during restore so
you can verify which backup is being applied.
The instance must be stopped before restore. Stop it first with `ark-cli stop instance --instance <name>`, then restore.

Restore the shared cluster data (every instance that uses the cluster must be
stopped; the error names any that are running):

```sh
ark-cli restore backup --instance cluster --archive latest
```

With `cluster`, `--archive` takes a backup name or `latest` without an
`<instance>:` prefix, and `--auto-start` is rejected. A
`cluster-pre-restore-<timestamp>.tar.gz` is written first.
`validate backup` and `show backup` do not apply to cluster archives.

## Instance visibility and operations

List all compose-managed instances:

```sh
ark-cli list instances
ark-cli list instances --json   # machine-readable output
```

Show instance status and config summary:

```sh
ark-cli show instance --instance asa-server-1
ark-cli show instance --instance asa-server-1 --json
```

Reconfigure selected compose-backed instance settings:

```sh
ark-cli reconfigure instance --instance asa-server-1 \
  --tz America/New_York \
  --map-name TheCenter \
  --session-name MySession \
  --admin-password AdminPass123 \
  --server-password ServerPass123 \
  --asa-port 28015 \
  --rcon-port 28020
```

If the server is running, the command refuses to reconfigure unless you pass
`--force`, then reminds you to restart after saving. Add `--restart` together
with `--force` to restart the instance automatically after saving:

```sh
ark-cli reconfigure instance --instance asa-server-1 --map-name Aberration --force --restart
```

Without `--restart`, the command applies compose changes and exits successfully
with a reminder that restart is required for settings to take effect.
If `--restart` is provided but the instance was not running, the command applies
changes and exits successfully without starting the instance.

Control whether RCON can be reached from other machines. New instances publish
RCON on `127.0.0.1` only, so RCON tools on the Docker host work and nothing else
can connect. The CLI's own RCON use (stop, save, backup) runs inside the
container and works either way.

```sh
ark-cli enable remote-rcon --instance asa-server-1    # publish on all interfaces (IPv4 and IPv6)
ark-cli disable remote-rcon --instance asa-server-1   # back to 127.0.0.1 only
```

Both commands change only the RCON port binding in compose and take the same
`--force`, `--restart` and `--compose-file` flags as `reconfigure instance`.
Instances created before this default keep their existing binding (all
interfaces) until you run `disable remote-rcon`.

RCON sends the admin password in plain text. Ports published by Docker bypass
`ufw` and `firewalld`, so a host firewall rule does not restrict them; when
remote RCON is enabled, limit who can reach it with a rule in Docker's
`DOCKER-USER` iptables chain. For example, to allow only `203.0.113.7` to
reach an instance whose RCON host port is `28020`:

```sh
sudo iptables -I DOCKER-USER -p tcp -m conntrack --ctorigdstport 28020 ! -s 203.0.113.7 -j DROP
```

Check image updates for all instances without applying (compares each
instance's `io.ark.server.build_id` image label with Steam's current public
build):

```sh
ark-cli update instance --check
```

Build the new image for one instance and leave compose unchanged:

```sh
ark-cli update instance --instance asa-server-1
# prints: Built ark-server:<id>. Switch with: ark-cli update instance --instance asa-server-1 --rollout
```

Switch to the new image with a safe rollout (`--restart` is an alias):

```sh
ark-cli update instance --instance all --rollout
```

For each instance, in name order, the rollout stops it (saving the world),
creates a `pre-update` backup, switches compose to the new tag, starts it and
waits until ready. If the new image fails, compose is switched back, the
pre-update backup is restored, the old image is started, and the command
prints `Update of <name> to <new> failed; rolled back to <old>: <reason>`,
skips the remaining instances and exits with code 1. A failed build changes
nothing.

Edit server INI files in the container (defaults to `nano`):

```sh
ark-cli edit ini --instance asa-server-1 --file gameusersettings
ark-cli edit ini --instance asa-server-1 --file game --editor vi
```

The CLI edits the INI file inside the container so volume file ownership stays
consistent. If the server is running, the command refuses to edit unless you
pass `--force`, then reminds you to restart after saving. Add `--restart`
together with `--force` to restart the instance automatically after saving.

Print an INI file to the terminal:

```sh
ark-cli show ini --instance asa-server-1 --file game
ark-cli show ini --instance asa-server-1 --file gameusersettings
```

`--file` accepts `game` (Game.ini) or `gameusersettings` / `gus`
(GameUserSettings.ini). The command uses `cat` inside the container for a
running instance, or an ephemeral container if the instance is stopped.
A missing file is reported: `GameUserSettings.ini` appears on the instance's
first start; `Game.ini` exists only once you create it with `edit ini`.
`edit ini --file gameusersettings` refuses until the file exists.

Show volume mount paths for an instance:

```sh
ark-cli show volume-paths --instance asa-server-1
ark-cli show vol-paths --instance asa-server-1   # short alias
```

Show container logs:

```sh
ark-cli logs instance --instance asa-server-1 --tail 200 --timestamps
```

Check instance health (running + startup-ready marker):

```sh
ark-cli check instance --instance asa-server-1
```

After `result:`, the output includes `last_update: <result> at <time> (<from> -> <to>)`
(and `last_update_message:` when there is a message) for the instance's most
recent update attempt. It does not change the exit code.

Wait for an instance to reach a target state:

```sh
ark-cli wait instance --instance asa-server-1 --for started       # container running (before advertising)
ark-cli wait instance --instance asa-server-1 --for advertising   # fully advertising for join (alias: ready)
ark-cli wait instance --instance asa-server-1 --for stopped       # service exited
```

Use `--timeout` to override the default 10-minute wait (e.g. `--timeout 5m`) and
`--interval` to change the polling interval (default 2s).

Validate compose configuration (parses and validates the compose file, reports
service count and any config errors):

```sh
ark-cli validate compose
ark-cli show compose
```

Both commands accept `--compose-file` to target a non-default compose file.

## Port reachability

Check if the instance ports are reachable from the internet:

```sh
ark-cli check reachability --instance asa-server-1
```

For an instance, the CLI derives ports from compose and checks both:

- ASA game port (UDP, derived from the `ark.port.asa` compose label or port binding)
- RCON port (TCP, derived from compose port bindings, fallback `27020`)

If the instance's RCON port is published on `127.0.0.1` only (the default), the
command prints a note: RCON showing CLOSED from remote or another host is then
expected, not a fault.

It reports pass/fail for each derived port. The CLI derives the published host
ports from compose (so non-default ARK ports are supported). External checks use
`starrupture-utilities.com` first, with `portchecker.io` used as a TCP-only
fallback when needed. Instance checks require the service to be running unless
you use `--listen`.

Optional instance target and port selection:

```sh
ark-cli check reachability --instance asa-server-1 --ports asa
ark-cli check reachability --instance asa-server-1 --ports rcon
ark-cli check reachability --instance asa-server-1 --ports all
```

To check a specific port without reading compose:

```sh
ark-cli check reachability --port 27015
```

Optional manual protocol selection:

```sh
ark-cli check reachability --port 27015 --protocol tcp
ark-cli check reachability --port 27015 --protocol udp
ark-cli check reachability --port 27015 --protocol both
```

Choose where to probe from (default: both):

```sh
ark-cli check reachability --port 27015 --from local
ark-cli check reachability --port 27015 --from remote
ark-cli check reachability --port 27015 --from 10.0.0.1
ark-cli check reachability --port 27015 --from both
```

To test port forwarding without relying on an already-running local service, add
`--listen` and the CLI will open temporary local listener(s) during the
external probe:

```sh
ark-cli check reachability --port 27015 --protocol tcp --listen
ark-cli check reachability --port 27015 --protocol udp --listen
```

Notes:

- `--protocol` is port mode only (`check reachability --port <num>`).
- `--ports` is instance mode only (`check reachability --instance <name>`).
- In instance mode, `--listen` will fail if the service is already running and
  already bound to the same ports. Stop the instance or omit `--listen`.
- UDP checks in `--listen` mode send a small reply payload so external UDP
  checkers can classify the port as open.

## Listen port

Open listeners on a port and block until Ctrl+C (no probe; for firewall testing):

```sh
ark-cli listen port --port 27015
ark-cli listen port --port 27015 --protocol udp
ark-cli listen port --port 27015 --protocol both
```

## Scheduling (managed backup schedules)

Schedules are stored in a repo-local config file and a managed crontab file is
generated from that config.

Default paths:

- `.ark-manage/schedules.yaml`
- `.ark-manage/generated/schedules.crontab`
- `.ark-manage/logs/schedules/<name>.log`
- `.ark-manage/state/schedules/<name>.last.json`
- `.ark-manage/locks/schedule-<name>.lock`

Create a backup schedule:

```sh
ark-cli create schedule backup --instance asa-server-1 --name nightly-asa1 --cron "0 3 * * *"
```

`--instance` takes an instance name, `cluster` or `all` (`all` includes the
cluster), here and in `update schedule`.

List and inspect schedules:

```sh
ark-cli list schedules
ark-cli list schedules --json
ark-cli show schedule --name nightly-asa1
ark-cli show schedule --name nightly-asa1 --json
```

Enable or disable a schedule without removing it:

```sh
ark-cli enable schedule --name nightly-asa1
ark-cli disable schedule --name nightly-asa1
```

Check a schedule's running status and last result (exits non-zero if last run failed):

```sh
ark-cli check schedule --name nightly-asa1
```

Update a schedule:

```sh
ark-cli update schedule --name nightly-asa1 --cron "0 4 * * *" --retain-days 14
```

Validate one schedule or all schedules:

```sh
ark-cli validate schedule --name nightly-asa1
ark-cli validate schedule
```

Run a schedule immediately (manual test/trigger):

```sh
ark-cli run schedule --name nightly-asa1
```

Apply the generated managed crontab to your user crontab:

```sh
ark-cli apply schedules
```

Remove a schedule:

```sh
ark-cli remove schedule --name nightly-asa1
```

## Host kernel configuration

ARK: Survival Ascended via Proton/Wine requires certain kernel settings (e.g.
`vm.max_map_count`). Use these commands to check, apply, and revert them.

Check current kernel settings:

```sh
ark-cli check host-kernel-config
```

Apply required settings (persisted to `/etc/sysctl.d/`; requires root):

```sh
sudo ark-cli fix host-kernel-config
```

Revert to the pre-fix backup:

```sh
sudo ark-cli restore host-kernel-config
```
