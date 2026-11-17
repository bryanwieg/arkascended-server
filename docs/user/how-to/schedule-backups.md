# Schedule Automatic Backups

How to set up automatic recurring backups for your ARK server instances.

## Create a schedule

```sh
ark-cli create schedule backup --instance my-server --name nightly-my-server --cron "0 3 * * *"
```

This creates a schedule named `nightly-my-server` that runs at 3:00 AM daily.
The cron expression uses standard 5-field format: `minute hour day month weekday`.

Common cron expressions:
- `"0 3 * * *"` — every day at 3:00 AM
- `"0 */6 * * *"` — every 6 hours
- `"0 4 * * 0"` — every Sunday at 4:00 AM

Optional retention flags:
- `--retain-count 20` — keep at most 20 backups (default)
- `--retain-days 7` — delete backups older than 7 days (default)
- `--retain-size-gb 5` — prune when backups exceed 5 GB total (default)

Set any of these to `0` to disable that particular rule.

## Apply the schedule to your crontab

Schedules are stored in a config file and need to be installed into your
user's crontab:

```sh
ark-cli apply schedules
```

Run this once after creating or updating any schedule.

## Verify the schedule

List all schedules:

```sh
ark-cli list schedules
```

Show details for a specific schedule:

```sh
ark-cli show schedule --name nightly-my-server
```

Test it immediately (runs the backup now):

```sh
ark-cli run schedule --name nightly-my-server
```

## Check schedule health

After a scheduled run, check if it succeeded:

```sh
ark-cli check schedule --name nightly-my-server
```

This exits non-zero if the last run failed, making it suitable for monitoring
scripts or health checks.

## Enable or disable a schedule

To temporarily pause a schedule without removing it:

```sh
ark-cli disable schedule --name nightly-my-server
ark-cli enable schedule --name nightly-my-server
```

## Update a schedule

Change the cron time or retention settings:

```sh
ark-cli update schedule --name nightly-my-server --cron "0 4 * * *" --retain-days 14
```

Then re-apply to update the crontab:

```sh
ark-cli apply schedules
```

## Remove a schedule

```sh
ark-cli remove schedule --name nightly-my-server
ark-cli apply schedules   # update the crontab
```

## Where backups are stored

Backups land in `<compose-dir>/backups/<instance>/` by default.
Override the backup root for all operations with the `ARK_BACKUP_DIR`
environment variable.

## Related

- `docs/user/how-to/restore-backup.md` — restore from a backup
- `docs/user/explanation/backups.md` — retention rules and safety guarantees
