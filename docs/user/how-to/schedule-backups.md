# Schedule Automatic Backups

Schedules live in a config file; `apply schedules` writes them to your
user's crontab. Re-run it after every create, update, enable, disable or
remove.

## Create

```sh
ark-cli create schedule backup --instance my-server --name nightly-my-server --cron "0 3 * * *"
ark-cli apply schedules
```

- `--cron`: standard 5-field expression.
- `--instance`: an instance, `cluster` (shared cluster data only) or `all`
  (every instance plus the cluster).
- `--format`: `tar`, `tar.gz` (default) or `tar.zst`.
- `--disabled`: create without scheduling it.

Retention, applied after each run (`0` disables a rule):

| Flag | Default |
|---|---|
| `--retain-count` | 20 |
| `--retain-days` | 7 |
| `--retain-size-gb` | 5 |

The newest backups are always kept; see
[Understanding backups](../explanation/backups.md).

## Verify and test

```sh
ark-cli list schedules
ark-cli show schedule --name nightly-my-server
ark-cli run schedule --name nightly-my-server     # run now
ark-cli check schedule --name nightly-my-server   # non-zero exit if the last run failed
```

## Change

```sh
ark-cli update schedule --name nightly-my-server --cron "0 4 * * *" --retain-days 14
ark-cli disable schedule --name nightly-my-server
ark-cli enable schedule --name nightly-my-server
ark-cli remove schedule --name nightly-my-server
ark-cli apply schedules
```

## Storage

Default `<compose-dir>/backups/<instance>/`. Override with `ARK_BACKUP_DIR`.

## Related

- [Restore a backup](restore-backup.md)
- [Understanding backups](../explanation/backups.md)
