# Repo Map

Key file locations for contributors and coding agents.

## Host CLI

- `app/ark-cli/` — Go binary, all host-side command handling (~19.5k lines)
- `app/ark-cli/internal/core/deps.go` — `AppDeps` struct: injectable seams and
  service instances (runtime, compose, health, backup, lifecycle, schedule, update)
- `app/ark-cli/internal/core/format.go` — aligned key-value and pipe-separated
  table output
- `app/ark-cli/internal/core/table.go` — space-aligned table output with
  ANSI-aware column width
- `app/ark-cli/internal/core/json_output.go` — JSON output structs and
  `PrintJSONOutput`
- `app/ark-cli/internal/core/flags.go` — `NewCommandFlagSet`,
  `ParseNoPositionalArgs` (commands take targets as flags)
- `app/ark-cli/app.go` — CLI entry point, `Main()` function, dependency wiring
- `app/ark-cli/internal/core/logging.go` — spinner-aware output seams
  (`Infof`, `Infoln`, `Warnf`, `SpinnerAwareStdoutWrite`,
  `SpinnerAwareStderrWrite`); defaults are plain `fmt` calls, overridden at
  startup by `utils.go`
- `app/ark-cli/utils.go` — `init()` wiring that connects the core output seams
  to the spinner-aware implementations
- `app/ark-cli/verbose.go` — verbose mode state and `--log-file` tee
- `app/ark-cli/internal/compose/start_progress_spinner.go` — spinner rendering
  and the two-phase start display
- `app/ark-cli/internal/cmd/registry_entries.go` — `CommandRegistry` slice; all
  verb/object routes and their handler bindings
- `app/ark-cli/internal/cmd/registry.go` — registry lookup, help line generation,
  route dispatch
- `app/ark-cli/internal/compose/assets/docker-compose.defaults.yaml` — base
  compose template used by `create instance`
- `app/ark-cli/internal/{backup,compose,create,health,host,ini,listenport,reachability,reconfigure,remove,schedule,show,update}/` —
   CLI command handler packages, one per verb group

## Web UI

- `app/ark-web/` — separate Go module (in `go.work`) serving a small HTML UI
  over the same `app/pkg/ark` services: instance list/detail,
  start/stop/restart, compose and INI view/edit, `GET /health`. Listens on
  `ARK_WEB_ADDR` (default `127.0.0.1:8080`). Backup and update services are
  not wired yet.
- `app/ark-web/internal/server/` — HTTP routes, templates, static assets
- `app/ark-web/Dockerfile` — image build for the web UI (build context is the
  repo root)

## Domain packages (app/pkg/ark)

- `app/pkg/ark/engine.go` — `ContainerEngine` interface
- `app/pkg/ark/compose_service.go` — `ComposeService` interface (18 methods
  grouped as Project / Lifecycle / Status / Exec / Logs / Copy); see the source
  for the full list
- `app/pkg/ark/errors.go` — sentinel errors: `ErrNotFound`, `ErrConflict`,
  `ErrTimeout`
- `app/pkg/ark/types.go` — core types: `ComposeProject`, `ContainerState`,
  `ExecResult`, `LogOptions`, `BuildRequest`, `UpOptions`, `ExecOptions`,
  `RunOptions`, `CommandOptions`
- `app/pkg/ark/cmdrunner/` — command runner utility for subprocess dispatch
- `app/pkg/ark/sdkbackend/` — Go SDK backend (implements full ContainerEngine + ComposeService interfaces)
- `app/pkg/ark/sdkbackend_test/` — SDK backend conformance tests (Podman + Docker)
- `app/pkg/ark/conformance/` — shared conformance tests the backend must pass
- `app/pkg/ark/backup/` — backup creation, restore, retention, and maintenance
- `app/pkg/ark/health/` — readiness and startup polling
- `app/pkg/ark/lifecycle/` — start/stop/down orchestration
- `app/pkg/ark/schedule/` — schedule CRUD, crontab management, state tracking
- `app/pkg/ark/update/` — image update checking, host builds, safe rollout and rollback, update state and webhook
- `app/pkg/ark/composeutil/` — shared compose helpers: env var access, port
  parsing, service mutation
- `app/pkg/ark/runtime/` — top-level orchestration: compose YAML editing, instance
  info summaries, container runtime detection, privilege escalation, port
  assignment; also `exec.go`, `types.go`, `local_build.go`
- `app/pkg/ark/host/` — host-level checks: kernel config (sysctl/NOFILE), port
  probing (TCP/UDP/UE5 handshake), external port checker
- `app/pkg/ark/arktest/` — test mocks for ContainerEngine and ComposeService

Default compose target: `docker-compose.yaml` in the current directory.
Override with the `COMPOSE_FILE` environment variable.

## Container runtime

- `container/scripts/runtime/container-entrypoint.sh` — Dockerfile ENTRYPOINT;
  drops to `ark-user` if running as root, then execs into Python
- `container/scripts/runtime/container-entrypoint.py` — Python main; handles
  command override args and delegates to `run_default_startup`
- `container/scripts/runtime/ark-server-start.py` — server process manager;
  starts via GE-Proton, tails logs, handles SIGTERM
