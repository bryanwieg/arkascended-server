# Restore a Backup

How to restore your ARK server from a backup archive.

## Before you restore

The instance must be stopped before restoring. Attempting a restore on a
running instance will be refused.

### Safety gate

Restore requires the container to be started with `ARK_MAINTENANCE_MODE=restore`.
If this variable is not set, the restore script exits immediately with an error.
This prevents accidental restore over a live instance. When using
`ark-cli restore backup`, the container entrypoint detects the restore script
path and sets `ARK_MAINTENANCE_MODE=restore` automatically. If running restore
out-of-band, set it manually:

```yaml
environment:
  - ARK_MAINTENANCE_MODE=restore
```

```sh
ark-cli stop instance --instance my-server
```

## Restore the most recent backup

```sh
ark-cli restore backup --instance my-server --archive latest
```

The CLI will:
1. Take a pre-restore safety backup (in case you need to undo the restore).
2. Extract the selected archive next to the current saves, in
   `Saved/.restore-staging/`. If this fails (for example, the volume runs out
   of space), the current saves are left unchanged. The volume needs room for
   an uncompressed copy of the backup while the restore runs.
3. Swap the restored saves into place, replacing everything in the save
   directory except the shared cluster directory (`Saved/clusters/`), which
   other instances also use.

## Restore a specific backup

List available backups first:

```sh
ark-cli list backups --instance my-server
```

Then restore by name:

```sh
ark-cli restore backup --instance my-server --archive my-server-backup-20260101-030000
```

## Restore and automatically start the server

Add `--auto-start` to bring the server back up immediately after the restore
completes:

```sh
ark-cli restore backup --instance my-server --archive latest --auto-start
```

## Verify a backup before restoring

To check that a backup archive is not corrupted before restoring:

```sh
ark-cli validate backup --instance my-server --backup latest
```

This opens the archive, finds the `.sqlite` files inside, and runs
SQLite integrity checks on them. It reports pass/fail per file and exits
non-zero on errors.

## Show backup metadata

```sh
ark-cli show backup --instance my-server --backup latest
ark-cli show backup --instance my-server --backup my-server-backup-20260101-030000
```

## After the restore

Start the server:

```sh
ark-cli start instance --instance my-server
```

Check that it comes up cleanly:

```sh
ark-cli check instance --instance my-server
```

## Related

- `docs/user/how-to/schedule-backups.md` — set up automatic backups
- `docs/user/explanation/backups.md` — how retention and safety guarantees work
