# Repo Map

Key file locations for contributors and coding agents. Go modules are tied
together by `go.work` (`app/ark-cli`, `app/ark-web`, `app/pkg/ark`,
`container`).

## Host CLI

- `app/ark-cli/` — Go binary, all host-side command handling
- `app/ark-cli/main.go` — `main()`; `version` set by `-ldflags` at release
- `app/ark-cli/app.go` — `Main()`, dependency wiring
- `app/ark-cli/options.go` — global flags (`--verbose`, `--log-file`,
  `--runtime`, `--help`, `--version`)
- `app/ark-cli/routing.go` — verb/object route parsing against the registry
- `app/ark-cli/utils.go` — `init()` wiring that connects the core output seams
  to the spinner-aware implementations
- `app/ark-cli/verbose.go` — verbose mode state and `--log-file` tee
- `app/ark-cli/internal/cmd/registry_entries.go` — `CommandRegistry` slice; all
  verb/object routes and their handler bindings
- `app/ark-cli/internal/cmd/registry.go` — registry lookup, help line generation,
  route dispatch
- `app/ark-cli/internal/core/deps.go` — `AppDeps` struct: injectable seams and
  service instances (engine, compose, health, lifecycle, backup, update,
  schedule)
- `app/ark-cli/internal/core/deps_defaults.go` — default `AppDeps` construction;
  `RepoRoot()` (current directory)
- `app/ark-cli/internal/core/format.go` — aligned key-value and pipe-separated
  table output
- `app/ark-cli/internal/core/table.go` — space-aligned table output with
  ANSI-aware column width
- `app/ark-cli/internal/core/json_output.go` — JSON output structs and
  `PrintJSONOutput`
- `app/ark-cli/internal/core/flags.go` — `NewCommandFlagSet`,
  `ParseNoPositionalArgs` (commands take targets as flags)
- `app/ark-cli/internal/core/logging.go` — spinner-aware output seams
  (`Infof`, `Infoln`, `Warnf`, `SpinnerAwareStdoutWrite`,
  `SpinnerAwareStderrWrite`); defaults are plain `fmt` calls, overridden at
  startup by `utils.go`
- `app/ark-cli/internal/compose/start_progress_spinner.go` — spinner rendering
  and the two-phase start display
- `app/ark-cli/internal/compose/assets/docker-compose.defaults.yaml` — base
  compose template used by `create instance`
- `app/ark-cli/internal/{backup,compose,create,health,host,ini,listenport,reachability,reconfigure,remove,schedule,show,update}/` —
  command handler packages, one per verb group
- `app/ark-cli/internal/scheduler/` — `enable`/`disable`/`run`/`check scheduler`:
  renders the `ark-scheduler` compose service (`service.go`), builds its image
  and edits compose (`enable.go`), runs the schedule and monitor loops and
  `GET /health` (`scheduler.go`), prints the health report (`check.go`),
  removes ark-cli's old crontab block (`crontab.go`)
- `app/ark-cli/internal/compose/lock.go` — `WithInstanceLocks`: lifecycle
  commands hold the instance lock, waiting up to 15m
- `app/ark-cli/internal/testutil/` — test deps, mocks, fixtures, output capture

Commands use `docker-compose.yaml` in the current directory; override with
`--compose-file` or `COMPOSE_FILE`.

## Web UI

- `app/ark-web/` — Go module serving a small HTML UI over the same
  `app/pkg/ark` services: instance list/detail, start/stop/restart, compose and
  INI view/edit, `GET /health`. Listens on `ARK_WEB_ADDR` (default
  `127.0.0.1:8080`). Backup and update services are not wired yet.
- `app/ark-web/internal/server/` — HTTP routes, templates, static assets
- `app/ark-web/Dockerfile` — image build for the web UI (build context is the
  repo root)

## Domain packages (app/pkg/ark)

- `app/pkg/ark/engine.go` — `ContainerEngine` interface
- `app/pkg/ark/compose_service.go` — `ComposeService` interface (18 methods
  grouped as Project / Lifecycle / Status / Exec / Logs / Copy)
- `app/pkg/ark/errors.go` — sentinel errors: `ErrNotFound`, `ErrConflict`,
  `ErrTimeout`
- `app/pkg/ark/types.go` — core types: `ComposeProject`, `ContainerState`,
  `ExecResult`, `LogOptions`, `BuildRequest`, `UpOptions`, `ExecOptions`,
  `RunOptions`, `CommandOptions`
- `app/pkg/ark/tmpdir.go` — `TempDir`: `ARK_TMP_DIR` or `./.ark-tmp/`
- `app/pkg/ark/sdkbackend/` — Docker SDK / Compose library backend
  (implements `ContainerEngine` and `ComposeService`)
- `app/pkg/ark/sdkbackend_test/` — SDK backend conformance tests (Podman + Docker)
- `app/pkg/ark/conformance/` — shared conformance tests the backend must pass
- `app/pkg/ark/arktest/` — test mocks for `ContainerEngine` and `ComposeService`
- `app/pkg/ark/cmdrunner/` — subprocess command runner
- `app/pkg/ark/composeutil/` — shared compose helpers: env var access, port
  parsing, service mutation
