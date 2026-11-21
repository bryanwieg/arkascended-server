# Health Checks and Readiness Polling

The CLI judges instance health by scanning container logs for marker strings
the ARK server prints. Host RAM is checked by the container entrypoint, not the
CLI.

## Log-marker mechanism

Two fixed markers, defined in `app/pkg/ark/health/checker.go`:

| Marker | Constant | Meaning |
|--------|----------|---------|
| `has successfully started!` | `health.StartedMarker` | Server process has launched |
| `Server has completed startup and is now advertising for join.` | `health.ReadyMarker` | Server accepts player connections |

`IsStarted` / `IsReady` call `LogsContainMarker`: fetch every log line since
`since` via `ComposeService.Logs` and substring-match (see [The `since`
filter](#the-since-filter)).

Users: `start instance` (waits for started, then advertising), `wait instance`,
`check instance`.

**Coupling:** if the ARK server changes these strings, health checks break
silently (waits time out, `check` reports `starting`). Keep the constants in
step with server output.

**300-line fallback:** only when no start time is known (no engine wired, no
container) does the search fall back to the last 300 lines.

## Poll loop

`WaitForStarted`, `WaitForReady` and `WaitForStopped` share `poll`: run the
check every interval until it returns true, the timeout expires, or the
context is cancelled.

| Caller | Timeout | Interval |
|--------|---------|----------|
| `start instance` | 5m (`--wait-timeout` / `ARK_START_WAIT_TIMEOUT`) | 2s |
| `wait instance` | 10m (`--timeout`) | 2s (`--interval`) |
| `lifecycle.Service.Start` with `WaitForReady` | 5m (`DefaultStartWaitTimeout`) | 2s (`DefaultStartWaitInterval`) |

The CLI calls `lifecycle.Service.Start` with `WaitForReady: false` and runs its
own wait to drive the progress display. With `--verbose`, `start instance`
follows the log stream for the ready marker instead of polling.

`wait instance --for` takes `started`, `advertising` (alias `ready`, the
default) or `stopped`.

### Premature exit detection

In `WaitForStarted` / `WaitForReady`, each iteration checks whether the
service is running. If not, `CheckNotExited` queries `ExitedServices`; an
exited service fails the wait at once instead of running out the timeout. An
error from `ExitedServices` is ignored (keeps polling). `WaitForStopped` only
checks the running state.

## The `since` filter

`WaitOptions.Since` limits the log search to lines after a timestamp, so a
marker from a previous run in the same container's log doesn't count. With
`since` set there is no tail limit.

When `since` is nil (`wait instance`, `check instance`,
`lifecycle.Service.Start`), the checker uses the container's start time:
`ComposeService.ServiceContainers` finds the container and
`ContainerEngine.InspectState` returns `StartedAt`. A long-running server's
marker therefore never scrolls out, and after an in-place restart the previous
run's marker doesn't match.

`start instance` passes its own `since` (now - 1s, taken before `compose up`)
for services that were not running.

## RAM headroom check

`check_memory_headroom` in `container/scripts/runtime/container-entrypoint.py`
runs at the start of the default startup workflow, so it covers every start
path: `start instance`, direct `docker compose up`, restart-policy restarts.

It compares `MemAvailable` from `/proc/meminfo` with the container's memory
reservation from cgroup `memory.low` (v2) or `memory.soft_limit_in_bytes` (v1).
Reservation unset, `0` or unlimited: no check. That includes runtimes that
don't apply `mem_reservation` to the cgroup.

Short on RAM: **warn and start anyway**. Wording depends on current memory
pressure (PSI `full avg10 > 0` in `/proc/pressure/memory`, or swap over 90%
full):

- Pressure: the server will likely be killed by the OOM killer.
- No pressure: the server may run out of memory; expected on balloon-memory
  VMs (e.g. Hyper-V Dynamic Memory), where `MemAvailable` stays low until the
  hypervisor adds memory on demand.

*Why warn, not refuse:* exiting failed the container, which
`restart: unless-stopped` restarted in an endless loop, and it refused balloon
VMs that would have supplied the memory. A pre-start check can only guess. The
definite signal comes after the fact: when the OOM killer kills the server,
`ark-server-start.py` sees the cgroup `oom_kill` counter rise and logs a
`[server-oom]` error.

*Why not in the CLI:* the container check runs on every start path; a CLI
check would only cover `start instance`, and it would read the memory of the
machine the CLI runs on, which isn't always the Docker host.

`ARK_SKIP_RAM_CHECK=1` in the instance's compose `environment` skips the check.

## Health result classification

`check instance` derives `status` (`running` or `stopped`) from the running
services, checks the ready marker only when running, and prints:

| Result | Condition | Exit |
|--------|-----------|------|
| `healthy` | Running + ready marker found | 0 |
| `starting` | Running + ready marker not found | non-zero |
| `stopped` | Not running | 0 |
| `unhealthy` | Fallback; unreachable while status is only `running`/`stopped` | non-zero |

It also prints the last update attempt (`last_update`), which never changes
the exit code.

## Scheduler monitor

`ark-cli run scheduler` (the `ark-scheduler` service) runs a monitor loop next
to the schedule loop: one round at start, then one a minute
(`app/pkg/ark/health/monitor.go`, wired in
`app/ark-cli/internal/scheduler/scheduler.go`). Each round reloads the compose
project, probes every instance in name order, checks every enabled backup
schedule, publishes a report and restarts at most one hung instance.

### Instance probe

`health.Probe.Check` (`app/pkg/ark/health/probe.go`) per instance:

| Container | State | Checks |
|---|---|---|
| none, or not running/restarting | `stopped` | none (all `skipped`) |
| `restarting` | `restarting` | `container` fails (restart loop) |
| running, not ready, under `StartGrace` (20m) since `StartedAt` | `starting` | none (all `skipped`) |
| running otherwise | `running` | `ready`, `rcon`, `udp` |

- `ready`: the ready marker since the container's `StartedAt` (cached per
  container run), or RCON answering. RCON counts because the marker may have
  left the log (rotation, journald).
- `rcon`: `ark-rcon.py listplayers` via compose exec, 15 s timeout.
- `udp`: `host.ProbeUE5UDP` to the service name on the compose network, the
  first UDP port's container side, 3 s timeout. Skipped when the service
  publishes no UDP port.

*Why stopped is skipped:* the monitor cannot tell a crash from a deliberate
stop once the container is gone; an operator who stops an instance must not
get an alert. A restart loop is still a failure.

### Busy: the instance lock

Before probing, the monitor tests the instance lock
(`backup.InstanceLockHeld`, `F_OFD_GETLK`, never taking it). A held lock means
a backup, a lifecycle command (`stop`, `start`, `restart instance`, the
`--restart` of `reconfigure`/INI commands) or an update rollout is working on
the instance: it reports `state: busy`, checks `skipped`, and is not probed.
Busy rounds neither count toward nor reset the hung-round counter.

*Why:* without it, a monitor restart could race a user's stop, a rollout or a
backup's save, and a restart in progress would read as a failure.

### Restart policy

An instance is hung when `ready` or `rcon` fails (`InstanceHealth.Hung`). A
failing `udp` alone is reported but never restarts: firewall or NAT problems
are not fixed by a restart.

- After `HungRoundsBeforeRestart` (5) consecutive hung rounds, the first such
  instance in name order is restarted; one restart per round.
- At most `MaxRestartsPerHour` (2) per instance; beyond that it logs
  `Not restarting <instance>: restart limit reached` (once an hour) and stays
  unhealthy.
- The restart takes the instance lock without waiting; held → skipped
  (`Not restarting <instance>: busy`), counter kept.
- Stop with `RestartStopTimeout` (2m; the engine SIGKILLs after it), then
  start even when the stop failed. *Why 2 minutes:* a hung server cannot save
  or reach players over RCON, and the player countdown is container env that
  one stop cannot turn off; a healthy empty server stops in ~30 s.
- A shutdown signal does not interrupt a restart (or a backup) midway.
- A failed restart is recorded with its error. While the instance stays
  stopped afterwards, its `container` check reports `fail` (`not running after
  a failed restart`) instead of `skipped`, until it has a container again.

### Schedule health

`schedule.Service.Health` (`app/pkg/ark/schedule/health.go`), per enabled
backup schedule: a held schedule lock is `ok` (running); a last run left
`running` with the lock free, a `failed` last run, or an invalid cron is
`fail`; otherwise `fail` when more than `OverdueGrace` (1h) past the next run
after the latest of the last start, the scheduler's start and the
`schedules.yaml` modification time (so a schedule enabled or edited after a
long pause is not overdue).

### Report, HTTP endpoint and healthcheck

The report (`health.Report`) uses maps so Gatus JSON paths address each value
(`[BODY].instances.<name>.checks.rcon`); check values are `ok`, `fail`,
`skipped`; `restarts` holds the last hour's automatic restarts and is always an
array. `healthy` is false when any check or schedule fails or the compose
project could not be read (`error`). Shape and Gatus example:
[Schedule automatic backups](../../user/how-to/schedule-backups.md#monitoring).

| Consumer | Source | Unhealthy when |
|---|---|---|
| `GET /health` on container port 8099 (`health.Handler`), published by `enable scheduler --health-port` (default 8099) | the monitor's in-memory latest report | `healthy: false` in the body (status stays 200); 503 when no report yet or older than `ReportStaleAfter` (5m) |
| `ark-cli check scheduler` | `<compose dir>/.ark-manage/state/scheduler/health.json`, written atomically each round | exit 1 when missing, older than 5m, or `healthy: false` |
| Docker healthcheck | `check scheduler` every 1m (timeout 30s, retries 1, start period 3m) | as `check scheduler`; Komodo shows it |

*Why the status code ignores `healthy`:* each Gatus endpoint watches one
instance; a 503 for any failure would fail all of them. 503 is kept for "the
monitor itself is down or stuck". `ReportStaleAfter` leaves room for a restart
round (2m stop, start, probes).

The monitor logs `Health: healthy` or `Health: unhealthy: <check>: <detail>;
...` only when the set of failing checks changes, so the service log does not
repeat a line every minute.
