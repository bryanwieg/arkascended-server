# Test Architecture

How to run tests: [Run tests](../how-to/run-tests.md). Log files and diagnostics:
[Test logging](test-logging.md).

## Test layers

| Layer | Location | Scope |
|---|---|---|
| Unit | `tests/container/unit/` | Python logic of container scripts; no container runtime; `@pytest.mark.unit` |
| Container integration (fast) | `tests/container/integration/fast/` | Container scripts in short-lived containers; `@pytest.mark.container` (plus `lite` to run on the [lite image](#lite-image-no-ark-server)) |
| Container integration (slow) | `tests/container/integration/slow/` | Full container lifecycle (start, RCON, backup, restore, stop); `@pytest.mark.container` |
| CLI integration (fast) | `tests/app/ark_cli/integration/docker/fast/` | CLI output, compose edits, fake API servers, lifecycle on the [lite image](#lite-image-no-ark-server); `@pytest.mark.container` (plus `lite`) |
| CLI integration (slow) | `tests/app/ark_cli/integration/docker/slow/` | End-to-end CLI workflows against running servers; `@pytest.mark.container` |
| CLI integration Podman | `tests/app/ark_cli/integration/podman/{fast,slow}/` | The `docker/` tests re-exported (`import *`) under `ARK_CONTAINER_RUNTIME=podman`; not in `run-tests.py` batches |
| CLI native | `tests/app/ark_cli/integration/native/` | CLI as a host subprocess, no container runtime; no `container` marker |
| Web integration | `tests/app/ark_web/integration/fast/` | `ark-web` HTTP endpoints; `web_binary` fixture runs `go build`; `@pytest.mark.container`; not in `run-tests.py` batches |
| Framework self-checks | `tests/test_*.py` | Baseline framework usage, CLI build, logging discipline |
| CLI contract (Go) | `app/ark-cli/*_test.go`, `app/ark-cli/internal/*/*_test.go` | Command result, output, state effects via seam injection |
| Domain service (Go) | `app/pkg/ark/*_test.go`, `app/pkg/ark/*/*_test.go` | Domain services against interface mocks; no runtime |
| Backend conformance (Go) | `app/pkg/ark/conformance/` (suite), `app/pkg/ark/sdkbackend_test/` (runner) | Same suite against Docker and rootless Podman; `integration` build tag |

Markers (`pytest.ini`): `unit`, `container`, `podman`, `lite`, `real_server`. Fast/slow
behavior follows the directory, not a marker.

### Fast vs slow tests

**Fast** (`.../fast/`): compose edits, CLI output, short-lived containers. No ARK server.
Seconds per test.

**Slow** (`.../slow/`): need a running ARK server (startup, RCON, backup/restore, shutdown).
Minutes per test. `tests/conftest.py` raises their pytest timeout to 1800 s (default 1200 s in
`pytest.ini`).

### Lite image (no ARK server)

A test that needs the container scripts or the CLI but not the real game server marks itself
`lite` next to `container`:

```python
pytestmark = [pytest.mark.container, pytest.mark.lite]
```

Both `test_env` fixtures then set `ARK_TEST_IMAGE` to the lite image, so the usual fixtures and
helpers (`container_instance_config`, `started_container_instance`, `start_container_instance`,
`build_container_instance_config(test_env, extra_env=...)`) run against it unchanged, and a
session of only `lite` tests never builds the ARK image. `tests/helpers/lite_image.py` builds it
from `tests/fixtures/lite_image/`: `python:3.14-slim`, `ark-user` (uid 3000), the real runtime
scripts at the real paths, a placeholder default `GameUserSettings.ini`, and a fake ARK server
behind a fake proton (`PROTON_PATH`). It is tagged `ark-server:lite-<hash of its inputs>`:
rebuilt only when those change, and in the `ark-server` repository because ark-cli only starts
`ark-server:<tag>` images.

The fake server (`fake_ark_server.py`) does what the scripts and CLI observe: its command line
contains `ArkAscendedServer.exe`; it writes the log markers (started, advertising,
`World Save Complete`, `Log file closed`); it answers RCON (`saveworld`, `DoExit`, `ListPlayers`)
with the admin password from `GameUserSettings.ini`; on `DoExit` it rewrites
`GameUserSettings.ini` (`FakeArkShutdowns=N`; `FAKE_ARK_INI_DUPLICATE=<key>` also writes that key
twice). So start, readiness, graceful stop and `--restart` flows take seconds. *Why:* the ARK
image is too big to build per PR; behaviour that depends on the real server (what ARK actually
writes, saves, mods) still needs a `slow` test, e.g. `test_ark_ini_history_real_server.py`
checks the fake's shutdown-write assumption against ARK.

**Lite mode** runs the whole container and CLI integration suite this way, fast and slow, with
no test duplicated: `ARK_TEST_LITE=1` (or `python run-tests.py --lite --suite slow`) points every
container test at the lite image, except tests marked `real_server`, which are skipped. Mark a
test `real_server` when what it checks is ARK itself, not our scripts or CLI: ARK's RCON replies
(`test_rcon`), what ARK writes (`test_ark_ini_history_real_server`), building or updating the ARK
image (`test_build_container_image`, `test_update_*`), Podman running ARK. Everything else must
pass in both modes; if a test needs more of the server than the fake gives, extend
`fake_ark_server.py` rather than marking it. Default mode (no `ARK_TEST_LITE`) is unchanged:
`slow` tests run on the real image.

Rules:

- **Fast tests are `lite`** unless they check ARK itself (`fast/test_update_local_check.py`
  reads the real image's build labels, so it is `real_server`). That way a fast run never needs
  the ARK image, including tests that start no container at all.
- **`create instance` in a lite test** still writes `ark-server:latest`; `run_cmd` points the new
  service at the lite image afterwards (`helpers.compose.use_lite_image`). Never tag the lite
  image as `ark-server:latest`: that replaces a real ARK image on the host.
- **Don't assume a slow start.** The lite image starts in about a second, which exposes waits
  that only held because ARK is slow (e.g. stopping as soon as the container is "running",
  before the server manager has its signal handler).

PR CI runs `pytest -m lite tests/container/integration tests/app/ark_cli/integration/docker`
(`image.yml`). Name `docker/`, not the whole `integration/` tree: `podman/conftest.py` switches
the session to Podman when imported.

`test_server_oom_report.py` (lite) runs the real `ark-server-start.py` with a different fake
proton whose child fills memory past a cgroup limit. *Why:* a real OOM kill needs a runtime that
enforces cgroup memory limits, which hosted runners do. It skips locally when the limit isn't
applied (e.g. rootless Docker without memory delegation) and fails in CI, so a runner change
can't make it pass silently.

### Test ordering

`tests/.test-order.yaml` lists ordered groups (directory relative to `tests/` → test file
stems). Listed files run in that order; if one fails, later ones in the group are skipped.
Results persist in `.run-state/` in the artifacts dir (`ARK_TEST_ARTIFACTS_DIR`, default `.tests/`) across runs; `ARK_TEST_WIPE_STATE=1` clears them.

### Slow test protection

Slow tests run one at a time. Each ARK server container reserves 12 GB
(`mem_reservation` in the generated compose file); concurrent slow tests cause OOM and
false failures.

Every slow test must take the `slow_test_protection` fixture (enforced, see
[Enforcement hooks](#enforcement-hooks)):

```python
def test_my_slow_test(
    test_env: dict[str, str],
    slow_test_protection,
    started_container_instance,
):
```

Defined in `tests/container/integration/conftest.py`; no-op outside a `slow` directory.

- **Lock file:** `<artifacts dir>/.lock` (default `.tests/.lock`), content `<PID>|<timestamp>|<test-name>`, created with `O_EXCL`.
- **Lock held by a live process:** the test is skipped.
- **Stale lock** (PID not in `/proc`): removed and re-acquired.
- **Teardown (`finally:`):** capture diagnostics to `docker.log`, compose down `--volumes --remove-orphans`, release lock.

Lock functions in `tests/helpers/infra.py`:

| Function | Purpose |
|---|---|
| `acquire_lock(lock_path, test_name)` | Acquire; `True`/`False`; handles stale locks |
| `release_lock(lock_path)` | Remove the lock file |
| `cleanup_compose_files(dc, compose_file)` | Compose down `--volumes --remove-orphans` via python-on-whales |
| `release_lock_and_cleanup(dc, lock_path, compose_file)` | Cleanup, then release |

### Container vs CLI test boundary

Container tests verify container-side scripts (entrypoint, shutdown manager, backup/restore)
in isolation. A failure must point at the container script, not at a CLI regression (wrong
arguments, bad compose file, changed SDK calls). So:

- **`tests/container/integration/`**: trigger with container primitives only (python-on-whales
  helpers such as `compose_stop`, `compose_exec`), never `ark-cli`. Compose files come from
  `write_single_service_compose` (`extra_env` for non-standard config). No `cli_path`.
- **`tests/app/ark_cli/integration/docker/`**: run `ark-cli` commands and check their effect on
  containers. `podman/` re-exports the same tests. `tests/app/ark_cli/integration/conftest.py`
  re-exports the container fixtures (`import *`). Container tests are expected to pass before
  CLI tests are meaningful.

## Outcome-driven testing

Default for new work:

- Assert user-visible behavior, state transitions, persisted side effects.
- Don't assert internal call order unless it is the contract.
- One strong contract test over several implementation-detail tests.
- Keep seam-mocked tests few and focused.

When adding commands or features:

1. Contract tests first (command result + output + state effects).
2. Integration tests against real workloads (real builds, API calls, container lifecycle).
   Anything touching images, external APIs or container behavior needs
   `@pytest.mark.container` tests against real containers. Fake/mocked Docker tests have
   repeatedly passed while real runs failed.
3. Seam/fallback tests only when a specific branch needs proof.

## Go CLI tests: the seam pattern

`core.AppDeps` (`app/ark-cli/internal/core/deps.go`) holds the side-effecting dependencies:

- `Engine` (`ark.ContainerEngine`), `ComposeService` (`ark.ComposeService`)
- `HealthChecker`, `Lifecycle`, `BackupService`, `UpdateService`, `ScheduleService`
- `HostDeps`, `RuntimeHelper`, `Runtime` (`RunCommand`, `LookPath`, ...)
- function fields such as `RunBackupFromSchedule`,
  `RunningServiceSnapshot`, `DiskUsageForPath`, `RemoveVolume`

Mocks:

- `app/pkg/ark/arktest/mocks.go`: `MockCompose` (one `<Method>Fn` field per `ComposeService`
  method), `MockEngine`, `MockCommandRunner`. Unset fields return zero values.
- `app/ark-cli/internal/testutil/mocks.go`: `MockHealthChecker`.

Tests swap fields via `testutil.WithPatchedAppDeps`:

```go
testutil.WithPatchedAppDeps(t, testutil.TestDepsConfig{}, func(d *core.AppDeps) {
    d.ComposeService = &arktest.MockCompose{}
    d.Runtime.RunCommand = func(cmd []string, _ ark.CommandOptions) (ark.ExecResult, error) {
        return ark.ExecResult{ExitCode: 0}, nil
    }
}, func() {
    // exercise the command under test
})
```

`TestDepsConfig` controls which services get wired (`WireBackupService`, `WireUpdateService`,
`WireScheduleService`, ...). Most packages wrap this in a local `withPatchedAppDeps` with a
package-specific config.

Other `testutil` helpers: `CaptureStdout`/`CaptureStderr`, `SetEnv`, `FakeProject`,
`FakeProjectWithVolumes`, `EmptyServices`, `TestEnv`, `TestPorts`, `CheckTableRow`,
`CreateBackupArchive`, `RepoRoot`.

Use unit tests for parsing, validation and compose edits; contract tests with seams for CLI
logic; `@pytest.mark.container` tests to prove the feature against real infrastructure.

### Go contract test coverage

Package tests in `app/ark-cli/internal/<package>/*_test.go` drive command handlers through
patched `AppDeps` and filesystem fixtures; no Docker/Podman. Distinct from the top-level
`app/ark-cli/*_test.go` and from the Python `native/` tests (CLI as a subprocess).

Packages and the commands they register (`app/ark-cli/internal/cmd/registry_entries.go`):

| Package | Commands | Key seams |
|---|---|---|
| `backup/` | `create backup`, `validate backup`, `prune backups`, `restore backup` | `BackupService` fields (`DiskUsage`), `MockCompose.LoadProjectFn`/`RunningServicesFn`/`RunOneOffFn`, `MockCommandRunner.RunFn`, `ARK_BACKUP_DIR` |
| `compose/` | `start`, `stop`, `restart`, `save`, `logs instance` | Own `withPatchedAppDeps` and helper copies (`testutil` imports `compose`, so importing it back is a cycle); `MockCompose.LoadProjectFn`, `MockEngine.ImageTagsFn`/`ImageLabelsFn` |
| `create/` | `create instance` | Settings validation; filesystem fixtures |
| `health/` | `check instance`, `wait instance`, `validate compose` | `MockHealthChecker.IsReadyFn`/`WaitFor*Fn` |
| `host/` | `check`/`fix`/`restore host-kernel-config` | `HostDeps.ReadKernelParam`, `HostDeps.GetNofileHardLimit` |
| `ini/` | `edit ini`, `get ini`, `set ini`, `show ini` | `MockCompose.RunningServicesFn`/`RunOneOffFn`/`ExecFn` (scripts), `RunOneOffInteractiveFn` (editor), `RestartStopInstance`/`RestartStartInstance` (`--restart` order) |
| `listenport/` | `listen port` | Real TCP/UDP listeners on test ports |
| `reachability/` | `check reachability` | `HostDeps.FetchText` |
| `reconfigure/` | `reconfigure instance`, `enable`/`disable remote-rcon` | `MockCompose.LoadProjectFn`/`SaveProjectFn`/`RunningServicesFn`, `RestartStopInstance`/`RestartStartInstance` |
| `remove/` | `remove instance`, `teardown instances` | `MockCompose.DownFn`/`KillFn`, `RemoveVolume`, `core.IsInteractiveTerminal` |
| `schedule/` | `create`, `update`, `enable`, `disable`, `remove`, `show`, `check`, `run`, `validate schedule`; `list schedules` | `overrideSchedulePathsForTest`, `writeScheduleFixture`, `RunBackupFromSchedule` |
| `scheduler/` | `enable`, `disable`, `run`, `check scheduler` | `executablePath` (static binary fixture), `healthListenAddr` (`127.0.0.1:0`), `MockEngine.RootlessFn`/`ImageExistsFn`/`BuildImageFn`, `MockCompose.UpFn`/`StopFn`/`RemoveFn`, `Runtime.LookPath`/`RunCommand`, `ARK_SCHEDULER_SOCKET` |
| `show/` | `show instance`, `list instances`, `show compose`, `show volume-paths`, `show backup`, `list backups` | `MockCompose.LoadProjectFn`, `MockEngine.VolumeMountpointFn`, `RunningServiceSnapshot`, `HostDeps.FetchText` |
| `update/` | `update instance` | `Engine` mock (`ImageLabelsFn`, `ImageExistsFn`), `HostDeps.FetchText`, `DiskUsageForPath`, `RestartStopInstance`/`RestartStartInstance` |

## Environment variables affecting tests

| Variable | Effect |
|---|---|
| `ARK_START_WAIT_TIMEOUT` | CLI start readiness timeout (e.g. `15m`) on loaded hosts |
| `ARK_TEST_IMAGE` | Image for integration tests (default `ark-server:latest`); built if missing |
| `ARK_TEST_LITE` | `1`: [lite mode](#lite-image-no-ark-server), every container test on the lite image; `real_server` tests skip |
| `ARK_CONTAINER_RUNTIME` | `docker` (default) or `podman`; the `docker/` and `podman/` conftests force their own value |
| `ARK_TEST_STARTUP_LOG_TIMEOUT` | Seconds to wait for the server startup log line (default `300`) |
| `ARK_TEST_RCON_TIMEOUT` | Seconds to wait for RCON after startup (default `300`) |
| `ARK_TEST_ARTIFACTS_DIR` | Artifact directory (default `.tests`) |
| `ARK_TEST_ARTIFACTS_RETENTION` | Number of most recent run directories kept (default `7`) |
| `ARK_TEST_ARTIFACTS_WIPE_THRESHOLD` | If more tests than this are collected, wipe the artifacts dir (keeping the retained run dirs and `.run-state/`); otherwise only prune old run dirs (default `3`) |
| `ARK_TEST_WIPE_STATE` | `1` clears `.tests/.run-state/` (test-order results) at startup |
| `ARK_TEST_RUN_DIR`, `ARK_TEST_BATCH_LABEL` | Set by `run-tests.py` to put every batch under one suite run directory |
| `TEST_BATCH_PYTHON` | Default for `run-tests.py --python` (default `./.venv/bin/python`) |

`ARK_SKIP_RAM_CHECK` is a container variable, not a shell one: generated test compose files
set it to `1` (`enable_skip_ram_check`), silencing the entrypoint's low-RAM warning. Setting
it in your shell has no effect.

## Integration test framework

Shared infrastructure in `tests/helpers/`:

| Module | Purpose |
|---|---|
| `artifacts` | Artifact paths, run dirs, retention, CLI binary build/discovery, pytest report artifact |
| `batch_runner` | `run-tests.py` batch execution and summary |
| `build` | `image_exists`, `build_server_image` |
| `compose` | Port allocation, `write_single_service_compose`, compose patching (`enable_skip_ram_check`, `disable_restart_policy`, ...), `asa_port_from_compose` |
| `compose_cleanup` | Compose file registry; SIGTERM/SIGINT handler runs compose down for registered files |
| `compose_client` | python-on-whales `DockerClient` factory (`get_docker_client`), container/compose helpers (`container_running`, `compose_exec`, `compose_stop`, `wait_for_container_state`, ...), log watchers, `start_container_instance` |
| `diagnostics` | `run_cmd`, `run_with_compose_patch`, command logging, failure formatting, `capture_diagnostics`, phase snapshots |
| `fakes` | `steam_api_server`: fake Steam API HTTP server |
| `lite_image` | `ensure_lite_image`, `image_for_test`: the [lite image](#lite-image-no-ark-server) for `lite` tests |
| `framework_logger` | Per-test `framework.log` |
| `infra` | `wait_for`, `prepare_test_env`, `cleanup_test_env`, `build_container_instance_config`, orphan pruning, lock functions |
| `native_fixtures` | File fixtures for `native/` tests (schedules, run state, backup archives, minimal compose) |
| `web_client` | stdlib HTTP client for `ark-web` tests |

Key fixtures (`tests/container/integration/conftest.py`, re-exported to CLI tests; `tests/conftest.py`
has minimal root versions of `test_env`, `run_cmd`, `compose_client`):

- `test_env`: env dict (copy of `os.environ`) with `COMPOSE_FILE`, `ARK_BACKUP_DIR`,
  `ARK_TEST_IMAGE`, artifact log paths (`ARK_TEST_COMMANDS_LOG`, ...). Tears down the project
  and prunes orphaned test resources afterwards.
- `run_cmd`: subprocess runner with command logging and compose patching.
- `compose_client`: python-on-whales client bound to the test's compose file.
- `wait_for`: poll with timeout.
- `container_instance_config`: writes a single-service compose file and returns `instance`,
  `admin_password`, `server_password` without starting it. For tests that control startup
  (first-boot init, pre-seeded state).
- `started_container_instance`: the above, then starts the container and waits for the startup
  log line and RCON.
- `started_instance_cli`: creates the instance with `ark-cli create instance`, then starts it
  the same way (CLI tests only).
- `slow_test_protection`: see [Slow test protection](#slow-test-protection).
- `log_streamer` (autouse): streams each service's log to `compose.<service>.log`.

Container fixtures are function-scoped: each test gets its own container. Don't add
module-scoped container fixtures.

`tmp_path` is overridden in `tests/conftest.py` to `<test_artifact_dir>/tmp` under `.tests/`
instead of `/tmp`, so it can be bind-mounted into a container (a remote or rootless daemon may
not see the caller's `/tmp`). It is pruned with the other artifacts.

### Enforcement hooks

`pytest_collection_modifyitems` in `tests/conftest.py` checks every `test_*.py` under an
`integration` directory. Violations fail collection (`pytest.fail()`) unless marked as a
warning. `native/` tests are exempt from rules 1–4.

| Rule | Check |
|---|---|
| 1 | No `subprocess.run`/`Popen`; use `run_cmd` |
| 2 | `@pytest.mark.container` required |
| 3 | CLI tests using `tmp_path` must also use `test_env`. Currently never fires: it checks for a `cli` path component, the directory is `ark_cli` |
| 4 | No `os.environ`; use `test_env.get()` or `dict(test_env)` (`podman_compat` exempt) |
| 5 | No in-place `test_env[...] =`; use `env = dict(test_env); env[KEY] = VALUE` |
| 6, 7 | No `["docker", ...]` / `["podman", ...]` command lists, single- or multi-line; use `compose_client` helpers (`podman_compat` exempt from 7) |
| 8 | Tests under `container/` must not reference `cli_path` |
| 9 | Tests under `slow/` must use `slow_test_protection` |
| 9b | No `tempfile` (`TemporaryDirectory`, `mkdtemp`, `gettempdir`); use `test_artifact_dir` or `tmp_path`. *Why:* `/tmp` dirs may be invisible to the daemon, so bind mounts come up empty and root-owned |
| 10 (warning) | File > 2500 chars, > 6 `def`s, or an inline `time.monotonic()` loop (use `wait_for`) |
| 11 (warning) | Session-scoped fixture in a `conftest.py` outside the allowlist; prefer function scope unless the value is immutable or setup idempotent |

Warnings print in a "Framework compliance warnings" section of the summary.

Don't work around a hook: fix the test or extend `tests/helpers/`. The helpers are the single
source for command execution, environment setup and container interaction.

Reference for new tests: `tests/test_baseline_framework.py`.

## Test artifacts

Per-test artifacts land in
`.tests/<YYMMDD-HHMMSS>-{suiterun|singlerun}/[<batch-label>/]<test-node>/` (batch label only
under `run-tests.py`). File list: [Run tests: test artifacts](../how-to/run-tests.md#test-artifacts);
contents and logging internals: [Test logging](test-logging.md).
