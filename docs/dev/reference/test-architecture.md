# Test Architecture

## Test layers

| Layer | Location | Scope |
|---|---|---|
| Unit | `tests/container/unit/` | Pure Python parsing, validation, and transform logic; no Docker |
| Container integration (fast) | `tests/container/integration/fast/` | Container runtime scripts, short-lived containers; `@pytest.mark.container` |
| Container integration (slow) | `tests/container/integration/slow/` | Full container lifecycle (start, RCON, backup, restore); `@pytest.mark.container` |
| CLI integration (fast) | `tests/app/ark_cli/integration/docker/fast/` | CLI output checks, compose manipulation, fake API servers; `@pytest.mark.container` |
| CLI integration (slow) | `tests/app/ark_cli/integration/docker/slow/` | End-to-end CLI workflows against running containers; `@pytest.mark.container` |
| CLI integration podman (fast) | `tests/app/ark_cli/integration/podman/fast/` | Same tests as docker/fast/ re-exported for Podman runtime; not in `run-tests.py` batches |
| CLI integration podman (slow) | `tests/app/ark_cli/integration/podman/slow/` | Same tests as docker/slow/ re-exported for Podman runtime; not in `run-tests.py` batches |
| Web integration (fast) | `tests/app/ark_web/integration/fast/` | `ark-web` HTTP endpoints against a locally built binary (`web_binary` fixture runs `go build`); `@pytest.mark.container`; not in `run-tests.py` batches |
| CLI integration native | `tests/app/ark_cli/integration/native/` | CLI commands that need no container runtime (e.g. port checks); no `@pytest.mark.container` |
| CLI contract (Go) | `app/ark-cli/*_test.go` and `app/ark-cli/internal/*/*_test.go` | Command result, output invariants, and state effects; uses seam injection via `WithPatchedAppDeps` |
| Domain service (Go) | `app/pkg/ark/*/*_test.go` | Domain service unit tests; no Docker, uses interface mocks |
| Backend conformance (Go) | `app/pkg/ark/conformance/` | Shared tests both backends must pass; behind `integration` build tag |

### Fast vs slow tests

**Fast** tests (`tests/*/integration/fast/`) exercise compose file manipulation, CLI output validation, and short-lived container operations without starting a full ARK server. They run in seconds.

`test_server_oom_report.py` needs only `python3.14` and `tail`, not the ARK image, so PR CI runs it on hosted runners with `ARK_TEST_IMAGE=python:3.14-slim`. *Why:* the ARK image is too big to build or pull per PR, and a real OOM kill needs a runtime that enforces cgroup memory limits, which hosted runners do. It skips locally when the limit isn't applied (e.g. rootless Docker without memory delegation) and fails in CI, so a runner change can't make it pass silently.

**Slow** tests (`tests/*/integration/slow/`) require a running ARK server container and exercise full lifecycle behaviors — startup, RCON connectivity, backup/restore, shutdown. They run in minutes.

Run only fast tests for quick feedback:
```sh
pytest tests/**/integration/fast/
# or
./run-tests.py --suite fast
```

Run slow tests for full verification:
```sh
pytest tests/**/integration/slow/
# or
./run-tests.py --suite slow
```

### Slow test protection

Slow tests (those in `tests/*/integration/slow/`) use a lock mechanism to ensure
only one runs at a time, preventing RAM exhaustion. ARK server instances require
~12GB RAM each, and the machine only has enough RAM for 2 instances — concurrent
slow tests would cause OOM and false test failures.

#### Lock mechanism

- **Lock file:** `.tests/.lock` containing `<PID>|<timestamp>|<test-name>`
- **On test start:** acquires lock via `acquire_lock()`; skips if already held by an alive process
- **On test end (always):** runs compose down (via the python-on-whales SDK) and releases the lock in a `finally:` block
- **Stale locks:** detected by checking if the PID in the lock file is still alive (`/proc/{pid}`); stale locks are removed and the lock is re-acquired

#### Usage

All slow tests must include the `slow_test_protection` fixture as an explicit
parameter:

```python
def test_my_slow_test(
    test_env: dict[str, str],
    slow_test_protection,  # Required for slow tests
    started_container_instance,
):
```

The fixture is defined in `tests/container/integration/conftest.py` and
re-exported to CLI tests via `tests/app/ark_cli/integration/conftest.py`. It only
activates for tests in a `slow` directory — fast tests are unaffected.

#### Lock functions (in `tests/helpers/infra.py`)

