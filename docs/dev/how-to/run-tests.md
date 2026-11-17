# Run Tests

How to run the test suites and clean up test artifacts.

## Test suites

| Suite | What it tests | Docker required? |
|---|---|---|
| `unit` | Python parsing, validation, and transform logic | No |
| `container` | Container runtime scripts in a running container | Yes |
| `cli` | `ark-cli` end-to-end command behavior | Yes (integration tag) |
| `smoke` | Healthy server start and clean stop; CI's image publish gate. Not part of `all` | Yes |

## Layout

- `tests/container/unit/` — fast unit-only checks for pure Python/runtime helper behavior
- `tests/container/integration/fast/` — fast container integration tests (short-lived containers)
- `tests/container/integration/slow/` — slow container integration tests (running ARK server)
- `tests/app/ark_cli/integration/docker/fast/` — fast CLI integration tests (compose manipulation, fake APIs)
- `tests/app/ark_cli/integration/docker/slow/` — slow CLI integration tests (running ARK server)
- `tests/app/ark_cli/integration/podman/fast/` — same tests re-exported for Podman runtime (fast)
- `tests/app/ark_cli/integration/podman/slow/` — same tests re-exported for Podman runtime (slow)
- `tests/app/ark_web/integration/fast/` — `ark-web` HTTP endpoint tests (builds the binary with `go build`)
- `tests/fixtures/` — shared test fixtures (e.g. restore archives)
- `app/ark-cli/*_test.go` — Go unit and contract tests live alongside source

## Running tests

Use the root `run-tests.py` runner:

```sh
./run-tests.py --suite unit                     # fast, no Docker
./run-tests.py --suite container-fast          # container tests (fast only)
./run-tests.py --suite container-slow          # container tests (slow only)
./run-tests.py --suite container              # all container tests
./run-tests.py --suite smoke                  # healthy start + stop (CI gate)
./run-tests.py --suite cli-fast               # CLI tests (fast only)
./run-tests.py --suite cli-slow               # CLI tests (slow only)
./run-tests.py --suite cli                    # all CLI tests
./run-tests.py --suite fast                   # all fast tests (container + CLI)
./run-tests.py --suite slow                   # all slow tests (container + CLI)
./run-tests.py --suite all                    # everything
./run-tests.py --list                         # list available tests
```

Pass extra pytest arguments after `--`:

```sh
./run-tests.py --suite container -- -k test_ark_backup_script -v
./run-tests.py --suite cli -- --maxfail=1
```

## Podman and web tests

`run-tests.py` batches cover the Docker suites only. Run the Podman and
`ark-web` suites with pytest directly:

```sh
.venv/bin/python -m pytest tests/app/ark_cli/integration/podman/fast/
.venv/bin/python -m pytest tests/app/ark_web/integration/
```

## Go CLI tests (without the runner)

Fast contract tests, no Docker:

```sh
cd app/ark-cli && go test ./...
```

Domain package tests, no Docker:

```sh
cd app/pkg/ark && go test ./...
```

Backend conformance tests (require Docker; Podman variants auto-skip without a
rootless Podman socket):

```sh
cd app/pkg/ark && go test -tags integration -v ./sdkbackend_test/
```

## Requirements

- Python venv at `./.venv` (create with `python3 -m venv .venv && .venv/bin/pip install -r requirements-dev.txt`)
- Docker daemon access for container and CLI integration suites
- Built `ark-server:latest` image for integration tests (override with `ARK_TEST_IMAGE`)
- Go toolchain for CLI tests

Override the venv location with `TEST_BATCH_PYTHON=/path/to/python`.

## Environment variables

Full list: [Test architecture — environment variables](../reference/test-architecture.md#environment-variables-affecting-tests).
Most commonly needed:

| Variable | Effect |
|---|---|
| `ARK_START_WAIT_TIMEOUT=15m` | Extend start readiness timeout on slow or loaded hosts |
| `ARK_TEST_IMAGE` | Override the Docker image used by integration tests |
| `ARK_TEST_ARTIFACTS_DIR` | Override the directory for test artifacts (default `.tests`) |
| `ARK_TEST_ARTIFACTS_RETENTION` | Days to keep test artifacts (default `7`) |
| `ARK_TEST_ARTIFACTS_WIPE_THRESHOLD` | Minimum artifacts to keep when pruning (default `3`) |
| `TEST_BATCH_PYTHON` | Override the Python interpreter used by the runner |

## Timing expectations

ARK server startup commonly takes several minutes. Graceful shutdown adds
about a minute. Prefer small batches (1–3 tests) for Docker-backed runs to
keep iteration manageable.

## Monitoring containers during a test run

```sh
# Terminal 1 — run tests
./run-tests.py --suite container -v

# Terminal 2 — watch containers
watch -n5 'docker ps --format "table {{.Names}}\t{{.Status}}"'
```

## Test artifacts

Each test node writes artifacts under `.tests/<run-timestamp>/<test-node>/`:

- `commands.log` — all `run_cmd` invocations with exit codes, stdout, stderr
- `compose.<service>.log` — per-service streaming log, captured continuously
- `docker.log` — container-level diagnostics (state, image, PID) from just before teardown
- `failure.log` — docker-level state captured immediately on test failure
- `framework.log` — framework-level trace (poll, lock, lifecycle, compose events, SDK calls)
- `pytest.log` — pytest phase outcomes with captured stdout/stderr
- `cli.log` — captured `ark-cli` output (when CLI was invoked)
- `compose/` — start and final compose file snapshots

Generated runtime compose files are placed under
`.tests/<run-timestamp>/compose-runtime/`.

## Rebuilding before tests

```sh
./.github/build-image.sh    # rebuild the Docker image
./.github/build-cli.sh      # rebuild the ark-cli binary
```

The CLI itself builds images natively via Docker buildx and never calls
`build-image.sh` or `update-check.sh`; those scripts exist for CI and manual
use (see [Image build pipeline](../reference/image-build.md)).

## Cleaning up

```sh
./clean-repo.sh
./clean-repo.sh --dry-run --verbose   # preview what would be removed
```

## CI vs. local split

CI minutes are limited. Run lightweight unit and crucial fast integration
tests in CI. Run the full Docker-backed suites locally.

## See also

- [Test architecture](../reference/test-architecture.md) — test layers, seam injection, and outcome-driven testing philosophy
