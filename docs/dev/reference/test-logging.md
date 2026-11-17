# Test logging architecture

How the integration test framework captures and preserves diagnostic
information for post-mortem analysis. This doc covers the logging
infrastructure that lives **within the test suite itself** (`tests/helpers/`
and `tests/conftest.py`), not the CLI's output channels (see
[logging-architecture.md](../explanation/logging-architecture.md)).

## Artifact directory structure

Every test run produces a directory under `.tests/`. The path depends on how
the run was invoked:

| Run mode | Artifact root | Example |
|---|---|---|
| `run-tests.py` (batch) | `.tests/<YYMMDD-HHMMSS>-suiterun/<batch-stamp>-<batch-label>/<test-node>/` | `.tests/260504-211605-suiterun/211605-scripts-server-oom-report/tests_container_integration_fast_test_server_oom_report.py__test_server_start_reports_oom_kill/` |
| `pytest` (single test) | `.tests/<YYMMDD-HHMMSS>-singlerun/<test-node>/` | `.tests/260504-220412-singlerun/tests_container_integration_slow_test_ark_server_start_script.py__test_ark_server_start_script/` |

Each test-node directory contains:

```
<test-node-dir>/
├── compose.<service>.log   # Per-service streaming log (one per compose service)
├── docker.log              # Container-level diagnostics (state, ports, image)
├── framework.log           # Framework-level trace (polls, locks, lifecycle)
├── pytest.log              # pytest phase report
├── pytest-compose-*.yaml   # The active compose file used by the SDK
└── failure.log             # Diagnostics captured at failure time (only on test failures)
```

### When logs are captured

| Log | Capture point | Captured by |
|---|---|---|
| `compose.<service>.log` | Streamed continuously by per-service watchers — start to teardown | `helpers/compose_client.py` |
| `docker.log` | `capture_diagnostics()` — in cleanup_test_env or slow_test_protection | `helpers/diagnostics.py` |
| `framework.log` | Every `log_framework()` call — poll loops, locks, compose ops | `helpers/framework_logger.py` |
| `failure.log` | `pytest_runtest_makereport` hook — immediately on test call failure | `tests/conftest.py` |
| `pytest-compose-*.yaml` | Created by `prepare_test_env()` — the SDK's active compose file | `helpers/infra.py` |

## Logger: `helpers.framework_logger`

The framework logger is the single point of collection for all
test-infrastructure events. It is **not** the same as the CLI's operational
event channel (`slog.Logger`) — it exists only inside the test process.

```python
from helpers.framework_logger import log_framework

log_framework("poll", "waiting for container (elapsed=%ds)", elapsed)
```

### Output channels

- **Always** — appends to `framework.log` in the test's artifact directory
- **When `ARK_TEST_DEBUG=1`** (default for `slow/` tests) — also writes to
  stderr, visible with `pytest -s` or in failure reports

### Categories

Every `log_framework` call has a category string that prefixes the log line:

| Category | Source module | When emitted |
|---|---|---|
| `framework` | `conftest.py`, `framework_logger.py` | Test start/end, logger configuration |
| `cleanup` | `helpers/infra.py` | Orphan pruning, stale lock removal |
| `lock` | `helpers/infra.py` | Slow-test lock acquire/release |
| `compose` | `helpers/compose.py`, `helpers/compose_client.py` | Compose file writing, port assignment, log streamer start/stop |
| `lifecycle` | `helpers/compose_client.py` | Container startup, RCON readiness |
| `poll` | `helpers/infra.py`, `helpers/compose_client.py` | `wait_for` and `wait_for_container_state` |
| `sdk` | `helpers/compose_client.py` | Every python-on-whales SDK call (up, down, exec, stop, kill) |
| `cli-heartbeat` | `helpers/diagnostics.py` | Long-running CLI commands (>15s) |
| `phase` | `helpers/diagnostics.py` | Phase snapshots during multi-step tests |

### Format

```
[HH:MM:SS.mmm] [category] message
```

Example:
```
[21:47:10.443] [lifecycle] waiting for startup log (instance=pytest-ark-ae9fb2d6, timeout=300s)
[21:47:10.443] [poll] waiting for startup log readiness (timeout=300s)
[21:47:10.603] [poll] waiting for startup log readiness (elapsed=0s, remaining=299s)
```

### Configuration

Logger state is set up per-test by the `configure_test_artifacts` autouse
fixture in `tests/conftest.py`:

```python
ARK_TEST_DEBUG=1          # Force debug output (stderr + log file)
ARK_TEST_DEBUG=0          # Suppress stderr, log file only
ARK_TEST_DEBUG=<unset>    # Auto: enabled for slow/ tests, disabled for fast/
```

## Per-service log watchers

The `log_streamer` autouse fixture (in `tests/container/integration/conftest.py`)
attaches per-service watcher threads that continuously stream container logs to
`compose.<service>.log` in the test artifact directory. Watchers also detect
container exits and log them to `framework.log`.

### How it works

1. At fixture setup, the watcher infra discovers service names from the compose
   YAML file.
2. For each service, it starts a shell subprocess:
   ```
   docker compose -f <compose_file> logs --follow --timestamps --tail=all <service>
   ```
3. A daemon thread reads the subprocess output (tee: log file + stderr).
4. When the subprocess exits (container stopped/removed), the thread checks
   ``_EXPECTED_STOP`` to distinguish planned stops from crashes and logs the
   event and exit code to ``framework.log``.
