# Health Checks and Readiness Polling

The CLI determines instance health by scanning container logs for specific marker
strings emitted by the ARK server process. Host RAM is checked by the container
entrypoint, not the CLI.

## Log-marker mechanism

`check instance` and `wait instance` work by reading recent Docker Compose logs and
searching for two fixed marker strings:

| Marker | Constant | Meaning |
|--------|----------|---------|
| `has successfully started!` | `health.StartedMarker` | Server process has launched |
| `Server has completed startup and is now advertising for join.` | `health.ReadyMarker` | Server is accepting player connections |

`IsStarted` scans for the started marker; `IsReady` scans for the ready marker. Both use
`LogsContainMarker`, which fetches the last 300 log lines via `ComposeService.Logs` and
checks for the marker substring.

**Coupling:** If the container-side ARK server output changes these strings, health
checks break silently. Any change to server log output must be coordinated with the
marker constants in `app/pkg/ark/health/checker.go`.

## Poll loop

`WaitForStarted`, `WaitForReady`, and `WaitForStopped` all use a `poll` function that
calls a check function at a configurable interval until it returns true, the timeout
expires, or the context is cancelled.

Default values:
- `lifecycle` package: start wait timeout 5 minutes (`DefaultStartWaitTimeout`), start wait interval 2 seconds (`DefaultStartWaitInterval`), graceful stop timeout 15 minutes (`DefaultStopTimeout`)
- `wait instance` CLI: `--timeout` default is 10 minutes (overrides the package default when the user does not pass `--timeout`)

The `wait instance` CLI command exposes `--timeout` and `--interval` flags.

### Premature exit detection

During `WaitForStarted` and `WaitForReady`, each poll iteration first checks whether
the service is still running. If the container has exited, `CheckNotExited` queries
`ExitedServices` and fails immediately rather than waiting for timeout.

## The `since` parameter

`WaitOptions.Since` filters log lines to only those after a given timestamp. This
prevents matching stale markers from a previous server run. The `lifecycle.Start`
method sets `Since` to the time just before `compose up`, so only logs from the current
startup are considered.

## RAM headroom check

The container entrypoint (`check_memory_headroom` in `container-entrypoint.py`)
checks host RAM on every container start, so it also covers direct
`docker compose up` and restart-policy restarts. It compares `MemAvailable` from
`/proc/meminfo` against the container's memory reservation, read from cgroup
`memory.low` (v2) or `memory.soft_limit_in_bytes` (v1). If the reservation reads
as unset or `0`, it says nothing. That includes runtimes that don't apply
`mem_reservation` to the cgroup.

When RAM is short it **warns and starts anyway**. The wording depends on whether
the host is already under memory pressure (PSI `full avg10 > 0` in
`/proc/pressure/memory`, or swap over 90% full):

- Under pressure: the server will likely be killed by the OOM killer.
- No pressure: the server may run out of memory, which is expected on
  balloon-memory VMs (e.g. Hyper-V Dynamic Memory), where `MemAvailable` stays
  low until the hypervisor adds memory on demand.

*Why a warning, not a refusal:* exiting failed the container, which
`restart: unless-stopped` then restarted in an endless loop, and it refused
balloon VMs that would have supplied the memory. A pre-start check can only
guess anyway. The definite signal comes after the fact: when the kernel's OOM
killer kills the server, `ark-server-start.py` sees the cgroup `oom_kill`
counter rise and logs a `[server-oom]` error.

The CLI does no RAM check. *Why:* the container check runs on every start path,
while a CLI check only covers `start instance`. The CLI also reads the memory of
the machine it runs on, which isn't always the Docker host.

`ARK_SKIP_RAM_CHECK=1` in the instance's compose `environment` silences the
warning.

## Health result classification

`check instance` returns one of four states:

| Result | Condition |
|--------|-----------|
| `healthy` | Container running + ready marker found |
| `starting` | Container running + ready marker not yet found |
| `stopped` | Container not running |
| `unhealthy` | None of the above |

The command exits 0 for `healthy` or `stopped`, non-zero for `starting` or `unhealthy`.