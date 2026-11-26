# Schedule Automatic Backups

Schedules live in `docker-compose.yaml`, under its `x-ark-schedules:` block;
the schedule commands edit that block and leave the rest of the file alone.
The `ark-scheduler` compose service runs them and watches the health of your
instances. The commands use `./docker-compose.yaml` (or `COMPOSE_FILE`); pass
`--compose-file <path>` for another one. Run state, logs and locks are under
`.ark-manage/` in the compose file's directory.

## Enable the scheduler

```sh
ark-cli enable scheduler
```

Run it once. It builds the local image `ark-scheduler:<hash>` from the
`ark-cli` binary you ran, adds the `ark-scheduler` service to
`docker-compose.yaml` and starts it. It runs with your instances' compose
project, so tools that show the project (Komodo, `docker compose ps`) show
it too; `list instances` and `--instance all` ignore it.

Rerun `ark-cli enable scheduler` after upgrading `ark-cli`: it rebuilds the
image and recreates the service. `check scheduler` warns when the scheduler
runs an image from an older binary.

Upgrading from an `ark-cli` that used `apply schedules`: that command is gone.
`enable scheduler` removes the schedule block the old `ark-cli` wrote to your
crontab and leaves your other crontab entries alone.

Upgrading from an `ark-cli` that kept schedules in `.ark-manage/schedules.yaml`:
that file is no longer read, and `enable scheduler` warns while it exists.
Recreate each schedule with `ark-cli create schedule`, then delete the file.

Before enabling, check two things:

- Every `${VAR}` that `docker-compose.yaml` uses must be set in `.env` next to
  it. The scheduler does not see your shell's environment, so an automatic
  restart would otherwise recreate an instance with other values. Do not use
  `HOME`, `TMPDIR`, `TZ`, `DOCKER_HOST`, `COMPOSE_FILE`,
  `COMPOSE_PROJECT_NAME`, `ARK_CONTAINER_RUNTIME` or `ARK_BACKUP_DIR` in
  `${VAR}` references: the scheduler sets these itself, whatever `.env` says.
- `docker-compose.yaml` must be a regular file in that directory, not a link
  to a file elsewhere: the scheduler mounts only the compose directory.

To remove the service: `ark-cli disable scheduler`. Schedules stay in
`docker-compose.yaml` but do not run.

## Create

```sh
ark-cli create schedule backup --instance my-server --name nightly-my-server --cron "0 3 * * *"
```

- `--cron`: standard 5-field expression, or a descriptor such as `@daily`.
  `TZ=`/`CRON_TZ=` prefixes and `@every` are refused.
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

## Schedule RCON commands, saves and restarts

Three action schedules work on running instances. They take `--instance`
(an instance or `all`; not `cluster`), `--name`, `--cron`, `--disabled` and
`--compose-file` like a backup schedule, but no `--format` or `--retain-*`.

```sh
ark-cli create schedule rcon --instance all --name hourly-chat --cron "0 * * * *" --command "ServerChat Backup in 5 minutes"
ark-cli create schedule save --instance my-server --name save-my-server --cron "*/30 * * * *"
ark-cli create schedule restart --instance my-server --name nightly-restart --cron "0 5 * * *"
```

- `rcon` sends `--command` over RCON. The server's reply is written to the
  run log, `.ark-manage/logs/schedules/<name>.log`. Change the command with
  `ark-cli update schedule --name hourly-chat --command "..."`.
- `save` sends `saveworld`.
- `restart` restarts the instance as `ark-cli restart instance` does. With
  players online the stop first counts down for the instance's
  `SHUTDOWN_WARNING_MINUTES` (see [Configuration](../reference/configuration.md)),
  naming `scheduled restart` as the reason, then saves.

An instance that is stopped when the schedule runs is skipped with
`Warning: <instance> is stopped; <type> command unsuccessful`; a restart
schedule does not start it. When every target is stopped, the run records
`skipped`; `check schedule` and the scheduler's health treat that as a
successful run, so a stopped instance does not make the schedule fail.

## When schedules run

- The scheduler rereads `docker-compose.yaml` every minute, so a new or changed
  schedule applies within a minute. Nothing else to run.
- Times are in the scheduler's `TZ`: the `TZ` of the instance whose name
  sorts first in `docker-compose.yaml` when you ran `enable scheduler`, else
  your shell's `TZ`, else UTC. After changing it, rerun `enable scheduler`.
- Runs that were missed while the scheduler was down run once when it starts
  again (not once per missed run). A schedule that never ran is not caught up.
- Schedules run one at a time. A scheduled backup waits up to 30 minutes for an
  instance that a stop, start, restart or update holds.

## Verify and test

```sh
ark-cli list schedules
ark-cli show schedule --name nightly-my-server
ark-cli run schedule --name nightly-my-server     # run now
ark-cli check schedule --name nightly-my-server   # non-zero exit if the last run failed
docker compose logs ark-scheduler                 # START/SUCCESS/FAIL lines of each run
```

## Change

```sh
ark-cli update schedule --name nightly-my-server --cron "0 4 * * *" --retain-days 14
ark-cli disable schedule --name nightly-my-server
ark-cli enable schedule --name nightly-my-server
ark-cli remove schedule --name nightly-my-server
```