5. A discovery poll runs every 15s to attach watchers to services created
   mid-test (e.g., by ``create instance``). It never restarts watchers for
   dead containers.
6. On teardown, all watcher subprocesses are killed (SIGTERM → SIGKILL).
7. Lifecycle helpers (``compose_stop``, ``compose_kill``, ``container_start``,
   ``start_container_instance``) manage the ``_EXPECTED_STOP`` flag so that
   framework-originated stops are logged cleanly.

### Shell over SDK

The watcher deliberately uses shell subprocess (``subprocess.Popen``) instead
of the python-on-whales SDK. The SDK's ``follow=True, stream=True`` blocks
forever on the subprocess read pipe and cannot be cleanly terminated in
fixture teardown — the iterator never returns until the container stops.
Shell subprocesses give clean PIDs for SIGTERM/SIGKILL, and the SDK is a
thin wrapper around the same CLI anyway. This is the one exception to the
"prefer SDK" rule in the test architecture.

## Diagnostics capture: `helpers.diagnostics`

### `capture_diagnostics(dc, compose_file, output_path)`

Captures container-level state in one call (the per-service streaming logs in
``compose.<service>.log`` are the authoritative source for log content):

1. **`docker.log`** — container-level detail for each service (name, id,
   image, status, running, exit code, PID)

Called from:
- `cleanup_test_env()` — normal teardown
- `slow_test_protection` fixture — before compose down (with
  `_ARK_DIAGNOSTICS_CAPTURED` marker to prevent double-capture)
- Failure hook in `pytest_runtest_makereport` — on test failure (writes
  `failure.log`)

### `fail_with_container_logs(reason, dc, instance, ...)`

Raises `AssertionError` with container logs appended. Used in readiness
polling to surface the container state at the moment of failure.

## Failure diagnostics hook

When a test call fails (`report.when == "call"` and `not report.passed`),
the `pytest_runtest_makereport` hook in `tests/conftest.py`:

1. Reads `test_env` from the fixture request to locate the compose file
2. Creates a `DockerClient` from the compose file
3. Calls `capture_diagnostics()` into `failure.log`
4. Logs a `[framework] captured failure diagnostics` entry

This captures container state **at the moment of failure**, before teardown
has a chance to destroy it. For full per-service log content, check the
``compose.<service>.log`` files in the same artifact directory.

## Active compose file

The test's active compose file is created by `prepare_test_env()` and stored
at the path in `COMPOSE_FILE`. It is the file the python-on-whales SDK uses
for all compose operations. The file persists in the test's artifact directory
as `pytest-compose-*.yaml` — no snapshot copies are needed since the SDK
always reads from the same file path.

## SDK operation logging

Every python-on-whales SDK call in `helpers/compose_client.py` emits a
`[sdk]` log entry via `log_framework`:

| Function | Log format |
|---|---|
| `compose_exec` | `compose execute <service>: <cmd>` |
| `compose_run` | `compose run <service>: <cmd>` |
| `compose_stop` | `compose stop <service> (timeout=<n>s)` |
| `compose_kill` | `compose kill <service> (signal=<sig>)` |

This provides a trace of every Docker operation the test framework performs,
even if the operation fails silently (e.g., missing compose project).

## No stdout pollution

Watcher and discovery-poll progress is written **only** through `log_framework`
— it goes to `framework.log` and optionally to stderr controlled by
`ARK_TEST_DEBUG`. There are no bare `print()` calls in any watcher or framework
helper.

## Troubleshooting

### "compose.\<service\>.log is missing or empty"

Possible causes:
- The test never created a compose file with services (no containers were started).
  Check if the compose file exists in the artifact directory.
- The `log_streamer` fixture did not run. This only happens for tests outside
  the ``tests/container/integration/`` or ``tests/app/ark_cli/integration/`` trees
  that do not inherit the container integration conftest.
- The streamer subprocess failed to start. Check ``framework.log`` for
  ``[compose] started log streamer`` entries. If missing, the YAML service
  discovery may have returned an empty list (check the compose file format).
- The container exited before the streamer could capture output. In this case,
  check ``docker.log`` for container exit state, and the per-service file
  will have whatever output was captured before exit.

### "framework.log is empty"

Possible causes:
- The `configure_test_artifacts` autouse fixture did not run. This only happens
  for tests outside the `tests/` directory tree or tests that import their own
  conftest without inheriting from `tests/conftest.py`.
- The logger failed to open the file. Check permissions on the artifact
  directory.

### "I see duplicate poll messages in pytest output"

The `print()` calls in `tests/helpers/infra.py` and
`tests/helpers/compose_client.py` have been removed — poll loop output now
flows only through `log_framework`. If you still see poll messages in stdout,
they are coming from `sys.stderr` when `ARK_TEST_DEBUG=1` (default for slow
tests). Set `ARK_TEST_DEBUG=0` to suppress stderr output.

### "The compose file snapshot is missing the start state"

The compose file (`pytest-compose-*.yaml`) is the single source of truth —
it is the file the SDK reads for all compose operations. If it is missing,
the test crashed before `prepare_test_env()` completed. No additional snapshot
copies are maintained.

## Related documentation

- [Logging architecture (CLI side)](../explanation/logging-architecture.md) —
  how the ark-cli binary emits output (not test-framework specific)
- [Test architecture](test-architecture.md) — integration test layers, fixture
  reference, artifact directory layout
- [Conventions](conventions.md#logging-and-cli-output) — coding rules for
  logging discipline
