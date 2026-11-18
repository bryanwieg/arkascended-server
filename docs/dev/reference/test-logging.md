# Test logging architecture

How the test framework (`tests/conftest.py`, `tests/helpers/`) records diagnostics
for post-mortem analysis. For the CLI's own output channels see
[logging-architecture.md](../explanation/logging-architecture.md).

## Artifact directory structure

Root is `.tests/` (override `ARK_TEST_ARTIFACTS_DIR`; retention variables in
[run-tests.md](../how-to/run-tests.md#environment-variables)).

| Run mode | Test-node directory |
|---|---|
| `run-tests.py` | `.tests/<YYMMDD-HHMMSS>-suiterun/<batch-label>/<test-node>/` |
| `pytest`, exactly one test collected | `.tests/<YYMMDD-HHMMSS>-singlerun/<test-node>/` |
| `pytest`, several tests collected | `.tests/<YYMMDD-HHMMSS>-suiterun/<test-node>/` |

- `<batch-label>`: the batch label from `run-tests.py` (e.g. `scripts-server-oom-report`).
  `run-tests.py` passes the run dir and label to each pytest batch via `ARK_TEST_RUN_DIR`
  and `ARK_TEST_BATCH_LABEL`, and writes `suite.log` (per-batch status and duration) at the run root.
- `<test-node>`: node ID with `/` and `::` replaced, e.g.
  `tests_container_integration_slow_test_ark_server_start_script.py__test_ark_server_start_script`.
- A name collision within the same second appends `-2`, `-3`, ...

Test-node directory contents:

| File | Contents | Written by |
|---|---|---|
| `framework.log` | Framework trace (see [Logger](#logger-helpersframework_logger)) | `log_framework()`; configured by `configure_test_artifacts` in `tests/conftest.py` |
| `pytest.log` | Phase outcomes with captured stdout/stderr | `write_pytest_report_artifact()` (`helpers/artifacts.py`), at teardown |
| `commands.log` | `run_cmd` invocations (START, exit code, stdout, stderr), CLI heartbeats, phase snapshots | `helpers/diagnostics.py` |
| `cli.log` | `ark-cli` verbose log; `run_with_compose_patch` adds `-v <cli.log>` unless the command sets `-v` | `ark-cli` |
| `compose.<service>.log` | Streamed service output, one per service | Watchers in `helpers/compose_client.py` |
| `docker.log` | Container state snapshots (name, id, image, created, status, running, exit code, PID) | `capture_diagnostics()` |
| `pytest-compose-<id>.yaml` | The test's compose file (`COMPOSE_FILE`), one per test | `prepare_test_env()` (`helpers/infra.py`) |
| `tmp/` | The test's `tmp_path` (replaces pytest's `/tmp` one so the daemon can bind-mount it) | `tests/conftest.py` |

Files appear only if the test exercises them (e.g. no `compose.*.log`, `docker.log` or
compose YAML for tests without `test_env`).

## Logger: `helpers.framework_logger`

Collects test-infrastructure events. Exists only in the test process; unrelated to
the CLI's `slog` channel.

```python
from helpers.framework_logger import log_framework

log_framework("poll", "waiting for container (elapsed=%ds)", elapsed)
```

### Output

Appends to `framework.log` in the current test's artifact directory, flushed per line.
No stderr output, no level or env var switch. Calls made outside a configured test
are dropped.

### Categories

The first argument is a free-form category string. Categories in use:

| Category | Source | Emitted for |
|---|---|---|
| `framework` | `conftest.py`, `framework_logger.py` | Test start/finish, logger configured, failure diagnostics captured |
| `cleanup` | `helpers/infra.py` | Orphaned resource pruning, stale lock removal |
| `lock` | `helpers/infra.py` | Slow-test lock acquire/skip/release |
| `compose` | `helpers/compose.py`, `helpers/compose_client.py` | Compose file edits (ports, restart policy, test-suite label), watcher start/stop |
| `container` | `helpers/compose_client.py` | Watcher saw its container stop (expected) or exit unexpectedly, with exit code |
| `lifecycle` | `helpers/compose_client.py` | Container start, startup-log and RCON readiness |
| `poll` | `helpers/infra.py`, `helpers/compose_client.py` | `wait_for`, `wait_for_container_state`: start, progress, done, TIMEOUT |
| `sdk` | `helpers/compose_client.py` | python-on-whales calls (see [SDK operation logging](#sdk-operation-logging)) |
| `cli-heartbeat` | `helpers/diagnostics.py` | `ark-cli` command run via `run_cmd` still running after 15s, every 15s |
| `phase` | `helpers/diagnostics.py` | `log_phase_snapshot()` in multi-step tests |

### Format

```
[HH:MM:SS.mmm] [category] message
```

```
[21:47:10.443] [lifecycle] waiting for startup log (instance=pytest-ark-ae9fb2d6, timeout=300s)
[21:47:10.443] [poll] waiting for startup log readiness (timeout=300s)
[21:47:10.603] [poll] waiting for startup log readiness (elapsed=0s, remaining=299s)
```

## Per-service log watchers

The autouse `log_streamer` fixture (`tests/container/integration/conftest.py`, also
pulled in by `tests/app/ark_cli/integration/conftest.py`) streams each compose service's
output to `compose.<service>.log`.

1. At setup, reads service names from the compose YAML and starts a watcher per service:
   ```
   <runtime> compose -f <compose_file> logs --follow --timestamps --tail=all <service>
   ```
   `<runtime>` is `ARK_CONTAINER_RUNTIME` (default `docker`). If the subprocess exits
   within 2s (no container yet), no watcher is kept.
2. A daemon thread copies the output to the log file and to stderr (captured by pytest
   into `pytest.log`; live with `pytest -s`).
3. When the subprocess exits, the thread checks `_EXPECTED_STOP` and logs a `[container]`
   stopped or exited-unexpectedly line with the exit code.
4. A discovery poll every 15s starts watchers for services added mid-test (e.g. by
   `create instance`). Exited watchers are never restarted.
5. `container_start` and `start_container_instance` clear the expected-stop flag and attach
   a watcher after `compose up`; `cleanup_test_env` marks services as expected stops before
   `compose down`.
6. At teardown, all watcher process groups get SIGTERM, then SIGKILL after 5s.

### Shell over SDK

Watchers use `subprocess.Popen` instead of python-on-whales: the SDK's
`follow=True, stream=True` iterator blocks until the container stops and cannot be
terminated cleanly in teardown. A subprocess has a PID (process group) to kill. This
is the one exception to the "prefer SDK" rule in [test-architecture.md](test-architecture.md).

## Diagnostics capture: `helpers.diagnostics`

### `capture_diagnostics(dc, compose_file, output_path)`

Appends a timestamped `container states` block from `compose ps --all` to `docker.log`
in `output_path`'s directory. The file name part of `output_path` is ignored. Log content
comes from the `compose.<service>.log` files, not from here.

Callers:
- `slow_test_protection` fixture (slow tests): after the test, before compose down; sets
  `_ARK_DIAGNOSTICS_CAPTURED` so `cleanup_test_env` skips a second capture.
- `cleanup_test_env()`: normal teardown.
- Failure hook (below).

### `fail_with_container_logs(reason, *, dc, instance, ...)`

Raises `AssertionError` with the last 200 compose log lines and the `cli.log` path.
Used when a container exits before startup-log or RCON readiness.

## Failure diagnostics hook

When the call phase fails, `pytest_runtest_makereport` in `tests/conftest.py`:

1. Gets `test_env` from the test's fixtures (skipped if the test has none or its compose file is missing)
2. Calls `capture_diagnostics()` with `failure.log` as the output path
3. Logs `[framework] captured failure diagnostics to <path>`

This records container state before teardown removes the containers. Because
`capture_diagnostics()` ignores the file name, the snapshot is appended to `docker.log`,
and `failure.log` is never created. On a failed test, `docker.log` therefore has two
blocks: one from the failure hook, one from teardown.

## Active compose file

`prepare_test_env()` writes `pytest-compose-<8 hex>.yaml` into the test-node directory
and sets `COMPOSE_FILE` to it. Every SDK and `ark-cli` compose operation in the test uses
this file, so it shows the final state. No snapshots of earlier states are kept. If it is
missing, the test failed before `prepare_test_env()` finished.

## SDK operation logging

python-on-whales wrappers in `helpers/compose_client.py` log an `[sdk]` line before the call:

| Function | Log format |
|---|---|
| `compose_exec` | `compose execute <service>: <cmd>` |
| `compose_run` | `compose run <service>: <cmd>` |
| `compose_stop` | `compose stop <service> (timeout=<n>s)` |
| `compose_kill` | `compose kill <service> (signal=<sig>)` |
| `container_start` | `compose start <service>` |

Other SDK calls (e.g. `compose up` in `start_container_instance`, `compose down` in
cleanup) are not logged as `[sdk]`.

## Troubleshooting

### `compose.<service>.log` missing or empty

- No compose services defined: check the `pytest-compose-*.yaml` in the test-node directory.
- `log_streamer` did not run: the test is outside `tests/container/integration/` and
  `tests/app/ark_cli/integration/`.
- No container when the watcher started and none appeared before teardown: look for
  `[compose] started watcher for <service>` in `framework.log`. A container that lives
  less than 15s between discovery polls can be missed.
- The container exited early: `docker.log` has the exit state; the log holds what was
  captured before exit.

### `framework.log` empty or missing

- `configure_test_artifacts` did not run: the test is not under `tests/`.
- The file could not be opened: check permissions on the artifact directory.

## Related documentation

- [Logging architecture (CLI side)](../explanation/logging-architecture.md): ark-cli output channels
- [Test architecture](test-architecture.md): test layers, fixture reference
- [Run tests](../how-to/run-tests.md): running suites, artifact env vars
- [Conventions](conventions.md#logging-and-cli-output): logging rules