| Function | Purpose |
|---|---|
| `acquire_lock(lock_path, test_name)` | Try to acquire lock; returns `True`/`False`; handles stale locks |
| `release_lock(lock_path)` | Remove the lock file |
| `cleanup_compose_files(dc, compose_file)` | Run compose down (via python-on-whales SDK) — `--volumes --remove-orphans` |
| `release_lock_and_cleanup(dc, lock_path, compose_file)` | Convenience for `finally:` blocks — cleanup then release |

### Container vs CLI test boundary

Container tests exist to verify that the container-side scripts and processes
work correctly **in isolation**, by direct invocation — the same way you would
test a backend API without routing requests through the frontend. The CLI is an
untrusted intermediary in this context: if a container test fails, the failure
must point to the container script, not to a CLI regression that passed wrong
arguments, generated a bad compose file, or changed how it issues compose
operations via the Go SDK. Introducing the CLI into container tests obscures which
layer broke and creates false coupling between components that are architecturally
independent.

**`tests/container/integration/fast/`** and **`tests/container/integration/slow/`** exercise
container-side scripts (entrypoint, shutdown manager, backup/restore) by sending
Docker commands directly (via the python-on-whales SDK: `compose_stop`, `compose_exec`). The
trigger is always a Docker primitive, never an `ark-cli` command. Compose files
are generated via `write_single_service_compose` (with `extra_env` for
non-standard configurations). Container tests must not depend on `cli_path` or
`ark-cli`.

**`tests/app/ark_cli/integration/docker/fast/`** and **`tests/app/ark_cli/integration/docker/slow/`** exercise
CLI commands (`ark-cli stop instance`, `ark-cli create instance`, etc.) and
verify their end-to-end effects on containers. **`tests/app/ark_cli/integration/podman/fast/`**
and **`tests/app/ark_cli/integration/podman/slow/`** re-export the same test functions for
the Podman runtime. The CLI conftest re-exports
container fixtures so both suites share infrastructure. It is expected that
container side tests pass, and container side processes are healthy before
running CLI tests.

## Outcome-driven testing

Default style for new work:

- Assert user-visible behavior, state transitions, and persisted side effects.
- Avoid asserting internal helper call ordering unless it is the explicit
  contract being tested.
- Prefer one strong contract test over multiple implementation-detail tests.
- Keep seam-mocked tests focused and minimal; do not expand seam-level
  coverage by default.

When adding commands or features:

1. Add contract-level tests first (command result + output + state effects).
2. Add integration tests that exercise real workloads — real Docker builds,
   real API calls, real container lifecycle. If a feature touches Docker images,
   external APIs, or container behavior, it needs `@pytest.mark.container`
   integration tests that run against real containers. Do not rely on
   tests that substitute fakes/mocks for real Docker operations — they have
   repeatedly passed while real container runs failed, providing false
   confidence.
3. Add seam/fallback tests only when risk requires branch-specific proof.

## Go CLI tests: the seam pattern

`AppDeps` groups side-effecting functions and domain services into injectable
fields. Container orchestration goes through `ark.ContainerEngine` and
`ark.ComposeService` interfaces (with `arktest.MockCompose` for tests —
`MockCompose` provides 18 function fields (one per `ComposeService` method) so tests
can simulate any compose interaction without a real Docker daemon. See
`app/pkg/ark/arktest/mocks.go` for the full list.).
Domain services (`backup.Service`, `health.InstanceHealthChecker`,
`lifecycle.Service`, `schedule.Service`, `update.Service`) are wired as
interface or pointer fields. Tests swap individual functions or services via
`testutil.WithPatchedAppDeps`:

```go
testutil.WithPatchedAppDeps(t, testutil.TestDepsConfig{}, func(d *core.AppDeps) {
    d.ComposeService = &arktest.MockCompose{}
    d.Runtime.RunCommand = func(cmd []string, _ ark.CommandOptions) (ark.ExecResult, error) {
        // capture or stub
        return ark.ExecResult{ExitCode: 0}, nil
    }
}, func() {
    // exercise the command under test
})
```

Favor unit tests for parsing, validation, and compose edits. Use
`@pytest.mark.container` integration tests for any behavior that involves Docker
images, external APIs, container lifecycle, or real builds. Contract tests
with seam injection verify CLI logic quickly; `@pytest.mark.container` tests
confirm the feature works against real infrastructure.

### Go contract test coverage

