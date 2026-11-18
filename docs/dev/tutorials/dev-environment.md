# Development Environment Setup

Fresh clone to a passing test run.

## Prerequisites

- Docker Engine with the Compose v2 and buildx plugins, and access to the Docker socket
  (`build-image.sh` uses `docker buildx build`; the test framework drives `docker compose`)
- Go at the version in `go.work` or later
- Python 3.14 (the version CI uses)
- `git`, `curl`

## 1. Clone the repo

```sh
git clone https://github.com/bryanwieg/arkascended-server.git
cd arkascended-server
```

## 2. Build the CLI binary

```sh
./.github/build-cli.sh
```

Output: `app/ark-cli/ark-cli`. The CLI test fixtures rebuild it on every run, so
this step is only needed to use the CLI by hand.

## 3. Build the Docker image

```sh
./.github/build-image.sh
```

Produces `ark-server:latest` plus tags for the ARK version and Steam build ID.
Downloads the current ARK server files with SteamCMD on first run. Integration
tests expect this image; re-run after changing anything under `container/`.
Build stages and env vars: [Image build pipeline](../reference/image-build.md).

## 4. Set up the Python venv

```sh
python3 -m venv .venv
.venv/bin/pip install -r requirements-dev.txt
```

`run-tests.py` uses `./.venv/bin/python` unless `--python` or
`TEST_BATCH_PYTHON` is set.

## 5. Run the unit suite

No Docker, runs in seconds:

```sh
./run-tests.py --suite unit
```

## 6. Run one container integration test

Run pytest directly to pick a single test file (with `run-tests.py`, `-k` is
applied to every batch and batches with no matching test fail):

```sh
.venv/bin/python -m pytest tests/container/integration/slow/test_ark_server_start_script.py -v
```

Starts an ARK container and checks the startup flow. Expect 5–10 minutes. A
pass confirms the image and Docker access.

## 7. Run the CLI suites

Python end-to-end tests against real containers (Docker required):

```sh
./run-tests.py --suite cli
```

Go unit and contract tests, no Docker:

```sh
cd app/ark-cli && go test ./...
```

## Next steps

- [Run tests](../how-to/run-tests.md) — all suites, env vars, monitoring, artifacts, cleanup
- [Test architecture](../reference/test-architecture.md) — test layers and seam injection
- [Add a CLI command](../how-to/add-cli-command.md)
- [Repo map](../reference/repo-map.md) — key file locations