- `app/pkg/ark/runtime/` — compose YAML editing, instance info summaries,
  container runtime detection, privilege escalation, port assignment, local
  image build
- `app/pkg/ark/host/` — host checks: kernel config (sysctl/NOFILE), port
  probing (TCP/UDP/UE5 handshake), external port checker
- `app/pkg/ark/health/` — readiness and startup polling; `probe.go` (the
  monitor's per-instance container/ready/RCON/UDP checks), `monitor.go`
  (rounds, restart policy, `health.json` report), `http.go` (`GET /health`)
- `app/pkg/ark/lifecycle/` — start/stop/down orchestration
- `app/pkg/ark/backup/` — backup creation, restore, retention, maintenance;
  `lock.go` (per-instance kernel lock)
- `app/pkg/ark/filelock/` — Linux OFD locks (`TryLock`, `Held`)
- `app/pkg/ark/schedule/` — schedule CRUD, cron parsing, state tracking;
  `runner.go` (due runs and catch-up for the scheduler), `health.go`
  (schedule health), `image.go` (builds `ark-scheduler:<hash>` from the
  binary)
- `app/pkg/ark/update/` — image update checking, host builds, safe rollout and
  rollback, update state and webhook

## Container runtime scripts

- `container/scripts/runtime/container-entrypoint.sh` — Dockerfile ENTRYPOINT;
  drops to `ark-user` if running as root, then execs into Python
- `container/scripts/runtime/container-entrypoint.py` — Python main; handles
  command override args and delegates to `run_default_startup`
- `container/scripts/runtime/ark-server-start.py` — server process manager;
  starts via GE-Proton, tails logs, handles SIGTERM
- `container/scripts/runtime/ark-server-stop.py` — graceful shutdown via RCON:
  countdown (`SHUTDOWN_WARNING_MINUTES`, default 5, while players are online) → saveworld with retries → DoExit
- `container/scripts/runtime/ark-stop-reason.py` — records why the next stop happens
  (`update instance` runs it before stopping) for the countdown's warnings
- `container/scripts/runtime/ark-backup.py` — backup entry point (tar/tar.gz/tar.zst);
  logic in `ark_backup/` (archive validation, RCON save, save confirmation, metadata)
- `container/scripts/runtime/ark-cluster-backup.py` — backup of the shared
  cluster directory (`Saved/clusters`) in a one-off container
- `container/scripts/runtime/ark-restore.py` — restore with path validation and
  pre-restore backup
- `container/scripts/runtime/rcon_client.py` — standard-library RCON client
  (`rcon_command`); `ark-rcon.py <command>` is its command-line entry point
- `container/scripts/runtime/runtime_utils.py` — container paths (`SAVED_DIR`,
  `SERVER_LOG`, …) and shared helpers: `env_int`/`ConfigError`, `run_rcon`,
  `server_command`, `ark_server_pids`/`ark_server_running`, `server_log_closed`
- `container/scripts/runtime/ini_settings.py` — `update_config`: writes the
  RCON and password settings from the environment into `GameUserSettings.ini`
  on every start
- `container/scripts/runtime/ini_history.py` — git history of `Game.ini` and
  `GameUserSettings.ini` (`Saved/Config/WindowsServer/.git`): each writer
  commits under its own author (`ARK server`, `instance env`, `ark-cli set ini`,
  `ark-cli edit ini`, `outside ark-cli`, `unknown origin`); `origins` turns
  `git blame` into a label per line. Commits come from the entrypoint (files as
  found, the image default, the env settings), the start script (the server's
  writes, after it exits) and `ark-ini.py` (`set`, `track`)
- `container/scripts/runtime/ark-ini.py` — `get`/`set` for `Game.ini` and
  `GameUserSettings.ini`; the CLI's only way into them (`show ini`, `get ini`,
  `set ini`, `show instance`). `get` with `--section`/`--key` lists a key's lines
  with their origin. `set` refuses while the server runs, refuses env-managed
  keys, and refuses repeated keys unless `--replace-all`, listing each line's
  origin. `track --by edit|outside` records `edit ini`'s changes
- `container/scripts/runtime/_logging.py` — canonical container logging
  (`emit_event`, `emit_warning`, `emit_error`)
- `container/scripts/installer/generate-default-ini.py` — image build only:
  runs ARK once and saves the default `GameUserSettings.ini` that the
  entrypoint installs when a volume has none
- `container/scripts/installer/ark-image-labels.sh` — image build only: reads
  build ID and version from the server files for image tags and labels

## Image definitions

- `container/base.dockerfile` — base image (packages, GE-Proton, XAudio, `ark-user`),
  published to `ghcr.io/bryanwieg/ark-server-base:<hash>`
- `container/dockerfile` — runtime image on the base: server files from the local
  files image, runtime scripts; runs server as `ark-user`