Package-level Go contract tests in `app/ark-cli/internal/<package>/*_test.go`
exercise command handlers through `WithPatchedAppDeps` with mock compose
services and filesystem fixtures — no Docker/Podman required. These are
distinct from the top-level tests in `app/ark-cli/*_test.go` and from the
Python native tests in `tests/app/ark_cli/integration/native/` (which invoke the CLI
as a subprocess).

Commands with Go contract test coverage:

| Package | Commands | Key seams |
|---|---|---|
| `schedule/` | `enable schedule`, `disable schedule`, `check schedule`, `show schedule`, `update schedule`, `run schedule`, `create schedule`, `remove schedule`, `validate schedule`, `apply schedules`, `list schedules` | `overrideSchedulePathsForTest`, `writeScheduleFixture`, `RunBackupFromSchedule`, `InstallManagedSchedulesCrontab` |
| `health/` | `check instance`, `wait instance`, `validate compose` | `MockCompose.RunningServicesFn`, `MockHealthChecker.IsReadyFn`, `MockHealthChecker.WaitFor*Fn`, compose file on disk |
| `show/` | `show instance`, `list instances`, `show compose`, `show volume-paths`, `show backup` | `MockCompose.LoadProjectFn`, `MockEngine.ImageLabelsFn`, `MockEngine.VolumeMountpointFn`, `RunningServiceSnapshot`, `HostDeps.FetchText` |
| `backup/` | `create backup`, `validate backup`, `prune backups`, `restore backup` | `BackupService` function fields (`DiskUsage`, `LookPath`), `MockCompose.ExecFn`, `MockCommandRunner.RunFn`, `ARK_BACKUP_DIR` env var |
| `remove/` | `remove instance`, `teardown instances` | `MockCompose.DownFn`, `MockCompose.KillFn`, `RemoveVolume`, `IsInteractiveTerminal` |
| `compose/` | `start instance`, `stop instance`, `down instance` | Local `withPatchedAppDeps` (import cycle prevents `testutil` import), `MockCompose.UpFn`, `MockCompose.StopFn` |
| `host/` | `check kernel-config`, `check ports` | `HostDeps.FetchText`, `HostDeps.CheckUDPPort`, `HostDeps.CheckTCPPort` |
| `reconfigure/` | `reconfigure instance` | `MockCompose.LoadProjectFn`/`SaveProjectFn`, `Runtime.RunCommand` |
| `ini/` | `edit ini` | Filesystem fixtures in `t.TempDir()` |
| `create/` | `create instance` | `MockCompose.LoadProjectFn`/`SaveProjectFn` |
| `update/` | `check update`, `update image` | `UpdateService.CheckForUpdateFn`, `UpdateService.BuildImageFn`, `HostDeps.FetchText` |

## Running tests

See [Run tests](../how-to/run-tests.md) for runner commands, timing
expectations, rebuild steps, and the CI vs. local split.

## Environment variables affecting tests

- `ARK_START_WAIT_TIMEOUT` — extend start readiness timeout (e.g. `15m`) when
  the host is under memory pressure.
- `ARK_SKIP_RAM_CHECK` is a container variable, not a shell one: test
  helpers set it to `1` in the compose files they create
  (`enable_skip_ram_check`), which silences the entrypoint's low-RAM warning
  in test server logs. Setting it in your shell has no effect.
- `ARK_TEST_IMAGE` — override the default `ark-server:latest` image used by
  integration tests.
- `ARK_TEST_ARTIFACTS_DIR` — override the directory for test artifacts
  (default `.tests`).
- `ARK_TEST_ARTIFACTS_RETENTION` — days to keep test artifacts (default `7`).
- `ARK_TEST_ARTIFACTS_WIPE_THRESHOLD` — minimum artifact runs to keep when
  pruning (default `3`).
- `TEST_BATCH_PYTHON` — override the default `.venv/bin/python` used by the
  test runner.

## Integration test framework

Shared test infrastructure lives in `tests/helpers/` (nine modules):

| Module | Purpose |
|---|---|
| `artifacts` | Paths, CLI binary discovery, report artifacts |
| `compose` | Port allocation, compose YAML generation, compose file patching, `asa_port_from_compose` |
| `compose_client` | `python-on-whales` `DockerClient` factory, high-level compose helpers (`container_running`, `container_exited`, `wait_for_container_state`, `compose_exec`), V2 startup |
| `diagnostics` | `run_cmd`, command logging, failure formatting, phase snapshots |
| `infra` | `wait_for`, `ensure_runtime_prereqs`, `prepare_test_env`, `cleanup_test_env`, `build_container_instance_config`, Docker env setup/teardown, lock management |
| `fakes` | `steam_api_server`, `dockerhub_tags_server` — fake HTTP servers for testing without real network calls |
| `framework_logger` | Per-test `framework.log` tracing, debug stderr output, smart auto-enable for slow tests |
| `batch_runner` | Batch test execution, test selection, and result collection |
| `compose_cleanup` | Orphan compose file pruning and stale lock removal during project teardown |

