# Understanding Backups

How backups work, what gets saved, and how retention rules protect your data.

## What gets backed up

Each backup is an archive of the `ShooterGame/Saved/` directory from the
server's Docker volume. This contains:

- The world save file (`.ark`) — a binary file that holds all world
  data: map state, structures, items, and dinos. (SQLite integrity checks
  apply to `.sqlite` profile/tribe databases.)
- Player profile files (`.arkprofile`) and tribe files (`.arktribe`)
- Config files under `Config/WindowsServer/` (`Game.ini`, `GameUserSettings.ini`)

Server logs (`Saved/Logs/`) are not included: the running server writes to them
continuously, which would make the archive step fail, and they are not needed
to restore a world.

Cluster data (`Saved/clusters/`) is not included either, and a restore never
touches it. That directory is a separate volume shared by every instance in the
cluster, so including it would let restoring one server roll back the cluster
transfers (uploaded characters, items and dinos) of all the others. Cluster data
currently has no backup of its own; if you need one, back up the cluster volume
(`ark-cluster-1`) separately.

## Backup formats

Backups can be created in three formats, controlled by `--format`:

| Format | Extension | Notes |
|---|---|---|
| `tar` | `.tar` | Uncompressed; fastest to create and restore |
| `tar.gz` | `.tar.gz` | Good compression; default |
| `tar.zst` | `.tar.zst` | Better compression than gzip; slightly slower |

## Where backups are stored

Backups land in `<compose-dir>/backups/<instance>/` by default. Set
`ARK_BACKUP_DIR` to use a different root location for all backup operations.

## Retention rules

The backup system applies three independent retention rules by default:

| Rule | Default | What it does |
|---|---|---|
| `--retain-count` | 20 | Keeps at most this many backups |
| `--retain-days` | 7 | Deletes backups older than this many days |
| `--retain-size-gb` | 5 | Prunes oldest backups when the total exceeds this size |

All three rules apply together. The oldest backups are deleted first when any
rule is triggered. Set a rule to `0` to disable it.

## The safety floor

Regardless of your retention settings, the system always keeps at least two
backups. This prevents a combination of aggressive retention settings and disk
pressure from leaving you with no recovery point.

Under extreme disk pressure (very low free space), this floor may drop to one
backup, and the startup process may block with a warning if there is not enough
space to keep even that.

## Pre-restore backup

When you run `ark-cli restore backup`, the system automatically takes a
pre-restore backup before clearing and extracting the new one. This means you
can always undo a restore by restoring again from the most recent backup (which
will be the pre-restore snapshot).

## Backup validation

Before restoring, you can verify a backup archive is intact and its database
files are not corrupted:

```sh
ark-cli validate backup --instance my-server --backup latest
```

This extracts the archive, finds the `.sqlite` files (tribe and profile databases), and runs
SQLite integrity checks on them. It exits non-zero if any file fails.

## Backing up a stopped server

If the server is stopped when `create backup` is run, a temporary container
starts to produce the archive and then exits. No need to start the full server.

That temporary container is only used when the server is known to be stopped.
It can't trigger an in-game save, so if a backup of a *running* server fails
(for example, the save is never confirmed), `create backup` reports the error
rather than copying files the server may still be writing. The same applies
when the CLI can't tell whether the server is running.

## Related

- `docs/user/how-to/schedule-backups.md` — schedule automatic backups
- `docs/user/how-to/restore-backup.md` — restore from a backup

## How saves are confirmed before backup

Before archiving, the backup script forces an in-game save via RCON
`saveworld` and waits for the server to acknowledge it in the log. This avoids
backing up a partially-written save.

### Only a save made after the request counts

Before issuing `saveworld`, the script records the current end of the server
log. Only a `World Save Complete` line written after that point confirms the
backup, so an earlier autosave can't stand in for a save that failed. The
shutdown script confirms its final save the same way.

### Confirmation polling

After `saveworld` is issued, the script polls the log lines written since the
recorded position for the world-saved marker. Within each confirmation attempt, it polls
for up to `BACKUP_SAVE_CONFIRM_TIMEOUT_SECONDS` (default `60`) with
`BACKUP_SAVE_CONFIRM_POLL_SECONDS` (default `2`) intervals. If the marker is not
found, the entire confirmation is retried up to
`BACKUP_SAVE_CONFIRM_MAX_RETRIES` times with `BACKUP_SAVE_CONFIRM_RETRY_DELAY_SECONDS`
delay between attempts.

### RCON retry

The `saveworld` RCON command itself can fail transiently. It is retried up to
`BACKUP_RCON_SAVE_MAX_RETRIES` times with
`BACKUP_RCON_SAVE_RETRY_DELAY_SECONDS` delay between attempts.

See [configuration.md](../reference/configuration.md) for the full list of
related env vars and their defaults.
