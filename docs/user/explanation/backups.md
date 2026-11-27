# Understanding Backups

What a backup contains, how archives are named, and how retention, locks and
disk-space checks behave.

## What gets backed up

An instance backup is an archive of `ShooterGame/Saved/` from the instance's
volume:

- `SavedArks/<map>/`: the world save (`.ark`), player profiles
  (`.arkprofile`) and tribe files (`.arktribe`)
- `Config/WindowsServer/` (`Game.ini`, `GameUserSettings.ini`)

Excluded:

- `Saved/Logs/`: the running server writes to it constantly, which makes
  `tar` fail; not needed to restore a world.
- `Saved/clusters/`: a separate volume shared by every instance in the
  cluster. Including it would let restoring one server roll back the cluster
  transfers (uploaded characters, items, dinos) of all the others. An instance
  restore never touches it. It has its own target; see
  [Cluster backups](#cluster-backups).

## Cluster backups

The shared cluster volume (`ark-cluster-1` in the default compose file) holds
everything players have uploaded for transfer between servers. Back it up with
the reserved target `cluster`:

```sh
ark-cli create backup --instance cluster
```

- Archives land in `<backup-root>/cluster/`, named
  `cluster-backup-<timestamp>.<ext>`. Formats, `--name`, `--dest`, locks and
  retention work as for instances.
- `--instance all` backs up every instance, then the cluster (if the compose
  file mounts one).
- One cluster volume per compose file. If instances mount different volumes
  at `Saved/clusters`, the cluster backup fails.
- Runs in a one-off container whether servers are running or not, with no
  world save: ARK writes a cluster file when a player uploads or downloads,
  not on world save. (Our understanding of ARK's behavior; not confirmed for
  every case.) If a file changes while being read, `tar` is retried up to three
  times, then the backup fails rather than keep a mixed copy.
- Restoring the cluster rolls back transfers for every instance that shares
  it, so all of them must be stopped first. See
  [Restore the cluster data](../how-to/restore-backup.md#restore-cluster-data).
- `validate backup` and `show backup` apply to instance backups only. A
  cluster backup is checked once, right after it is written, by listing the
  archive.
- `cluster` is reserved: no instance can be named `cluster`.

## Formats and naming

`--format` (default `tar.gz`):

| Format | Notes |
|---|---|
| `tar` | Uncompressed; fastest |
| `tar.gz` | Default |
| `tar.zst` | zstd level 9: about a quarter smaller than gzip, and faster |

| Archive | Name |
|---|---|
| Regular | `<target>-backup-<YYYYMMDD-HHMMSS>.<ext>` (UTC) |
| `--name <n>` | `<target>-<n>-<YYYYMMDD-HHMMSS>.<ext>` |
| Pre-restore | `<instance>-pre-restore-<YYYYMMDD-HHMMSS>.tar.gz` |
| Pre-update | `<instance>-pre-update-<YYYYMMDD-HHMMSS>.tar.gz` |

`--backup latest` (and retention) order archives by the timestamp at the
end of every name, so a fresh pre-restore or pre-update archive counts as the
newest, not the oldest.

## Where backups are stored

Default root: `<compose-dir>/backups/`, one subdirectory per target
(`<instance>` or `cluster`). `ARK_BACKUP_DIR` replaces the root for all backup
commands. `--dest` on `create backup` writes the archive (and applies
retention) in another directory; the lock file stays under the root.

## Retention rules

Retention runs after every `create backup` (on the directory the archive was
written to) and on `prune backups` (`--dry-run` to preview). It covers every
archive in that directory named `<target>-*.tar[.gz|.zst]`, including
pre-restore, pre-update and custom-named ones.

| Flag | Default | Removes |
|---|---|---|
| `--retain-days` | 7 | Backups older than N days (file mtime) |
| `--retain-count` | 20 | Backups beyond the newest N |
| `--retain-size-gb` | 5 | Oldest backups until the total is at most N GiB |

`0` disables a rule. Rules apply in the order above, then a free-space rule:
if free space in the backup directory would still be under 5 GiB, the oldest
backups are removed until it would reach 5 GiB. Every rule removes oldest
first, so untimestamped archives go before regular backups.

## The safety floor

No rule removes the last two backups. When free space in the backup directory
is under 5 GiB, the floor drops to one, so the newest backup can be replaced.
If a rule can't be met because of the floor (size limit, free-space target),
retention stops and logs a warning instead of deleting more.

## Disk-space check at start

`start instance` checks free space at the backup root before starting:

| Free space | Result |
|---|---|
| 5 GiB or more | Start |
| 1 to 5 GiB | Warning, start |
| Under 1 GiB | Refuse if any instance being started has no backup yet |
| 0 | Refuse |

## Locks

`create backup` takes a per-target lock on the file
`<backup-root>/<target>/.<target>-backup.lock` for the whole run including
retention. A second backup of the same target waits up to `--lock-timeout`
(default 2m; 30m for scheduled backups), then fails. Restores (instance and
cluster) and `prune backups` take the same lock without waiting: they fail at
once if a backup is running.

The same per-instance lock is held by `stop`, `start` and `restart instance`,
by each instance's `update instance --rollout`, and by the `--restart` of
`reconfigure instance` and the INI commands. These wait up to 15 minutes for
it (`Waiting for backup lock: <instance>`); `stop instance --kill` never waits.
So a backup, a lifecycle command and the scheduler's automatic restart never
run on one instance at the same time.

The locks are kernel file locks: the kernel releases them when the holding
process exits, even when it crashes or is killed, so there is never a stale
lock to remove. The lock file stays on disk; it only shows who held it last.
They work between the host and the `ark-scheduler` container, which shares the
backup directory.

## Pre-restore backup

`restore backup` first archives the instance's current `Saved/` as
`<instance>-pre-restore-<timestamp>.tar.gz` in the same directory as the
archive being restored, then clears and extracts. To undo a restore, restore
that file by name. `--backup latest` does not pick it while timestamped
backups exist (see [Formats and naming](#formats-and-naming)), and retention
prunes it before regular backups.

## Backup validation

Every new instance archive is checked inside the container right after it is
written: the archive is listed and the SQLite-backed world files under
`SavedArks/` get an integrity check. A failed check is retried
(`BACKUP_VERIFY_MAX_RETRIES`).

To check an existing archive on the host before restoring:

```sh
ark-cli validate backup --instance my-server --backup latest
```

This extracts the archive and runs `PRAGMA integrity_check` on each `.sqlite`
file (needs `sqlite3` on the host). It fails if any check fails or the archive
contains no `.sqlite` files.

## Backing up a stopped server

If the instance is stopped, `create backup` runs the backup script in a
temporary container from the same service, then removes it. The full server
does not start.

The temporary container can't trigger an in-game save, so it is used only
when the instance is known to be stopped. If a backup of a running server
fails (for example, the save is never confirmed), or the CLI can't tell
whether the instance is running, `create backup` fails rather than copy files
the server may still be writing.

## How saves are confirmed before backup

On a running server the backup script sends RCON `saveworld` and waits for
`World Save Complete` in the server log before archiving, so a partly written
save is never archived.

- The script records the end of the log before sending `saveworld`. Only a
  marker written after that point counts, so an earlier autosave can't stand
  in for a failed save. The shutdown script confirms its final save the same
  way.
- Each confirmation attempt polls for up to
  `BACKUP_SAVE_CONFIRM_TIMEOUT_SECONDS` (default 60) every
  `BACKUP_SAVE_CONFIRM_POLL_SECONDS` (default 2). A missed marker retries the
  whole confirmation (`BACKUP_SAVE_CONFIRM_MAX_RETRIES`,
  `BACKUP_SAVE_CONFIRM_RETRY_DELAY_SECONDS`).
- A failed `saveworld` RCON call is retried (`BACKUP_RCON_SAVE_MAX_RETRIES`,
  `BACKUP_RCON_SAVE_RETRY_DELAY_SECONDS`).

All defaults: [configuration.md](../reference/configuration.md).

## Related

- [Schedule automatic backups](../how-to/schedule-backups.md)
- [Restore a backup](../how-to/restore-backup.md)
