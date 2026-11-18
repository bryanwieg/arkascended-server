# Restore a Backup

## Restore an instance

The instance must be stopped; restore refuses a running one.

```sh
ark-cli stop instance --instance my-server
ark-cli list backups --instance my-server
ark-cli restore backup --instance my-server --archive latest
ark-cli start instance --instance my-server
```

`--archive` takes `latest` or a backup name
(`my-server-backup-20260101-030000`). `--auto-start` starts the instance
when the restore finishes.

Steps:

1. Pre-restore backup of the current saves, so the restore can be undone.
2. Extract into `Saved/.restore-staging/` on the same volume. On failure
   (for example, out of space) the current saves are untouched. The volume
   needs room for an uncompressed copy of the backup.
3. Swap the restored files in. Everything in `Saved/` is replaced except
   the shared `Saved/clusters/`.

## Restore into a new instance

```sh
ark-cli restore backup --new-instance my-server-copy --archive my-server:latest
```

Creates the instance, then restores into it. Passwords default to the ones in
the backup; override with `--admin-password`, `--server-password` or
`--no-server-password`.

`--archive <src-instance>:<backup>` also restores another instance's backup
into an existing one.

## Restore cluster data

Restoring the shared cluster data rolls back transfers for every instance on
the cluster. Background:
[Cluster backups](../explanation/backups.md#cluster-backups).

```sh
ark-cli stop instance --instance all
ark-cli list backups --instance cluster
ark-cli restore backup --instance cluster --archive latest
```

- Refused while any instance using the cluster is running; the error names
  them.
- Saves the current data first as `cluster-pre-restore-<timestamp>.tar.gz`
  in `backups/cluster/`.
- `--archive`: a backup name or `latest`, no instance prefix.
- No `--auto-start`; start the instances afterwards.

## Inspect a backup first

```sh
ark-cli validate backup --instance my-server --backup latest   # SQLite integrity check per file; non-zero exit on errors
ark-cli show backup --instance my-server --backup latest       # metadata
```

## Related

- [Schedule backups](schedule-backups.md)
- [Understanding backups](../explanation/backups.md)
