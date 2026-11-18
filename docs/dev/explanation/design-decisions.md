# Design Decisions

Key architectural choices and the reasoning behind them. Reading this prevents
re-litigating settled questions.

## Why Proton/Wine instead of a native Linux binary

ARK: Survival Ascended ships only a Windows binary. There is no native Linux
dedicated server release. GE-Proton (a community-maintained Proton fork with
extra patches) provides the most reliable Wine-based compatibility layer for
the Windows binary on Linux Docker hosts.

The image pins a specific GE-Proton version via the `PROTON_VERSION`
Dockerfile build arg (currently `11-12`) rather than tracking latest, to
prevent unplanned breakage from upstream Proton changes.

## Why RCON is the primary control plane

Direct process management (signals, exec) is unreliable for a game server
that needs to flush player-save data before shutdown. RCON provides a
structured channel to issue `saveworld` and `do exit` commands, wait for
confirmation, and only then fall back to a kill signal if the server is
unresponsive. This eliminates the most common source of corrupted save files.

The container's exit code follows the same rule: the RCON confirmations, not
the game's exit code, decide whether a stop was clean. After a confirmed
`saveworld` and `DoExit`, ARK intermittently exits with code 2 or 5 instead of
0. Proton passes the game's code through unchanged, and the logs look the same
whether the code is 0 or not. So once save, shutdown and stop are all
confirmed, the runtime manager exits 0 and logs the game's code. An
unconfirmed step, or a server killed by a signal, still exits non-zero.

## Why YAML AST editing instead of template replacement

Regenerating `docker-compose.yaml` from a template on every `reconfigure`
would erase user-added services, comments, and custom fields. Surgical
AST-level edits via `goccy/go-yaml` touch only the keys being changed,
preserving everything else. The tradeoff is that the parser must handle YAML
edge cases carefully.

`SaveProject` is where this happens. It reloads the file, compares it with the
project the command changed, and rewrites only the image, environment, label
and port lines that differ, using the `goccy/go-yaml` AST to find them. Every
other byte stays as it was. Before writing, it loads the edited copy and checks
that it matches the requested project. If it doesn't, or if the change needs
flow style (`{...}`), a port range, a section inherited through a `<<` merge
key, or any other field, it refuses and leaves the file alone. A refusal asks
the user to edit by hand. The alternative it replaced, re-marshaling the loaded
project, baked `.env` values and `env_file` secrets into the file, made paths
absolute, dropped comments, and deleted services with an inactive profile.

## Why outcome-driven tests instead of mock-heavy unit tests

Mocking every internal call produces tests that pass even when observable
behavior is broken — the mocks are testing the mocks. Outcome-driven tests
(assert command output, exit code, and persisted state) catch real regressions
while remaining resilient to internal refactors. Seam injection via `AppDeps`
gives the isolation benefits of mocking without the fragility.

See `docs/dev/reference/test-architecture.md` for the testing patterns.

## Why the two-layer host CLI + container runtime split

Keeping orchestration logic in a statically compiled Go binary (the CLI) and
runtime logic in Python container scripts gives two independent seams:

- The CLI can be updated or replaced without rebuilding the Docker image.
- The container scripts can be tested in isolation without the full CLI stack.
- The boundary between layers is well-defined: the CLI speaks Docker Compose Go SDK
  calls; the container speaks process lifecycle and RCON.

## Why the backup ≥2 floor

Retention rules (count, age, size) could in theory delete all backups under
severe disk pressure. The ≥2 floor (dropping to 1 only under extreme
pressure) ensures there is always a recovery point. This is a deliberate
trade-off: some disk space is permanently reserved for safety, regardless of
user retention settings.

## Why two output channels with a strict user-text / operational-event split

Unified single-channel output models (everything structured; consumer
demuxes) were rejected. Integration tests parse substrings from
stdout/stderr, and operators pipe CLI output through text tools. Wrapping
`"Created schedule nightly"` in a JSON envelope would churn ~50 test
assertions with no gain and would break the human reading-flow. Keeping a
separate `*slog.Logger` stream for operational events (subprocess exec
trace, domain lifecycle) preserves `-v` as a useful debug mode without
contaminating the channel operators and tests already depend on.