Key fixtures (defined in `tests/container/integration/conftest.py`, re-exported by `tests/app/ark_cli/integration/conftest.py`):
- `test_env` — env dict with `COMPOSE_FILE`, `ARK_TEST_COMMANDS_LOG`, etc.
- `run_cmd` — wraps subprocess with command logging and compose patching
- `wait_for` — poll with timeout
- `container_instance_config` — creates a compose service definition and returns
  config (instance name, passwords, compose path) without starting the container.
  Use this when the test needs to control startup itself (e.g., first-boot init
  tests, pre-seeding state before start).
- `started_container_instance` — calls `container_instance_config`, then starts
  the container and waits for RCON readiness. Use this when the test needs a
  fully running server as a precondition.

> **Fixture scope note:** All container integration fixtures are currently
> function-scoped — each test gets its own container. Module-scoped fixtures
> (allowing tests that share a lifecycle to reuse one container, e.g., start and
> stop tests) are planned but not yet implemented. Do not add module-scoped
> fixtures until the feature is completed.

`tmp_path` is overridden in `tests/conftest.py` to return `<test_artifact_dir>/tmp`
under `.tests/` instead of pytest's `/tmp` directory, for the same reason: it
must be safe to bind-mount into a container. It is kept with the test's other
artifacts and pruned by the artifact retention.

Enforcement hooks in `tests/conftest.py` fail (raise `pytest.fail()`) on violations of Rules 1–9 (except `native/` tests, which are exempt). Rules 10–11 emit warnings instead:
- Raw `subprocess.run`/`Popen` in integration test files (use `run_cmd` instead)
- Missing `@pytest.mark.container` marker on integration tests
- CLI integration tests using `tmp_path` without `test_env` — prefer `test_env` for consistent environment setup; add a comment if `tmp_path` alone is intentional
- Direct `os.environ` access in integration test files — use `test_env.get()` or `dict(test_env)` instead for consistent environment setup
- In-place mutation of `test_env` dict — use `env = dict(test_env); env[KEY] = VALUE` to avoid state leakage between tests
- Hardcoded `["docker", ...]` or `["podman", ...]` command lists (single-line or multi-line) — use `compose_client` helpers instead
- Container tests importing `cli_path` — container tests must exercise scripts directly via Docker primitives, never through `ark-cli`
- Slow tests missing `slow_test_protection` fixture — prevents OOM from concurrent slow tests
- `tempfile` in integration test files (`TemporaryDirectory`, `mkdtemp`, `gettempdir`) — use `test_artifact_dir` or `tmp_path`. *Why:* `tempfile` dirs live in `/tmp`, which a remote or rootless Docker daemon may not see, so bind mounts from them come up empty and root-owned
- Session-scoped fixtures in `conftest.py` files (Rule 11, warning) — prefer function scope unless the fixture returns an immutable value or performs idempotent setup

These hooks catch the most common ways tests bypass the framework. Do not
work around them — if a hook fires, fix the test or extend the helpers. The
test helpers in `tests/helpers/` are the single source of truth for command
execution, environment setup, and Docker interaction. If the helpers don't
support something you need, add to the helpers rather than inlining raw
subprocess or environment manipulation in a test file.

The canonical reference for writing new tests is
`tests/test_baseline_framework.py`.

## Test artifacts

Each test node directory under `.tests/<run-timestamp>/<test-node>/` contains:

- `commands.log` — all `run_cmd` invocations with exit codes, stdout, stderr
- `compose.<service>.log` — per-service streaming log, captured continuously from fixture setup through teardown
- `docker.log` — container-level diagnostics (state, image, PID) from just before teardown
- `failure.log` — docker-level state captured immediately on test failure (only written on failures)
- `framework.log` — framework-level trace (poll, lock, lifecycle, compose events, SDK calls)
- `pytest.log` — pytest phase outcomes with captured stdout/stderr
- `cli.log` — captured `ark-cli` output when the CLI was invoked by the test (via `ARK_TEST_CLI_LOG`)

For detailed logging documentation, see [test-logging.md](test-logging.md).