- `container/scripts/runtime/ark-server-stop.py` — graceful shutdown via RCON:
  warning (if `SHUTDOWN_WARNING_MINUTES` > 0) → saveworld with retries → DoExit
- `container/scripts/runtime/ark-backup.py` — backup archive creator (tar/tar.gz/tar.zst)
- `container/scripts/runtime/rcon_client.py` — standard-library RCON client
  (`rcon_command`); `ark-rcon.py <command>` is its command-line entry point
- `container/scripts/runtime/runtime_utils.py` — container paths (`SAVED_DIR`,
  `SERVER_LOG`, …) and shared helpers: `env_int`/`ConfigError`, `run_rcon`,
  `server_command`, `ark_server_pids`/`ark_server_running`, `server_log_closed`
- `container/scripts/runtime/ini_settings.py` — `update_config`: writes the
  RCON and password settings from the environment into `GameUserSettings.ini`
  on every start
- `container/scripts/installer/generate-default-ini.py` — image build only:
  runs ARK once and saves the default `GameUserSettings.ini` that the
  entrypoint installs when a volume has none
- `container/scripts/runtime/_logging.py` — canonical container logging
  (`emit_event`, `emit_warning`, `emit_error`)
- `container/scripts/runtime/ark-restore.py` — restore handler with path
  validation and pre-restore backup

## Image definitions

- `container/base.dockerfile` — base image (packages, GE-Proton, XAudio, `ark-user`),
  published to `ghcr.io/bryanwieg/ark-server-base:<hash>`
- `container/dockerfile` — runtime image on the base: server files from the local
  files image, runtime scripts; runs server as `ark-user` (least privilege)
- `container/dockerfile.labels` — labeled variant; adds version/build-id tags
- `container/files/` — plain files: `build.sh`, `download.sh` and their
  dockerfiles build the temporary server files image (SteamCMD in a throwaway
  container, layer split)
- `container/embed.go` — Go module `arkcontainer` embedding the build recipe
  (`dockerfile`, scripts, `files/`) in the `ark-cli` binary; extracted to
  `<compose dir>/.ark-manage/build/context` at build time

## Tests

- `tests/container/unit/` — fast Python unit tests; no Docker required
- `tests/container/integration/fast/` — fast container integration tests; short-lived
  containers, no running ARK server; `@pytest.mark.container`; trigger is always the compose SDK, never `ark-cli`
- `tests/container/integration/slow/` — slow container integration tests; require running
  ARK server; `@pytest.mark.container`; trigger is always the compose SDK, never `ark-cli`
- `tests/app/ark_cli/integration/docker/fast/` — fast CLI integration tests; compose manipulation,
  fake API servers; `@pytest.mark.container`
- `tests/app/ark_cli/integration/docker/slow/` — slow CLI integration tests; require running ARK server;
  `@pytest.mark.container`; trigger is `ark-cli` commands
- `tests/app/ark_cli/integration/podman/{fast,slow}/` — Podman re-exports of the
  docker suites
- `tests/app/ark_web/integration/fast/` — `ark-web` HTTP endpoint tests
- `tests/app/ark_cli/integration/native/` — native CLI integration tests; no container runtime
  required; exercises the built binary directly (e.g. port checks)
- `tests/test_baseline_framework.py` — canonical test structure reference
- `tests/test_canary.py` — smoke test for test infrastructure
- `app/ark-cli/` — Go tests live alongside source (`*_test.go`)

## Build and CI scripts

- `.github/build-image.sh` — build and tag the Docker image (CI/manual use;
  the CLI builds natively via Docker buildx and never calls this script)
- `.github/build-cli.sh` — build the `ark-cli` Go binary
- `.github/update-check.sh` — CI image update check helper (the CLI checks
  via the Steam API natively and never calls this script)
- `.github/ci-integration.sh` — integration smoke test (create, start, backup,
  stop, restore)
- `.github/ci-integration.sh` is not wired into a workflow yet.
- `.github/workflows/` — GitHub Actions: `cli.yml` (Go tests, CLI release
  binary), `image.yml` (container unit tests, build and smoke tests, base image
  publishing), `base-refresh.yml` (monthly base refresh PR), `proton-update.yml`
  (GE-Proton update PR), `auto-merge.yml` (merges passing automated updates and
  the release PR), `release.yml` (release-please) and `dependabot.yml`. See [ci.md](ci.md).
- `release-please-config.json`, `.release-please-manifest.json` — release
  versioning for the CLI and container

## Docs

- `docs/user/` and `docs/dev/` — Diátaxis trees (`tutorials/`, `how-to/`,
  `reference/`, `explanation/`); each has a `README.md` index
- `docs/dev/reference/performance-testing/` — Proton/Wine benchmark scripts and
  findings

## Managed state (repo-local, gitignored)

- `.ark-manage/schedules.yaml` — schedule definitions
- `.ark-manage/generated/schedules.crontab` — generated crontab file
- `.ark-manage/logs/schedules/<name>.log` — per-schedule run logs
- `.ark-manage/state/schedules/<name>.last.json` — last run state
- `.ark-manage/locks/schedule-<name>.lock` — per-schedule lock files
- `.tests/<run-timestamp>/` — test artifacts (docker.log, cli.log, compose
  snapshots per test node)
- `.tests/<run-timestamp>/<test-node>/compose/` — runtime compose files for a test node