## Check health

The scheduler checks each instance and schedule once a minute and writes the
result to `.ark-manage/state/scheduler/health.json`.

```sh
ark-cli check scheduler          # table of checks; exit 1 when unhealthy
ark-cli check scheduler --json   # the report as JSON
```

`check scheduler` also exits 1 when the report is missing or older than 5
minutes (the scheduler is down or stuck). It is the service's Docker
healthcheck, so `docker compose ps` shows `ark-scheduler` as `healthy` or
`unhealthy`.

What is checked, per instance:

| Check | Fails when |
|---|---|
| `container` | The container is in a restart loop, cannot be inspected, or was left stopped by a failed automatic restart. |
| `ready` | The server has not logged that it is advertising for join 20 minutes after its container started, and RCON does not answer. |
| `rcon` | RCON `ListPlayers` does not answer within 15 s. |
| `udp` | The game port does not answer within 3 s. |

An instance's `state` is `running`, `starting` (not ready yet, at most
its first 20 minutes), `stopped`, `restarting` or `busy` (a backup, stop,
start, restart or update holds its lock). `stopped`, `starting` and `busy` report all checks
as `skipped`, so an instance you stopped on purpose is not a failure; the one
exception is `container: fail` on an instance a failed automatic restart left
stopped.

A schedule fails when its last run failed, a run did not finish, or it is more
than one hour overdue.

## Monitoring

The scheduler serves the report over HTTP:

```sh
curl http://<host>:8099/health
```

`enable scheduler --health-port <port>` publishes it on another host port;
`--health-port 0` does not publish it. Without the flag a rerun keeps the
current port. The endpoint has no authentication and shows state only (names,
check results, error text); restrict it with your firewall if needed.

Response:

```json
{
  "time": "2026-11-21T03:00:00Z",
  "healthy": false,
  "instances": {
    "my-server": {
      "state": "running",
      "checks": {"container": "ok", "ready": "ok", "rcon": "fail", "udp": "ok"},
      "details": {"rcon": "Error: RCON command timed out"}
    }
  },
  "schedules": {"nightly-my-server": "ok"},
  "schedule_details": {},
  "restarts": [{"instance": "my-server", "time": "2026-11-21T02:55:00Z", "reason": "rcon"}]
}
```

- Check values are `ok`, `fail` or `skipped`; schedule values `ok` or `fail`.
- `details` and `schedule_details` hold the reason for each failure.
- `restarts` lists the automatic restarts of the last hour; `error` is set on
  a restart that failed.
- `error` (top level) is set when the compose project could not be read.
- Status `200` while the report is fresh, whatever `healthy` says, so one bad
  instance does not fail every per-instance check. Status `503` when there is
  no report yet or it is older than 5 minutes.

[Gatus](https://github.com/TwiN/gatus) example, one endpoint per instance and
one for schedules and restarts:

```yaml
endpoints:
  - name: my-server
    group: ark
    url: http://ark-host:8099/health
    interval: 1m
    conditions:
      - "[STATUS] == 200"
      - "has([BODY].instances.my-server) == true"
      - "[BODY].instances.my-server.checks.container != fail"
      - "[BODY].instances.my-server.checks.ready != fail"
      - "[BODY].instances.my-server.checks.rcon != fail"
      - "[BODY].instances.my-server.checks.udp != fail"
  - name: ark scheduler
    group: ark
    url: http://ark-host:8099/health
    interval: 1m
    conditions:
      - "[STATUS] == 200"
      - "[BODY].schedules.nightly-my-server != fail"
      - "len([BODY].restarts) == 0"
```

Tools that watch Docker health (Komodo, Uptime Kuma) show `ark-scheduler`
unhealthy whenever any check fails. `ark-cli` sends no alerts itself.

## Automatic restart

The scheduler restarts an instance whose `ready` or `rcon` check failed in 5
rounds in a row (about 5 minutes). A failing `udp` check is reported but never
restarts. The restart is a stop and a start:

- Never while the instance is `busy` (its lock is held by a backup, stop,
  start, restart or update); those rounds do not count.
- Never while the instance is `starting` (until it logs ready, at most its
  first 20 minutes).
- At most one instance per minute, and at most 2 restarts per instance per
  hour. After that it stays unhealthy until you act.
- The stop is cut off after 2 minutes (the engine kills the container): a
  hung server cannot save anyway.
- If the restart fails and leaves the instance stopped, its `container` check
  stays `fail` until it runs again.

## Storage

Default `<compose-dir>/backups/<instance>/`. Override with `ARK_BACKUP_DIR`
(set it before `enable scheduler`; the scheduler mounts that directory). A
relative `ARK_BACKUP_DIR` is resolved against the compose directory by
`enable scheduler` but against the current directory by host commands such as
`run schedule`; set it absolute to avoid the difference.

## Related

- [Restore a backup](restore-backup.md)
- [Understanding backups](../explanation/backups.md)
- [CLI commands: schedules and scheduler](../reference/cli-commands.md#schedules)
