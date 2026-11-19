# Run Tests

## Requirements

- Python venv at `./.venv`: `python3 -m venv .venv && .venv/bin/pip install -r requirements-dev.txt`
- Docker (or Podman via `ARK_CONTAINER_RUNTIME=podman`) with the `compose` CLI plugin. The test
  harness shells out to `docker compose`, unlike `ark-cli`.
- Image `ark-server:latest` (override: `ARK_TEST_IMAGE`). If missing, integration tests skip.
- Go toolchain. CLI and `ark-web` tests rebuild their binary with `go build` on every session.

## Test suites (`run-tests.py`)

`run-tests.py` runs fixed batches of test files, one pytest process per batch. `--list` prints
the batches a suite selects.

| `--suite` | Batches |
|---|---|
| `unit` | `test_log_rotation.py`, `test_rcon_client.py` only (not all of `tests/container/unit/`) |
| `container-fast` / `container-slow` / `container` | Container script tests (fast, slow, both) |
| `cli-fast` / `cli-slow` / `cli` | `ark-cli` Docker integration tests (fast, slow, both) |
| `fast` / `slow` | Container + CLI, fast or slow |
| `all` (default) | All of the above except `smoke` |
| `smoke` | Healthy start + clean stop; CI's image publish gate |

```sh
./run-tests.py --suite container-fast
./run-tests.py --suite cli --list
./run-tests.py --suite cli-fast -- --maxfail=1 -v   # pytest args after --, applied to every batch
./run-tests.py --lite --suite slow                  # slow suites on the fake ARK server, minutes not hours
```

| Option | Effect |
|---|---|
| `--python PATH` | Interpreter for pytest. Default `TEST_BATCH_PYTHON`, else `./.venv/bin/python`. Relative paths resolve against the repo root, so pass an absolute path for a system Python (`--python "$(command -v python3)"`). |
| `--lite` | Run every container test on the lite image (fake ARK server, no ARK download); `real_server` tests skip. [Lite image](../reference/test-architecture.md#lite-image-no-ark-server) |
| `--list` | Print selected batches, run nothing |
| `-- ARGS` | Extra pytest args. Without `--`, flags such as `-v` are rejected. |

Notes:

- `-k` after `--` filters every batch. Batches with no match exit 5 and show as `FAIL(5)`. To
  run one test, call pytest directly (below).
- Batch output is captured and printed when the batch finishes, not live.
- A batch that exceeds its timeout is killed and reported `FAIL(124)`.

## Running pytest directly

Use this for single files or tests, and for directories `run-tests.py` doesn't cover:

```sh
.venv/bin/python -m pytest tests/container/unit/                       # all Python unit tests
.venv/bin/python -m pytest tests/container/integration/slow/test_rcon.py::test_rcon_client_against_a_real_server
.venv/bin/python -m pytest tests/app/ark_cli/integration/native/       # CLI on the host, no containers
.venv/bin/python -m pytest tests/app/ark_cli/integration/podman/fast/
.venv/bin/python -m pytest tests/app/ark_web/integration/
```

Markers (`pytest.ini`): `unit`, `container`, `fast`, `slow`, `podman`, `sdk_backend`. Example:
`pytest -m unit tests/container/unit` (what CI runs).

## Layout

| Path | Contents |
|---|---|
| `tests/container/unit/` | Python unit tests for container scripts |
| `tests/container/integration/{fast,slow}/` | Container script tests: short-lived containers / running ARK server |
| `tests/app/ark_cli/integration/docker/{fast,slow}/` | `ark-cli` against Docker |
| `tests/app/ark_cli/integration/podman/{fast,slow}/` | The Docker tests re-exported for Podman |
| `tests/app/ark_cli/integration/native/` | `ark-cli` arg parsing, create, schedules, kernel config; no containers |
| `tests/app/ark_web/integration/fast/` | `ark-web` HTTP endpoints |
| `tests/test_*.py` | Framework self-checks (baseline, CLI build, logging discipline) |
| `tests/fixtures/` | Shared fixtures (e.g. restore archives) |
| `tests/helpers/` | Test framework ([Test architecture](../reference/test-architecture.md)) |

## Go tests

Go tests live next to the source. From the repo root (`go.work` covers all modules):

```sh
go test ./app/ark-cli/... ./app/pkg/ark/... ./app/ark-web/... ./container/...   # what CI runs
```

Backend conformance tests need Docker; Podman variants skip without a rootless Podman socket:

```sh
cd app/pkg/ark && go test -tags integration -v ./sdkbackend_test/
```

## Environment variables

Full list: [Test architecture: environment variables](../reference/test-architecture.md#environment-variables-affecting-tests).

| Variable | Effect |
|---|---|
| `ARK_START_WAIT_TIMEOUT=15m` | Longer start readiness timeout on slow or loaded hosts |
| `ARK_TEST_IMAGE` | Image for integration tests (default `ark-server:latest`) |
| `ARK_TEST_LITE=1` | Lite mode, as `run-tests.py --lite` |
| `ARK_CONTAINER_RUNTIME` | `docker` (default) or `podman` |
| `ARK_TEST_ARTIFACTS_DIR` | Artifact directory (default `.tests`) |
| `ARK_TEST_ARTIFACTS_RETENTION` | Number of recent run directories to keep (default `7`) |
| `ARK_TEST_ARTIFACTS_WIPE_THRESHOLD` | If more tests than this are collected, wipe old artifacts; else prune (default `3`) |
| `TEST_BATCH_PYTHON` | Default for `run-tests.py --python` |

## Timing

ARK server startup takes several minutes; graceful stop about one more. Keep Docker-backed
runs to 1–3 tests while iterating.

## Watching containers

`run-tests.py` prints batch output only at batch end, so watch from a second terminal:

```sh
watch -n5 'docker ps --format "table {{.Names}}\t{{.Status}}"'
```

## Test artifacts

Check these first when a test fails. Each run writes
`.tests/<YYMMDD-HHMMSS>-{suiterun|singlerun}/[<batch-label>/]<test-node>/`
([details](../reference/test-logging.md)):

| File | Contents |
|---|---|
| `commands.log` | `run_cmd` invocations: exit code, stdout, stderr |
| `compose.<service>.log` | Streamed service log |
| `docker.log` | Container diagnostics before teardown, plus a `compose ps --all` snapshot on failure |
| `framework.log` | Framework trace (polls, locks, lifecycle, compose, SDK calls) |
| `pytest.log` | Phase outcomes with captured stdout/stderr |
| `cli.log` | `ark-cli` output, when the CLI ran |
| `pytest-compose-<id>.yaml` | Compose file generated for the test |

Details: [Test logging](../reference/test-logging.md).

## Rebuilding the image

Integration tests use the existing image; rebuild after changing `container/`:

```sh
./.github/build-image.sh
```

`ark-cli` builds images itself through the Docker API with BuildKit and never calls
`build-image.sh`; the script is for CI and manual use ([Image build pipeline](../reference/image-build.md)).

## Cleaning up

```sh
./clean-repo.sh --dry-run --verbose   # preview
./clean-repo.sh
```

Removes generated files only; no tracked files, no Docker resources.

## CI

CI runs the Go tests, `pytest -m unit tests/container/unit`, the `lite` tests and `--suite smoke`.
Run the other Docker-backed suites locally. Workflows: [CI and releases](../reference/ci.md).