- `container/dockerfile.labels` — labeled variant; adds version/build-id tags
- `container/files/` — temporary server files image: `build.sh`, `download.sh`,
  `steamcmd.dockerfile`, `dockerfile`, `scripts/installer/` (SteamCMD install
  in a throwaway container, layer split)
- `container/embed.go` — Go module `arkcontainer` embedding the build recipe
  (`dockerfile`, scripts, `files/`) in the `ark-cli` binary; extracted to
  `<compose dir>/.ark-manage/build/context` at build time

See [image-build.md](image-build.md).

## Tests

- `run-tests.py` — batch test runner; `pytest.ini` defines the `unit`,
  `container` and `slow` markers. See [test-architecture.md](test-architecture.md).
- `tests/container/unit/` — fast Python unit tests; no container runtime
- `tests/container/integration/fast/` — short-lived containers, no running ARK
  server; `@pytest.mark.container`; driven through the compose SDK, never `ark-cli`
- `tests/container/integration/slow/` — require a running ARK server;
  `@pytest.mark.container`; driven through the compose SDK, never `ark-cli`
- `tests/app/ark_cli/integration/docker/fast/` — CLI tests: compose
  manipulation, fake API servers; `@pytest.mark.container`
- `tests/app/ark_cli/integration/docker/slow/` — CLI tests that require a
  running ARK server; `@pytest.mark.container`; driven through `ark-cli`
- `tests/app/ark_cli/integration/podman/{fast,slow}/` — Podman re-exports of the
  docker suites
- `tests/app/ark_cli/integration/native/` — run the built binary directly; no
  container runtime (e.g. port checks)
- `tests/app/ark_web/integration/fast/` — `ark-web` HTTP endpoint tests
- `tests/helpers/` — shared test helpers (image build, compose client,
  artifacts, fakes, web client, lite image); `tests/fixtures/` — test data,
  including `lite_image/` (fake ARK server image for `lite` tests)
- `tests/conftest.py` — root pytest fixtures
- `tests/test_baseline_framework.py` — canonical test structure reference
- `tests/test_canary.py` — smoke test for test infrastructure
- `tests/test_build_cli.py` — builds `ark-cli` with `go build` if missing
- Go tests live alongside source (`*_test.go`)

## Build and CI

- `.github/build-image.sh` — build and tag the image (CI, dev, test helpers;
  uses buildx). `ark-cli` builds through the Docker SDK and never calls it.
- `.github/build-cli.sh` — build the `ark-cli` Go binary
- `.github/update-check.sh` — standalone helper: compares local image metadata
  with the Steam API and optionally rebuilds. No workflow uses it; `ark-cli`
  checks natively.
- `.github/ci-integration.sh` — integration smoke test (create, start, backup,
  stop, restore); not wired into a workflow
- `.github/workflows/` — GitHub Actions: `cli.yml` (Go tests, CLI release
  binary), `image.yml` (container unit tests, build and smoke tests, base image
  publishing), `python.yml` (ruff lint), `base-refresh.yml` (monthly base
  refresh PR), `proton-update.yml` and `go-update.yml` (GE-Proton and Go update
  PRs), `auto-merge.yml` (merges passing automated updates and the release PR),
  `release.yml` (release-please). See [ci.md](ci.md).
- `.github/dependabot.yml` — Dependabot config
- `release-please-config.json`, `.release-please-manifest.json` — release
  versioning for the CLI and container
- `docker-compose.dev.yaml` — dev compose file building `container/dockerfile`, plus an `ark-scheduler` service built from `app/ark-cli/ark-cli`
- `clean-repo.sh` — remove generated temp data, logs and caches
  (`--dry-run`)

## Docs

- `docs/user/` and `docs/dev/` — Diátaxis trees (`tutorials/`, `how-to/`,
  `reference/`, `explanation/`); each has a `README.md` index
- `docs/dev/reference/performance-testing/` — Proton/Wine benchmark scripts and
  findings

## Runtime state (gitignored)

Schedule state is relative to the current directory; build context and
backups are relative to the compose file's directory.

- `.ark-manage/schedules.yaml` — schedule definitions
- `.ark-manage/logs/schedules/<name>.log` — per-schedule run logs
- `.ark-manage/state/schedules/<name>.last.json` — last run state
- `.ark-manage/locks/schedule-<name>.lock` — per-schedule lock files
- `.ark-manage/state/scheduler/health.json` — scheduler health report
  (compose dir)
- `.ark-manage/build/scheduler/` — scheduler image build context (compose dir)
- `.ark-manage/build/context/` — extracted image build recipe
- `backups/<instance>/` — default backup root (override `ARK_BACKUP_DIR`)
- `.ark-tmp/` — CLI temp files (override `ARK_TMP_DIR`)
- `.tests/<run-timestamp>/` — test artifacts (docker.log, cli.log, compose
  snapshots per test node); `<test-node>/compose/` holds runtime compose files