See [explanation/logging-architecture.md](logging-architecture.md) for
the full design and [reference/conventions.md](../reference/conventions.md#logging-and-cli-output)
for the channel-rule table.

## Why the SDK backend

The CLI uses a single SDK backend (`app/pkg/ark/sdkbackend/`) that implements
both `ContainerEngine` and `ComposeService` interfaces via the Docker/Podman Go
SDK. This avoids subprocess overhead and provides full API coverage.

The SDK backend works with both Docker and Podman. When the runtime is Podman,
the SDK client connects to Podman's Docker-compatible API socket via
`DOCKER_HOST`. Podman's REST API includes a Docker v1.40 compatibility layer,
allowing the Docker SDK to operate transparently. The SDK also provides
`NewSDKEngineWithSocket` and `NewSDKComposeWithSocket` constructors for
explicit socket path control (used by tests).

Conformance tests in `app/pkg/ark/conformance/` verify correct behavior against
both Docker and Podman sockets.

See [explanation/backends.md](backends.md) for the full interface contract and
conformance test details.

## Why schedule state uses file-based storage

Schedules are the only CLI subsystem with their own persistent state. Schedule
definitions live in `.ark-manage/schedules.yaml`, generated crontabs in
`.ark-manage/generated/schedules.crontab`, lock files in `.ark-manage/locks/`, and per-schedule
last-run state in `.ark-manage/state/schedules/<name>.last.json`. This file-based approach was
chosen because:

1. **Transparency** — `cat .ark-manage/schedules.yaml` shows the full state
   without running a command. Users and operators can inspect, backup, and
   restore schedule state with standard file tools.
2. **No database dependency** — the CLI has no database. File-based state avoids
   introducing one for a subsystem that changes infrequently (typically only on
   `create schedule` or `apply schedules`).
3. **Crontab integration** — the `apply schedules` command generates a crontab
   file and installs it via the `crontab` command. Storing the crontab alongside
   the schedule definition makes the mapping direct and auditable.

The lock files prevent concurrent `run schedule` invocations from colliding on
the same schedule name. The state JSON files record last-run timestamp, exit
code, and duration for the `check schedule` command.

## Relative Temp Directory

The CLI uses `./.ark-tmp/` by default instead of `/tmp`. This keeps temporary
files within the project directory for easier cleanup and avoids `/tmp`
size restrictions. Configure via `ARK_TMP_DIR` environment variable.

## Why GameUserSettings.ini is baked into the image

ARK has no documented default `GameUserSettings.ini`, and its contents change
between ARK versions, so the image lets ARK write it. The image build's
`ini_defaults` stage (`container/scripts/installer/generate-default-ini.py`)
starts the server once, sends SIGTERM, keeps the file ARK writes and copies it
to `/home/ark-user/config-defaults/GameUserSettings.ini`. On every start the
entrypoint installs that file into the save volume if the volume has none, then
re-applies the RCON and password settings (below).

*Why at build time, not on first boot:* first boot used to start a temporary
server, grab the INI and kill the server, gated by `.ark-config-init.done` /
`.not_done` marker files. That was brittle:

- ARK writes a partial INI shortly after launch and the complete one, with all
  `[ServerSettings]` defaults, only after SIGTERM. Runtime staging kept the
  partial file. The generator waits for the complete one (it has
  `[ServerSettings]` and its size has settled); ARK doesn't reliably exit after
  writing it, so the file, not the process, is the signal.
- `.not_done` only reached the volume when Docker pre-filled an empty named
  volume. Bind mounts, Kubernetes volumes, pre-filled volumes and some restores
  lacked it, and config init then exited 1 on every start: an endless restart
  loop.
- The markers guarded deleting the temporary server's world in `SavedArks/`,
  but the server was stopped before it created one.

Now there is one rule, which works for any volume: no `GameUserSettings.ini` →
install the default. An existing file is never touched. The default is copied to
a temporary file and renamed into place, so an interrupted start can't leave a
partial INI that later starts would keep. Marker files left on
older volumes are ignored. The build fails if ARK doesn't write the complete
file, rather than shipping a partial default. The generation runs in its own
stage because Proton writes thousands of prefix files and ARK writes logs that
must not ship.

**SessionName is stripped.** ARK writes a random `[SessionSettings]
SessionName=ARK #NNNNNN` during the build run. The generator removes it so no
server inherits it; the server's name comes from `SESSION_NAME` /
`INSTANCE_NAME` on its command line.

**RCON and password settings are re-applied on every start.** `RCONEnabled`,
`RCONPort`, `ServerAdminPassword` and `ServerPassword` are written from the
environment each time (`ini_settings.update_config`). *Why:* the stop and
backup scripts authenticate with the env password. If the INI kept an older
value, changing `SERVER_ADMIN_PASSWORD` would make every RCON call fail,
turning every stop into a forced kill without a save. `update_config` skips the
write when nothing changed.

## Why images are built on the host

`ark-cli` builds `ark-server:<build-id>-<recipe-id>` on the user's machine from Steam; no
ARK image is published anywhere. *Why:* an image holding Studio Wildcard's
server binaries is redistribution, and publishing it risks a DMCA takedown (for
example Apple against Docker-OSX on Docker Hub). Building on the host means
this project distributes no ARK files. The build recipe is embedded in the
`ark-cli` binary, so no repo clone is needed.

Server files stay inside immutable images and are never a volume; only
`ShooterGame/Saved` and clusters are volumes. *Why:* tools that kept the server
files on a volume and updated them in place broke on ARK or Proton updates,
with no rollback and no notice.

Immutable images make rollback a tag switch: the previous image is kept, and a
failed update switches compose back to the old tag and restores the pre-update
backup. Cleanup keeps each instance's current and previous image.

SteamCMD runs in its own throwaway container, not in a BuildKit stage.
BuildKit evicted its cache mounts (Docker's default build-cache limits dropped
the SteamCMD install), so "delta" updates became full 12 GB downloads without
warning, and raising the limits needs host config on every machine. The
install is cached in `.ark-manage/build/work/cache` instead, outside BuildKit's
control. Don't move SteamCMD back into the runtime build.
