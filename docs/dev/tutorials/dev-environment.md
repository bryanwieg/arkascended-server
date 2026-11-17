# Development Environment Setup

This tutorial takes you from a fresh clone to a passing test run.

## Prerequisites

- Docker Engine (with Compose v2 plugin) and access to the Docker socket
- Go at the version in `go.work` or later (required to build the CLI from source)
- Python 3.10 or later
- `git`

## 1. Clone the repo

```sh
git clone <repo-url>
cd ark-docker
```

## 2. Build the CLI binary

```sh
./.github/build-cli.sh
```

The binary is written to `app/ark-cli/ark-cli`. Add it to your `PATH`,
or invoke it as `./app/ark-cli/ark-cli` from the repo root.

## 3. Build the Docker image

```sh
./.github/build-image.sh
```

This produces `ark-server:latest` and tagged variants. The integration test
suite expects this image to exist. Re-run this script any time you change
container scripts or the Dockerfile.

## 4. Set up the Python virtual environment

```sh
python3 -m venv .venv
.venv/bin/pip install -r requirements-dev.txt
```

The test runner and container test suite expect the venv at `./.venv`.

## 5. Run the unit test suite

Unit tests require no Docker and run in seconds:

```sh
./run-tests.py --suite unit
```

All tests should pass before you proceed to integration tests.

## 6. Run a container integration test

Pick a single integration test to verify your image and Docker setup:

```sh
./run-tests.py --suite container -- -k test_ark_server_start_script -v
```

This test starts an ARK container and verifies the startup flow. Expect it to
take 5–10 minutes on first run (server startup is slow). A pass here means
your image and Docker permissions are correctly set up.

## 7. Run the CLI test suite

```sh
./run-tests.py --suite cli
```

CLI tests exercise `ark-cli` end-to-end, including compose editing and backup
operations. The integration-tagged tests require Docker; the Go unit/contract
tests do not.

To run only the fast Go contract tests without Docker:

```sh
cd app/ark-cli && go test ./...
```

## Useful environment variables

| Variable | Purpose |
|---|---|
| `ARK_START_WAIT_TIMEOUT=15m` | Extend the start readiness timeout on slow or loaded hosts |
| `ARK_TEST_IMAGE` | Override the Docker image used by integration tests (default `ark-server:latest`) |
| `ARK_TEST_ARTIFACTS_DIR` | Override the directory for test artifacts (default `.tests`) |
| `ARK_TEST_ARTIFACTS_RETENTION` | Days to keep test artifacts (default `7`) |
| `ARK_TEST_ARTIFACTS_WIPE_THRESHOLD` | Minimum artifacts to keep when pruning (default `3`) |

## Monitoring containers during a test run

```sh
# Terminal 1 — run tests
./run-tests.py --suite container -v

# Terminal 2 — watch running containers
watch -n5 'docker ps --format "table {{.Names}}\t{{.Status}}"'
```

## Cleaning up

Remove generated test artifacts, caches, and temp files without touching
tracked files or Docker resources:

```sh
./clean-repo.sh
./clean-repo.sh --dry-run --verbose   # preview what would be removed
```

## Next steps

- `docs/dev/reference/test-architecture.md` — test layers and seam injection
  patterns
- `docs/dev/how-to/add-cli-command.md` — add a new `ark-cli` command
- `docs/dev/reference/repo-map.md` — key file locations
