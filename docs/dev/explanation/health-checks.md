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
